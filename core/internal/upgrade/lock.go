package upgrade

// The upgrade lock.
//
// Two upgrades of one machine at the same time is the failure this package
// exists to prevent, and it is entirely plausible: an operator on SSH runs
// "vkai upgrade" while the scheduled job fires, or a browser tab is refreshed
// and posts the API call twice. The lock is a file rather than an in-process
// mutex because the two racers are usually different processes.
//
// # Why the lock is held by the kernel and not by the file's existence
//
// The obvious implementation - create the file with O_EXCL, and if it is
// already there decide whether its owner is still alive and delete it if not -
// has a race that two upgrades win together:
//
//	P1 reads the lock, sees pid 4242, probes it, finds it dead
//	P2 reads the lock, sees pid 4242, probes it, finds it dead
//	P1 removes the lock and creates its own
//	P2 removes P1's brand new lock and creates its own
//
// Both then believe they hold "the" lock, and the machine is upgraded twice at
// once. No amount of re-reading closes it: whatever P2 checks, it checks before
// the remove, and P1 can always land in between.
//
// So the lock is a flock(2) on the file, taken non-blocking. Exclusion is then
// the kernel's problem rather than a sequence of file operations we have to get
// right, and the stale-lock case - a process killed by an OOM, by a deploy, by
// Ctrl-C - stops being a case at all: the kernel drops the lock when the
// process dies, no matter how it died. Nothing is ever deleted in order to take
// the lock.
//
// Two details that the kernel does not hand us for free:
//
//   - After the flock succeeds, the file at that path is stat-ed and compared
//     with the file the descriptor refers to. A holder that released the lock
//     between our open and our flock will have unlinked the file, and we would
//     otherwise be holding an exclusive lock on an inode nobody will ever open
//     again while a third process happily creates a new one.
//   - A version of this binary older than this change takes the lock by
//     creating the file and does not flock anything. During the upgrade that
//     installs this change, that older process may be the holder. So the
//     recorded pid is still read and still probed, and a live recorded owner
//     still refuses the lock even when the flock succeeded. That check can only
//     ever refuse; it can never grant.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// lockInfo is what the lock file contains. It is diagnostics - who is upgrading
// this machine, since when, to what - rather than the lock itself.
type lockInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
	Host      string    `json:"host,omitempty"`
}

// lockHandle is a held lock. The open file descriptor is the lock: closing it
// releases it, and so does the process exiting.
type lockHandle struct {
	path string
	file *os.File
	u    *Upgrader
}

// acquireLock takes the upgrade lock. It returns *LockedError when another
// upgrade holds it.
func (u *Upgrader) acquireLock(targetVersion string) (*lockHandle, error) {
	if err := os.MkdirAll(u.EtcDir(), 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", u.EtcDir(), err)
	}
	path := u.LockFile()

	// The retries are for one situation only: the holder released and
	// unlinked the lock file between our open and our flock, so the
	// descriptor we hold no longer names the lock. Three attempts, because a
	// fourth would mean something is releasing and retaking the lock in a
	// tight loop, which is not a race to join.
	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|oNoFollow, 0o640)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}

		locked, err := flockExclusiveNonBlocking(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if !locked {
			// The holder is whoever owns the flock; the file only
			// says who wrote it last. In the moment between another
			// upgrade taking the lock and recording itself, this
			// names the previous owner. That is a worse diagnostic,
			// not a worse decision: the refusal is correct either
			// way, and it is the refusal that matters.
			info, _ := readLockInfo(path)
			_ = f.Close()
			return nil, lockedError(path, info)
		}

		fresh, err := describesSameFile(f, path)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if !fresh {
			// The holder unlinked this inode as it released. Open the
			// file that is there now.
			_ = f.Close()
			continue
		}

		held, info, err := u.legacyHolder(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if held {
			_ = f.Close()
			return nil, lockedError(path, info)
		}

		if err := u.writeLockInfo(f, targetVersion); err != nil {
			_ = f.Close()
			return nil, err
		}
		return &lockHandle{path: path, file: f, u: u}, nil
	}

	info, _ := readLockInfo(path)
	return nil, lockedError(path, info)
}

func lockedError(path string, info lockInfo) *LockedError {
	started := ""
	if !info.StartedAt.IsZero() {
		started = info.StartedAt.Format(time.RFC3339)
	}
	return &LockedError{Path: path, PID: info.PID, StartedAt: started, Version: info.Version}
}

// writeLockInfo records who we are in the file we now hold the lock on.
func (u *Upgrader) writeLockInfo(f *os.File, targetVersion string) error {
	info := lockInfo{
		PID:       u.deps.PID,
		StartedAt: u.deps.Clock.Now().UTC(),
		Version:   targetVersion,
	}
	if host, herr := os.Hostname(); herr == nil {
		info.Host = host
	}
	enc, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lock file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("truncate %s: %w", f.Name(), err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("rewind %s: %w", f.Name(), err)
	}
	if _, err := f.Write(append(enc, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", f.Name(), err)
	}
	return nil
}

// legacyHolder reports whether the lock file names an owner that is still
// running. It exists for the one process that can hold this lock without
// holding a flock on it: an older build of the panel, mid-upgrade to this one.
//
// It is deliberately one-directional. A live recorded owner refuses the lock we
// just took from the kernel; nothing here ever grants a lock the kernel
// refused, and nothing here deletes anything.
func (u *Upgrader) legacyHolder(f *os.File) (bool, lockInfo, error) {
	var info lockInfo

	st, err := f.Stat()
	if err != nil {
		return false, info, fmt.Errorf("stat %s: %w", f.Name(), err)
	}
	if st.Size() == 0 {
		return false, info, nil
	}

	data := make([]byte, st.Size())
	if _, err := f.ReadAt(data, 0); err != nil {
		// Unreadable content with a lock we hold: treat it as free
		// rather than wedging the panel. The flock is the real answer.
		return false, info, nil //nolint:nilerr // the kernel lock already decided
	}

	if err := json.Unmarshal(data, &info); err != nil || info.PID <= 0 {
		// Corrupt or half-written. Honour it until it ages out, because
		// a lock we cannot read is a lock we cannot reason about; the
		// age is the escape hatch that stops it wedging the panel.
		if u.deps.Clock.Now().Sub(st.ModTime()) > u.cfg.StaleLockAge {
			return false, lockInfo{}, nil
		}
		return true, lockInfo{}, nil
	}
	if info.PID == u.deps.PID {
		// Our own leftover: this process is the one holding the flock.
		return false, info, nil
	}
	if u.deps.ProcessAlive(info.PID) {
		return true, info, nil
	}
	return false, info, nil
}

// recordVersion updates the held lock with the version being installed. The
// lock is taken before the feed is fetched, so at that point nobody knows what
// the upgrade is to; a second upgrade that arrives later should still be told.
func (h *lockHandle) recordVersion(version string) {
	if h == nil || h.file == nil {
		return
	}
	// Best effort: failing to improve a diagnostic message is not a reason
	// to abort an upgrade that holds the lock and is otherwise fine.
	_ = h.u.writeLockInfo(h.file, version)
}

// readLockInfo reads the lock file for diagnostics. A lock we could not take is
// still a lock we have to describe to the operator.
func readLockInfo(path string) (lockInfo, error) {
	var info lockInfo
	data, err := os.ReadFile(path)
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return lockInfo{}, err
	}
	return info, nil
}

// describesSameFile reports whether f is still the file at path.
func describesSameFile(f *os.File, path string) (bool, error) {
	fromFD, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	onDisk, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return os.SameFile(fromFD, onDisk), nil
}

// release drops the lock: the file is removed while we still hold it, and the
// descriptor is closed afterwards, which is what makes the removal safe.
//
// It is safe to call twice - Run releases in a defer, and an earlier cleanup
// path may already have done it - and a second call removes nothing. Removing
// unconditionally would delete whatever lock file is at that path by then,
// which after a fast release-and-retake is somebody else's.
func (h *lockHandle) release() error {
	if h == nil || h.file == nil {
		return nil
	}
	var firstErr error

	// Only remove the file this handle actually holds. We still hold the
	// kernel lock at this point, so no other upgrade can have taken it, and
	// the answer cannot change under us.
	same, err := describesSameFile(h.file, h.path)
	switch {
	case err != nil:
		firstErr = err
	case same:
		if rerr := os.Remove(h.path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			firstErr = fmt.Errorf("release lock %s: %w", h.path, rerr)
		}
	}

	if cerr := h.file.Close(); cerr != nil && firstErr == nil && !errors.Is(cerr, os.ErrClosed) {
		firstErr = fmt.Errorf("close lock %s: %w", h.path, cerr)
	}
	h.file = nil
	return firstErr
}

// ---------------------------------------------------------------- state file

// state is the small amount the upgrader has to remember between runs. It lives
// in /vkai-panel/etc rather than in the database because the upgrader must keep
// working when the database is what is broken.
type state struct {
	CurrentVersion     string    `json:"current_version"`
	PreviousVersion    string    `json:"previous_version,omitempty"`
	LastUpgradeAt      time.Time `json:"last_upgrade_at,omitempty"`
	LastDatabaseBackup string    `json:"last_database_backup,omitempty"`
	// LastDatabaseBackupSHA256 is the digest of that dump as it was written.
	// An operator restoring a machine by hand can tell whether the file they
	// are holding is the one this upgrade produced.
	LastDatabaseBackupSHA256 string `json:"last_database_backup_sha256,omitempty"`
}

func (u *Upgrader) loadState() (state, error) {
	var st state
	data, err := os.ReadFile(u.StateFile())
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return state{}, fmt.Errorf("parse %s: %w", u.StateFile(), err)
	}
	return st, nil
}

// saveState writes the state file atomically. A half-written state file would
// make the next upgrade unable to name the version it is running.
func (u *Upgrader) saveState(st state) error {
	if err := os.MkdirAll(u.EtcDir(), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", u.EtcDir(), err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode upgrade state: %w", err)
	}
	tmp := u.StateFile() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o640); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, u.StateFile()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install %s: %w", u.StateFile(), err)
	}
	return nil
}

// readCurrentLink returns the absolute path the current symlink points at. A
// relative target is resolved against the directory holding the link, which is
// how the layout stays relocatable.
func (u *Upgrader) readCurrentLink() (string, error) {
	link := u.CurrentLink()
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", link, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	return filepath.Clean(target), nil
}
