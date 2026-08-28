package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noBuildInfo stands in for a build that carried no VCS stamps at all.
func noBuildInfo() (string, string, bool, bool) { return "", "", false, false }

// saveState lets a test drive resolve without leaking its values into the
// other tests, which read the package-level variables directly.
func saveState(t *testing.T) {
	t.Helper()
	version, commit, date, goVersion := Version, Commit, BuildDate, GoVersion
	t.Cleanup(func() {
		Version, Commit, BuildDate, GoVersion = version, commit, date, goVersion
	})
}

func TestResolveFallsBackToTheVersionFileCopy(t *testing.T) {
	saveState(t)
	Version, Commit, BuildDate = "", "", ""

	resolve(noBuildInfo)

	if Version != defaultVersion {
		t.Errorf("Version = %q, want the compiled-in default %q", Version, defaultVersion)
	}
	if Commit != unknown || BuildDate != unknown {
		t.Errorf("Commit/BuildDate = %q/%q, want both %q", Commit, BuildDate, unknown)
	}
}

func TestResolveKeepsLinkerValues(t *testing.T) {
	saveState(t)
	Version, Commit, BuildDate = " v9.9.9 ", " 1f4c9ab0d2e3 ", " 2026-08-28T09:41:07+07:00 "

	resolve(func() (string, string, bool, bool) {
		return "0000000000000000000000000000000000000000", "1999-01-01T00:00:00Z", true, true
	})

	if Version != "9.9.9" {
		t.Errorf("Version = %q, want %q: surrounding space and the leading v are trimmed", Version, "9.9.9")
	}
	if Commit != "1f4c9ab0d2e3" {
		t.Errorf("Commit = %q, want the linker value untouched by build info", Commit)
	}
	if BuildDate != "2026-08-28T09:41:07+07:00" {
		t.Errorf("BuildDate = %q, want the linker value untouched by build info", BuildDate)
	}
}

func TestResolveUsesBuildInfoWhenTheLinkerSaidNothing(t *testing.T) {
	saveState(t)
	Version, Commit, BuildDate = "", "", ""

	resolve(func() (string, string, bool, bool) {
		return "1f4c9ab0d2e3aabbccddeeff0011223344556677", "2026-08-28T09:41:07Z", true, true
	})

	if want := "1f4c9ab0d2e3-dirty"; Commit != want {
		t.Errorf("Commit = %q, want %q", Commit, want)
	}
	if want := "2026-08-28T09:41:07Z"; BuildDate != want {
		t.Errorf("BuildDate = %q, want %q", BuildDate, want)
	}
}

func TestNothingIsEverEmpty(t *testing.T) {
	saveState(t)
	Version, Commit, BuildDate, GoVersion = "", "", "", ""

	resolve(noBuildInfo)

	for name, got := range map[string]string{
		"Version":   Version,
		"Commit":    Commit,
		"BuildDate": BuildDate,
		"GoVersion": GoVersion,
		"Platform":  Platform,
	} {
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s is empty; every field must carry a value a reader can quote", name)
		}
	}
}

func TestStringAndInfoAgree(t *testing.T) {
	saveState(t)
	Version, Commit, BuildDate = "0.3.0", "1f4c9ab0d2e3", "2026-08-28T09:41:07Z"
	resolve(noBuildInfo)

	banner := String()
	for _, want := range []string{Version, Commit, BuildDate, GoVersion, Platform} {
		if !strings.Contains(banner, want) {
			t.Errorf("String() = %q, missing %q", banner, want)
		}
	}
	if Short() != Version {
		t.Errorf("Short() = %q, want %q", Short(), Version)
	}

	encoded, err := json.Marshal(Get())
	if err != nil {
		t.Fatalf("marshal Info: %v", err)
	}
	// The field names below are the /api/v1/version contract.
	var decoded map[string]string
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal Info: %v", err)
	}
	for _, key := range []string{"version", "commit", "build_date", "go_version", "platform"} {
		if decoded[key] == "" {
			t.Errorf("Info JSON is missing %q: %s", key, encoded)
		}
	}
}

// TestDefaultVersionMatchesTheVersionFile is the drift guard: version_default.go
// is a mechanical copy of /VERSION, and this fails the moment the two disagree.
// The same check runs in the "Version Check" workflow for pull requests, which
// also covers panel/package.json.
func TestDefaultVersionMatchesTheVersionFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "VERSION")
	raw, err := os.ReadFile(path)
	if err != nil {
		// Building from an extracted release tarball, where the repository
		// root is not present. Nothing to compare against.
		t.Skipf("no VERSION file next to the module (%v)", err)
	}

	want := strings.TrimSpace(string(raw))
	if want != defaultVersion {
		t.Errorf("VERSION says %q but version_default.go says %q; run \"make sync-version\"", want, defaultVersion)
	}
}

// TestLinkerInjection proves that the -X flags the Makefile passes actually land
// in this package. It is skipped by default because it needs those flags; run it
// exactly as the Makefile builds:
//
//	VKAI_TEST_EXPECT_VERSION=9.9.9 go test ./internal/version -count=1 \
//	  -run TestLinkerInjection \
//	  -ldflags "-X github.com/hitechcloud-vietnam/vkai-panel/internal/version.Version=9.9.9"
func TestLinkerInjection(t *testing.T) {
	want := os.Getenv("VKAI_TEST_EXPECT_VERSION")
	if want == "" {
		t.Skip("set VKAI_TEST_EXPECT_VERSION and pass matching -ldflags to run this")
	}
	if Version != want {
		t.Errorf("Version = %q, want %q: the -X linker flag did not reach this package", Version, want)
	}
}
