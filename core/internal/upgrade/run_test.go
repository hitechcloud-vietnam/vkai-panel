package upgrade

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestUpgradeEndToEnd is requirement 5's happy path: from a running 1.0.0 to a
// healthy 1.1.0, against a temporary directory, a fake command runner and a
// fake clock. Nothing here touches the real system or the network.
func TestUpgradeEndToEnd(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	m := env.publish("1.1.0", "1.0.0")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !res.Succeeded || res.RolledBack || res.NeedsManualIntervention {
		t.Fatalf("result = %+v, want a clean success", res)
	}
	if res.FromVersion != "1.0.0" || res.ToVersion != "1.1.0" {
		t.Errorf("versions = %s -> %s, want 1.0.0 -> 1.1.0", res.FromVersion, res.ToVersion)
	}
	if res.Manifest.ChangelogURL != m.ChangelogURL {
		t.Errorf("manifest not carried into the result: %+v", res.Manifest)
	}

	// The symlink moved, and it moved to a relative target so the whole
	// installation stays relocatable.
	link, err := os.Readlink(filepath.Join(env.root, "current"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if link != filepath.Join("releases", "1.1.0") {
		t.Errorf("current -> %q, want the relative path releases/1.1.0", link)
	}

	// The new release is on disk and is the real thing.
	body, err := os.ReadFile(filepath.Join(env.root, "releases", "1.1.0", "VERSION"))
	if err != nil || strings.TrimSpace(string(body)) != "1.1.0" {
		t.Fatalf("new release VERSION = %q, %v", body, err)
	}
	// The old release was never written to.
	body, err = os.ReadFile(filepath.Join(env.root, "releases", "1.0.0", "VERSION"))
	if err != nil || strings.TrimSpace(string(body)) != "1.0.0" {
		t.Fatalf("the previous release was modified: VERSION = %q, %v", body, err)
	}

	// The database was dumped and the location recorded.
	if res.DatabaseBackupPath == "" {
		t.Fatal("no database backup path was recorded")
	}
	if !exists(res.DatabaseBackupPath) {
		t.Fatalf("recorded database backup %s does not exist", res.DatabaseBackupPath)
	}
	if !strings.HasPrefix(res.DatabaseBackupPath, filepath.Join(env.root, "www", "backup", "databases")) {
		t.Errorf("database backup went to %s, outside the backup directory", res.DatabaseBackupPath)
	}
	if !strings.Contains(filepath.Base(res.DatabaseBackupPath), "1.0.0-to-1.1.0") {
		t.Errorf("backup filename %q does not say which upgrade it belongs to", filepath.Base(res.DatabaseBackupPath))
	}

	// Every service was restarted, in the configured order.
	restarts := env.runner.callsMatching("systemctl restart")
	want := []string{"systemctl restart vkai-api", "systemctl restart vkai-ui", "systemctl restart vkai-agent"}
	if !equalStrings(restarts, want) {
		t.Errorf("restarts = %v, want %v", restarts, want)
	}

	// The state file records where we are and where we came from.
	raw, err := os.ReadFile(env.u.StateFile())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if st.CurrentVersion != "1.1.0" || st.PreviousVersion != "1.0.0" || st.LastDatabaseBackup != res.DatabaseBackupPath {
		t.Errorf("state = %+v, want 1.1.0 over 1.0.0 with the dump path", st)
	}

	// Nothing temporary was left behind.
	if exists(env.u.LockFile()) {
		t.Error("the upgrade lock was not released")
	}
	entries, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			t.Errorf("downloaded tarball %s was not cleaned up", e.Name())
		}
	}
	releaseEntries, _ := os.ReadDir(filepath.Join(env.root, "releases"))
	for _, e := range releaseEntries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("staging directory %s was left behind", e.Name())
		}
	}
}

// The steps must be reported in order, and the database dump must be reported
// as finished before the switch begins - requirement 5 is "before anything is
// switched", and a test that only checks the file exists would not catch a
// reordering.
func TestUpgradeReportsStepsInOrder(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	if _, err := env.u.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := env.steps()
	want := []string{
		"lock:started", "lock:succeeded",
		"check:started", "check:succeeded",
		"download:started", "download:succeeded",
		"verify:started", "verify:succeeded",
		"stage:started", "stage:succeeded",
		"preflight:started", "preflight:succeeded",
		"backup_database:started", "backup_database:succeeded",
		"switch:started", "switch:succeeded",
		"restart:started", "restart:succeeded",
		"health_check:started", "health_check:succeeded",
		"prune:started", "prune:skipped",
		"cleanup:started", "cleanup:succeeded",
		"done:succeeded",
	}
	if len(got) != len(want) {
		t.Fatalf("steps = %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d = %q, want %q\nfull sequence: %v", i, got[i], want[i], got)
		}
	}
}

func TestUpgradeRecordsEventsWithoutACallback(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.Progress = nil })
	env.publish("1.1.0", "")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Events) == 0 {
		t.Fatal("Result.Events is empty; a caller with no callback has no way to see what happened")
	}
	if res.Events[0].Step != StepLock || res.Events[len(res.Events)-1].Step != StepDone {
		t.Errorf("events run from %v to %v, want lock to done",
			res.Events[0].Step, res.Events[len(res.Events)-1].Step)
	}
	for _, ev := range res.Events {
		if ev.At.IsZero() {
			t.Errorf("event %v has no timestamp", ev.Step)
		}
	}
}

// TestHealthCheckFailureTriggersRollback is requirement 7: the new release
// comes up, fails its health check, and the previous release is restored.
func TestHealthCheckFailureTriggersRollback(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	// 1.1.0 starts but never reports healthy. 1.0.0 is fine, which is what
	// makes the rollback the right move.
	env.runner.setUnhealthy("1.1.0", true)

	res, err := env.u.Run(context.Background())

	var rolled *RolledBackError
	if !errors.As(err, &rolled) {
		t.Fatalf("Run error = %#v, want *RolledBackError", err)
	}
	if IsRollbackFailed(err) {
		t.Fatal("a successful rollback must not be reported as needing manual intervention")
	}
	if !IsRolledBack(err) {
		t.Fatal("IsRolledBack should recognise this error")
	}
	if rolled.FailedRelease != "1.1.0" || !strings.HasSuffix(rolled.RestoredRelease, "1.0.0") {
		t.Errorf("RolledBackError = %+v, want 1.1.0 failed and 1.0.0 restored", rolled)
	}
	// Both the original failure and the rollback outcome have to be in the
	// message: an operator reading only one of them is misinformed.
	if !containsAll(rolled.Error(), "1.1.0", "1.0.0", "rolled back", "not active") {
		t.Errorf("error does not report both the failure and the rollback: %q", rolled.Error())
	}
	// The original failure is still reachable underneath.
	if errors.Unwrap(rolled) == nil {
		t.Error("the triggering failure was not preserved for errors.Is/As")
	}

	if !res.Switched {
		t.Error("Result.Switched should record that the symlink did move")
	}
	if !res.RolledBack || res.Succeeded || res.NeedsManualIntervention {
		t.Errorf("result = %+v, want RolledBack with no manual intervention", res)
	}

	// The machine is back on 1.0.0.
	if env.currentTarget() != "1.0.0" {
		t.Fatalf("current = %s, want 1.0.0", env.currentTarget())
	}
	// The failed release is kept on disk: it is the evidence.
	if !exists(filepath.Join(env.root, "releases", "1.1.0")) {
		t.Error("the failed release was deleted; there is nothing left to diagnose")
	}
	// The pre-upgrade dump is still recorded.
	if res.DatabaseBackupPath == "" || !exists(res.DatabaseBackupPath) {
		t.Errorf("database backup missing from a rolled-back upgrade: %q", res.DatabaseBackupPath)
	}
	// The state file must not claim we are on 1.1.0.
	if st, err := env.u.loadState(); err == nil && st.CurrentVersion == "1.1.0" {
		t.Error("the state file records 1.1.0 after a rollback to 1.0.0")
	}

	if !env.hasStep(StepHealthCheck, StatusFailed) || !env.hasStep(StepRollback, StatusSucceeded) {
		t.Errorf("steps = %v, want a failed health check and a successful rollback", env.steps())
	}
	if env.hasStep(StepPrune, StatusStarted) {
		t.Error("pruning ran after a failed upgrade")
	}
}

// A service that will not restart at all is the same class of failure and takes
// the same path.
func TestRestartFailureTriggersRollback(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.setRestartFails("1.1.0", true)

	_, err := env.u.Run(context.Background())
	if !IsRolledBack(err) {
		t.Fatalf("Run error = %v, want a rollback", err)
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
	if !env.hasStep(StepRestart, StatusFailed) {
		t.Errorf("steps = %v, want a failed restart", env.steps())
	}
}

// TestRollbackFailureIsReportedDistinctly is the case a human must wake up for:
// the upgrade failed AND the rollback failed. It must not be confusable with a
// successful rollback, and the message has to say so in as many words.
func TestRollbackFailureIsReportedDistinctly(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	// 1.1.0 never becomes healthy, and the old release will not restart
	// either - a kernel or dependency change that broke both.
	env.runner.setUnhealthy("1.1.0", true)
	env.runner.setRestartFails("1.0.0", true)

	res, err := env.u.Run(context.Background())

	var fatal *RollbackFailedError
	if !errors.As(err, &fatal) {
		t.Fatalf("Run error = %#v, want *RollbackFailedError", err)
	}
	if !IsRollbackFailed(err) {
		t.Fatal("IsRollbackFailed must recognise this error - it is what pages a human")
	}
	if IsRolledBack(err) {
		t.Fatal("a failed rollback must never be classified as a successful one")
	}
	if fatal.Cause == nil || fatal.RollbackErr == nil {
		t.Errorf("RollbackFailedError = %+v, want both the cause and the rollback error", fatal)
	}
	if fatal.FailedRelease != "1.1.0" || !strings.HasSuffix(fatal.AttemptedRelease, "1.0.0") {
		t.Errorf("RollbackFailedError = %+v, want 1.1.0 failed and 1.0.0 attempted", fatal)
	}
	if fatal.DatabaseBackup == "" {
		t.Error("the dump location must be repeated in the fatal error; it is the first thing a recovery needs")
	}
	msg := fatal.Error()
	if !containsAll(msg, "MANUAL INTERVENTION REQUIRED", "1.1.0", "1.0.0", fatal.DatabaseBackup) {
		t.Errorf("fatal error is not unmistakable: %q", msg)
	}

	if !res.NeedsManualIntervention {
		t.Error("Result.NeedsManualIntervention must be set - it is what a monitor alerts on")
	}
	if res.RolledBack || res.Succeeded {
		t.Errorf("result = %+v, want neither success nor a completed rollback", res)
	}

	if !env.hasStep(StepRollback, StatusFailed) {
		t.Errorf("steps = %v, want a failed rollback step", env.steps())
	}
	// The lock is still released: a wedged lock would stop the operator's
	// own recovery attempt.
	if exists(env.u.LockFile()) {
		t.Error("the upgrade lock was not released after a fatal failure")
	}
}

// With no previous release there is nothing to roll back to, and that is also a
// wake-a-human outcome rather than a quiet failure.
func TestRollbackWithNoPreviousReleaseIsFatal(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.setUnhealthy("1.1.0", true)

	// An installation whose current symlink is missing: a half-finished
	// install, or an operator who deleted it by hand. The switch creates it
	// for the first time, so nothing was recorded to roll back to.
	if err := os.Remove(filepath.Join(env.root, "current")); err != nil {
		t.Fatalf("remove current: %v", err)
	}
	env.u.cfg.HealthCheck = func(context.Context) error {
		target, err := os.Readlink(filepath.Join(env.root, "current"))
		if err != nil {
			return nil // before the switch: the old release is fine
		}
		if filepath.Base(target) == "1.1.0" {
			return errors.New("the new release is not serving")
		}
		return nil
	}

	res, err := env.u.Run(context.Background())

	var fatal *RollbackFailedError
	if !errors.As(err, &fatal) {
		t.Fatalf("Run error = %#v, want *RollbackFailedError", err)
	}
	if !errors.Is(fatal.RollbackErr, ErrNoPreviousRelease) {
		t.Errorf("RollbackErr = %v, want ErrNoPreviousRelease", fatal.RollbackErr)
	}
	if !res.NeedsManualIntervention {
		t.Error("Result.NeedsManualIntervention must be set")
	}
}

// The health check has to be bounded, or a service that hangs turns the upgrade
// into a process that never returns.
func TestHealthCheckIsBounded(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) {
		c.HealthTimeout = 30 * time.Second
		c.HealthInterval = 5 * time.Second
	})
	env.publish("1.1.0", "")
	env.runner.setUnhealthy("1.1.0", true)

	start := env.clock.Now()
	if _, err := env.u.Run(context.Background()); !IsRolledBack(err) {
		t.Fatalf("Run error = %v, want a rollback", err)
	}
	elapsed := env.clock.Now().Sub(start)
	if elapsed > 60*time.Second {
		t.Errorf("the health check waited %s of simulated time; the 30s bound was not honoured", elapsed)
	}
	// The failure message has to say it timed out rather than only quoting
	// the last probe.
	probes := env.runner.callsMatching("systemctl is-active")
	if len(probes) < 2 {
		t.Errorf("only %d health probes were made; the check did not retry", len(probes))
	}
}

// ------------------------------------------------------------- preflight

func TestPreflightRefusesWithoutDiskSpace(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.u.deps.DiskFree = func(string) (uint64, error) { return 1 << 20, nil } // 1 MiB

	res, err := env.u.Run(context.Background())

	var pre *PreflightError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %#v, want *PreflightError", err)
	}
	if !containsAll(pre.Error(), "free space", "database dump", "headroom") {
		t.Errorf("preflight message does not explain the shortfall: %q", pre.Error())
	}
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing switched", res)
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
	if exists(filepath.Join(env.root, "releases", "1.1.0")) {
		t.Error("the release was promoted despite preflight failing")
	}
	if len(env.runner.callsMatching("systemctl restart")) != 0 {
		t.Error("services were restarted despite preflight failing")
	}
	if len(env.runner.callsMatching("pg_dump")) != 0 {
		t.Error("the database was dumped despite preflight failing")
	}
}

func TestPreflightRefusesWhenServicesAreAlreadyUnhealthy(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.setUnhealthy("1.0.0", true)

	_, err := env.u.Run(context.Background())
	var pre *PreflightError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %#v, want *PreflightError", err)
	}
	if !strings.Contains(pre.Error(), "not healthy before the upgrade") {
		t.Errorf("preflight message = %q, want it to name the pre-existing outage", pre.Error())
	}
}

// Preflight reports every problem at once, so the operator fixes them in one
// pass rather than rerunning the upgrade per failure.
func TestPreflightCollectsEveryFailure(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.u.deps.DiskFree = func(string) (uint64, error) { return 1 << 20, nil }
	env.runner.setUnhealthy("1.0.0", true)

	_, err := env.u.Run(context.Background())
	var pre *PreflightError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %#v, want *PreflightError", err)
	}
	if len(pre.Failures) < 2 {
		t.Errorf("preflight reported %d failures, want both the disk and the health problem: %v",
			len(pre.Failures), pre.Failures)
	}
}

// An already-installed version is refused before anything is extracted.
func TestUpgradeRefusesAnExistingReleaseDirectory(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	makeReleases(t, env, "1.1.0")
	mustWriteFile(t, filepath.Join(env.root, "releases", "1.1.0", "SENTINEL"), "do not touch\n")

	_, err := env.u.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("Run error = %v, want a refusal naming the existing directory", err)
	}
	if !exists(filepath.Join(env.root, "releases", "1.1.0", "SENTINEL")) {
		t.Error("the existing release directory was overwritten")
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
}

// ------------------------------------------------------------- database

func TestUpgradeAbortsWhenTheDatabaseDumpFails(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.dumpFails = true

	res, err := env.u.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to upgrade without one") {
		t.Fatalf("Run error = %v, want the upgrade to refuse to proceed without a dump", err)
	}
	if res.Switched {
		t.Error("the switch happened without a database backup")
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
}

// A dump command that exits zero without writing anything is not a backup, and
// treating it as one would be the worst kind of quiet failure.
func TestUpgradeRejectsAnEmptyDump(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.dumpWritesNothing = true

	_, err := env.u.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "was not created") {
		t.Fatalf("Run error = %v, want a complaint that no dump file appeared", err)
	}
}

func TestUpgradeSkipsTheDumpWhenNoDatabaseIsConfigured(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.Database = DatabaseBackupConfig{} })
	env.publish("1.1.0", "")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DatabaseBackupPath != "" {
		t.Errorf("DatabaseBackupPath = %q, want empty", res.DatabaseBackupPath)
	}
	if !env.hasStep(StepBackupDatabase, StatusSkipped) {
		t.Errorf("steps = %v, want the database step reported as skipped", env.steps())
	}
}

// ------------------------------------------------------------- concurrency

// Requirement 10: an upgrade must not be startable twice.
func TestRunRefusesWhenAnotherUpgradeHoldsTheLock(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	m := env.publish("1.1.0", "")

	env.u.deps.ProcessAlive = func(int) bool { return true }
	if _, err := env.u.acquireLock("1.1.0"); err != nil {
		t.Fatalf("acquireLock: %v", err)
	}

	res, err := env.u.Run(context.Background())
	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("Run error = %#v, want *LockedError", err)
	}
	if env.http.hitCount(testFeedURL) != 0 {
		t.Error("the feed was fetched even though the lock was held")
	}
	if env.http.hitCount(m.TarballURL) != 0 {
		t.Error("the tarball was fetched even though the lock was held")
	}
	if res.Switched {
		t.Error("the switch happened even though the lock was held")
	}
	if !env.hasStep(StepLock, StatusFailed) {
		t.Errorf("steps = %v, want a failed lock step", env.steps())
	}
}

// ------------------------------------------------------------- nothing to do

func TestRunOnAnUpToDateInstallation(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.1.0", nil)
	makeReleases(t, env, "1.1.0")
	env.publish("1.1.0", "")

	res, err := env.u.Run(context.Background())
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("Run error = %v, want ErrUpToDate", err)
	}
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing done", res)
	}
	if exists(env.u.LockFile()) {
		t.Error("the lock was not released")
	}
	if len(env.runner.calls) != 0 {
		t.Errorf("commands were run for a no-op upgrade: %v", env.runner.calls)
	}
}

// ------------------------------------------------------------- config

func TestNewFillsDefaults(t *testing.T) {
	t.Parallel()
	u, err := New(Config{FeedURL: "https://example.test/feed.json"}, Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cfg := u.Config()
	if cfg.Root != DefaultRoot {
		t.Errorf("Root = %q, want %q", cfg.Root, DefaultRoot)
	}
	if !equalStrings(cfg.Services, defaultServices) {
		t.Errorf("Services = %v, want %v", cfg.Services, defaultServices)
	}
	if cfg.KeepReleases != DefaultKeepReleases || cfg.HealthTimeout != DefaultHealthTimeout {
		t.Errorf("retention/timeout defaults not applied: %+v", cfg)
	}
	if u.LockFile() != filepath.Join(DefaultRoot, "etc", "upgrade.lock") {
		t.Errorf("LockFile = %q", u.LockFile())
	}
	if u.CurrentLink() != filepath.Join(DefaultRoot, "current") {
		t.Errorf("CurrentLink = %q", u.CurrentLink())
	}
}

func TestNewRejectsARelativeRoot(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Root: "relative/path"}, Deps{}); err == nil {
		t.Fatal("New accepted a relative root; every derived path would move with the working directory")
	}
}

func TestStepStringsAreStable(t *testing.T) {
	t.Parallel()
	// These identifiers appear in API payloads, so a rename is a breaking
	// change and has to be a deliberate one.
	want := map[Step]string{
		StepNone: "none", StepLock: "lock", StepCheck: "check", StepDownload: "download",
		StepVerify: "verify", StepStage: "stage", StepPreflight: "preflight",
		StepBackupDatabase: "backup_database", StepSwitch: "switch", StepRestart: "restart",
		StepHealthCheck: "health_check", StepRollback: "rollback", StepPrune: "prune",
		StepCleanup: "cleanup", StepDone: "done",
	}
	for step, name := range want {
		if step.String() != name {
			t.Errorf("Step(%d).String() = %q, want %q", step, step.String(), name)
		}
		if step.Description() == "" {
			t.Errorf("Step %q has no description", name)
		}
	}
	blob, err := json.Marshal(Event{Step: StepHealthCheck, Status: StatusFailed})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if !strings.Contains(string(blob), `"step":"health_check"`) {
		t.Errorf("event JSON = %s, want the step as a name", blob)
	}
}

// A dump is not a backup until something has read it back. The default
// configuration runs pg_restore --list over the file, which parses the whole
// archive - including its trailer - without touching a database.
func TestUpgradeReadsTheDatabaseDumpBack(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.DatabaseBackupVerified {
		t.Error("the dump was not verified; DatabaseBackupVerified is false")
	}
	verifications := env.runner.callsMatching("pg_restore")
	if len(verifications) != 1 || !strings.Contains(verifications[0], res.DatabaseBackupPath) {
		t.Errorf("dump verification calls = %v, want one pg_restore over %s", verifications, res.DatabaseBackupPath)
	}
	if res.DatabaseBackupSHA256 == "" {
		t.Error("no digest was recorded for the dump")
	}
	onDisk, err := fileSHA256(res.DatabaseBackupPath)
	if err != nil {
		t.Fatalf("hash the dump: %v", err)
	}
	if onDisk != res.DatabaseBackupSHA256 {
		t.Errorf("recorded digest %s does not match the file (%s)", res.DatabaseBackupSHA256, onDisk)
	}

	// The digest is in the state file too, which is where an engineer
	// rebuilding the machine by hand will look.
	raw, err := os.ReadFile(env.u.StateFile())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if st.LastDatabaseBackupSHA256 != res.DatabaseBackupSHA256 {
		t.Errorf("state digest = %q, want %q", st.LastDatabaseBackupSHA256, res.DatabaseBackupSHA256)
	}
}

// A dump that exists, is non-empty, and cannot be restored is the failure mode
// the old "it exited zero and the file is there" check could not see.
func TestUpgradeAbortsWhenTheDumpCannotBeReadBack(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")
	env.runner.verifyFails = true

	res, err := env.u.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "could not be read back") {
		t.Fatalf("Run error = %v, want the unreadable dump to abort the upgrade", err)
	}
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing switched", res)
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
	// The file is kept: it is the only evidence of what went wrong.
	if res.DatabaseBackupPath == "" || !exists(res.DatabaseBackupPath) {
		t.Errorf("the unreadable dump at %q was deleted; it is the only thing left to look at", res.DatabaseBackupPath)
	}
	if len(env.runner.callsMatching("systemctl restart")) != 0 {
		t.Error("services were restarted despite the dump being unusable")
	}
}

// An operator who has no way to read the dump back can say so, and is then
// told nothing was verified rather than being quietly told it was.
func TestDumpVerificationCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.Database.SkipVerify = true })
	env.publish("1.1.0", "")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DatabaseBackupVerified {
		t.Error("DatabaseBackupVerified is true even though verification was skipped")
	}
	if len(env.runner.callsMatching("pg_restore")) != 0 {
		t.Error("the dump was read back even though verification was skipped")
	}
}

// The disk check that matters happens before the download, not after it: there
// is no point pulling a release onto a machine that cannot hold it, and
// discovering that after the extraction is how a disk ends up full.
func TestDiskIsCheckedBeforeAnythingIsDownloaded(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	m := env.publish("1.1.0", "")
	env.u.deps.DiskFree = func(string) (uint64, error) { return 1 << 20, nil }

	_, err := env.u.Run(context.Background())
	var pre *PreflightError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %#v, want *PreflightError", err)
	}
	if env.http.hitCount(m.TarballURL) != 0 {
		t.Error("the tarball was downloaded onto a disk that could not hold it")
	}
	entries, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	if len(entries) != 0 {
		t.Errorf("temporary files were written despite the disk check failing: %d entries", len(entries))
	}
}

// A download of a different length is a different file, and saying so is
// clearer than letting it fail as a checksum mismatch.
func TestDownloadOfTheWrongLengthIsRefused(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	m := env.publish("1.1.0", "")
	m.SizeBytes += 512
	env.publishManifests(m)

	res, err := env.u.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bytes, but the manifest says") {
		t.Fatalf("Run error = %v, want the length mismatch to be named", err)
	}
	if res.Switched {
		t.Error("the switch happened despite the download being the wrong length")
	}
	tmpEntries, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	for _, e := range tmpEntries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			t.Errorf("the rejected download %s was left on disk", e.Name())
		}
	}
}

// An archive that would escape the staging directory must fail the upgrade with
// the installation untouched - the extractor's own tests prove the refusal, this
// one proves Run is wired to it.
func TestUpgradeRefusesAnArchiveThatWouldEscapeStaging(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	evil := buildTarGz(t, []tarEntry{
		{Name: "VERSION", Body: "1.1.0\n"},
		{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "."},
		{Name: "a/b", Typeflag: tar.TypeSymlink, Linkname: ".."},
		{Name: "b/c/etc/cron.d/pwn", Body: "* * * * * root /bin/sh\n"},
	})
	url := testTarBase + "vkai-panel-1.1.0.tar.gz"
	env.http.serve(url, evil)
	env.publishManifests(Manifest{
		Version:    "1.1.0",
		TarballURL: url,
		SHA256:     sha256Hex(evil),
		SizeBytes:  int64(len(evil)),
	})

	res, err := env.u.Run(context.Background())
	var unsafeErr *UnsafeArchiveError
	if !errors.As(err, &unsafeErr) {
		t.Fatalf("Run error = %v, want *UnsafeArchiveError", err)
	}
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing switched", res)
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
	if exists(filepath.Join(env.root, "etc", "cron.d")) {
		t.Fatal("the extraction escaped the staging directory")
	}
	// The staging directory is cleaned up rather than left half-written.
	releaseEntries, _ := os.ReadDir(filepath.Join(env.root, "releases"))
	for _, e := range releaseEntries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("staging directory %s was left behind after a refused archive", e.Name())
		}
	}
}
