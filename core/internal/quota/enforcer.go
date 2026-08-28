package quota

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SiteController is the part of the website service the enforcer needs in order
// to make a suspension visible. It disables and re-enables vhosts and NOTHING
// ELSE: a suspension must never delete customer data, so there is no method
// here that could.
type SiteController interface {
	SuspendTenantSites(ctx context.Context, tenantID uuid.UUID) error
	ResumeTenantSites(ctx context.Context, tenantID uuid.UUID) error
}

// Enforcer is the single quota gate.
//
// One instance is built in cmd/api/main.go and handed to every service that
// creates a limited resource. There is deliberately no second constructor that
// produces a permissive one.
type Enforcer struct {
	store  Store
	logger *zap.Logger

	mu    sync.RWMutex
	sites SiteController

	// unmanagedLogged keeps "this account has no package" to one log line per
	// account per process, so an unmanaged admin tenant does not fill the log.
	unmanagedLogged sync.Map

	// Event throttles. A client retrying a refused creation in a loop must not
	// be able to fill tenant_quota_events.
	warnThrottle   time.Duration
	refuseThrottle time.Duration
}

// New builds the enforcer. A nil store is accepted and every Check then refuses
// with ErrNotWired, which is the loudest correct answer: it is a wiring mistake,
// and a wiring mistake must never read as "no limits".
func New(store Store, logger *zap.Logger) *Enforcer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Enforcer{
		store:          store,
		logger:         logger,
		warnThrottle:   time.Hour,
		refuseThrottle: 5 * time.Minute,
	}
}

// SetSiteController installs the hook a suspension uses to take sites offline.
//
// It is a setter rather than a constructor argument because the website service
// needs the enforcer and the enforcer needs the website service. Leaving it
// unset costs a suspension its effect on the vhosts - the account still refuses
// new resources - and says so in the log at the moment it matters.
func (e *Enforcer) SetSiteController(c SiteController) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.sites = c
	e.mu.Unlock()
}

func (e *Enforcer) siteController() SiteController {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sites
}

// Check is THE quota gate. Every path that creates a website, a database, a
// mailbox, a subdomain or a cron job calls this and nothing else.
//
// It answers nil when the creation may go ahead, and an error naming the limit
// and the current usage when it may not. The order of the two refusals is
// deliberate: the account-wide over-quota state is reported before the specific
// limit, because it is the more serious condition and the one that leads to
// suspension.
//
// It is safe on a nil receiver and refuses there. See ErrNotWired.
func (e *Enforcer) Check(ctx context.Context, tenantID uuid.UUID, r Resource) error {
	if e == nil || e.store == nil {
		return &UnavailableError{Cause: ErrNotWired}
	}
	if !r.Counted() {
		// A programming error, not a customer-facing one: disk and bandwidth
		// are consumed, not created.
		return fmt.Errorf("quota: Check accepts counted resources only, got %q", r)
	}
	if tenantID == uuid.Nil {
		return &UnavailableError{Cause: errors.New("the request carries no tenant, so no quota can be resolved")}
	}

	assignment, err := e.store.Assignment(ctx, tenantID)
	if err != nil {
		return &UnavailableError{Cause: err}
	}
	if assignment == nil {
		e.noteUnmanaged(tenantID)
		return nil
	}

	if assignment.Suspended {
		return &SuspendedError{
			Reason:      assignment.SuspendedReason,
			SuspendedAt: assignment.SuspendedAt,
			Automatic:   assignment.SuspendedAutomatically,
		}
	}

	measured, err := e.store.MeasuredUsage(ctx, tenantID)
	if err != nil {
		return &UnavailableError{Cause: err}
	}
	if err := e.measuredGate(ctx, assignment, measured, r); err != nil {
		return err
	}

	counts, err := e.store.Counts(ctx, tenantID)
	if err != nil {
		return &UnavailableError{Cause: err}
	}
	return e.countedGate(ctx, assignment, counts, r)
}

// measuredGate refuses a creation because the account is over its disk or
// bandwidth quota, when the package's policy says to refuse.
//
// A partial disk sample is skipped: it is a lower bound, and refusing on it
// would accuse a customer on a measurement that did not finish.
func (e *Enforcer) measuredGate(ctx context.Context, a *Assignment, m Measured, requested Resource) error {
	if !a.Package.OverQuotaAction.Blocking() {
		return nil
	}

	for _, r := range MeasuredResources {
		limit, fromOverride := a.Effective(r)
		if limit == nil {
			continue
		}
		usage := m.DiskUsedMB
		var measuredAt *time.Time = m.DiskMeasuredAt
		if r == ResourceBandwidth {
			usage = m.BandwidthUsedMB
			measuredAt = m.BandwidthMeasuredAt
		} else if m.DiskPartial {
			continue
		}
		if usage <= *limit+a.Package.GraceMB(*limit) {
			continue
		}

		exceeded := &ExceededError{
			Resource:     r,
			Limit:        *limit,
			Usage:        usage,
			FromOverride: fromOverride,
			Requested:    requested,
			MeasuredAt:   measuredAt,
			PackageName:  a.Package.Name,
		}
		e.record(ctx, a.TenantID, r, EventRefuse, limit, usage, exceeded.Error(), e.refuseThrottle)
		return exceeded
	}
	return nil
}

// countedGate refuses when one more of the requested resource would pass its
// limit, and warns when it comes close.
func (e *Enforcer) countedGate(ctx context.Context, a *Assignment, counts map[Resource]int64, r Resource) error {
	limit, fromOverride := a.Effective(r)
	if limit == nil {
		return nil // unlimited
	}
	usage := counts[r]

	if usage+1 > *limit {
		exceeded := &ExceededError{
			Resource:     r,
			Limit:        *limit,
			Usage:        usage,
			FromOverride: fromOverride,
			PackageName:  a.Package.Name,
		}
		e.record(ctx, a.TenantID, r, EventRefuse, limit, usage, exceeded.Error(), e.refuseThrottle)
		return exceeded
	}

	// Warn before refusing, so the first the customer hears of a limit is not a
	// refusal. The threshold counts the resource about to be created.
	if *limit > 0 && (usage+1)*100 >= int64(a.Package.WarnPercent)*(*limit) {
		msg := fmt.Sprintf("%s usage is at %d of %d allowed by hosting package %s",
			r.Label(), usage+1, *limit, a.Package.Name)
		e.record(ctx, a.TenantID, r, EventWarn, limit, usage+1, msg, e.warnThrottle)
	}
	return nil
}

// Suspend takes an account offline: it refuses every new resource, and its
// websites stop being served.
//
// Nothing is deleted. Not a file, not a database, not a row. Resume puts every
// site back exactly as it was, which is why the sites are DISABLED rather than
// removed from the web server.
//
// The flag is written before the sites are touched, so an interruption between
// the two leaves an account that refuses new resources with its sites still up
// - degraded in the safe direction - rather than sites down with no record why.
func (e *Enforcer) Suspend(ctx context.Context, tenantID uuid.UUID, reason string, automatic bool) error {
	if e == nil || e.store == nil {
		return &UnavailableError{Cause: ErrNotWired}
	}
	if err := e.store.SetSuspended(ctx, tenantID, true, reason, automatic); err != nil {
		return err
	}
	if err := e.store.RecordEvent(ctx, Event{
		TenantID: tenantID, Resource: ResourceDisk, Kind: EventSuspend,
		Message: reason,
	}); err != nil {
		e.logger.Warn("quota: could not record the suspension event", zap.Error(err))
	}

	sites := e.siteController()
	if sites == nil {
		e.logger.Warn("quota: account suspended, but no site controller is wired, so its websites keep serving",
			zap.String("tenant_id", tenantID.String()))
		return nil
	}
	if err := sites.SuspendTenantSites(ctx, tenantID); err != nil {
		return fmt.Errorf("account marked suspended but its websites could not be taken offline: %w", err)
	}
	e.logger.Info("quota: account suspended",
		zap.String("tenant_id", tenantID.String()),
		zap.String("reason", reason),
		zap.Bool("automatic", automatic))
	return nil
}

// Resume reverses a suspension: the flag is cleared and every site is enabled
// again.
func (e *Enforcer) Resume(ctx context.Context, tenantID uuid.UUID, reason string) error {
	if e == nil || e.store == nil {
		return &UnavailableError{Cause: ErrNotWired}
	}
	if err := e.store.SetSuspended(ctx, tenantID, false, "", false); err != nil {
		return err
	}
	if err := e.store.RecordEvent(ctx, Event{
		TenantID: tenantID, Resource: ResourceDisk, Kind: EventResume, Message: reason,
	}); err != nil {
		e.logger.Warn("quota: could not record the resume event", zap.Error(err))
	}

	sites := e.siteController()
	if sites == nil {
		e.logger.Warn("quota: suspension lifted, but no site controller is wired, so its websites were not re-enabled here",
			zap.String("tenant_id", tenantID.String()))
		return nil
	}
	if err := sites.ResumeTenantSites(ctx, tenantID); err != nil {
		return fmt.Errorf("suspension lifted but the websites could not be re-enabled: %w", err)
	}
	e.logger.Info("quota: account resumed",
		zap.String("tenant_id", tenantID.String()), zap.String("reason", reason))
	return nil
}

// record appends an event, throttled, and never fails the caller. A refusal
// that could not be written down is still a refusal.
func (e *Enforcer) record(ctx context.Context, tenantID uuid.UUID, r Resource, kind string, limit *int64, usage int64, message string, within time.Duration) {
	ev := Event{
		TenantID: tenantID, Resource: r, Kind: kind,
		LimitValue: limit, UsageValue: usage, Message: message,
	}
	if _, err := e.store.RecordEventThrottled(ctx, ev, within); err != nil {
		e.logger.Warn("quota: could not record a quota event",
			zap.String("tenant_id", tenantID.String()),
			zap.String("resource", string(r)),
			zap.String("event", kind),
			zap.Error(err))
	}
}

func (e *Enforcer) noteUnmanaged(tenantID uuid.UUID) {
	if _, loaded := e.unmanagedLogged.LoadOrStore(tenantID, struct{}{}); loaded {
		return
	}
	e.logger.Info("quota: account has no hosting package, so no limit is enforced for it",
		zap.String("tenant_id", tenantID.String()),
		zap.String("remedy", "assign a package with POST /api/v1/quota/{tenantId}/package"))
}

// Store exposes the store to the service layer, which needs the same one for
// package administration. The enforcer is the owner so that there is exactly
// one connection between the panel and these tables.
func (e *Enforcer) Store() Store {
	if e == nil {
		return nil
	}
	return e.store
}
