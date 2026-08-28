package upgrade

// Semantic version ordering, the subset of SemVer 2.0.0 the panel releases use.
//
// Ordering matters twice in an upgrade: deciding whether the feed offers
// anything newer than what is running, and deciding whether the running version
// is at least the manifest's min_upgrade_from. Both have to agree that
// "1.10.0" is newer than "1.9.0" and that "2.0.0-rc.1" is older than "2.0.0",
// which is exactly what string comparison gets wrong.

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is kept for display but,
// as the specification requires, takes no part in ordering.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease []string
	Build      string

	text string // as written, minus any leading "v"
}

// maxVersionLength bounds a version string. A version becomes a directory name
// under /vkai-panel/releases, appears in a filename and is printed in every
// error message, and nothing legitimate is anywhere near this long.
const maxVersionLength = 100

// ParseVersion parses "1.2.3", "v1.2.3", "1.2.3-rc.1" or "1.2.3-rc.1+build.5".
// A leading "v" is accepted because that is how the tags are written, and
// dropped so that "v1.2.3" and "1.2.3" are the same version.
//
// Every character of every section is validated, including the build metadata,
// which the specification says takes no part in ordering. That used to make it
// look unimportant, and it was where a hostile feed got in: build metadata was
// copied into the version string unchecked, the version string became a path
// element, and "1.0.0+/../../../../root/.ssh" turned filepath.Join into a write
// outside the installation root. Ordering is not the only thing a version is
// used for.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Version{}, fmt.Errorf("empty version string")
	}
	if len(raw) > maxVersionLength {
		return Version{}, fmt.Errorf("version string is %d characters, longer than the %d allowed", len(raw), maxVersionLength)
	}
	raw = strings.TrimPrefix(raw, "v")

	v := Version{text: raw}

	// Build metadata first: it may itself contain hyphens, so it has to come
	// off before the pre-release is split away.
	if i := strings.IndexByte(raw, '+'); i >= 0 {
		v.Build = raw[i+1:]
		raw = raw[:i]
		if v.Build == "" {
			return Version{}, fmt.Errorf("version %q has an empty build metadata section", s)
		}
		for _, id := range strings.Split(v.Build, ".") {
			if id == "" {
				return Version{}, fmt.Errorf("version %q has an empty build metadata identifier", s)
			}
			if !isValidIdentifier(id) {
				return Version{}, fmt.Errorf("version %q has an invalid build metadata identifier %q", s, id)
			}
		}
	}

	core := raw
	if i := strings.IndexByte(raw, '-'); i >= 0 {
		core = raw[:i]
		pre := raw[i+1:]
		if pre == "" {
			return Version{}, fmt.Errorf("version %q has an empty pre-release section", s)
		}
		v.PreRelease = strings.Split(pre, ".")
		for _, id := range v.PreRelease {
			if id == "" {
				return Version{}, fmt.Errorf("version %q has an empty pre-release identifier", s)
			}
			if !isValidIdentifier(id) {
				return Version{}, fmt.Errorf("version %q has an invalid pre-release identifier %q", s, id)
			}
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := parseNumericComponent(p)
		if err != nil {
			return Version{}, fmt.Errorf("version %q: %w", s, err)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

// MustParseVersion is ParseVersion for versions that are compiled in rather
// than read from a feed. It panics, which is the right outcome for a constant
// the build itself got wrong.
func MustParseVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func parseNumericComponent(p string) (int, error) {
	if p == "" {
		return 0, fmt.Errorf("empty numeric component")
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-numeric component %q", p)
		}
	}
	// The specification forbids leading zeros, and so does this: "01.0.0"
	// and "1.0.0" would otherwise be two spellings of one version that
	// compare equal but produce two different release directories.
	if len(p) > 1 && p[0] == '0' {
		return 0, fmt.Errorf("numeric component %q has a leading zero", p)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("numeric component %q out of range", p)
	}
	return n, nil
}

// isValidIdentifier reports whether id is a legal pre-release or build
// metadata identifier: ASCII alphanumerics and hyphens, nothing else. Slashes,
// dots and NULs are what a traversal is made of, so they are not "unusual", they
// are the reason this function exists.
func isValidIdentifier(id string) bool {
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// String renders the version without the leading "v", including pre-release and
// build metadata.
func (v Version) String() string {
	if v.text != "" {
		return v.text
	}
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.PreRelease) > 0 {
		s += "-" + strings.Join(v.PreRelease, ".")
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Canonical renders the version in the one spelling that orders identically:
// no leading "v", no build metadata. Two versions are the same release exactly
// when their canonical forms are equal, which is what makes it usable as a map
// key when deduplicating a feed.
func (v Version) Canonical() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.PreRelease) > 0 {
		s += "-" + strings.Join(v.PreRelease, ".")
	}
	return s
}

// IsPreRelease reports whether this is a pre-release, which orders below the
// release of the same core version.
func (v Version) IsPreRelease() bool { return len(v.PreRelease) > 0 }

// Compare returns -1, 0 or 1 as v sorts before, equal to, or after other.
// Build metadata is ignored, so 1.0.0+a and 1.0.0+b compare equal.
func (v Version) Compare(other Version) int {
	if c := compareInt(v.Major, other.Major); c != 0 {
		return c
	}
	if c := compareInt(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := compareInt(v.Patch, other.Patch); c != 0 {
		return c
	}
	return comparePreRelease(v.PreRelease, other.PreRelease)
}

// LessThan reports whether v sorts strictly before other.
func (v Version) LessThan(other Version) bool { return v.Compare(other) < 0 }

// Equal reports whether v and other are the same version, build metadata aside.
func (v Version) Equal(other Version) bool { return v.Compare(other) == 0 }

// comparePreRelease implements SemVer clause 11.4: a version with a pre-release
// sorts below the same core version without one; identifiers are compared left
// to right, numerically when both are numeric, otherwise as ASCII; a numeric
// identifier always sorts below an alphanumeric one; and when every shared
// identifier is equal the longer list sorts higher.
func comparePreRelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := comparePreReleaseIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

func comparePreReleaseIdentifier(a, b string) int {
	an, aNumeric := numericIdentifier(a)
	bn, bNumeric := numericIdentifier(b)
	switch {
	case aNumeric && bNumeric:
		return compareInt64(an, bn)
	case aNumeric:
		return -1
	case bNumeric:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func numericIdentifier(id string) (int64, bool) {
	if id == "" {
		return 0, false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		// A run of digits too long for int64. Treat it as alphanumeric
		// rather than silently comparing it wrong.
		return 0, false
	}
	return n, true
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// CompareVersionStrings parses both sides and orders them. It is the entry
// point for callers holding version strings, such as the CLI comparing what is
// installed against what the feed offers.
func CompareVersionStrings(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}
	return va.Compare(vb), nil
}
