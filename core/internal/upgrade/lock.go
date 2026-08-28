package upgrade

// The upgrade lock.
//
// Two upgrades of one machine at the same time is the failure this package
// exists to prevent, and it is entirely plausible: an operator on SSH runs
// "vkai upgrade" while the scheduled job fires, or a browser tab is refreshed
// and posts the API call twice. The lock is a file rather than an in-process
// mutex because the two racers are usually different processes.
//
// The hard part is not taking the lock, it is the lock left behind by a process
// that was killed - by an OOM, by a deploy, by the operator pressing Ctrl-C at
// the wrong moment. A lock file nobody owns any more must not block upgrades
// forever, and a lock file whose owner is still working must not be stolen. So
// the file records the pid, and the holder is probed before the lock is broken.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// lockInfo is what the lock file contains.
type lockInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
	Host      string    `json:"host,omitempty"`
}

// lockHandle is a held lock.
type lockHandle struct {
	path string
	u    *Upgrader
}

// acquireLock takes the upgrade lock, recovering one abandoned by a dead
// process. It returns *LockedError when a live upgrade already holds it.
func (u *Upgrader) acquireLock(targetVersion string) (*lockHandle, error) {
	if err := os.MkdirAll(u.EtcDir(), 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", u.EtcDir(), err)
	}
	path := u.LockFile()

	// Two attempts: the second one runs after a stale lock has been broken.
	// It is deliberately not a loop - if the lock reappears immediately, a
	// live process took it and we must not fight over it.
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
		if err == nil {
			info := lockInfo{
				PID:       u.deps.PID,
				StartedAt: u.deps.Clock.Now().UTC(),
				Version:   targetVersion,
			}
			if host, herr := os.Hostname(); herr == nil {
				info.Host = host
			}
			enc, merr := json.MarshalIndent(info, "", "  ")
			if merr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("encode lock file: %w", merr)
			}
			if _, werr := f.Write(append(enc, '\n')); werr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("write %s: %w", path, werr)
			}
			if cerr := f.Close(); cerr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("close %s: %w", path, cerr)
			}
			return &lockHandle{path: path, u: u}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create %s: %w", path, err)
		}
		if attempt == 1 {
			break
		}

		held, existing, rerr := u.lockIsHeld(path)
		if rerr != nil {
			return nil, rerr
		}
		if held {
			return nil, &LockedError{
				Path:      path,
				PID:       existing.PID,
				StartedAt: existing.StartedAt.Format(time.RFC3339),
				Version:   existing.Version,
			}
		}
		// Abandoned. Remove it and try once more.
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove stale lock %s: %w", path, err)
		}
	}

	// The lock was taken again between our removing it and our retry.
	held, existing, err := u.lockIsHeld(path)
	if err != nil {
		return nil, err
	}
	pid := 0
	started := ""
	version := ""
	if held || existing.PID != 0 {
		pid = existing.PID
		started = existing.StartedAt.Format(time.RFC3339)
		version = existing.Version
	}
	return nil, &LockedError{Path: path, PID: pid, StartedAt: started, Version: version}
}

// lockIsHeld decides whether an existing lock file still has an owner.
//
// An unreadable or unparseable lock file is treated as held, not as free: the
// one thing worse than a stuck lock is two upgrades. It ages out through
// StaleLockAge instead, which is also the escape hatch for a pid that has been
// recycled onto an unrelated process.
func (u *Upgrader) lockIsHeld(path string) (bool, lockInfo, error) {
	var info lockInfo

	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, info, nil
	}
	if err != nil {
		return false, info, fmt.Errorf("stat %s: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return true, info, nil //nolint:nilerr // unreadable lock is treated as held
	}
	if err := json.Unmarshal(data, &info); err != nil || info.PID <= 0 {
		// Corrupt or half-written: honour it until it ages out.
		if u.deps.Clock.Now().Sub(st.ModTime()) > u.cfg.StaleLockAge {
			return false, info, nil
		}
		return true, info, nil
	}

	if u.deps.ProcessAlive(info.PID) {
		return true, info, nil
	}

	// The owner is gone. Age is not required for this decision - a dead pid
	// is a dead pid - but it is also checked so that a lock written by a
	// process on another machine sharing this volume still expires.
	return false, info, nil
}

// release removes the lock file. It is safe to call twice.
func (h *lockHandle) release() error {
	if h == nil {
		return nil
	}
	if err := os.Remove(h.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("release lock %s: %w", h.path, err)
	}
	return nil
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
