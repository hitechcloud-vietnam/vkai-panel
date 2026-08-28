package quota

import (
	"errors"
	"fmt"
	"time"
)

// ExceededError is returned when a creation is refused because a limit is
// reached.
//
// The message names the limit, the current usage and where the limit came
// from. Refusing without saying which limit was hit is the difference between a
// product and an obstacle: the customer cannot tell whether to delete a site,
// buy more disk or call support.
type ExceededError struct {
	Resource     Resource
	Limit        int64
	Usage        int64
	FromOverride bool

	// Requested is the resource the caller was actually trying to create, when
	// that differs from Resource. Being refused a database because the account
	// is over its disk quota is confusing unless both are named.
	Requested Resource

	// MeasuredAt is when the usage figure was sampled, for measured resources.
	// A customer told they are over quota deserves to know the number is not
	// from this instant.
	MeasuredAt *time.Time

	// PackageName is the package the limit came from, so a reseller reading a
	// ticket knows which product is involved.
	PackageName string
}

func (e *ExceededError) Error() string {
	source := "hosting package " + e.PackageName
	if e.FromOverride {
		source = "account override"
	}

	var b string
	if e.Requested != "" && e.Requested != e.Resource {
		b = fmt.Sprintf(
			"cannot create a new %s: this account is over its %s quota (%s used of %s allowed by its %s)",
			singular(e.Requested), e.Resource.Label(),
			e.Resource.format(e.Usage), e.Resource.format(e.Limit), source)
	} else {
		b = fmt.Sprintf(
			"cannot create a new %s: the %s limit of %s is reached and %s are in use (%s)",
			singular(e.Resource), e.Resource.Label(),
			e.Resource.format(e.Limit), e.Resource.format(e.Usage), source)
	}

	if e.MeasuredAt != nil {
		b += fmt.Sprintf(", measured %s", humanAge(time.Since(*e.MeasuredAt)))
	}
	return b
}

// SuspendedError is returned to every creation attempt on a suspended account.
// It is separate from ExceededError because the remedy is different: nothing
// the customer deletes will help, an operator has to lift the suspension.
type SuspendedError struct {
	Reason      string
	SuspendedAt *time.Time
	Automatic   bool
}

func (e *SuspendedError) Error() string {
	who := "an operator"
	if e.Automatic {
		who = "the quota enforcer"
	}
	msg := "this hosting account is suspended and cannot create new resources"
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	msg += fmt.Sprintf(" (suspended by %s", who)
	if e.SuspendedAt != nil {
		msg += ", " + humanAge(time.Since(*e.SuspendedAt))
	}
	return msg + "). Existing websites, databases and files are untouched."
}

// UnavailableError is returned when the enforcer cannot find out whether the
// account is within its limits.
//
// This refuses rather than allows, and that choice is the whole point of the
// type. Allowing here is how a limit that nobody applied looks exactly like a
// limit that everybody passed - the failure this package was written to end.
type UnavailableError struct{ Cause error }

func (e *UnavailableError) Error() string {
	return "quota enforcement is unavailable, so no resource can be created: " + e.Cause.Error()
}

func (e *UnavailableError) Unwrap() error { return e.Cause }

// ErrNotWired is the cause reported when Check is called on a nil enforcer or
// on one with no store. It exists so the failure names itself instead of
// arriving as a nil dereference.
var ErrNotWired = errors.New(
	"the quota enforcer was never wired into this service; " +
		"pass a *quota.Enforcer built from quota.NewPostgresStore in cmd/api/main.go")

// IsExceeded reports whether err is a quota refusal, for callers that map it
// onto an HTTP status.
func IsExceeded(err error) bool {
	var e *ExceededError
	return errors.As(err, &e)
}

// IsSuspended reports whether err is a suspension refusal.
func IsSuspended(err error) bool {
	var e *SuspendedError
	return errors.As(err, &e)
}

// IsUnavailable reports whether err is the enforcer failing closed.
func IsUnavailable(err error) bool {
	var e *UnavailableError
	return errors.As(err, &e)
}

// singular turns a resource label into the noun used in "cannot create a new
// ...". Only the labels this package produces reach it.
func singular(r Resource) string {
	switch r {
	case ResourceWebsites:
		return "website"
	case ResourceDatabases:
		return "database"
	case ResourceMailboxes:
		return "mailbox"
	case ResourceSubdomains:
		return "subdomain"
	case ResourceCronJobs:
		return "cron job"
	}
	return r.Label()
}

// humanAge renders an age the way a support conversation does.
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
