package upgrade

// Deciding whether an upgrade applies, and to what.
//
// This is the only step that can refuse an upgrade for a reason the operator
// has to act on rather than fix - min_upgrade_from is a property of the
// release, not of the machine - so the refusal has to carry the way forward
// with it.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Check fetches the feed and reports whether an upgrade applies, without
// touching the installation.
//
// It returns ErrUpToDate when nothing newer is published and an
// *IncompatibleJumpError when the newest release refuses this starting point;
// in both cases the CheckResult is still filled in, so a caller can show the
// operator what it found alongside the reason it will not proceed.
func (u *Upgrader) Check(ctx context.Context) (CheckResult, error) {
	current, err := u.currentVersion()
	if err != nil {
		return CheckResult{}, err
	}
	res := CheckResult{CurrentVersion: current.String()}

	releases, err := u.fetchFeed(ctx)
	if err != nil {
		return res, err
	}
	sortManifestsDescending(releases)
	res.LatestVersion = releases[0].Version

	target, err := selectTarget(current, releases)
	if err != nil {
		var jump *IncompatibleJumpError
		if errors.As(err, &jump) {
			res.Blocked = true
			res.InstallFirst = jump.InstallFirst
		}
		return res, err
	}

	res.Target = &target
	res.UpdateAvailable = true
	return res, nil
}

// currentVersion resolves what is running: Config.CurrentVersion if given, then
// the state file, then the basename of the current symlink's target.
func (u *Upgrader) currentVersion() (Version, error) {
	if s := strings.TrimSpace(u.cfg.CurrentVersion); s != "" {
		v, err := ParseVersion(s)
		if err != nil {
			return Version{}, fmt.Errorf("configured current version: %w", err)
		}
		return v, nil
	}
	if st, err := u.loadState(); err == nil && strings.TrimSpace(st.CurrentVersion) != "" {
		if v, err := ParseVersion(st.CurrentVersion); err == nil {
			return v, nil
		}
	}
	target, err := u.readCurrentLink()
	if err != nil {
		return Version{}, fmt.Errorf("cannot determine the running version: no version configured, no usable state file, and %w", err)
	}
	v, err := ParseVersion(filepath.Base(target))
	if err != nil {
		return Version{}, fmt.Errorf("cannot determine the running version: %s points at %q, which is not a version: %w",
			u.CurrentLink(), target, err)
	}
	return v, nil
}

// selectTarget picks the newest installable release and enforces
// min_upgrade_from.
func selectTarget(current Version, releases []Manifest) (Manifest, error) {
	// releases is sorted newest first.
	newest := releases[0]
	newestVer, err := newest.ParsedVersion()
	if err != nil {
		return Manifest{}, err
	}
	if !current.LessThan(newestVer) {
		return Manifest{}, fmt.Errorf("%w: running %s, newest published is %s", ErrUpToDate, current, newestVer)
	}

	if newest.MinUpgradeFrom == "" {
		return newest, nil
	}
	minFrom, err := ParseVersion(newest.MinUpgradeFrom)
	if err != nil {
		return Manifest{}, fmt.Errorf("release %s: %w", newest.Version, err)
	}
	if !current.LessThan(minFrom) {
		return newest, nil
	}

	return Manifest{}, &IncompatibleJumpError{
		From:           current.String(),
		To:             newest.Version,
		MinUpgradeFrom: minFrom.String(),
		InstallFirst:   intermediateVersion(current, newestVer, minFrom, releases),
	}
}

// intermediateVersion names the release the operator should install first.
//
// The best answer is a real published release that is newer than what is
// running, no newer than the blocked target, at least min_upgrade_from, and
// whose own min_upgrade_from accepts the running version - the smallest such
// release, so the operator takes the shortest legal step. When the feed carries
// only the one manifest there is nothing to search, and min_upgrade_from itself
// is the honest answer.
func intermediateVersion(current, target, minFrom Version, releases []Manifest) string {
	best := ""
	var bestVer Version
	for _, m := range releases {
		v, err := m.ParsedVersion()
		if err != nil {
			continue
		}
		if !current.LessThan(v) || target.LessThan(v) {
			continue
		}
		if v.LessThan(minFrom) {
			continue
		}
		if m.MinUpgradeFrom != "" {
			mf, err := ParseVersion(m.MinUpgradeFrom)
			if err != nil || current.LessThan(mf) {
				continue
			}
		}
		if best == "" || v.LessThan(bestVer) {
			best, bestVer = m.Version, v
		}
	}
	if best != "" {
		return best
	}
	return minFrom.String()
}

func sortManifestsDescending(releases []Manifest) {
	// Insertion sort: a release feed has tens of entries, not thousands,
	// and this keeps the comparison total-order-safe without a closure that
	// has to swallow parse errors.
	for i := 1; i < len(releases); i++ {
		for j := i; j > 0; j-- {
			a, errA := releases[j-1].ParsedVersion()
			b, errB := releases[j].ParsedVersion()
			if errA != nil || errB != nil {
				break
			}
			if a.Compare(b) >= 0 {
				break
			}
			releases[j-1], releases[j] = releases[j], releases[j-1]
		}
	}
}
