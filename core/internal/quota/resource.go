package quota

import "fmt"

// Resource names one limited thing. The string values are the ones stored in
// tenant_quota_overrides.resource and tenant_quota_events.resource, and they are
// constrained by a CHECK there, so adding a member here without adding it to the
// migration fails loudly at the first write rather than quietly at the first
// check.
type Resource string

const (
	// Measured resources. Sampled on a schedule, never counted.
	ResourceDisk      Resource = "disk"
	ResourceBandwidth Resource = "bandwidth"

	// Counted resources. Counted from the owning table at check time.
	ResourceWebsites   Resource = "websites"
	ResourceDatabases  Resource = "databases"
	ResourceMailboxes  Resource = "mailboxes"
	ResourceSubdomains Resource = "subdomains"
	ResourceCronJobs   Resource = "cron_jobs"
)

// AllResources is the complete set, in the order the status endpoint reports
// them.
var AllResources = []Resource{
	ResourceDisk,
	ResourceBandwidth,
	ResourceWebsites,
	ResourceDatabases,
	ResourceMailboxes,
	ResourceSubdomains,
	ResourceCronJobs,
}

// CountedResources are the ones Check can be asked about: creating one more of
// something. Disk and bandwidth are not creatable, only consumable.
var CountedResources = []Resource{
	ResourceWebsites,
	ResourceDatabases,
	ResourceMailboxes,
	ResourceSubdomains,
	ResourceCronJobs,
}

// MeasuredResources are the ones the Sampler writes.
var MeasuredResources = []Resource{ResourceDisk, ResourceBandwidth}

// Valid reports whether r is a resource this package knows.
func (r Resource) Valid() bool {
	switch r {
	case ResourceDisk, ResourceBandwidth, ResourceWebsites, ResourceDatabases,
		ResourceMailboxes, ResourceSubdomains, ResourceCronJobs:
		return true
	}
	return false
}

// Measured reports whether the usage figure comes from the sampler rather than
// from a COUNT. Measured resources carry a grace band; counted ones do not.
func (r Resource) Measured() bool {
	return r == ResourceDisk || r == ResourceBandwidth
}

// Counted is the complement of Measured, spelled out because every call site
// reads better one way or the other.
func (r Resource) Counted() bool { return r.Valid() && !r.Measured() }

// Unit is what the numbers for r are expressed in, for error messages.
func (r Resource) Unit() string {
	if r.Measured() {
		return "MB"
	}
	return ""
}

// Label is the human name used in refusal messages. It is what a customer sees,
// so it is a noun phrase and not an identifier.
func (r Resource) Label() string {
	switch r {
	case ResourceDisk:
		return "disk space"
	case ResourceBandwidth:
		return "monthly bandwidth"
	case ResourceWebsites:
		return "websites"
	case ResourceDatabases:
		return "databases"
	case ResourceMailboxes:
		return "mailboxes"
	case ResourceSubdomains:
		return "subdomains"
	case ResourceCronJobs:
		return "cron jobs"
	}
	return string(r)
}

// format renders an amount of r with its unit, for messages.
func (r Resource) format(v int64) string {
	if r.Measured() {
		return fmt.Sprintf("%d MB", v)
	}
	return fmt.Sprintf("%d", v)
}

// ParseResource turns a request parameter into a Resource, refusing anything
// that is not one. Callers must never build a Resource from user input by
// conversion: the value reaches a CHECK constraint.
func ParseResource(s string) (Resource, error) {
	r := Resource(s)
	if !r.Valid() {
		return "", fmt.Errorf("unknown quota resource %q", s)
	}
	return r, nil
}
