package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// makeReleases creates release directories under the environment's root.
func makeReleases(t *testing.T, env *testEnv, versions ...string) {
	t.Helper()
	for _, v := range versions {
		dir := filepath.Join(env.root, "releases", v)
		mustMkdirAll(t, dir)
		mustWriteFile(t, filepath.Join(dir, "VERSION"), v+"\n")
	}
}

func remainingReleases(t *testing.T, env *testEnv) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(env.root, "releases"))
	if err != nil {
		t.Fatalf("read releases: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// TestPruneKeepsTheNewestN is the ordinary case: the current release is the
// newest, so KeepReleases is exactly how many survive.
func TestPruneKeepsTheNewestN(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.KeepReleases = 3 })
	makeReleases(t, env, "1.1.0", "1.2.0", "1.9.0", "1.10.0", "2.0.0-rc.1")

	current := filepath.Join(env.root, "releases", "2.0.0-rc.1")
	previous := filepath.Join(env.root, "releases", "1.10.0")

	removed, err := env.u.prune(current, previous)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	got := remainingReleases(t, env)
	want := []string{"1.10.0", "1.9.0", "2.0.0-rc.1"}
	if !equalStrings(got, want) {
		t.Errorf("remaining = %v, want %v (removed %v)", got, want, removed)
	}
	if len(removed) != 3 {
		t.Errorf("removed %d releases, want 3", len(removed))
	}
}

// TestPruneNeverRemovesCurrentOrPrevious is requirement 8's real constraint.
// The current release is what the machine is running; the previous one is the
// only thing a rollback can restore. Retention must not be able to delete
// either, however aggressive it is set.
func TestPruneNeverRemovesCurrentOrPrevious(t *testing.T) {
	t.Parallel()

	// KeepReleases is 1, and current/previous are deliberately NOT the two
	// newest - the situation after an operator has rolled back and newer
	// releases are still lying around.
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.KeepReleases = 1 })
	makeReleases(t, env, "1.1.0", "1.2.0", "1.3.0", "1.4.0", "1.5.0")

	current := filepath.Join(env.root, "releases", "1.2.0")
	previous := filepath.Join(env.root, "releases", "1.1.0")
	// Keep the installation coherent: this is what a machine looks like
	// after an operator rolled back from 1.5.0 onto 1.2.0.
	if err := os.Remove(filepath.Join(env.root, "current")); err != nil {
		t.Fatalf("remove current: %v", err)
	}
	mustSymlink(t, filepath.Join("releases", "1.2.0"), filepath.Join(env.root, "current"))

	removed, err := env.u.prune(current, previous)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if !exists(current) {
		t.Fatal("prune removed the current release")
	}
	if !exists(previous) {
		t.Fatal("prune removed the previous release - a rollback would now be impossible")
	}
	// The retention budget of 1 goes to the newest release, 1.5.0; the
	// protected pair survives on top of it; everything else goes.
	got := remainingReleases(t, env)
	want := []string{"1.1.0", "1.2.0", "1.5.0"}
	if !equalStrings(got, want) {
		t.Errorf("remaining = %v, want %v (removed %v)", got, want, removed)
	}
	for _, r := range removed {
		if r == current || r == previous {
			t.Errorf("prune reported removing a protected release: %s", r)
		}
	}
}

// Anything that is not a semantic version is left alone entirely: staging
// directories, an operator's manual copy, a stray file.
func TestPruneIgnoresNonReleaseEntries(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", func(c *Config) { c.KeepReleases = 1 })
	makeReleases(t, env, "1.1.0", "1.2.0")
	mustMkdirAll(t, filepath.Join(env.root, "releases", "1.2.0.bak"))
	mustMkdirAll(t, filepath.Join(env.root, "releases", ".staging-1.3.0-999"))
	mustWriteFile(t, filepath.Join(env.root, "releases", "README"), "notes\n")

	if _, err := env.u.prune(filepath.Join(env.root, "releases", "1.2.0")); err != nil {
		t.Fatalf("prune: %v", err)
	}

	for _, name := range []string{"1.2.0.bak", ".staging-1.3.0-999", "README"} {
		if !exists(filepath.Join(env.root, "releases", name)) {
			t.Errorf("prune removed %q, which is not a release directory", name)
		}
	}
}

func TestListReleasesOrdersNewestFirst(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	makeReleases(t, env, "1.9.0", "1.10.0", "2.0.0", "2.0.0-rc.1")

	releases, err := env.u.listReleases()
	if err != nil {
		t.Fatalf("listReleases: %v", err)
	}
	var got []string
	for _, r := range releases {
		got = append(got, r.Name)
	}
	want := []string{"2.0.0", "2.0.0-rc.1", "1.10.0", "1.9.0", "1.0.0"}
	if !equalStrings(got, want) {
		t.Errorf("listReleases = %v, want %v", got, want)
	}
}

// After a real upgrade, prune must have protected the release that just went
// live and the one it replaced.
func TestUpgradePrunesButProtectsCurrentAndPrevious(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.5.0", func(c *Config) { c.KeepReleases = 2 })
	makeReleases(t, env, "1.1.0", "1.2.0", "1.3.0", "1.4.0")
	env.publish("1.6.0", "")

	res, err := env.u.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("result = %+v", res)
	}

	got := remainingReleases(t, env)
	want := []string{"1.5.0", "1.6.0"}
	if !equalStrings(got, want) {
		t.Errorf("remaining = %v, want %v (pruned %v)", got, want, res.Pruned)
	}
	if !exists(filepath.Join(env.root, "releases", "1.5.0")) {
		t.Error("the previous release was pruned; there is nothing to roll back to")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
