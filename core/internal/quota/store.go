package quota

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Measured is the sampled half of an account's usage: the two resources that
// cannot be counted. Present is false when the account has never been sampled,
// which is different from having been sampled at zero.
type Measured struct {
	Present bool `json:"-"`

	DiskUsedMB     int64      `json:"disk_used_mb"`
	DiskFileCount  int64      `json:"disk_file_count"`
	DiskMeasuredAt *time.Time `json:"disk_measured_at"`
	DiskMeasureMS  int        `json:"disk_measure_ms"`
	// DiskPartial marks a walk that hit its budget and stopped early. The
	// figure is then a lower bound and must not be used to refuse anything.
	DiskPartial bool `json:"disk_partial"`

	BandwidthUsedMB      int64      `json:"bandwidth_used_mb"`
	BandwidthPeriodStart time.Time  `json:"bandwidth_period_start"`
	BandwidthMeasuredAt  *time.Time `json:"bandwidth_measured_at"`
}

// Event is one line of tenant_quota_events.
type Event struct {
	ID         uuid.UUID `json:"id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	Resource   Resource  `json:"resource"`
	Kind       string    `json:"event"`
	LimitValue *int64    `json:"limit_value"`
	UsageValue int64     `json:"usage_value"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// Event kinds. They match the CHECK on tenant_quota_events.event.
const (
	EventWarn    = "warn"
	EventRefuse  = "refuse"
	EventSuspend = "suspend"
	EventResume  = "resume"
)

// Override is one row of tenant_quota_overrides, as the API reports it.
type Override struct {
	Resource   Resource   `json:"resource"`
	LimitValue *int64     `json:"limit_value"`
	Reason     string     `json:"reason"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Store is everything the enforcer, the sampler and the HTTP layer need from
// the database. It is an interface so the enforcer's behaviour can be tested
// against deliberately broken stores - one that fails, one that returns no
// assignment - which are exactly the cases that must not silently allow.
//
// The production implementation is PostgresStore.
type Store interface {
	// Assignment returns the account's package, overrides and suspension
	// state. It returns (nil, nil) when the account has no package at all:
	// unmanaged, not an error. Expired overrides are not returned.
	Assignment(ctx context.Context, tenantID uuid.UUID) (*Assignment, error)

	// Counts returns the live count of every counted resource, from the tables
	// that own them. Nothing here is cached.
	Counts(ctx context.Context, tenantID uuid.UUID) (map[Resource]int64, error)

	// MeasuredUsage returns the stored disk and bandwidth sample.
	MeasuredUsage(ctx context.Context, tenantID uuid.UUID) (Measured, error)

	// RecordEvent appends to tenant_quota_events.
	RecordEvent(ctx context.Context, ev Event) error

	// RecordEventThrottled appends only if no event of the same kind for the
	// same tenant and resource was written within the window. It reports
	// whether it wrote. A client that retries a refused creation in a loop must
	// not be able to fill this table.
	RecordEventThrottled(ctx context.Context, ev Event, within time.Duration) (bool, error)

	// SetSuspended flips tenant_packages.suspended. It writes no other table:
	// suspension must never be expressed by deleting anything.
	SetSuspended(ctx context.Context, tenantID uuid.UUID, suspended bool, reason string, automatic bool) error

	// ManagedTenants lists the accounts that have a package, for the sampler.
	ManagedTenants(ctx context.Context) ([]uuid.UUID, error)

	// SiteRoots lists the document roots of an account's websites, for the disk
	// walk.
	SiteRoots(ctx context.Context, tenantID uuid.UUID) ([]string, error)

	// DatabaseBytes is the recorded size of the account's databases. It is only
	// as fresh as whatever maintains database_entries.size.
	DatabaseBytes(ctx context.Context, tenantID uuid.UUID) (int64, error)

	// BandwidthBytes sums the account's daily website statistics over a period.
	BandwidthBytes(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (int64, error)

	// SaveDiskUsage and SaveBandwidthUsage write one half of
	// tenant_quota_usage each, so a failed disk walk does not discard a good
	// bandwidth figure.
	SaveDiskUsage(ctx context.Context, tenantID uuid.UUID, s DiskSample) error
	SaveBandwidthUsage(ctx context.Context, tenantID uuid.UUID, usedMB int64, periodStart time.Time) error

	// Package administration.
	ListPackages(ctx context.Context) ([]Package, error)
	GetPackage(ctx context.Context, id uuid.UUID) (*Package, error)
	GetPackageBySlug(ctx context.Context, slug string) (*Package, error)
	CreatePackage(ctx context.Context, p *Package) error
	UpdatePackage(ctx context.Context, p *Package) error
	DeletePackage(ctx context.Context, id uuid.UUID) error

	// AssignPackage puts an account on a package, replacing any previous
	// assignment. It never clears a suspension: moving a suspended customer to
	// a bigger package is a sale, not an amnesty.
	AssignPackage(ctx context.Context, tenantID, packageID uuid.UUID, assignedBy *uuid.UUID) error

	// Overrides.
	SetOverride(ctx context.Context, tenantID uuid.UUID, r Resource, limit *int64, reason string, expiresAt *time.Time, createdBy *uuid.UUID) error
	DeleteOverride(ctx context.Context, tenantID uuid.UUID, r Resource) error
	ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]Override, error)
	SetFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string, allowed bool, reason string, expiresAt *time.Time) error
	DeleteFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string) error

	// ListEvents returns the most recent events for an account.
	ListEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]Event, error)
}
