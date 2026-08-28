package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLockPreventsASecondUpgrade(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	// The holder is alive.
	env.u.deps.ProcessAlive = func(int) bool { return true }

	first, err := env.u.acquireLock("1.1.0")
	if err != nil {
		t.Fatalf("first acquireLock: %v", err)
	}

	_, err = env.u.acquireLock("1.1.0")
	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("second acquireLock = %v, want *LockedError", err)
	}
	if locked.PID != 4242 || locked.Version != "1.1.0" {
		t.Errorf("LockedError = %+v, want pid 4242 upgrading to 1.1.0", locked)
	}
	if !containsAll(locked.Error(), "already running", "4242", "1.1.0") {
		t.Errorf("LockedError message is not actionable: %q", locked.Error())
	}

	if err := first.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Releasing twice must not be an error - Run's deferred release can run
	// after an earlier cleanup path already removed the file.
	if err := first.release(); err != nil {
		t.Fatalf("second release: %v", err)
	}

	third, err := env.u.acquireLock("1.1.0")
	if err != nil {
		t.Fatalf("acquireLock after release: %v", err)
	}
	_ = third.release()
}

// TestStaleLockIsRecovered is requirement 10: a lock left by a process that was
// killed must not block upgrades forever.
func TestStaleLockIsRecovered(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	stale := lockInfo{
		PID:       99999,
		StartedAt: env.clock.Now().Add(-2 * time.Hour),
		Version:   "1.1.0",
		Host:      "panel-01",
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(env.u.LockFile(), data, 0o640); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	probed := 0
	env.u.deps.ProcessAlive = func(pid int) bool {
		probed++
		if pid != 99999 {
			t.Errorf("probed pid %d, want the pid recorded in the lock file", pid)
		}
		return false // the process is gone
	}

	handle, err := env.u.acquireLock("1.2.0")
	if err != nil {
		t.Fatalf("acquireLock over a stale lock: %v", err)
	}
	if probed == 0 {
		t.Error("the recorded pid was never probed; the lock was broken without checking its owner")
	}

	// The lock file now belongs to us.
	raw, err := os.ReadFile(env.u.LockFile())
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	var got lockInfo
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse lock: %v", err)
	}
	if got.PID != 4242 || got.Version != "1.2.0" {
		t.Errorf("lock file = %+v, want pid 4242 upgrading to 1.2.0", got)
	}
	_ = handle.release()
}

// A stale lock must be recovered by the full Run path too, not only by the
// helper - otherwise one killed upgrade takes the feature out of service.
func TestRunRecoversStaleLock(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	stale, _ := json.Marshal(lockInfo{PID: 99999, StartedAt: env.clock.Now().Add(-time.Hour), Version: "1.1.0"})
	if err := os.WriteFile(env.u.LockFile(), stale, 0o640); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run over a stale lock: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("result = %+v, want a successful upgrade", res)
	}
	if exists(env.u.LockFile()) {
		t.Error("the lock file was left behind after a successful upgrade")
	}
}

// A lock whose owner is still alive must survive, even if it is old: the
// upgrade it belongs to may legitimately be downloading a large tarball.
func TestLiveLockIsNotBrokenByAge(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.u.deps.ProcessAlive = func(int) bool { return true }

	old, _ := json.Marshal(lockInfo{PID: 1234, StartedAt: env.clock.Now().Add(-48 * time.Hour), Version: "1.1.0"})
	if err := os.WriteFile(env.u.LockFile(), old, 0o640); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if _, err := env.u.acquireLock("1.2.0"); !errors.As(err, new(*LockedError)) {
		t.Fatalf("acquireLock = %v, want *LockedError for a live holder", err)
	}
}

// A corrupt lock file is honoured rather than ignored - two concurrent upgrades
// is the worse failure - but it does age out so it cannot wedge the panel.
func TestCorruptLockIsHeldUntilItAgesOut(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	if err := os.WriteFile(env.u.LockFile(), []byte("{half-written"), 0o640); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	// A corrupt lock carries no start time, so its age is its mtime. Pin
	// that to the fake clock, or the test would be comparing injected time
	// against the machine's real clock.
	now := env.clock.Now()
	if err := os.Chtimes(env.u.LockFile(), now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if _, err := env.u.acquireLock("1.1.0"); !errors.As(err, new(*LockedError)) {
		t.Fatalf("acquireLock = %v, want *LockedError for a corrupt but fresh lock", err)
	}

	// Move the clock past StaleLockAge; the file's mtime stays where it is.
	env.clock.Sleep(DefaultStaleLockAge + time.Hour)

	handle, err := env.u.acquireLock("1.1.0")
	if err != nil {
		t.Fatalf("acquireLock after the corrupt lock aged out: %v", err)
	}
	_ = handle.release()
}

func TestStateFileRoundTrip(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	want := state{
		CurrentVersion:     "1.1.0",
		PreviousVersion:    "1.0.0",
		LastUpgradeAt:      env.clock.Now().UTC(),
		LastDatabaseBackup: "/vkai-panel/www/backup/databases/x.dump",
	}
	if err := env.u.saveState(want); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := env.u.loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.CurrentVersion != want.CurrentVersion || got.PreviousVersion != want.PreviousVersion ||
		got.LastDatabaseBackup != want.LastDatabaseBackup || !got.LastUpgradeAt.Equal(want.LastUpgradeAt) {
		t.Errorf("loadState = %+v, want %+v", got, want)
	}
	if exists(env.u.StateFile() + ".tmp") {
		t.Error("saveState left its temporary file behind")
	}
}

// With no CurrentVersion configured the running version comes from the state
// file, and failing that from the current symlink.
func TestCurrentVersionFallsBackToStateThenSymlink(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, "1.0.0", func(c *Config) { c.CurrentVersion = "" })

	v, err := env.u.currentVersion()
	if err != nil {
		t.Fatalf("currentVersion from symlink: %v", err)
	}
	if v.String() != "1.0.0" {
		t.Errorf("currentVersion = %s, want 1.0.0 from the symlink", v)
	}

	if err := env.u.saveState(state{CurrentVersion: "1.3.0"}); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	v, err = env.u.currentVersion()
	if err != nil {
		t.Fatalf("currentVersion from state: %v", err)
	}
	if v.String() != "1.3.0" {
		t.Errorf("currentVersion = %s, want 1.3.0 from the state file", v)
	}
}

// newLockPeer builds an independent Upgrader against a shared root, as a second
// process would see it.
func newLockPeer(t *testing.T, root string, pid int, alive func(int) bool) *Upgrader {
	t.Helper()
	if alive == nil {
		alive = func(int) bool { return false }
	}
	u, err := New(Config{Root: root, StaleLockAge: DefaultStaleLockAge}, Deps{
		Clock:        newFakeClock(),
		ProcessAlive: alive,
		PID:          pid,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return u
}

// TestConcurrentLockAcquisitionHasExactlyOneWinner is the test the audit said
// was missing: not "can the lock be taken" but "can it be taken twice".
//
// Every goroutine here is a separate Upgrader with its own pid, which is what a
// second process looks like from the filesystem's point of view. flock(2) is
// per open file description, so two descriptors in one process contend exactly
// as two processes would.
func TestConcurrentLockAcquisitionHasExactlyOneWinner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	const peers = 8
	var (
		mu      sync.Mutex
		handles []*lockHandle
		locked  int
		other   []error
		wg      sync.WaitGroup
		start   = make(chan struct{})
	)

	for i := 0; i < peers; i++ {
		u := newLockPeer(t, root, 5000+i, nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			h, err := u.acquireLock("2.0.0")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				handles = append(handles, h)
			case errors.As(err, new(*LockedError)):
				locked++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("unexpected errors: %v", other)
	}
	if len(handles) != 1 {
		t.Fatalf("%d of %d upgrades took the lock at once; want exactly 1 (the rest reported: %d locked)",
			len(handles), peers, locked)
	}
	if locked != peers-1 {
		t.Errorf("%d peers were told the lock was held, want %d", locked, peers-1)
	}

	// And the winner can hand it on once it is done.
	if err := handles[0].release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	next, err := newLockPeer(t, root, 6000, nil).acquireLock("2.0.1")
	if err != nil {
		t.Fatalf("acquireLock after release: %v", err)
	}
	_ = next.release()
}

// TestStaleLockRecoveryIsNotRacy is the regression test for the second finding
// in the audit's proof of concept: two upgrades that both find the same
// abandoned lock, both decide it is abandoned, and both take it.
//
// The interleaving is forced rather than hoped for. While the second upgrade is
// probing the recorded owner - the exact window the old implementation decided
// in - the first one runs to completion and takes the lock.
func TestStaleLockRecoveryIsNotRacy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	var (
		mu         sync.Mutex
		firstHold  *lockHandle
		firstErr   error
		firstTried bool
	)

	first := newLockPeer(t, root, 1001, nil)
	second := newLockPeer(t, root, 1002, func(pid int) bool {
		if pid == 4242 {
			mu.Lock()
			if !firstTried {
				firstTried = true
				firstHold, firstErr = first.acquireLock("2.0.0")
			}
			mu.Unlock()
		}
		return false // pid 4242 is gone either way
	})

	if err := os.MkdirAll(first.EtcDir(), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale, _ := json.Marshal(lockInfo{PID: 4242, StartedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Version: "1.0.0"})
	if err := os.WriteFile(first.LockFile(), stale, 0o640); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	secondHold, secondErr := second.acquireLock("2.0.0")

	mu.Lock()
	defer mu.Unlock()
	held := 0
	if firstErr == nil && firstHold != nil {
		held++
	}
	if secondErr == nil && secondHold != nil {
		held++
	}
	if held != 1 {
		t.Fatalf("%d upgrades hold the lock (first: %v, second: %v); want exactly 1", held, firstErr, secondErr)
	}
	// Whichever lost was told why, in a form the CLI can act on.
	loser := firstErr
	if loser == nil {
		loser = secondErr
	}
	if !errors.As(loser, new(*LockedError)) {
		t.Errorf("the upgrade that lost the race got %v, want *LockedError", loser)
	}
	_ = firstHold.release()
	_ = secondHold.release()
}

// The lock survives its own release-and-retake cycle: a handle that has already
// removed the file must not let a later release remove somebody else's lock.
func TestReleasingTwiceDoesNotStealTheNextLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	first := newLockPeer(t, root, 7001, nil)
	h1, err := first.acquireLock("1.0.0")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := h1.release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	second := newLockPeer(t, root, 7002, func(int) bool { return true })
	h2, err := second.acquireLock("1.1.0")
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}

	// The first handle releasing again must be a no-op, not a theft.
	if err := h1.release(); err != nil {
		t.Fatalf("second release of the first handle: %v", err)
	}
	if !exists(second.LockFile()) {
		t.Fatal("releasing a handle twice removed a lock held by someone else")
	}
	third := newLockPeer(t, root, 7003, func(int) bool { return true })
	if _, err := third.acquireLock("1.2.0"); !errors.As(err, new(*LockedError)) {
		t.Fatalf("acquireLock = %v, want *LockedError while the second peer holds it", err)
	}
	_ = h2.release()
}
