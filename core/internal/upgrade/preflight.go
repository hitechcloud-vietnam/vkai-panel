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

// releaseExpansionEstimate is how much bigger an extracted release is assumed
// to be than its gzipped tarball, used only before the download when the real
// size is not known yet. Three is what the panel's own releases measure at;
// four is that rounded up. Guessing high costs a refusal on a nearly full disk,
// which is the failure worth having: the alternative is filling the disk half
// way through extraction, on a machine nobody can log in to.
const releaseExpansionEstimate = 4

// earlyPreflight runs the checks that do not need the release on disk, before
// anything is downloaded.
//
// The full preflight below happens after the download and the extraction, which
// is the right place for the checks that measure what was extracted - and the
// wrong place for "is there room for any of this at all". Pulling a gigabyte
// onto a disk with a hundred megabytes free, expanding it, and only then
// reporting that there was never enough room is a bad way to find out, so the
// disk and the services are checked here as well.
func (u *Upgrader) earlyPreflight(ctx context.Context, m Manifest) error {
	var failures []string

	need := u.requiredFreeBeforeDownload(m)
	free, err := u.deps.DiskFree(u.cfg.Root)
	if err != nil {
		failures = append(failures, fmt.Sprintf("cannot measure free space on %s: %v", u.cfg.Root, err))
	} else if int64(free) < need {
		failures = append(failures, fmt.Sprintf(
			"not enough free space on %s: %s available, %s required (%s for the download and the release it expands to, %s for the database dump, %s headroom)",
			u.cfg.Root, humanBytes(int64(free)), humanBytes(need),
			humanBytes(u.downloadFootprint(m)), humanBytes(u.dumpReservation()), humanBytes(u.cfg.DiskSafetyMargin)))
	}

	// Upgrading an installation that is already broken means the health
	// check after the switch cannot distinguish "the new release is bad"
	// from "it was never up". That is worth knowing before the download,
	// not after it.
	if err := u.healthCheck(ctx); err != nil {
		failures = append(failures, fmt.Sprintf("services are not healthy before the upgrade: %v", err))
	}

	if len(failures) > 0 {
		return &PreflightError{Failures: failures}
	}
	return nil
}

// downloadFootprint is what the download plus its extraction is expected to
// occupy. Zero when the manifest does not publish a size, in which case this
// check reduces to "there is room for the dump and the headroom", which is
// still better than no check at all.
func (u *Upgrader) downloadFootprint(m Manifest) int64 {
	if m.SizeBytes <= 0 {
		return 0
	}
	return m.SizeBytes + m.SizeBytes*releaseExpansionEstimate
}

func (u *Upgrader) dumpReservation() int64 {
	if !u.cfg.Database.Enabled {
		return 0
	}
	return u.cfg.Database.EstimateBytes
}

// requiredFreeBeforeDownload is what has to be free before the first byte is
// fetched.
func (u *Upgrader) requiredFreeBeforeDownload(m Manifest) int64 {
	return u.downloadFootprint(m) + u.dumpReservation() + u.cfg.DiskSafetyMargin
}

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
	// margin.
	//
	// The accounting is deliberately conservative. By this point the tarball
	// and the extracted release are both already on disk, and the promotion
	// is a rename, which costs nothing; what still has to fit is the dump
	// and the headroom. Counting the staged release again on top of that is
	// an over-estimate roughly the size of the tarball plus the extraction,
	// which is exactly the space a failed upgrade has to have free to clean
	// up after itself.
	need := u.requiredFreeBytes(stagedSize)
	free, err := u.deps.DiskFree(u.cfg.Root)
	if err != nil {
		failures = append(failures, fmt.Sprintf("cannot measure free space on %s: %v", u.cfg.Root, err))
	} else if int64(free) < need {
		failures = append(failures, fmt.Sprintf(
			"not enough free space on %s: %s available, %s required (%s for the release, %s for the database dump, %s headroom)",
			u.cfg.Root, humanBytes(int64(free)), humanBytes(need),
			humanBytes(stagedSize), humanBytes(u.dumpReservation()), humanBytes(u.cfg.DiskSafetyMargin)))
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
	return stagedSize + u.dumpReservation() + u.cfg.DiskSafetyMargin
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
