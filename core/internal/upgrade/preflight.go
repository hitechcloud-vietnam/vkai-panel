package upgrade

// The last checks before anything about the running installation changes.
//
// Every one of these is a condition that, if ignored, turns into a failure
// half-way through the switch - the moment when the panel is already down and
// the options have narrowed to "roll back and hope". Checking them here costs a
// second and turns those into a refusal with the installation untouched, which
// is why preflight collects every failure instead of returning the first: an
// operator who has to free disk space, then rerun, then discover a service was
// already dead, has been made to do the upgrade three times.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// preflight runs every pre-switch check against the staged release.
func (u *Upgrader) preflight(ctx context.Context, staging string, m Manifest) error {
	var failures []string

	releaseDir := u.ReleaseDir(m.Version)

	// 1. The target must not already exist. If it does, either this version
	// is already installed or a previous run left a directory behind, and
	// overwriting it would mutate a release that something may be running.
	if _, err := os.Lstat(releaseDir); err == nil {
		failures = append(failures, fmt.Sprintf(
			"release directory %s already exists; remove it or pick another version before upgrading", releaseDir))
	} else if !errors.Is(err, fs.ErrNotExist) {
		failures = append(failures, fmt.Sprintf("cannot inspect %s: %v", releaseDir, err))
	}

	// 2. The staged release must be there and non-empty.
	stagedSize, err := dirSize(staging)
	switch {
	case err != nil:
		failures = append(failures, fmt.Sprintf("staged release at %s is unreadable: %v", staging, err))
	case stagedSize == 0:
		failures = append(failures, fmt.Sprintf("staged release at %s contains no data", staging))
	}

	// 3. The releases directory has to be writable, because the promotion
	// and the symlink swap both write into it.
	if err := checkWritableDir(u.ReleasesDir()); err != nil {
		failures = append(failures, fmt.Sprintf("release directory is not writable: %v", err))
	}
	// The install root itself has to be writable too: the current symlink
	// is replaced by a rename inside it.
	if err := checkWritableDir(u.cfg.Root); err != nil {
		failures = append(failures, fmt.Sprintf("install root is not writable: %v", err))
	}

	// 4. Disk space for the new release plus the database dump plus a
	// margin. The new release is counted even though it is already staged,
	// because the promotion is a rename and costs nothing - what has to fit
	// is the dump and the headroom the next steps will use.
	need := u.requiredFreeBytes(stagedSize)
	free, err := u.deps.DiskFree(u.cfg.Root)
	if err != nil {
		failures = append(failures, fmt.Sprintf("cannot measure free space on %s: %v", u.cfg.Root, err))
	} else if int64(free) < need {
		failures = append(failures, fmt.Sprintf(
			"not enough free space on %s: %s available, %s required (%s for the release, %s for the database dump, %s headroom)",
			u.cfg.Root, humanBytes(int64(free)), humanBytes(need),
			humanBytes(stagedSize), humanBytes(u.cfg.Database.EstimateBytes), humanBytes(u.cfg.DiskSafetyMargin)))
	}

	// 5. The services have to be healthy now. Upgrading an installation
	// that is already broken means the health check after the switch cannot
	// distinguish "the new release is bad" from "it was never up", and the
	// rollback would restore something equally dead.
	if err := u.healthCheck(ctx); err != nil {
		failures = append(failures, fmt.Sprintf("services are not healthy before the upgrade: %v", err))
	}

	if len(failures) > 0 {
		return &PreflightError{Failures: failures}
	}
	return nil
}

// requiredFreeBytes is what preflight insists on: the release, the dump, and
// the margin.
func (u *Upgrader) requiredFreeBytes(stagedSize int64) int64 {
	need := stagedSize + u.cfg.DiskSafetyMargin
	if u.cfg.Database.Enabled {
		need += u.cfg.Database.EstimateBytes
	}
	return need
}

// checkWritableDir proves a directory exists and that this process can create
// entries in it, by creating one and removing it. Checking the mode bits
// instead would be wrong under a read-only mount, an ACL or a full filesystem.
func checkWritableDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	probe, err := os.CreateTemp(dir, ".vkai-upgrade-probe-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("%s: cannot remove probe file %s: %w", dir, filepath.Base(name), err)
	}
	return nil
}

// humanBytes renders a size the way an operator reading a refusal wants it.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}
