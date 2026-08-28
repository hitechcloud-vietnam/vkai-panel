package phpfpm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// fakeRunner records every argv and answers from a script. It exists so the
// whole write -> validate -> reload -> roll back path can be driven without
// php-fpm, systemd or root, which is the only way that path gets tested at all.
type fakeRunner struct {
	calls []string
	// fail maps a substring of the argv to the error it should produce.
	fail map[string]error
	// active is what `systemctl is-active` answers. Empty means "active".
	active string
	// beforeCall runs before a command is answered, so a test can change the
	// filesystem in the middle of a transaction the way a real host would.
	beforeCall func(call string)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{fail: map[string]error{}}
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)

	if f.beforeCall != nil {
		f.beforeCall(call)
	}

	for marker, err := range f.fail {
		if strings.Contains(call, marker) {
			return []byte("simulated failure output"), err
		}
	}
	if strings.Contains(call, "is-active") {
		state := f.active
		if state == "" {
			state = "active"
		}
		return []byte(state + "\n"), nil
	}
	return []byte("ok"), nil
}

func (f *fakeRunner) called(marker string) bool {
	for _, call := range f.calls {
		if strings.Contains(call, marker) {
			return true
		}
	}
	return false
}

func (f *fakeRunner) countOf(marker string) int {
	n := 0
	for _, call := range f.calls {
		if strings.Contains(call, marker) {
			n++
		}
	}
	return n
}

// debianHost is a Ubuntu 24.04 for the tests: the family where multi-version
// PHP really works.
func debianHost() Distro {
	return classify("ubuntu", "24.04", "Ubuntu 24.04 LTS", "debian")
}

// newTestManager builds a manager rooted in a temporary directory, with every
// pool directory and php-fpm binary already in place, so ApplyPool has
// somewhere real to write.
func newTestManager(t *testing.T, runner Runner, versions ...string) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	distro := debianHost()

	manager, err := New(Options{
		Distro:  &distro,
		Runner:  runner,
		Logger:  zap.NewNop(),
		RootDir: root,
	})
	if err != nil {
		t.Fatalf("could not build the manager: %v", err)
	}

	for _, version := range versions {
		layout, err := manager.Layout(version)
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{layout.PoolDir, layout.ConfDir, filepath.Dir(layout.Binary)} {
			if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, layout.Binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return manager, root
}

func testSpec(name, version string) *PoolSpec {
	spec := validSpec()
	spec.Name = name
	spec.Version = version
	return spec
}

// ---------------------------------------------------------------------------
// The happy path
// ---------------------------------------------------------------------------

func TestApplyPoolWritesValidatesAndReloads(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")

	result, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), "")
	if err != nil {
		t.Fatalf("a valid pool did not apply: %v", err)
	}

	want := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")
	if result.PoolPath != want {
		t.Fatalf("the pool file went to %s, want %s", result.PoolPath, want)
	}
	content, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("the pool file was not written: %v", err)
	}
	if !strings.Contains(string(content), "php_admin_value[memory_limit] = 512M") {
		t.Fatal("the pool file does not carry the settings that were applied")
	}
	if !result.Created {
		t.Fatal("a pool that did not exist before was not reported as created")
	}

	// The order is the point: the configuration is tested BEFORE the reload,
	// and the service is proved active AFTER it.
	if !runner.called("--test") {
		t.Fatal("php-fpm --test was never run: a bad directive would only be discovered when " +
			"the reload took the site down")
	}
	if !runner.called("systemctl reload php8.3-fpm") {
		t.Fatal("php8.3-fpm was never reloaded, so the new pool file is not in force")
	}
	if !runner.called("systemctl is-active php8.3-fpm") {
		t.Fatal("the service was never proved active after the reload; a reload that returns 0 " +
			"and a master that then died is exactly the failure this guards against")
	}

	// A pool file must not be world readable: it names the site's unix user and
	// its log paths.
	info, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Fatalf("the pool file is %v; it must not be world readable", info.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// The rollback: the requirement that a settings change never leaves a site down
// ---------------------------------------------------------------------------

// TestFailedReloadRollsBackToThePreviousPoolFile is the central test of this
// package. A pool is applied and serving; a second apply changes it; the reload
// fails; the file on disk must be byte-for-byte what it was, and FPM must have
// been reloaded again so the site is serving that.
func TestFailedReloadRollsBackToThePreviousPoolFile(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")
	poolPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	// The pool the site is serving.
	original := testSpec("example-com", "8.3")
	original.MemoryLimit = "256M"
	if _, err := manager.ApplyPool(context.Background(), original, ""); err != nil {
		t.Fatalf("the first apply failed: %v", err)
	}
	before, err := os.ReadFile(poolPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "memory_limit] = 256M") {
		t.Fatal("the first apply did not write the settings it was given")
	}

	// Now the reload fails.
	runner.calls = nil
	runner.fail["systemctl reload"] = fmt.Errorf("Job for php8.3-fpm.service failed")

	changed := testSpec("example-com", "8.3")
	changed.MemoryLimit = "1024M"
	_, err = manager.ApplyPool(context.Background(), changed, "")
	if err == nil {
		t.Fatal("a failed reload was reported as success; the site would be down and the panel " +
			"would say the change was applied")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("the error does not tell the operator a rollback happened: %v", err)
	}
	if !strings.Contains(err.Error(), "previous configuration") {
		t.Fatalf("the error does not say the site is still serving: %v", err)
	}

	after, err := os.ReadFile(poolPath)
	if err != nil {
		t.Fatalf("the pool file is gone after a rollback: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("the pool file was not restored.\n--- before ---\n%s\n--- after ---\n%s",
			before, after)
	}
	if strings.Contains(string(after), "1024M") {
		t.Fatal("the failed settings are still in the pool file")
	}

	// The rollback has to reload too, or the file is right and the running
	// master is still holding the broken configuration.
	if runner.countOf("systemctl reload php8.3-fpm") < 2 {
		t.Fatalf("the rollback did not reload php-fpm again (%d reloads); restoring the file "+
			"without reloading leaves the broken configuration in the running master",
			runner.countOf("systemctl reload php8.3-fpm"))
	}
}

// TestFailedValidationRollsBackAndNeverReloads proves the cheaper failure is
// caught earlier: php-fpm --test fails, so the reload is never attempted at all.
func TestFailedValidationRollsBackAndNeverReloads(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")
	poolPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	original := testSpec("example-com", "8.3")
	if _, err := manager.ApplyPool(context.Background(), original, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(poolPath)

	runner.calls = nil
	runner.fail["--test"] = fmt.Errorf("[ERROR] unknown entry 'memory_limitt'")

	changed := testSpec("example-com", "8.3")
	changed.MemoryLimit = "999M"
	if _, err := manager.ApplyPool(context.Background(), changed, ""); err == nil {
		t.Fatal("a configuration php-fpm rejected was reported as applied")
	}

	after, _ := os.ReadFile(poolPath)
	if string(after) != string(before) {
		t.Fatal("the pool file was not restored after php-fpm rejected the configuration")
	}
}

// TestApplyToANewPoolThatFailsRemovesTheFileEntirely covers the case the naive
// rollback gets wrong: the pool did not exist before, so restoring it means
// DELETING it, not writing back an empty file that FPM then refuses to parse.
func TestApplyToANewPoolThatFailsRemovesTheFileEntirely(t *testing.T) {
	runner := newFakeRunner()
	runner.fail["systemctl reload"] = fmt.Errorf("reload failed")
	manager, root := newTestManager(t, runner, "8.3")
	poolPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/brand-new.conf")

	if _, err := manager.ApplyPool(context.Background(), testSpec("brand-new", "8.3"), ""); err == nil {
		t.Fatal("a failed reload on a brand-new pool was reported as success")
	}
	if _, err := os.Stat(poolPath); !os.IsNotExist(err) {
		content, _ := os.ReadFile(poolPath)
		t.Fatalf("the pool file for a pool that never existed was left behind:\n%s", content)
	}
}

// TestAReloadThatSucceedsButLeavesTheServiceDeadIsARollback is the failure the
// is-active check exists for: systemctl returns 0, the master died anyway.
func TestAReloadThatSucceedsButLeavesTheServiceDeadIsARollback(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")
	poolPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	original := testSpec("example-com", "8.3")
	original.MemoryLimit = "256M"
	if _, err := manager.ApplyPool(context.Background(), original, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(poolPath)

	runner.active = "failed"
	changed := testSpec("example-com", "8.3")
	changed.MemoryLimit = "1024M"
	_, err := manager.ApplyPool(context.Background(), changed, "")
	if err == nil {
		t.Fatal("a reload that left php-fpm dead was reported as success")
	}
	if !strings.Contains(err.Error(), "took the service down") {
		t.Fatalf("the error does not say the service went down: %v", err)
	}
	after, _ := os.ReadFile(poolPath)
	if string(after) != string(before) {
		t.Fatal("the pool file was not restored after the service failed to come back")
	}
}

// TestRollbackFailureIsReportedLoudly covers the worst case: the change failed
// AND the previous pool file could not be put back. The site may now be down,
// and an operator has to be told that in the API response, not only in a log
// file - so the message is asserted here.
//
// The failure is produced the way it would really happen: something removes the
// pool directory out from under the transaction while it is in flight.
func TestRollbackFailureIsReportedLoudly(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")
	poolDir := filepath.Join(root, "/etc/php/8.3/fpm/pool.d")

	// A pool that is applied and serving.
	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), ""); err != nil {
		t.Fatal(err)
	}

	// The next apply fails validation, and by the time the rollback tries to
	// write the previous file back, the directory is gone.
	runner.fail["--test"] = fmt.Errorf("[ERROR] rejected")
	runner.beforeCall = func(call string) {
		if strings.Contains(call, "--test") {
			_ = os.RemoveAll(poolDir)
		}
	}

	changed := testSpec("example-com", "8.3")
	changed.MemoryLimit = "1024M"
	_, err := manager.ApplyPool(context.Background(), changed, "")
	if err == nil {
		t.Fatal("expected a failure")
	}
	if !strings.Contains(err.Error(), "may be") || !strings.Contains(err.Error(), "no PHP at all") {
		t.Fatalf("when the rollback itself fails the operator must be told the site may be "+
			"down, in the error they receive; got: %v", err)
	}
	if !strings.Contains(err.Error(), poolDir) {
		t.Fatalf("the error does not name the file an operator has to go and look at: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Switching a site between PHP versions
// ---------------------------------------------------------------------------

// TestVersionSwitchMovesThePoolFileAndReloadsBothServices is the feature
// customers migrate for. The pool file has to LEAVE the old version's directory
// - a pool file left in both means two FPM masters listening on one socket.
func TestVersionSwitchMovesThePoolFileAndReloadsBothServices(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.1", "8.3")

	oldPath := filepath.Join(root, "/etc/php/8.1/fpm/pool.d/example-com.conf")
	newPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.1"), ""); err != nil {
		t.Fatalf("the site could not be put on PHP 8.1: %v", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("the 8.1 pool file was not written: %v", err)
	}

	runner.calls = nil
	result, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), "8.1")
	if err != nil {
		t.Fatalf("the version switch failed: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("the pool file is still in the PHP 8.1 directory: two FPM masters would now " +
			"both try to listen on this site's socket")
	}
	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("the pool file is not in the PHP 8.3 directory: %v", err)
	}
	if !strings.Contains(string(content), "php:  8.3") && !strings.Contains(string(content), "php: 8.3") {
		t.Fatalf("the pool file does not record the new version:\n%s", content)
	}
	if result.PreviousVersion != "8.1" {
		t.Fatalf("the result says the previous version was %q, want 8.1", result.PreviousVersion)
	}

	if !runner.called("systemctl reload php8.3-fpm") {
		t.Fatal("the new version's FPM was not reloaded, so the site is not being served by it")
	}
	if !runner.called("systemctl reload php8.1-fpm") {
		t.Fatal("the old version's FPM was not reloaded, so its master still holds a pool whose " +
			"file is gone")
	}
}

// TestAFailedVersionSwitchPutsTheSiteBackOnTheOldVersion is the rollback for a
// switch, and it is the one that matters most: a customer clicking "PHP 8.3"
// and getting a 500 must still have a working site on 8.1.
func TestAFailedVersionSwitchPutsTheSiteBackOnTheOldVersion(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.1", "8.3")

	oldPath := filepath.Join(root, "/etc/php/8.1/fpm/pool.d/example-com.conf")
	newPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.1"), ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	runner.calls = nil
	runner.fail["php-fpm8.3 --test"] = fmt.Errorf("[ERROR] failed to open configuration file")

	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), "8.1"); err == nil {
		t.Fatal("a switch to a PHP that could not parse the pool was reported as success")
	}

	restored, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("the site's pool file was not put back on PHP 8.1; the site is down: %v", err)
	}
	if string(restored) != string(before) {
		t.Fatal("the restored 8.1 pool file differs from what was there before the switch")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("a pool file was left in the PHP 8.3 directory after a failed switch")
	}
	if !runner.called("systemctl reload php8.1-fpm") {
		t.Fatal("the rollback did not reload PHP 8.1, so its master is still missing the pool")
	}
}

// ---------------------------------------------------------------------------
// Removing a pool
// ---------------------------------------------------------------------------

func TestRemovePoolRestoresTheFileWhenTheReloadFails(t *testing.T) {
	runner := newFakeRunner()
	manager, root := newTestManager(t, runner, "8.3")
	poolPath := filepath.Join(root, "/etc/php/8.3/fpm/pool.d/example-com.conf")

	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), ""); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(poolPath)

	runner.fail["systemctl reload"] = fmt.Errorf("reload failed")
	if err := manager.RemovePool(context.Background(), "8.3", "example-com"); err == nil {
		t.Fatal("a removal whose reload failed was reported as success")
	}
	after, err := os.ReadFile(poolPath)
	if err != nil {
		t.Fatalf("the pool file was deleted even though the reload failed: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("the pool file was not restored byte for byte")
	}
}

func TestRemovePoolRefusesAnInjectedName(t *testing.T) {
	manager, _ := newTestManager(t, newFakeRunner(), "8.3")
	if err := manager.RemovePool(context.Background(), "8.3", "../../../etc/passwd"); err == nil {
		t.Fatal("a pool name that is a path traversal was accepted")
	}
}

// ---------------------------------------------------------------------------
// Version installation refuses cleanly where it must
// ---------------------------------------------------------------------------

func TestVersionRemovalRefusesWhilePoolsExist(t *testing.T) {
	runner := newFakeRunner()
	manager, _ := newTestManager(t, runner, "8.3")
	if _, err := manager.ApplyPool(context.Background(), testSpec("example-com", "8.3"), ""); err != nil {
		t.Fatal(err)
	}
	err := manager.RemoveVersion(context.Background(), "8.3")
	if err == nil {
		t.Fatal("PHP 8.3 was removed while a site was still running on it")
	}
	if !strings.Contains(err.Error(), "example-com") {
		t.Fatalf("the refusal does not name the site that would go down: %v", err)
	}
}

func TestInstalledVersionsReadsTheFilesystemNotTheDatabase(t *testing.T) {
	manager, _ := newTestManager(t, newFakeRunner(), "8.1", "8.3")
	installed := manager.InstalledVersions()
	if len(installed) != 2 {
		t.Fatalf("InstalledVersions returned %v, want 8.3 and 8.1", installed)
	}
	if installed[0] != "8.3" || installed[1] != "8.1" {
		t.Fatalf("InstalledVersions returned %v, want newest first", installed)
	}
}

// TestPackageArgvIsAVectorNeverAShellString proves the package manager is
// invoked with an argument vector. A version or extension name that reached a
// shell would be a root command on the customer's VPS.
func TestPackageArgvIsAVectorNeverAShellString(t *testing.T) {
	runner := newFakeRunner()
	manager, _ := newTestManager(t, runner, "8.3")

	if err := manager.InstallVersion(context.Background(), "8.3", []string{"redis"}); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "sh ") || strings.HasPrefix(call, "bash ") ||
			strings.Contains(call, " -c ") {
			t.Fatalf("a command was routed through a shell: %q", call)
		}
	}
	if !runner.called("apt-get install") || !runner.called("php8.3-fpm") {
		t.Fatalf("the FPM package was not installed; calls were %v", runner.calls)
	}
	if !runner.called("php8.3-redis") {
		t.Fatalf("the extension package was not installed; calls were %v", runner.calls)
	}
}
