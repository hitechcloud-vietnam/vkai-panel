package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Default sampler timings. See the WalkBudget comment in measure.go for why the
// interval is half an hour and not a minute.
const (
	DefaultSampleInterval = 30 * time.Minute
	DefaultTenantPause    = 2 * time.Second
)

// SamplerOptions configures the background measurement.
type SamplerOptions struct {
	Store    Store
	Enforcer *Enforcer

	// Interval between full sweeps of every account with a package.
	Interval time.Duration

	// TenantPause is the gap between two accounts inside one sweep. It exists
	// so a panel with forty accounts does not walk forty trees back to back and
	// evict every customer's page cache in one go.
	TenantPause time.Duration

	Budget WalkBudget
	Logger *zap.Logger

	// Now is injectable so tests can put the clock in a different month.
	Now func() time.Time
}

// Sampler measures disk and bandwidth on a schedule and applies the over-quota
// policy. It is the only thing that writes tenant_quota_usage, and the only
// thing that suspends an account automatically.
//
// Suspension deliberately does not happen on the request path. An account being
// taken offline as a side effect of somebody clicking "create website" is a
// surprise; it belongs to the measurement that discovered the overage.
type Sampler struct {
	store    Store
	enforcer *Enforcer
	interval time.Duration
	pause    time.Duration
	budget   WalkBudget
	logger   *zap.Logger
	now      func() time.Time
}

// NewSampler builds the sampler, filling in the defaults.
func NewSampler(o SamplerOptions) *Sampler {
	if o.Logger == nil {
		o.Logger = zap.NewNop()
	}
	if o.Interval <= 0 {
		o.Interval = DefaultSampleInterval
	}
	if o.TenantPause < 0 {
		o.TenantPause = DefaultTenantPause
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Sampler{
		store:    o.Store,
		enforcer: o.Enforcer,
		interval: o.Interval,
		pause:    o.TenantPause,
		budget:   o.Budget.withDefaults(),
		logger:   o.Logger,
		now:      o.Now,
	}
}

// Run sweeps until the context is cancelled. It is meant to be started in a
// goroutine from main.
//
// The first sweep is delayed by one interval on purpose: a panel restart is
// when the machine is busiest, and the quota figures survive the restart in the
// database.
func (s *Sampler) Run(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	s.logger.Info("quota sampler started",
		zap.Duration("interval", s.interval),
		zap.Duration("tenant_pause", s.pause),
		zap.Int64("max_files_per_account", s.budget.MaxFiles),
		zap.Duration("max_walk_duration_per_account", s.budget.MaxDuration))

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("quota sampler stopped")
			return
		case <-ticker.C:
			if err := s.SweepOnce(ctx); err != nil {
				s.logger.Error("quota sweep failed", zap.Error(err))
			}
		}
	}
}

// SweepOnce measures every account that has a package, one at a time.
func (s *Sampler) SweepOnce(ctx context.Context) error {
	tenants, err := s.store.ManagedTenants(ctx)
	if err != nil {
		return err
	}

	started := s.now()
	for i, id := range tenants {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.SampleTenant(ctx, id); err != nil {
			// One account's measurement failing must not stop the sweep: the
			// remaining accounts still need their numbers.
			s.logger.Warn("quota measurement failed for one account",
				zap.String("tenant_id", id.String()), zap.Error(err))
		}

		if i < len(tenants)-1 && s.pause > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.pause):
			}
		}
	}

	s.logger.Info("quota sweep finished",
		zap.Int("accounts", len(tenants)),
		zap.Duration("took", s.now().Sub(started)))
	return nil
}

// SampleTenant measures one account and applies the over-quota policy to it.
// It is exported so the API can offer "recompute now" for a support case.
func (s *Sampler) SampleTenant(ctx context.Context, tenantID uuid.UUID) error {
	if s == nil || s.store == nil {
		return &UnavailableError{Cause: ErrNotWired}
	}

	roots, err := s.store.SiteRoots(ctx, tenantID)
	if err != nil {
		return err
	}

	sample := MeasureTrees(ctx, roots, s.budget)

	// The databases the account owns count against the same disk allowance.
	// This figure is only as fresh as whatever maintains
	// database_entries.size; it is added rather than ignored because a
	// customer's databases are unquestionably their disk usage.
	if dbBytes, err := s.store.DatabaseBytes(ctx, tenantID); err == nil {
		sample.UsedBytes += dbBytes
	} else {
		s.logger.Warn("quota: database sizes unavailable, disk figure excludes them",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
	}

	if err := s.store.SaveDiskUsage(ctx, tenantID, sample); err != nil {
		return err
	}
	if sample.Partial {
		s.logger.Warn("quota: disk walk hit its budget; the figure is a lower bound and will not be used to refuse anything",
			zap.String("tenant_id", tenantID.String()),
			zap.Int64("files_walked", sample.FileCount),
			zap.Duration("took", sample.Duration))
	}

	from, to := MonthWindow(s.now())
	bwBytes, err := s.store.BandwidthBytes(ctx, tenantID, from, to)
	if err != nil {
		return err
	}
	if err := s.store.SaveBandwidthUsage(ctx, tenantID, MBFromBytes(bwBytes), from); err != nil {
		return err
	}

	return s.applyPolicy(ctx, tenantID)
}

// MonthWindow returns the half-open range covering the calendar month t falls
// in, in UTC. Bandwidth limits are per month, so the boundary has to be
// explicit rather than implied by how long a counter has been running.
func MonthWindow(t time.Time) (from, to time.Time) {
	u := t.UTC()
	from = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 1, 0)
	return from, to
}

// applyPolicy is where "warn, then refuse, and only suspend if configured to"
// actually happens.
func (s *Sampler) applyPolicy(ctx context.Context, tenantID uuid.UUID) error {
	assignment, err := s.store.Assignment(ctx, tenantID)
	if err != nil {
		return err
	}
	if assignment == nil {
		return nil
	}
	measured, err := s.store.MeasuredUsage(ctx, tenantID)
	if err != nil {
		return err
	}

	pkg := assignment.Package
	overAny := false

	for _, r := range MeasuredResources {
		limit, _ := assignment.Effective(r)
		if limit == nil {
			continue
		}

		usage := measured.DiskUsedMB
		if r == ResourceBandwidth {
			usage = measured.BandwidthUsedMB
		} else if measured.DiskPartial {
			// A lower bound cannot put anybody over quota.
			continue
		}

		grace := pkg.GraceMB(*limit)
		switch {
		case usage > *limit+grace:
			overAny = true
			msg := fmt.Sprintf(
				"%s usage is %d MB, past the %d MB allowed by hosting package %s and its %d MB grace",
				r.Label(), usage, *limit, pkg.Name, grace)
			s.event(ctx, tenantID, r, EventWarn, limit, usage, msg)
		case *limit > 0 && usage*100 >= int64(pkg.WarnPercent)*(*limit):
			msg := fmt.Sprintf("%s usage is %d MB of the %d MB allowed by hosting package %s",
				r.Label(), usage, *limit, pkg.Name)
			s.event(ctx, tenantID, r, EventWarn, limit, usage, msg)
		}
	}

	switch {
	case overAny && pkg.OverQuotaAction == ActionSuspend && !assignment.Suspended:
		return s.enforcer.Suspend(ctx, tenantID,
			"automatic: the account is over a measured quota and its hosting package suspends on overage", true)

	case !overAny && assignment.Suspended && assignment.SuspendedAutomatically:
		// The reversal is automatic only for a suspension the enforcer made.
		// An operator's suspension is a decision, and usage dropping is not a
		// reason to overrule it.
		return s.enforcer.Resume(ctx, tenantID,
			"automatic: measured usage is back inside the hosting package limits")
	}

	return nil
}

func (s *Sampler) event(ctx context.Context, tenantID uuid.UUID, r Resource, kind string, limit *int64, usage int64, msg string) {
	throttle := time.Hour
	if _, err := s.store.RecordEventThrottled(ctx, Event{
		TenantID: tenantID, Resource: r, Kind: kind,
		LimitValue: limit, UsageValue: usage, Message: msg,
	}, throttle); err != nil {
		s.logger.Warn("quota: could not record a quota event",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
	}
}
