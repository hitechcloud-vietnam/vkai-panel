package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
)

// PackageService is the administrative face of hosting packages and quota.
//
// It owns no policy of its own. Every limit decision belongs to
// quota.Enforcer.Check, and this service exists to create packages, assign
// them, grant exceptions, read the report and drive a measurement by hand. If a
// limit were ever re-implemented here there would be two answers to the same
// question, and the generous one would win by being the one nobody updates.
type PackageService struct {
	enforcer *quota.Enforcer
	sampler  *quota.Sampler
	logger   *zap.Logger
}

// NewPackageService wires the administrative surface onto the same enforcer the
// creation paths use, so the numbers the panel shows are the numbers the panel
// enforces.
func NewPackageService(enforcer *quota.Enforcer, sampler *quota.Sampler, logger *zap.Logger) *PackageService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PackageService{enforcer: enforcer, sampler: sampler, logger: logger}
}

// ErrQuotaUnavailable is returned when the service was built without an
// enforcer. It refuses rather than pretending the feature is absent.
var ErrQuotaUnavailable = errors.New(
	"hosting packages are unavailable: the quota enforcer is not wired into this panel")

func (s *PackageService) store() (quota.Store, error) {
	if s == nil || s.enforcer == nil || s.enforcer.Store() == nil {
		return nil, ErrQuotaUnavailable
	}
	return s.enforcer.Store(), nil
}

// slugPattern keeps a package slug usable in a URL and in a support
// conversation.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// PackageRequest is the body of a package create or update.
//
// Every limit is a pointer and NULL MEANS UNLIMITED, matching the column. A PUT
// replaces the whole package, so a limit left out of the body becomes
// unlimited; that is stated in the API documentation and in the handler,
// because the alternative - treating "absent" as "unchanged" - makes it
// impossible to ever set a limit back to unlimited.
type PackageRequest struct {
	OwnerTenantID *uuid.UUID `json:"owner_tenant_id"`
	Name          string     `json:"name" binding:"required,max=120"`
	Slug          string     `json:"slug" binding:"required,max=80"`
	Description   string     `json:"description" binding:"max=2000"`

	DiskMB      *int64 `json:"disk_mb"`
	BandwidthMB *int64 `json:"bandwidth_mb"`
	Websites    *int64 `json:"max_websites"`
	Databases   *int64 `json:"max_databases"`
	Mailboxes   *int64 `json:"max_mailboxes"`
	Subdomains  *int64 `json:"max_subdomains"`
	CronJobs    *int64 `json:"max_cron_jobs"`

	Features map[string]bool `json:"features"`

	OverQuotaAction string   `json:"over_quota_action"`
	WarnPercent     *int     `json:"warn_percent"`
	GracePercent    *float64 `json:"grace_percent"`
	GraceFloorMB    *int64   `json:"grace_floor_mb"`

	IsActive *bool `json:"is_active"`
}

// toPackage validates the request and turns it into a package.
func (r *PackageRequest) toPackage() (*quota.Package, error) {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return nil, errors.New("a package needs a name")
	}
	slug := strings.ToLower(strings.TrimSpace(r.Slug))
	if !slugPattern.MatchString(slug) {
		return nil, fmt.Errorf("slug %q must be lowercase letters, digits and single hyphens", r.Slug)
	}

	action := quota.Action(strings.TrimSpace(r.OverQuotaAction))
	if action == "" {
		action = quota.ActionRefuse
	}
	if !action.Valid() {
		return nil, fmt.Errorf("over_quota_action must be warn, refuse or suspend, got %q", r.OverQuotaAction)
	}

	warn := 90
	if r.WarnPercent != nil {
		warn = *r.WarnPercent
	}
	if warn < 1 || warn > 100 {
		return nil, fmt.Errorf("warn_percent must be between 1 and 100, got %d", warn)
	}

	grace := 2.0
	if r.GracePercent != nil {
		grace = *r.GracePercent
	}
	if grace < 0 || grace > 100 {
		return nil, fmt.Errorf("grace_percent must be between 0 and 100, got %v", grace)
	}

	graceFloor := int64(16)
	if r.GraceFloorMB != nil {
		graceFloor = *r.GraceFloorMB
	}
	if graceFloor < 0 {
		return nil, errors.New("grace_floor_mb cannot be negative")
	}

	limits := quota.Limits{
		DiskMB:      r.DiskMB,
		BandwidthMB: r.BandwidthMB,
		Websites:    r.Websites,
		Databases:   r.Databases,
		Mailboxes:   r.Mailboxes,
		Subdomains:  r.Subdomains,
		CronJobs:    r.CronJobs,
	}
	for _, res := range quota.AllResources {
		if v := limits.For(res); v != nil && *v < 0 {
			return nil, fmt.Errorf("the %s limit cannot be negative", res.Label())
		}
	}

	features := map[string]bool{}
	for name, allowed := range r.Features {
		if !quota.KnownFeature(name) {
			return nil, fmt.Errorf("unknown feature %q; the panel knows %s",
				name, strings.Join(quota.KnownFeatures, ", "))
		}
		features[name] = allowed
	}

	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}

	return &quota.Package{
		OwnerTenantID:   r.OwnerTenantID,
		Name:            name,
		Slug:            slug,
		Description:     strings.TrimSpace(r.Description),
		Limits:          limits,
		Features:        features,
		OverQuotaAction: action,
		WarnPercent:     warn,
		GracePercent:    grace,
		GraceFloorMB:    graceFloor,
		IsActive:        active,
	}, nil
}

// ListPackages returns every package the panel offers.
func (s *PackageService) ListPackages(ctx context.Context) ([]quota.Package, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.ListPackages(ctx)
}

// GetPackage returns one package.
func (s *PackageService) GetPackage(ctx context.Context, id uuid.UUID) (*quota.Package, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.GetPackage(ctx, id)
}

// CreatePackage adds a package.
func (s *PackageService) CreatePackage(ctx context.Context, req *PackageRequest) (*quota.Package, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	p, err := req.toPackage()
	if err != nil {
		return nil, err
	}
	if err := st.CreatePackage(ctx, p); err != nil {
		return nil, err
	}
	s.logger.Info("hosting package created",
		zap.String("slug", p.Slug), zap.String("id", p.ID.String()))
	return p, nil
}

// UpdatePackage replaces a package. See PackageRequest for why this is a
// replacement and not a merge.
func (s *PackageService) UpdatePackage(ctx context.Context, id uuid.UUID, req *PackageRequest) (*quota.Package, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	p, err := req.toPackage()
	if err != nil {
		return nil, err
	}
	p.ID = id
	if err := st.UpdatePackage(ctx, p); err != nil {
		return nil, err
	}
	s.logger.Info("hosting package updated",
		zap.String("slug", p.Slug), zap.String("id", p.ID.String()))
	return p, nil
}

// DeletePackage removes a package that nobody is on.
func (s *PackageService) DeletePackage(ctx context.Context, id uuid.UUID) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	return st.DeletePackage(ctx, id)
}

// Status is one account's whole quota picture.
func (s *PackageService) Status(ctx context.Context, tenantID uuid.UUID) (*quota.Status, error) {
	if s == nil || s.enforcer == nil {
		return nil, ErrQuotaUnavailable
	}
	return s.enforcer.Status(ctx, tenantID)
}

// AssignPackage puts an account on a package.
func (s *PackageService) AssignPackage(ctx context.Context, tenantID, packageID uuid.UUID, assignedBy *uuid.UUID) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	if _, err := st.GetPackage(ctx, packageID); err != nil {
		return err
	}
	if err := st.AssignPackage(ctx, tenantID, packageID, assignedBy); err != nil {
		return err
	}
	s.logger.Info("hosting package assigned",
		zap.String("tenant_id", tenantID.String()),
		zap.String("package_id", packageID.String()))
	return nil
}

// OverrideRequest is the body of a per-account exception.
type OverrideRequest struct {
	// LimitValue null means "unlimited for this account". The field is
	// distinguished from an absent override by the presence of the row, not by
	// the value.
	LimitValue *int64     `json:"limit_value"`
	Reason     string     `json:"reason" binding:"max=1000"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// SetOverride grants or changes one account's exception for one resource.
func (s *PackageService) SetOverride(ctx context.Context, tenantID uuid.UUID, resource string, req *OverrideRequest, createdBy *uuid.UUID) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	r, err := quota.ParseResource(resource)
	if err != nil {
		return err
	}
	if req.LimitValue != nil && *req.LimitValue < 0 {
		return errors.New("limit_value cannot be negative")
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		return errors.New("expires_at is already in the past, so the override would have no effect")
	}
	if err := st.SetOverride(ctx, tenantID, r, req.LimitValue, strings.TrimSpace(req.Reason), req.ExpiresAt, createdBy); err != nil {
		return err
	}
	s.logger.Info("quota override set",
		zap.String("tenant_id", tenantID.String()),
		zap.String("resource", string(r)))
	return nil
}

// DeleteOverride removes an exception, putting the account back on its package.
func (s *PackageService) DeleteOverride(ctx context.Context, tenantID uuid.UUID, resource string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	r, err := quota.ParseResource(resource)
	if err != nil {
		return err
	}
	return st.DeleteOverride(ctx, tenantID, r)
}

// ListOverrides returns the exceptions in force for an account.
func (s *PackageService) ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]quota.Override, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.ListOverrides(ctx, tenantID)
}

// FeatureOverrideRequest is the body of a per-account feature exception.
type FeatureOverrideRequest struct {
	Allowed   bool       `json:"allowed"`
	Reason    string     `json:"reason" binding:"max=1000"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// SetFeatureOverride grants or withdraws one feature for one account.
func (s *PackageService) SetFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string, req *FeatureOverrideRequest) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	if !quota.KnownFeature(feature) {
		return fmt.Errorf("unknown feature %q; the panel knows %s",
			feature, strings.Join(quota.KnownFeatures, ", "))
	}
	return st.SetFeatureOverride(ctx, tenantID, feature, req.Allowed, strings.TrimSpace(req.Reason), req.ExpiresAt)
}

// DeleteFeatureOverride removes a feature exception.
func (s *PackageService) DeleteFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	return st.DeleteFeatureOverride(ctx, tenantID, feature)
}

// Suspend takes an account offline. Reversible, and it deletes nothing.
func (s *PackageService) Suspend(ctx context.Context, tenantID uuid.UUID, reason string) error {
	if s == nil || s.enforcer == nil {
		return ErrQuotaUnavailable
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "suspended by an operator"
	}
	return s.enforcer.Suspend(ctx, tenantID, reason, false)
}

// Resume puts a suspended account back exactly as it was.
func (s *PackageService) Resume(ctx context.Context, tenantID uuid.UUID, reason string) error {
	if s == nil || s.enforcer == nil {
		return ErrQuotaUnavailable
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "suspension lifted by an operator"
	}
	return s.enforcer.Resume(ctx, tenantID, reason)
}

// Recompute measures one account now, out of the sampler's schedule.
//
// It exists for the support case - "the customer deleted 4GB and the panel
// still says they are full" - and it is deliberately an explicit operator
// action rather than something a page render triggers: a walk that any request
// can start is a denial of service waiting to be found.
func (s *PackageService) Recompute(ctx context.Context, tenantID uuid.UUID) (*quota.Status, error) {
	if s == nil || s.sampler == nil {
		return nil, errors.New("the quota sampler is not wired into this panel, so usage cannot be recomputed on demand")
	}
	if err := s.sampler.SampleTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	return s.Status(ctx, tenantID)
}

// ListEvents returns the recent quota decisions for an account: what was
// warned, what was refused, what was suspended.
func (s *PackageService) ListEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]quota.Event, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.ListEvents(ctx, tenantID, limit)
}
