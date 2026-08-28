// Package quota enforces hosting packages: the limits a customer bought, and
// what happens when they are reached.
//
// # One gate
//
// Every path that creates a limited resource calls exactly one function,
// (*Enforcer).Check. There is no second implementation of any limit anywhere in
// this repository, and there must never be one: two implementations of a limit
// is how one path stays generous forever, and the generous one is always the
// one nobody remembers to update.
//
// Check is nil-receiver safe and REFUSES when it is nil or has no store. A
// quota enforcer that was never wired is a wiring mistake, and a wiring mistake
// must not read as "this account has no limits". The service constructors take
// the enforcer as a required argument for the same reason: forgetting to pass
// it is a compile error, not a silent hole.
//
// # What is counted and what is measured
//
// Counted resources - websites, databases, mailboxes, subdomains, cron jobs -
// are counted from the tables that own them, at the moment of the check, by one
// query. Nothing caches them. A cached count drifts, and a drifted count is a
// limit that does not hold.
//
// Measured resources - disk and bandwidth - cannot be counted. They are sampled
// by Sampler on a schedule and stored in tenant_quota_usage. See measure.go for
// the cost of the disk walk and the budget that bounds it; the short version is
// that a du over a large tree every minute is its own outage, so the default
// interval is 30 minutes, the walk is bounded in both inodes and seconds, and
// it never runs on a request.
//
// # Grace and rounding
//
// Sizes are mebibytes everywhere: 1 MB = 1048576 bytes, 1 GB = 1024 MB. Usage
// in bytes is rounded UP to whole MB, because rounding down lets a limit be
// exceeded by almost a megabyte per sample with nothing to show for it.
//
// Measured resources get a grace band - by default 2% of the limit with a 16MB
// floor - before anything is refused. A customer at 10.001GB of a 10GB quota is
// not a fraud case: the disk figure is a sample up to one interval old, taken
// from allocated blocks, which differ from apparent size on any filesystem with
// compression or tail packing. The band is the measurement's own error bar,
// made explicit. Counted resources get no grace at all: "5 websites" is an
// exact integer and must mean 5.
//
// Before anything is refused the customer is warned, at 90% of the limit by
// default. Warnings are recorded in tenant_quota_events, throttled so a client
// that retries cannot flood the table.
//
// # Over quota is a policy, not a crash
//
// hosting_packages.over_quota_action decides what an over-quota MEASURED
// resource costs:
//
//	warn    - record it and do nothing else
//	refuse  - refuse new resources of every kind; everything that exists keeps
//	          serving, and nothing is deleted
//	suspend - refuse, and take the account's sites offline
//
// Suspension is one boolean in tenant_packages plus a call to a SiteController
// that disables the vhosts. It deletes nothing - not a file, not a database,
// not a row - and Resume puts every site back. An operator's suspension is
// marked as manual and is never lifted automatically by usage dropping.
//
// Counted resources are always refused at the limit regardless of the policy,
// because "warn" on a website count would mean the number sold is not the
// number enforced.
//
// # The account with no package
//
// An account with no row in tenant_packages is UNMANAGED: Check imposes nothing
// and says so in the status endpoint and once in the log. This is deliberate.
// The panel's own administrative tenant has no package and never will, and a
// panel that refuses to create anything until somebody assigns a package to it
// is a panel that cannot be installed. The migration puts every account that
// already exists onto a no-limits package, so "no package" is an anomaly rather
// than the normal case, and the status endpoint reports enforced=false where an
// operator can see it.
//
// A STORE FAILURE is the opposite case and is refused. If the quota tables are
// missing or the database is unreachable, Check cannot know whether the account
// is within its limits, and answering "yes" would be the exact failure this
// package exists to prevent: a limit that silently is not enforced. The error
// names the cause so an operator can fix it.
package quota
