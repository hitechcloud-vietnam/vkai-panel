package upgrade

// Run: the whole upgrade, as a sequence of named steps.
//
// The shape is deliberate. Each step is a method that does one thing and
// returns an error; Run does nothing but call them in order, report the
// transition, and decide what a failure means at that point. There is exactly
// one place where a failure turns into a rollback, and it is visible in this
// file rather than distributed across the steps.

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Run performs an upgrade.
//
// It returns the Result even on failure - the operator needs to know how far it
// got and where the database dump went - alongside an error describing what
// went wrong. Four error shapes matter to callers:
//
//	errors.Is(err, ErrUpToDate)  nothing to do, not a failure
//	*IncompatibleJumpError       refused; InstallFirst names the way forward
//	*RolledBackError             failed, previous release restored and healthy
//	*RollbackFailedError         failed and could not be undone - wake someone
//
// Anything else failed before the switch, with the installation untouched.
func (u *Upgrader) Run(ctx context.Context) (*Result, error) {
	u.events = nil
	res := &Result{StartedAt: u.deps.Clock.Now()}
	defer func() {
		res.FinishedAt = u.deps.Clock.Now()
		res.Events = append([]Event(nil), u.events...)
	}()

	// -------------------------------------------------- lock
	u.started(StepLock)
	lock, err := u.acquireLock("")
	if err != nil {
		return res, u.failed(StepLock, err)
	}
	defer func() {
		if rerr := lock.release(); rerr != nil {
			u.emit(StepCleanup, StatusFailed, "could not release the upgrade lock", rerr)
		}
	}()
	u.succeeded(StepLock, "Holding "+u.LockFile())

	// -------------------------------------------------- check
	u.started(StepCheck)
	check, err := u.Check(ctx)
	res.FromVersion = check.CurrentVersion
	if err != nil {
		if errors.Is(err, ErrUpToDate) {
			u.skipped(StepCheck, fmt.Sprintf("Already on %s", check.CurrentVersion))
			u.succeeded(StepDone, "Nothing to upgrade")
			return res, err
		}
		return res, u.failed(StepCheck, err)
	}
	target := *check.Target
	res.Manifest = target
	res.ToVersion = target.Version
	res.ReleaseDir = u.ReleaseDir(target.Version)
	u.succeeded(StepCheck, fmt.Sprintf("%s is available (running %s)", target.Version, check.CurrentVersion))

	// From here on, temporary files are ours to clean up whatever happens.
	var tarball string
	staging := u.stagingDir(target.Version)
	defer func() {
		if tarball != "" {
			_ = os.Remove(tarball)
		}
		_ = os.RemoveAll(staging)
	}()

	// -------------------------------------------------- download
	u.started(StepDownload)
	tarball, sum, err := u.download(ctx, target)
	if err != nil {
		return res, u.failed(StepDownload, err)
	}
	u.succeeded(StepDownload, "Downloaded "+target.TarballURL)

	// -------------------------------------------------- verify
	u.started(StepVerify)
	if err := u.verifyChecksum(tarball, sum, target); err != nil {
		tarball = "" // verifyChecksum already deleted it
		return res, u.failed(StepVerify, err)
	}
	u.succeeded(StepVerify, "sha256 matches the manifest")

	// -------------------------------------------------- stage
	u.started(StepStage)
	if err := u.stage(tarball, staging, target); err != nil {
		return res, u.failed(StepStage, err)
	}
	u.succeeded(StepStage, "Staged at "+staging)

	// -------------------------------------------------- preflight
	u.started(StepPreflight)
	if err := u.preflight(ctx, staging, target); err != nil {
		return res, u.failed(StepPreflight, err)
	}
	u.succeeded(StepPreflight, "All preflight checks passed")

	// -------------------------------------------------- database backup
	u.started(StepBackupDatabase)
	if u.cfg.Database.Enabled {
		path, err := u.backupDatabase(ctx, res.FromVersion, res.ToVersion)
		if err != nil {
			return res, u.failed(StepBackupDatabase, err)
		}
		res.DatabaseBackupPath = path
		u.succeeded(StepBackupDatabase, "Database dumped to "+path)
	} else {
		u.skipped(StepBackupDatabase, "No database backup is configured for this installation")
	}

	// -------------------------------------------------- switch
	// The previous release is read before anything moves: it is the only
	// rollback target, and reading it afterwards would read the new one.
	previous, prevErr := u.readCurrentLink()
	if prevErr == nil {
		res.PreviousRelease = previous
	}

	u.started(StepSwitch)
	if err := u.promote(staging, res.ReleaseDir); err != nil {
		return res, u.failed(StepSwitch, err)
	}
	staging = "" // promoted; the deferred cleanup must not remove it now
	if err := u.pointCurrentAt(res.ReleaseDir); err != nil {
		// The symlink swap is atomic, so a failure here means it never
		// happened and the old release is still live. Nothing to undo
		// beyond the promoted directory, which is inert.
		return res, u.failed(StepSwitch, err)
	}
	res.Switched = true
	u.succeeded(StepSwitch, fmt.Sprintf("%s now points at %s", u.CurrentLink(), res.ReleaseDir))

	// -------------------------------------------------- restart
	u.started(StepRestart)
	if err := u.restartServices(ctx); err != nil {
		u.emit(StepRestart, StatusFailed, "Restart failed", err)
		return res, u.rollbackOrEscalate(ctx, res, err)
	}
	u.succeeded(StepRestart, "Services restarted")

	// -------------------------------------------------- health check
	u.started(StepHealthCheck)
	if err := u.waitHealthy(ctx); err != nil {
		u.emit(StepHealthCheck, StatusFailed, "The new release did not become healthy", err)
		return res, u.rollbackOrEscalate(ctx, res, err)
	}
	u.succeeded(StepHealthCheck, "Services are healthy on "+res.ToVersion)

	res.Succeeded = true

	// -------------------------------------------------- record
	if err := u.saveState(state{
		CurrentVersion:     res.ToVersion,
		PreviousVersion:    res.FromVersion,
		LastUpgradeAt:      u.deps.Clock.Now().UTC(),
		LastDatabaseBackup: res.DatabaseBackupPath,
	}); err != nil {
		// The upgrade worked; only the bookkeeping did not. Say so and
		// carry on rather than rolling back a healthy installation.
		u.emit(StepCleanup, StatusFailed, "Could not update "+u.StateFile(), err)
	}

	// -------------------------------------------------- prune
	u.started(StepPrune)
	pruned, err := u.prune(res.ReleaseDir, res.PreviousRelease)
	res.Pruned = pruned
	if err != nil {
		// Failing to delete an old release does not undo a good upgrade.
		u.emit(StepPrune, StatusFailed, "Some old releases could not be removed", err)
	} else if len(pruned) == 0 {
		u.skipped(StepPrune, "No releases to prune")
	} else {
		u.succeeded(StepPrune, fmt.Sprintf("Removed %d old release(s)", len(pruned)))
	}

	// -------------------------------------------------- cleanup
	u.started(StepCleanup)
	if tarball != "" {
		_ = os.Remove(tarball)
		tarball = ""
	}
	u.cleanupStagingDirs()
	u.succeeded(StepCleanup, "Temporary files removed")

	u.succeeded(StepDone, fmt.Sprintf("Upgraded %s to %s", res.FromVersion, res.ToVersion))
	return res, nil
}

// stage extracts the verified tarball into its staging directory.
func (u *Upgrader) stage(tarball, staging string, m Manifest) error {
	releaseDir := u.ReleaseDir(m.Version)
	if _, err := os.Lstat(releaseDir); err == nil {
		return fmt.Errorf("release %s is already installed at %s; remove that directory to reinstall it", m.Version, releaseDir)
	}
	if err := os.MkdirAll(u.ReleasesDir(), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", u.ReleasesDir(), err)
	}
	// A staging directory carrying our own pid can only be debris from a
	// previous run of this process.
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear stale staging directory %s: %w", staging, err)
	}
	if err := extractTarGz(tarball, staging, extractOptions{
		MaxBytes:   u.cfg.MaxExtractBytes,
		MaxEntries: u.cfg.MaxArchiveEntries,
	}); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	return nil
}

// rollbackOrEscalate is the single rollback decision point, reached only from a
// failure that happened after the symlink moved.
//
// It has three outcomes, and the difference between the last two is the whole
// reason this package exists:
//
//	rollback succeeded  -> *RolledBackError: the panel is up on the old
//	                       release, an operator reads this in the morning
//	rollback failed     -> *RollbackFailedError: the panel is down and no
//	                       further automatic action is safe
//	nothing to roll back-> *RollbackFailedError wrapping ErrNoPreviousRelease
func (u *Upgrader) rollbackOrEscalate(ctx context.Context, res *Result, cause error) error {
	u.started(StepRollback)

	if res.PreviousRelease == "" {
		err := &RollbackFailedError{
			Cause:            cause,
			RollbackErr:      ErrNoPreviousRelease,
			AttemptedRelease: "",
			FailedRelease:    res.ToVersion,
			DatabaseBackup:   res.DatabaseBackupPath,
		}
		res.NeedsManualIntervention = true
		u.emit(StepRollback, StatusFailed, "There is no previous release to roll back to", err)
		return err
	}

	// The rollback gets a context of its own. If the caller's context is
	// what failed - a cancelled upgrade, a timed-out API request - the
	// rollback still has to run: leaving the machine on a broken release
	// because the request went away is not an option.
	rbCtx := ctx
	if ctx.Err() != nil {
		rbCtx = context.Background()
	}

	if rbErr := u.rollback(rbCtx, res.PreviousRelease); rbErr != nil {
		err := &RollbackFailedError{
			Cause:            cause,
			RollbackErr:      rbErr,
			AttemptedRelease: res.PreviousRelease,
			FailedRelease:    res.ToVersion,
			DatabaseBackup:   res.DatabaseBackupPath,
		}
		res.NeedsManualIntervention = true
		u.emit(StepRollback, StatusFailed, "THE ROLLBACK ALSO FAILED - MANUAL INTERVENTION REQUIRED", err)
		return err
	}

	res.RolledBack = true
	u.succeeded(StepRollback, "Rolled back to "+res.PreviousRelease)
	return &RolledBackError{
		Cause:           cause,
		RestoredRelease: res.PreviousRelease,
		FailedRelease:   res.ToVersion,
	}
}
