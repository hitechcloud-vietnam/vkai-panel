// Package version carries the identity of a built VKAI Panel binary: which
// release it is, which commit it was built from, when that commit was made and
// which Go toolchain produced it.
//
// # One fact, one place
//
// The semantic version of the whole product lives in the file VERSION at the
// repository root. Nothing in this package invents a version of its own:
//
//   - A release build gets its values injected at link time by the Makefile
//     (see LDFLAGS there), which reads the same VERSION file.
//   - A plain "go build ./..." has no linker flags, so the version falls back to
//     defaultVersion in version_default.go - a copy of the VERSION file kept in
//     sync by "make sync-version" and verified by the Version Check workflow.
//     The fallback is what makes an un-flagged build report the truth instead of
//     "dev" or an empty string.
//   - Commit and build date fall back to the VCS stamps the Go toolchain embeds
//     on its own (runtime/debug build info), so even an un-flagged build usually
//     knows which commit it came from.
//
// Values are resolved once, at package initialisation, so every reader sees the
// same strings and none of them is ever empty.
//
// # Who reads this
//
// The API serves it at /api/v1/version, the CLI prints it in its banner, and the
// upgrade client compares the running Version against a release manifest. Those
// callers must be able to trust that Version is the version actually installed.
//
// Semantic version *ordering* is deliberately not implemented here: the upgrade
// client owns that (internal/upgrade.ParseVersion), and two semver
// implementations in one binary would be one too many.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// unknown is what a field says when neither the linker nor the Go build info
// could tell us the truth. It is never left empty: an empty string in a banner
// or an API response reads as a bug in the reader, not as missing information.
const unknown = "unknown"

// Injected at link time. The Makefile passes, for every Go build target:
//
//	-X github.com/hitechcloud-vietnam/vkai-panel/internal/version.Version=...
//	-X github.com/hitechcloud-vietnam/vkai-panel/internal/version.Commit=...
//	-X github.com/hitechcloud-vietnam/vkai-panel/internal/version.BuildDate=...
//
// Each is declared as a plain string with a literal initialiser, which is the
// only shape the Go linker can overwrite. Do not turn them into constants and do
// not compute their initial values.
var (
	// Version is the semantic version of this build, without a leading "v",
	// for example "0.3.0". Never empty.
	Version = ""

	// Commit is the git commit this build came from: a short hex hash,
	// optionally suffixed with "-dirty" when the tree had uncommitted
	// changes. "unknown" when the build carried no VCS information.
	Commit = ""

	// BuildDate is the commit date in RFC 3339, for example
	// "2026-08-28T09:41:07+07:00". It is the date of the *commit*, not of the
	// moment the compiler ran, so rebuilding the same commit produces the
	// same binary. "unknown" when the build carried no VCS information.
	BuildDate = ""
)

// GoVersion is the Go toolchain that produced this binary, for example
// "go1.22.2". It is derived, not injected: the runtime already knows it and a
// linker flag could only be used to lie about it.
var GoVersion = runtime.Version()

// Platform is the target this binary was compiled for, for example
// "linux/amd64". A support ticket that quotes the wrong architecture costs an
// hour, so the banner says it out loud.
var Platform = runtime.GOOS + "/" + runtime.GOARCH

func init() { resolve(buildInfo) }

// buildInfo is the real source of VCS stamps; resolve takes it as an argument so
// the tests can drive the fallback logic without a rebuild.
func buildInfo() (revision, commitTime string, modified bool, ok bool) {
	info, available := debug.ReadBuildInfo()
	if !available {
		return "", "", false, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			commitTime = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, commitTime, modified, revision != "" || commitTime != ""
}

// resolve fills in whatever the linker did not, in this order of trust:
// linker flags, then the VCS stamps the toolchain embedded, then the version
// compiled in from the VERSION file, then "unknown".
func resolve(info func() (string, string, bool, bool)) {
	Version = strings.TrimSpace(Version)
	Commit = strings.TrimSpace(Commit)
	BuildDate = strings.TrimSpace(BuildDate)

	// A release always sets Version through the linker; a plain "go build"
	// never does. defaultVersion is the VERSION file compiled in, so both
	// report the same release.
	if Version == "" {
		Version = strings.TrimSpace(defaultVersion)
	}
	if Version == "" {
		Version = unknown
	}
	Version = strings.TrimPrefix(Version, "v")

	revision, commitTime, modified, ok := info()
	if ok {
		if Commit == "" && revision != "" {
			Commit = shortCommit(revision)
			if modified {
				Commit += "-dirty"
			}
		}
		if BuildDate == "" && commitTime != "" {
			BuildDate = commitTime
		}
	}

	if Commit == "" {
		Commit = unknown
	}
	if BuildDate == "" {
		BuildDate = unknown
	}
	if strings.TrimSpace(GoVersion) == "" {
		GoVersion = unknown
	}
}

// shortCommit trims a full 40-character hash to the 12 characters the Makefile
// also passes, so the two build paths print the same thing.
func shortCommit(revision string) string {
	const shortLen = 12
	if len(revision) > shortLen {
		return revision[:shortLen]
	}
	return revision
}

// Info is the machine-readable form of a build's identity. It is what
// GET /api/v1/version answers with, so the JSON field names are part of that
// endpoint's contract: add fields, do not rename them.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the identity of this build.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
		Platform:  Platform,
	}
}

// Short returns just the semantic version, for example "0.3.0". Use it where a
// version is compared or written to a file; use String for something a human
// reads.
func Short() string { return Version }

// String returns the one-line banner form, for example:
//
//	0.3.0 (commit 1f4c9ab0d2e3, built 2026-08-28T09:41:07+07:00, go1.22.2, linux/amd64)
//
// It deliberately does not name the product: callers that need a banner prefix
// it with their own name ("VKAI Panel API", "VKAI Panel Agent"), so the product
// name is not duplicated into yet another file.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s, %s)",
		Version, Commit, BuildDate, GoVersion, Platform)
}
