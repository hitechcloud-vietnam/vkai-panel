package upgrade

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCheckFindsNewerRelease(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.4.0", nil)
	env.publish("1.5.0", "")

	res, err := env.u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected an update to be available")
	}
	if res.Target == nil || res.Target.Version != "1.5.0" {
		t.Fatalf("target = %+v, want 1.5.0", res.Target)
	}
	if res.CurrentVersion != "1.4.0" || res.LatestVersion != "1.5.0" {
		t.Errorf("current/latest = %s/%s, want 1.4.0/1.5.0", res.CurrentVersion, res.LatestVersion)
	}
}

func TestCheckPicksTheNewestOfSeveral(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.4.0", nil)
	// Deliberately out of order, and with 1.10.0 present so that a string
	// comparison would pick 1.9.0.
	env.publishManifests(
		manifestFor("1.5.0", ""),
		manifestFor("1.10.0", ""),
		manifestFor("1.9.0", ""),
	)

	res, err := env.u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Target.Version != "1.10.0" {
		t.Errorf("target = %s, want 1.10.0", res.Target.Version)
	}
}

func TestCheckUpToDate(t *testing.T) {
	t.Parallel()

	for _, current := range []string{"1.5.0", "1.6.0"} {
		env := newTestEnv(t, current, nil)
		env.publish("1.5.0", "")

		res, err := env.u.Check(context.Background())
		if !errors.Is(err, ErrUpToDate) {
			t.Fatalf("running %s: Check error = %v, want ErrUpToDate", current, err)
		}
		if res.UpdateAvailable {
			t.Errorf("running %s: UpdateAvailable should be false", current)
		}
		if res.LatestVersion != "1.5.0" {
			t.Errorf("running %s: LatestVersion = %q, want 1.5.0 - the caller still needs to see what is published",
				current, res.LatestVersion)
		}
	}
}

// A pre-release must not be offered as an upgrade over the matching release:
// 2.0.0-rc.1 is older than 2.0.0.
func TestCheckDoesNotOfferAPreReleaseOverTheRelease(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "2.0.0", nil)
	env.publishManifests(manifestFor("2.0.0-rc.2", ""))

	if _, err := env.u.Check(context.Background()); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("Check error = %v, want ErrUpToDate", err)
	}
}

// The core of requirement 1: a release that demands a newer starting point is
// refused, and the refusal names the version to install first.
func TestCheckRefusesIncompatibleJump(t *testing.T) {
	t.Parallel()

	t.Run("feed carries the release history", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(
			manifestFor("1.5.0", "1.0.0"),
			manifestFor("1.8.0", "1.5.0"),
			manifestFor("2.0.0", "1.5.0"),
		)

		res, err := env.u.Check(context.Background())
		var jump *IncompatibleJumpError
		if !errors.As(err, &jump) {
			t.Fatalf("Check error = %v, want *IncompatibleJumpError", err)
		}
		if jump.From != "1.0.0" || jump.To != "2.0.0" || jump.MinUpgradeFrom != "1.5.0" {
			t.Errorf("jump = %+v, want from 1.0.0 to 2.0.0 requiring 1.5.0", jump)
		}
		// 1.5.0 is the smallest published release that both satisfies
		// 2.0.0's requirement and accepts 1.0.0 as a starting point.
		if jump.InstallFirst != "1.5.0" {
			t.Errorf("InstallFirst = %q, want 1.5.0", jump.InstallFirst)
		}
		if !res.Blocked || res.InstallFirst != "1.5.0" {
			t.Errorf("CheckResult = %+v, want Blocked with InstallFirst 1.5.0", res)
		}
		if res.UpdateAvailable {
			t.Error("a blocked jump must not be reported as an available update")
		}
		if !containsAll(jump.Error(), "1.0.0", "2.0.0", "1.5.0", "install") {
			t.Errorf("error message does not tell the operator what to do: %q", jump.Error())
		}
	})

	t.Run("feed carries only the newest manifest", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(manifestFor("2.0.0", "1.5.0"))

		_, err := env.u.Check(context.Background())
		var jump *IncompatibleJumpError
		if !errors.As(err, &jump) {
			t.Fatalf("Check error = %v, want *IncompatibleJumpError", err)
		}
		// Nothing to search, so min_upgrade_from is the honest answer.
		if jump.InstallFirst != "1.5.0" {
			t.Errorf("InstallFirst = %q, want 1.5.0", jump.InstallFirst)
		}
	})
}

// The boundary: min_upgrade_from is inclusive, so running exactly that version
// is allowed through.
func TestCheckAllowsExactlyMinUpgradeFrom(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.5.0", nil)
	env.publishManifests(manifestFor("2.0.0", "1.5.0"))

	res, err := env.u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Target.Version != "2.0.0" {
		t.Errorf("target = %+v, want 2.0.0", res.Target)
	}
}

// An upgrade blocked by min_upgrade_from must never download or extract
// anything: the refusal has to happen before the installation is touched.
func TestRunRefusesIncompatibleJumpBeforeDownloading(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	m := manifestFor("2.0.0", "1.5.0")
	env.publishManifests(m)

	res, err := env.u.Run(context.Background())
	var jump *IncompatibleJumpError
	if !errors.As(err, &jump) {
		t.Fatalf("Run error = %v, want *IncompatibleJumpError", err)
	}
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing switched", res)
	}
	if env.http.hitCount(m.TarballURL) != 0 {
		t.Error("the tarball was fetched for a jump that was going to be refused")
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current moved to %s", env.currentTarget())
	}
	if !env.hasStep(StepCheck, StatusFailed) {
		t.Errorf("expected the check step to be reported as failed, got %v", env.steps())
	}
}

func TestCheckReportsFeedFailures(t *testing.T) {
	t.Parallel()

	t.Run("http error status", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.http.serveStatus(testFeedURL, http.StatusInternalServerError, []byte("boom"))
		if _, err := env.u.Check(context.Background()); err == nil {
			t.Fatal("expected an error for a 500 from the feed")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.http.serve(testFeedURL, []byte("{not json"))
		if _, err := env.u.Check(context.Background()); err == nil {
			t.Fatal("expected an error for a malformed feed")
		}
	})

	t.Run("manifest without a checksum", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		m := manifestFor("1.1.0", "")
		m.SHA256 = ""
		env.publishManifests(m)
		if _, err := env.u.Check(context.Background()); err == nil {
			t.Fatal("expected a manifest with no sha256 to be rejected by the feed parser")
		}
	})
}

func TestParseFeedAcceptsBothShapes(t *testing.T) {
	t.Parallel()

	single := `{"version":"1.2.3","released_at":"2026-02-01T00:00:00Z","min_upgrade_from":"1.0.0",` +
		`"tarball_url":"https://example.test/a.tar.gz",` +
		`"sha256":"` + sha256Hex([]byte("x")) + `","changelog_url":"https://example.test/c"}`

	got, err := parseFeed([]byte(single))
	if err != nil {
		t.Fatalf("parseFeed(single): %v", err)
	}
	if len(got) != 1 || got[0].Version != "1.2.3" || got[0].MinUpgradeFrom != "1.0.0" {
		t.Fatalf("parseFeed(single) = %+v", got)
	}
	if got[0].ReleasedAt.UTC() != time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("released_at = %v", got[0].ReleasedAt)
	}
	if got[0].ChangelogURL != "https://example.test/c" {
		t.Errorf("changelog_url = %q", got[0].ChangelogURL)
	}

	wrapped := `{"releases":[` + single + `]}`
	got, err = parseFeed([]byte(wrapped))
	if err != nil || len(got) != 1 {
		t.Fatalf("parseFeed(wrapped) = %+v, %v", got, err)
	}

	list := `[` + single + `]`
	got, err = parseFeed([]byte(list))
	if err != nil || len(got) != 1 {
		t.Fatalf("parseFeed(list) = %+v, %v", got, err)
	}

	for _, bad := range []string{"", "  ", "null", "[]", `"nope"`, "{}"} {
		if _, err := parseFeed([]byte(bad)); err == nil {
			t.Errorf("parseFeed(%q) should have failed", bad)
		}
	}
}

// ------------------------------------------------------------------ helpers

func manifestFor(version, minUpgradeFrom string) Manifest {
	body := []byte("release " + version)
	return Manifest{
		Version:        version,
		ReleasedAt:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		MinUpgradeFrom: minUpgradeFrom,
		TarballURL:     testTarBase + "vkai-panel-" + version + ".tar.gz",
		SHA256:         sha256Hex(body),
		ChangelogURL:   "https://docs.example.test/changelog/" + version,
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// TestMinUpgradeFromCannotBeBypassedByOmittingIt is the audit's question about
// this file: the field is optional, so what stops a release simply leaving it
// out? Nothing did. min_upgrade_from is a property of the migration inside a
// release, and the jump skips that release whatever the release on the far side
// says about itself, so the constraint is now read from everything the upgrade
// would step over.
func TestMinUpgradeFromCannotBeBypassedByOmittingIt(t *testing.T) {
	t.Parallel()

	t.Run("the newest release omits it, an intermediate one does not", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(
			manifestFor("3.0.0", ""),      // says nothing about where you may come from
			manifestFor("2.0.0", "1.9.0"), // but 3.0.0 cannot be reached without it
		)

		res, err := env.u.Check(context.Background())
		var jump *IncompatibleJumpError
		if !errors.As(err, &jump) {
			t.Fatalf("Check error = %v, want *IncompatibleJumpError", err)
		}
		if jump.To != "3.0.0" || jump.BlockedBy != "2.0.0" || jump.MinUpgradeFrom != "1.9.0" {
			t.Errorf("jump = %+v, want 3.0.0 blocked by 2.0.0 which requires 1.9.0", jump)
		}
		// Nothing published is installable from here, so the honest answer
		// is the version 2.0.0 demands.
		if jump.InstallFirst != "1.9.0" {
			t.Errorf("InstallFirst = %q, want 1.9.0", jump.InstallFirst)
		}
		if !containsAll(jump.Error(), "1.0.0", "3.0.0", "2.0.0", "1.9.0") {
			t.Errorf("error message does not name the release in the way: %q", jump.Error())
		}
		if res.UpdateAvailable || !res.Blocked {
			t.Errorf("CheckResult = %+v, want a blocked jump", res)
		}
	})

	t.Run("the intermediate release is reachable", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.9.0", nil)
		env.publishManifests(
			manifestFor("3.0.0", ""),
			manifestFor("2.0.0", "1.9.0"),
		)

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Target.Version != "3.0.0" {
			t.Errorf("target = %s, want 3.0.0 once the intermediate constraint is satisfied", res.Target.Version)
		}
	})

	t.Run("a release below the running version does not block anything", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(
			manifestFor("1.1.0", ""),
			manifestFor("0.9.0", "0.8.0"), // ancient, already passed
		)

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Target.Version != "1.1.0" {
			t.Errorf("target = %s, want 1.1.0", res.Target.Version)
		}
	})
}

// A feed that publishes release candidates must not thereby install one on
// every customer machine. An installation already running a pre-release has
// opted in, and an operator can opt in explicitly.
func TestPreReleasesAreNotInstalledUnlessAskedFor(t *testing.T) {
	t.Parallel()

	t.Run("ignored by default", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(manifestFor("2.0.0-rc.1", ""), manifestFor("1.1.0", ""))

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Target.Version != "1.1.0" {
			t.Errorf("target = %s, want the newest stable release 1.1.0", res.Target.Version)
		}
	})

	t.Run("nothing but pre-releases means nothing to do", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", nil)
		env.publishManifests(manifestFor("2.0.0-rc.1", ""))

		if _, err := env.u.Check(context.Background()); !errors.Is(err, ErrUpToDate) {
			t.Fatalf("Check error = %v, want ErrUpToDate", err)
		}
	})

	t.Run("offered when the operator asks", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", func(c *Config) { c.AllowPreRelease = true })
		env.publishManifests(manifestFor("2.0.0-rc.1", ""), manifestFor("1.1.0", ""))

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Target.Version != "2.0.0-rc.1" {
			t.Errorf("target = %s, want 2.0.0-rc.1", res.Target.Version)
		}
	})

	t.Run("offered when a pre-release is already running", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "2.0.0-rc.1", nil)
		env.publishManifests(manifestFor("2.0.0-rc.2", ""))

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Target.Version != "2.0.0-rc.2" {
			t.Errorf("target = %s, want 2.0.0-rc.2", res.Target.Version)
		}
	})
}

// One version, two manifests, two different tarballs: there is no honest way to
// pick, so the feed is refused rather than resolved by sort stability.
func TestFeedRefusesDuplicateVersions(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	a := manifestFor("1.1.0", "")
	b := manifestFor("1.1.0", "")
	b.TarballURL = testTarBase + "somewhere-else.tar.gz"
	b.SHA256 = sha256Hex([]byte("a different release"))
	env.publishManifests(a, b)

	_, err := env.u.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("Check error = %v, want the duplicate to be named", err)
	}
}

// Build metadata does not change a version, so two entries that differ only
// there are still the same release published twice.
func TestFeedRefusesVersionsThatDifferOnlyInBuildMetadata(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	a := manifestFor("1.1.0+one", "")
	b := manifestFor("1.1.0+two", "")
	env.publishManifests(a, b)

	if _, err := env.u.Check(context.Background()); err == nil {
		t.Fatal("Check accepted one release published twice under different build metadata")
	}
}

// Without a signature TLS is the only thing authenticating a release, so a
// plaintext URL is an unauthenticated root install.
func TestInsecureURLsAreRefused(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{Root: "/vkai-panel", FeedURL: "http://releases.example.test/feed.json"}, Deps{}); err == nil {
		t.Error("New accepted a plaintext http feed URL")
	}
	if _, err := New(Config{Root: "/vkai-panel", FeedURL: "file:///etc/passwd"}, Deps{}); err == nil {
		t.Error("New accepted a file:// feed URL")
	}
	if _, err := New(Config{
		Root: "/vkai-panel", FeedURL: "http://mirror.internal/feed.json", AllowInsecureURLs: true,
	}, Deps{}); err != nil {
		t.Errorf("New refused an http feed the operator explicitly allowed: %v", err)
	}

	env := newTestEnv(t, "1.0.0", nil)
	m := manifestFor("1.1.0", "")
	m.TarballURL = "http://releases.example.test/vkai-panel-1.1.0.tar.gz"
	env.publishManifests(m)
	if _, err := env.u.Check(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("Check error = %v, want the plaintext tarball URL to be refused", err)
	}
}
