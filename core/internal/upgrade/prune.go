package upgrade

// Release retention.
//
// Releases accumulate, and on a small VPS a year of them is the reason the disk
// filled up. Pruning is therefore part of a successful upgrade rather than a
// separate chore an operator has to remember.
//
// Two directories are never candidates, no matter what the retention count
// says: the one the current symlink points at, and the one before it. The
// current one is obvious. The previous one is the rollback target, and a
// retention policy that deletes it turns the next bad release from "rolled back
// in ninety seconds" into "restore the machine from backup".

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// releaseEntry is one directory under releases/ whose name parses as a version.
type releaseEntry struct {
	Name    string
	Path    string
	Version Version
}

// listReleases returns every release directory, newest first. Anything whose
// name is not a semantic version - staging directories, an operator's
// "1.2.3.bak" - is ignored rather than deleted: this package only removes what
// it recognises as its own.
func (u *Upgrader) listReleases() ([]releaseEntry, error) {
	dir := u.ReleasesDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var out []releaseEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		v, err := ParseVersion(e.Name())
		if err != nil {
			continue
		}
		out = append(out, releaseEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Version: v,
		})
	}

	// Newest first.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Version.Compare(out[j].Version) < 0; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}

// prune keeps the newest Config.KeepReleases releases and removes the rest.
//
// keepPaths are directories that survive regardless of the count - the current
// release and the previous one. They are counted against the budget, so in the
// normal case, where the current release is the newest, KeepReleases is exactly
// how many survive. When the current release is not the newest - an operator
// mid-rollback - the protected pair is kept on top of the budget, because the
// alternative is deleting the release the machine is running from.
func (u *Upgrader) prune(keepPaths ...string) ([]string, error) {
	releases, err := u.listReleases()
	if err != nil {
		return nil, err
	}

	protected := make(map[string]bool, len(keepPaths))
	for _, p := range keepPaths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		protected[filepath.Clean(p)] = true
	}

	kept := 0
	var removed []string
	var failures []string

	for _, r := range releases {
		if protected[r.Path] {
			kept++
			continue
		}
		if kept < u.cfg.KeepReleases {
			kept++
			continue
		}
		if err := os.RemoveAll(r.Path); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.Path, err))
			continue
		}
		removed = append(removed, r.Path)
	}

	if len(failures) > 0 {
		return removed, fmt.Errorf("could not remove %d old release(s): %s",
			len(failures), strings.Join(failures, "; "))
	}
	return removed, nil
}

// cleanupStagingDirs removes staging directories this process left behind.
// A staging directory from a crashed run is dead weight, but one belonging to a
// live upgrade is not ours to touch, so only our own pid's are removed.
func (u *Upgrader) cleanupStagingDirs() {
	entries, err := os.ReadDir(u.ReleasesDir())
	if err != nil {
		return
	}
	mine := fmt.Sprintf("-%d", u.deps.PID)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), ".staging-") {
			continue
		}
		if !strings.HasSuffix(e.Name(), mine) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(u.ReleasesDir(), e.Name()))
	}
}
