package quota

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ResourceStatus is one line of an account's quota report.
type ResourceStatus struct {
	Resource     Resource `json:"resource"`
	Label        string   `json:"label"`
	Unit         string   `json:"unit"`
	Limit        *int64   `json:"limit"`
	Unlimited    bool     `json:"unlimited"`
	FromOverride bool     `json:"from_override"`
	Usage        int64    `json:"usage"`
	// Percent is 0 for an unlimited resource, so a progress bar drawn from it
	// reads "nothing consumed of no limit" rather than "full".
	Percent float64 `json:"percent"`
	// State is one of ok, warning, over. It is the same arithmetic Check uses,
	// so the panel cannot show green while a creation is being refused.
	State string `json:"state"`
	// GraceMB is the band allowed above the limit before anything is refused.
	// Zero for counted resources, which get no grace.
	GraceMB    int64      `json:"grace_mb"`
	MeasuredAt *time.Time `json:"measured_at,omitempty"`
	Partial    bool       `json:"partial,omitempty"`
}

// States a resource can be in.
const (
	StateOK      = "ok"
	StateWarning = "warning"
	StateOver    = "over"
)

// Status is the whole answer for one account: which package, which limits,
// which usage, and whether anything is being enforced at all.
type Status struct {
	TenantID uuid.UUID `json:"tenant_id"`

	// Enforced is false when the account has no package. The field exists so
	// "nothing is being enforced here" is something an operator can see on a
	// screen rather than something they have to infer from an empty list.
	Enforced bool     `json:"enforced"`
	Package  *Package `json:"package"`

	Suspended   bool       `json:"suspended"`
	SuspendedAt *time.Time `json:"suspended_at,omitempty"`
	// Always reported, false included: whether the suspension was the
	// enforcer's or an operator's decides whether usage dropping may lift it,
	// so an interface that cannot see the flag cannot explain the account.
	SuspendedReason        string `json:"suspended_reason,omitempty"`
	SuspendedAutomatically bool   `json:"suspended_automatically"`

	Resources []ResourceStatus `json:"resources"`
	Features  map[string]bool  `json:"features"`

	DiskMeasuredAt *time.Time `json:"disk_measured_at,omitempty"`
	// The cost of the last walk, always reported. A measurement that starts
	// taking forty seconds over three million inodes is an outage forming, and
	// omitting the number when it happens to be small is how nobody notices it
	// growing.
	DiskMeasureMS       int        `json:"disk_measure_ms"`
	DiskFileCount       int64      `json:"disk_file_count"`
	DiskPartial         bool       `json:"disk_partial"`
	BandwidthPeriodFrom *time.Time `json:"bandwidth_period_start,omitempty"`
}

// Status builds the report for one account.
//
// It reads exactly what Check reads and applies exactly the same thresholds, so
// the panel can never show a green bar for a limit that is refusing creations.
func (e *Enforcer) Status(ctx context.Context, tenantID uuid.UUID) (*Status, error) {
	if e == nil || e.store == nil {
		return nil, &UnavailableError{Cause: ErrNotWired}
	}

	out := &Status{TenantID: tenantID, Features: map[string]bool{}, Resources: []ResourceStatus{}}

	assignment, err := e.store.Assignment(ctx, tenantID)
	if err != nil {
		return nil, &UnavailableError{Cause: err}
	}
	if assignment == nil {
		return out, nil
	}

	out.Enforced = true
	pkg := assignment.Package
	out.Package = &pkg
	out.Suspended = assignment.Suspended
	out.SuspendedAt = assignment.SuspendedAt
	out.SuspendedReason = assignment.SuspendedReason
	out.SuspendedAutomatically = assignment.SuspendedAutomatically

	for name, allowed := range pkg.Features {
		out.Features[name] = allowed
	}
	for name, allowed := range assignment.FeatureOverrides {
		out.Features[name] = allowed
	}

	counts, err := e.store.Counts(ctx, tenantID)
	if err != nil {
		return nil, &UnavailableError{Cause: err}
	}
	measured, err := e.store.MeasuredUsage(ctx, tenantID)
	if err != nil {
		return nil, &UnavailableError{Cause: err}
	}

	out.DiskMeasuredAt = measured.DiskMeasuredAt
	out.DiskMeasureMS = measured.DiskMeasureMS
	out.DiskFileCount = measured.DiskFileCount
	out.DiskPartial = measured.DiskPartial
	if measured.Present {
		start := measured.BandwidthPeriodStart
		out.BandwidthPeriodFrom = &start
	}

	for _, r := range AllResources {
		limit, fromOverride := assignment.Effective(r)

		rs := ResourceStatus{
			Resource:     r,
			Label:        r.Label(),
			Unit:         r.Unit(),
			Limit:        limit,
			Unlimited:    limit == nil,
			FromOverride: fromOverride,
			State:        StateOK,
		}

		switch r {
		case ResourceDisk:
			rs.Usage = measured.DiskUsedMB
			rs.MeasuredAt = measured.DiskMeasuredAt
			rs.Partial = measured.DiskPartial
		case ResourceBandwidth:
			rs.Usage = measured.BandwidthUsedMB
			rs.MeasuredAt = measured.BandwidthMeasuredAt
		default:
			rs.Usage = counts[r]
		}

		if limit != nil {
			if r.Measured() {
				rs.GraceMB = pkg.GraceMB(*limit)
			}
			if *limit > 0 {
				rs.Percent = float64(rs.Usage) * 100 / float64(*limit)
			} else if rs.Usage > 0 {
				rs.Percent = 100
			}
			switch {
			case rs.Usage > *limit+rs.GraceMB:
				rs.State = StateOver
			case rs.Usage*100 >= int64(pkg.WarnPercent)*(*limit):
				rs.State = StateWarning
			}
		}

		out.Resources = append(out.Resources, rs)
	}

	return out, nil
}
