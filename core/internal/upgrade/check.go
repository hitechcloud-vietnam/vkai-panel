package upgrade

// Deciding whether an upgrade applies, and to what.
//
// This is the only step that can refuse an upgrade for a reason the operator
// has to act on rather than fix - min_upgrade_from is a property of the
// release, not of the machine - so the refusal has to carry the way forward
// with it.
//
// # Why min_upgrade_from is read from every release, not just the target
//
// The first version of this file asked one question: does the newest release
// name a min_upgrade_from that this installation fails? A release that omits
// the field was therefore installable from anywhere. But the field is not a
// property of the jump, it is a property of the migration inside a particular
// release, and jumping over that release skips the migration whatever the
// release on the far side says about itself. So 1.0.0 -> 3.0.0 is refused when
// 2.0.0 is in the way and 2.0.0 demands 1.9.0, even if 3.0.0 says nothing at
// all - and, in the case that matters, even if 3.0.0 says nothing at all
// because whoever wrote the feed left the field out on purpose.
//
// That is a real narrowing of what a hostile feed can do, and it is not a
// substitute for signing the feed: a feed that can invent 3.0.0 can also invent
// the history that makes it installable. See Config.ReleasePublicKeys.

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
	res := CheckResult{
		CurrentVersion:    current.String(),
		SignaturesChecked: len(u.releaseKeys) > 0,
	}

	releases, err := u.fetchFeed(ctx)
	if err != nil {
		return res, err
	}
	sortManifestsDescending(releases)
	res.LatestVersion = releases[0].Version

	eligible := u.eligibleReleases(current, releases)
	if len(eligible) > 0 {
		res.LatestVersion = eligible[0].Version
	}

	target, err := u.selectTarget(current, eligible, releases)
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

// eligibleReleases drops the releases this installation must never be moved to
// automatically. Today that is pre-releases: a feed that publishes 2.0.0-rc.1
// for its own testing must not thereby install a release candidate as root on
// every customer machine overnight. An installation already running a
// pre-release has opted in and keeps seeing them.
//
// The input is sorted newest first and so is the output.
func (u *Upgrader) eligibleReleases(current Version, releases []Manifest) []Manifest {
	if u.cfg.AllowPreRelease || current.IsPreRelease() {
		return releases
	}
	out := make([]Manifest, 0, len(releases))
	for _, m := range releases {
		v, err := m.ParsedVersion()
		if err != nil || v.IsPreRelease() {
			continue
		}
		out = append(out, m)
	}
	return out
}

// selectTarget picks the newest installable release and enforces
// min_upgrade_from across everything the upgrade would step over.
//
// all is the full feed, eligible the subset this installation may move to.
// Constraints are read from all, because a release that is not an eligible
// target is still a release the jump skips.
func (u *Upgrader) selectTarget(current Version, eligible, all []Manifest) (Manifest, error) {
	if len(eligible) == 0 {
		return Manifest{}, fmt.Errorf("%w: running %s", ErrUpToDate, current)
	}
	newest := eligible[0]
	newestVer, err := newest.ParsedVersion()
	if err != nil {
		return Manifest{}, err
	}
	if !current.LessThan(newestVer) {
		return Manifest{}, fmt.Errorf("%w: running %s, newest published is %s", ErrUpToDate, current, newestVer)
	}

	blocker, minFrom, err := blockingRelease(current, newestVer, all)
	if err != nil {
		return Manifest{}, err
	}
	if blocker == nil {
		return newest, nil
	}

	jump := &IncompatibleJumpError{
		From:           current.String(),
		To:             newest.Version,
		MinUpgradeFrom: minFrom.String(),
		InstallFirst:   intermediateVersion(current, newestVer, minFrom, all),
	}
	if blocker.Version != newest.Version {
		jump.BlockedBy = blocker.Version
	}
	return Manifest{}, jump
}

// blockingRelease returns the oldest release in (current, target] whose
// min_upgrade_from the running version does not satisfy, or nil when the jump
// is legal. The oldest is the one to report: it is the first wall the operator
// will hit, and the shortest step is towards it.
func blockingRelease(current, target Version, releases []Manifest) (*Manifest, Version, error) {
	var (
		found    *Manifest
		foundVer Version
		minFrom  Version
	)
	for i := range releases {
		m := releases[i]
		if strings.TrimSpace(m.MinUpgradeFrom) == "" {
			continue
		}
		v, err := m.ParsedVersion()
		if err != nil {
			return nil, Version{}, err
		}
		if !current.LessThan(v) || target.LessThan(v) {
			continue
		}
		mf, err := ParseVersion(m.MinUpgradeFrom)
		if err != nil {
			return nil, Version{}, fmt.Errorf("release %s: %w", m.Version, err)
		}
		if !current.LessThan(mf) {
			continue // this release accepts the running version
		}
		if found == nil || v.LessThan(foundVer) {
			found, foundVer, minFrom = &releases[i], v, mf
		}
	}
	return found, minFrom, nil
}

// intermediateVersion names the release the operator should install first.
//
// The best answer is a real published release that is newer than what is
// running, strictly older than the blocked target, at least min_upgrade_from,
// and installable from here in its own right - which now means not only that
// its own min_upgrade_from accepts the running version, but that nothing it
// steps over refuses it either. The smallest such release is the shortest legal
// step. When the feed carries no such release there is nothing to search, and
// min_upgrade_from itself is the honest answer.
func intermediateVersion(current, target, minFrom Version, releases []Manifest) string {
	best := ""
	var bestVer Version
	for _, m := range releases {
		v, err := m.ParsedVersion()
		if err != nil {
			continue
		}
		if !current.LessThan(v) || !v.LessThan(target) {
			continue
		}
		if v.LessThan(minFrom) {
			continue
		}
		if blocker, _, err := blockingRelease(current, v, releases); err != nil || blocker != nil {
			continue
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
