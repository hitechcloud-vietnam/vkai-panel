package quota

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BytesPerMB is the one definition of a megabyte in this panel: a mebibyte.
// 1 GB = 1024 MB. Every limit, every usage figure and every message uses it, so
// the number a customer is quoted is the number the enforcer compares.
const BytesPerMB int64 = 1 << 20

// MBFromBytes converts bytes to whole megabytes, rounding UP.
//
// Up, not down: rounding down lets usage exceed a limit by almost a megabyte
// per sample while the panel reports it as compliant. Up costs a customer at
// most one megabyte of their allowance and never lies in the panel's favour.
func MBFromBytes(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + BytesPerMB - 1) / BytesPerMB
}

// Action is what an over-quota measured resource costs. It matches the CHECK on
// hosting_packages.over_quota_action.
type Action string

const (
	// ActionWarn records the event and refuses nothing.
	ActionWarn Action = "warn"
	// ActionRefuse refuses new resources of every kind. Nothing that exists
	// stops serving and nothing is deleted.
	ActionRefuse Action = "refuse"
	// ActionSuspend does everything ActionRefuse does and additionally takes
	// the account's sites offline. Reversible; deletes nothing.
	ActionSuspend Action = "suspend"
)

// Valid reports whether a is one of the three policies.
func (a Action) Valid() bool {
	return a == ActionWarn || a == ActionRefuse || a == ActionSuspend
}

// Blocking reports whether this policy refuses new resources once a measured
// resource is past its limit and its grace band.
func (a Action) Blocking() bool {
	return a == ActionRefuse || a == ActionSuspend
}

// Limits is one package's numbers. A nil pointer is UNLIMITED; a pointer to
// zero is a real limit of zero. See the migration header for why 0 must not
// mean unlimited.
type Limits struct {
	DiskMB      *int64 `json:"disk_mb"`
	BandwidthMB *int64 `json:"bandwidth_mb"`
	Websites    *int64 `json:"max_websites"`
	Databases   *int64 `json:"max_databases"`
	Mailboxes   *int64 `json:"max_mailboxes"`
	Subdomains  *int64 `json:"max_subdomains"`
	CronJobs    *int64 `json:"max_cron_jobs"`
}

// For returns the limit this package sets for r, or nil for unlimited.
func (l Limits) For(r Resource) *int64 {
	switch r {
	case ResourceDisk:
		return l.DiskMB
	case ResourceBandwidth:
		return l.BandwidthMB
	case ResourceWebsites:
		return l.Websites
	case ResourceDatabases:
		return l.Databases
	case ResourceMailboxes:
		return l.Mailboxes
	case ResourceSubdomains:
		return l.Subdomains
	case ResourceCronJobs:
		return l.CronJobs
	}
	return nil
}

// Set writes the limit for r. Used by the package CRUD service so that adding a
// resource does not need a new switch in three more places.
func (l *Limits) Set(r Resource, v *int64) error {
	switch r {
	case ResourceDisk:
		l.DiskMB = v
	case ResourceBandwidth:
		l.BandwidthMB = v
	case ResourceWebsites:
		l.Websites = v
	case ResourceDatabases:
		l.Databases = v
	case ResourceMailboxes:
		l.Mailboxes = v
	case ResourceSubdomains:
		l.Subdomains = v
	case ResourceCronJobs:
		l.CronJobs = v
	default:
		return fmt.Errorf("unknown quota resource %q", r)
	}
	return nil
}

// Package is a row of hosting_packages.
type Package struct {
	ID            uuid.UUID  `json:"id"`
	OwnerTenantID *uuid.UUID `json:"owner_tenant_id"`
	Name          string     `json:"name"`
	Slug          string     `json:"slug"`
	Description   string     `json:"description"`

	Limits   Limits          `json:"limits"`
	Features map[string]bool `json:"features"`

	OverQuotaAction Action  `json:"over_quota_action"`
	WarnPercent     int     `json:"warn_percent"`
	GracePercent    float64 `json:"grace_percent"`
	GraceFloorMB    int64   `json:"grace_floor_mb"`

	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GraceMB is the band allowed above a measured limit before anything is
// refused: a percentage of the limit, never less than the floor.
//
// The floor is what keeps the band meaningful on a small package - 2% of 512MB
// is 10MB, which is smaller than the error a single sampling interval can
// introduce.
func (p Package) GraceMB(limitMB int64) int64 {
	if limitMB <= 0 {
		return p.GraceFloorMB
	}
	band := int64(float64(limitMB) * p.GracePercent / 100.0)
	if band < p.GraceFloorMB {
		band = p.GraceFloorMB
	}
	return band
}

// Assignment is one account's effective configuration: its package, its
// exceptions, and whether it is suspended.
type Assignment struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Package  Package   `json:"package"`

	// Overrides holds the exceptions actually in force. A key being present is
	// an override; a nil value is an override to "unlimited". Expired
	// overrides are not loaded, so this map is always current.
	Overrides map[Resource]*int64 `json:"overrides"`

	// FeatureOverrides is the same idea for the booleans.
	FeatureOverrides map[string]bool `json:"feature_overrides"`

	Suspended              bool       `json:"suspended"`
	SuspendedAt            *time.Time `json:"suspended_at"`
	SuspendedReason        string     `json:"suspended_reason"`
	SuspendedAutomatically bool       `json:"suspended_automatically"`

	AssignedAt time.Time `json:"assigned_at"`
}

// Effective returns the limit actually in force for r and whether it came from
// a per-account override.
//
// An override beats the package unconditionally, including an override that
// LOWERS a limit: an exception is an exception in both directions, and a
// support case that needs one account capped below its package is as real as
// one that needs it raised.
func (a *Assignment) Effective(r Resource) (limit *int64, fromOverride bool) {
	if a == nil {
		return nil, false
	}
	if v, ok := a.Overrides[r]; ok {
		return v, true
	}
	return a.Package.Limits.For(r), false
}

// FeatureAllowed reports whether the account may use a named feature, and
// whether the answer came from an override.
//
// A feature the package does not mention is NOT allowed. Defaulting an unknown
// feature to "allowed" would mean every feature added after a package was sold
// is included in it retroactively.
func (a *Assignment) FeatureAllowed(feature string) (allowed bool, fromOverride bool) {
	if a == nil {
		return false, false
	}
	if v, ok := a.FeatureOverrides[feature]; ok {
		return v, true
	}
	return a.Package.Features[feature], false
}
