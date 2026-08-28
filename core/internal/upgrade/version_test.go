package upgrade

import (
	"sort"
	"testing"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	ok := []struct {
		in    string
		major int
		minor int
		patch int
		pre   int
		build string
	}{
		{in: "1.2.3", major: 1, minor: 2, patch: 3},
		{in: "v1.2.3", major: 1, minor: 2, patch: 3},
		{in: "  1.2.3  ", major: 1, minor: 2, patch: 3},
		{in: "0.0.1", patch: 1},
		{in: "10.20.30", major: 10, minor: 20, patch: 30},
		{in: "1.0.0-rc.1", major: 1, pre: 2},
		{in: "1.0.0-alpha-2", major: 1, pre: 1},
		{in: "1.0.0-rc.1+build.7", major: 1, pre: 2, build: "build.7"},
		{in: "1.0.0+20260301", major: 1, build: "20260301"},
	}
	for _, tc := range ok {
		v, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q): unexpected error %v", tc.in, err)
			continue
		}
		if v.Major != tc.major || v.Minor != tc.minor || v.Patch != tc.patch {
			t.Errorf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tc.in, v.Major, v.Minor, v.Patch, tc.major, tc.minor, tc.patch)
		}
		if len(v.PreRelease) != tc.pre {
			t.Errorf("ParseVersion(%q) has %d pre-release identifiers, want %d", tc.in, len(v.PreRelease), tc.pre)
		}
		if v.Build != tc.build {
			t.Errorf("ParseVersion(%q).Build = %q, want %q", tc.in, v.Build, tc.build)
		}
	}

	bad := []string{
		"", "   ", "1", "1.2", "1.2.3.4", "1.2.x", "one.two.three",
		"1.2.-3", "1.2.3-", "1.2.3+", "1.2.3-rc..1", "1.2.3-rc.1!",
		"-1.2.3", "1..3",
	}
	for _, in := range bad {
		if v, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) = %v, want an error", in, v)
		}
	}
}

// TestVersionOrdering walks the ordering example from the semantic versioning
// specification, which is the case list that catches every naive
// implementation: 10 after 9, pre-releases before their release, numeric
// pre-release identifiers below alphanumeric ones, and longer pre-release lists
// after their prefixes.
func TestVersionOrdering(t *testing.T) {
	t.Parallel()

	ascending := []string{
		"0.9.9",
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
		"1.0.1",
		"1.0.10",
		"1.1.0",
		"1.9.0",
		"1.10.0",
		"2.0.0-rc.1",
		"2.0.0",
	}

	for i := 0; i < len(ascending); i++ {
		for j := 0; j < len(ascending); j++ {
			c, err := CompareVersionStrings(ascending[i], ascending[j])
			if err != nil {
				t.Fatalf("CompareVersionStrings(%q, %q): %v", ascending[i], ascending[j], err)
			}
			var want int
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if c != want {
				t.Errorf("compare(%q, %q) = %d, want %d", ascending[i], ascending[j], c, want)
			}
		}
	}

	// Sorting a shuffled copy has to reproduce the list exactly.
	shuffled := []string{
		"1.10.0", "1.0.0-beta.11", "2.0.0", "1.0.0", "0.9.9", "1.0.0-alpha",
		"1.0.10", "2.0.0-rc.1", "1.0.0-rc.1", "1.9.0", "1.0.0-alpha.beta",
		"1.1.0", "1.0.0-beta.2", "1.0.1", "1.0.0-alpha.1", "1.0.0-beta",
	}
	sort.Slice(shuffled, func(a, b int) bool {
		c, err := CompareVersionStrings(shuffled[a], shuffled[b])
		if err != nil {
			t.Fatalf("compare: %v", err)
		}
		return c < 0
	})
	for i := range ascending {
		if shuffled[i] != ascending[i] {
			t.Fatalf("sorted[%d] = %q, want %q (got %v)", i, shuffled[i], ascending[i], shuffled)
		}
	}
}

func TestVersionBuildMetadataIsIgnoredInOrdering(t *testing.T) {
	t.Parallel()
	c, err := CompareVersionStrings("1.2.3+alpha", "1.2.3+zulu")
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if c != 0 {
		t.Errorf("build metadata changed the ordering: got %d, want 0", c)
	}
}

func TestVersionStringComparisonWouldBeWrong(t *testing.T) {
	t.Parallel()
	// The regression this package exists to avoid: "1.9.0" > "1.10.0" as
	// strings, which would make the panel refuse every release after 1.9.
	if "1.10.0" > "1.9.0" {
		t.Fatal("test premise broken: string comparison unexpectedly agrees with semver")
	}
	c, err := CompareVersionStrings("1.10.0", "1.9.0")
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if c != 1 {
		t.Errorf("compare(1.10.0, 1.9.0) = %d, want 1", c)
	}
}

func TestVersionIsPreRelease(t *testing.T) {
	t.Parallel()
	if !MustParseVersion("2.0.0-rc.1").IsPreRelease() {
		t.Error("2.0.0-rc.1 should be a pre-release")
	}
	if MustParseVersion("2.0.0").IsPreRelease() {
		t.Error("2.0.0 should not be a pre-release")
	}
	if !MustParseVersion("1.0.0").Equal(MustParseVersion("v1.0.0+meta")) {
		t.Error("1.0.0 and v1.0.0+meta should compare equal")
	}
}
