package upgrade

// The failure modes an upgrade has, as types rather than as strings, because
// the CLI exits with a different code for each and the API returns a different
// HTTP status.

import (
	"errors"
	"fmt"
)

// ErrUpToDate is returned by Check and Run when the feed offers nothing newer
// than the running version. It is not a failure; callers test for it with
// errors.Is and report "already on the latest release".
var ErrUpToDate = errors.New("already running the latest release")

// ErrNoPreviousRelease is reported when a rollback is needed but there is
// nothing to roll back to, which happens only on an installation whose current
// symlink was missing before the upgrade started.
var ErrNoPreviousRelease = errors.New("no previous release to roll back to")

// LockedError is returned when another upgrade already holds the lock.
type LockedError struct {
	Path      string
	PID       int
	StartedAt string
	Version   string
}

func (e *LockedError) Error() string {
	v := ""
	if e.Version != "" {
		v = fmt.Sprintf(" to %s", e.Version)
	}
	return fmt.Sprintf("an upgrade%s is already running (pid %d, started %s, lock %s)",
		v, e.PID, e.StartedAt, e.Path)
}

// IncompatibleJumpError is returned when the manifest's min_upgrade_from is
// newer than the running version. It always names the version the operator has
// to install first, because "unsupported upgrade path" on its own leaves them
// with nothing to do.
type IncompatibleJumpError struct {
	// From is the running version.
	From string
	// To is the release that was refused.
	To string
	// BlockedBy names the release whose min_upgrade_from is the obstacle,
	// when that is not To itself: the jump steps over it, so its migration
	// would be skipped. Empty when To is its own obstacle.
	BlockedBy string
	// MinUpgradeFrom is what that release demands.
	MinUpgradeFrom string
	// InstallFirst is the version to install before retrying. When the feed
	// lists the release history this is a real intermediate release; with a
	// single-release feed it falls back to MinUpgradeFrom itself.
	InstallFirst string
}

func (e *IncompatibleJumpError) Error() string {
	obstacle := e.To
	if e.BlockedBy != "" {
		obstacle = e.BlockedBy
		return fmt.Sprintf(
			"cannot upgrade from %s straight to %s: it would skip %s, which requires at least %s to be installed first; install %s, then run the upgrade again",
			e.From, e.To, obstacle, e.MinUpgradeFrom, e.InstallFirst)
	}
	return fmt.Sprintf(
		"cannot upgrade from %s straight to %s: %s requires at least %s to be installed first; install %s, then run the upgrade again",
		e.From, e.To, obstacle, e.MinUpgradeFrom, e.InstallFirst)
}

// ChecksumMismatchError is returned when a tarball does not hash to what the
// manifest promised, either after the download - in which case the archive is
// deleted unopened - or immediately before extraction, where the check is made
// against the open file descriptor rather than the path.
type ChecksumMismatchError struct {
	// URL is the download the digest was promised for. Empty when the
	// mismatch was found at extraction time.
	URL string
	// Path is the file on disk that failed. Empty when the mismatch was
	// found while downloading.
	Path     string
	Expected string
	Actual   string
}

func (e *ChecksumMismatchError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("checksum mismatch for %s: expected sha256 %s, the file on disk hashes to %s; refusing to extract it",
			e.Path, e.Expected, e.Actual)
	}
	return fmt.Sprintf("checksum mismatch for %s: manifest says sha256 %s, download hashes to %s; the archive was deleted without being opened",
		e.URL, e.Expected, e.Actual)
}

// UnsafeArchiveError is returned when a member of the tarball would write
// outside the staging directory: an absolute path, a "..", or a link pointing
// out of the tree. Extraction stops at the first such member.
type UnsafeArchiveError struct {
	Member string
	Target string
	Reason string
}

func (e *UnsafeArchiveError) Error() string {
	if e.Target != "" {
		return fmt.Sprintf("refusing to extract archive member %q (link target %q): %s", e.Member, e.Target, e.Reason)
	}
	return fmt.Sprintf("refusing to extract archive member %q: %s", e.Member, e.Reason)
}

// PreflightError collects everything preflight found wrong, so the operator
// fixes all of it in one pass instead of rerunning the upgrade per problem.
type PreflightError struct {
	Failures []string
}

func (e *PreflightError) Error() string {
	if len(e.Failures) == 1 {
		return "preflight check failed: " + e.Failures[0]
	}
	msg := fmt.Sprintf("%d preflight checks failed:", len(e.Failures))
	for _, f := range e.Failures {
		msg += "\n  - " + f
	}
	return msg
}

// RollbackFailedError is the one outcome a human has to be woken up for: the
// upgrade failed, the automatic rollback was attempted, and the rollback failed
// too. The panel is not running and this package has no safe move left.
//
// Callers must special-case it - louder log level, alert, non-zero exit code of
// its own - which is what IsRollbackFailed is for.
type RollbackFailedError struct {
	// Cause is the failure that triggered the rollback.
	Cause error
	// RollbackErr is why the rollback itself did not work.
	RollbackErr error
	// AttemptedRelease is the release the rollback tried to restore.
	AttemptedRelease string
	// FailedRelease is the release the upgrade was switching to.
	FailedRelease string
	// DatabaseBackup is where the pre-upgrade dump was written, empty if no
	// dump was taken. It is repeated here because it is the first thing a
	// human recovering the machine by hand needs.
	DatabaseBackup string
}

func (e *RollbackFailedError) Error() string {
	msg := fmt.Sprintf(
		"MANUAL INTERVENTION REQUIRED: upgrade to %s failed (%v) and the automatic rollback to %s also failed (%v); the panel is not serving and must be restored by hand",
		e.FailedRelease, e.Cause, e.AttemptedRelease, e.RollbackErr)
	if e.DatabaseBackup != "" {
		msg += fmt.Sprintf("; the pre-upgrade database dump is at %s", e.DatabaseBackup)
	}
	return msg
}

// Unwrap exposes the original failure so errors.Is/As can still find it.
func (e *RollbackFailedError) Unwrap() error { return e.Cause }

// IsRollbackFailed reports whether err is, or wraps, a failed rollback. This is
// the condition that must page a human.
func IsRollbackFailed(err error) bool {
	var e *RollbackFailedError
	return errors.As(err, &e)
}

// RolledBackError is returned when the upgrade failed but the rollback worked:
// the installation is back on its previous release and serving. Bad news, not
// an emergency.
type RolledBackError struct {
	// Cause is the failure that triggered the rollback.
	Cause error
	// RestoredRelease is the release now running again.
	RestoredRelease string
	// FailedRelease is the release that did not come up.
	FailedRelease string
}

func (e *RolledBackError) Error() string {
	return fmt.Sprintf("upgrade to %s failed (%v); rolled back to %s, which is running normally",
		e.FailedRelease, e.Cause, e.RestoredRelease)
}

// Unwrap exposes the original failure.
func (e *RolledBackError) Unwrap() error { return e.Cause }

// IsRolledBack reports whether err is, or wraps, a successful rollback.
func IsRolledBack(err error) bool {
	var e *RolledBackError
	return errors.As(err, &e)
}
