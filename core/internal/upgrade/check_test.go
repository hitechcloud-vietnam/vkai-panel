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
