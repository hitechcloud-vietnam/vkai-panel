package phpfpm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheSupportMatrixIsHonestAboutAllNineFamilies is the test for the task's
// fourth PHP requirement: "say which families you support properly and which
// you refuse cleanly, rather than pretending uniformity".
//
// Seven of the nine can install several PHP versions side by side, because
// somebody publishes packages for them (deb.sury.org, Remi). Two cannot, and
// the panel says so by name with the reason, rather than running a package
// manager that installs the wrong thing and reporting success.
func TestTheSupportMatrixIsHonestAboutAllNineFamilies(t *testing.T) {
	cases := []struct {
		id            string
		versionID     string
		pretty        string
		idLike        string
		wantFamily    Family
		wantPkg       string
		wantSupported bool
		// wantReason is a phrase the refusal must contain, so a refusal always
		// explains itself.
		wantReason string
	}{
		// The seven where multi-version PHP genuinely works.
		{"ubuntu", "24.04", "Ubuntu 24.04 LTS", "debian", FamilyDebian, "apt-get", true, ""},
		{"debian", "12", "Debian GNU/Linux 12 (bookworm)", "", FamilyDebian, "apt-get", true, ""},
		{"rhel", "9", "Red Hat Enterprise Linux 9", "fedora", FamilyRHEL, "dnf", true, ""},
		{"centos", "9", "CentOS Stream 9", "rhel fedora", FamilyRHEL, "dnf", true, ""},
		{"rocky", "9", "Rocky Linux 9", "rhel centos fedora", FamilyRHEL, "dnf", true, ""},
		{"almalinux", "9", "AlmaLinux 9", "rhel centos fedora", FamilyRHEL, "dnf", true, ""},
		{"fedora", "40", "Fedora Linux 40", "", FamilyRHEL, "dnf", true, ""},

		// The two that are refused, by name, with the reason.
		{"opensuse-leap", "15.6", "openSUSE Leap 15.6", "suse opensuse", FamilySUSE, "zypper", false,
			"no maintained side-by-side repository"},
		{"amzn", "2023", "Amazon Linux 2023", "fedora", FamilyRHEL, "dnf", false,
			"Remi does not publish packages for it"},
	}

	for _, tc := range cases {
		t.Run(tc.pretty, func(t *testing.T) {
			d := classify(tc.id, tc.versionID, tc.pretty, tc.idLike)

			if d.Family != tc.wantFamily {
				t.Errorf("family is %q, want %q", d.Family, tc.wantFamily)
			}
			if d.PackageManager != tc.wantPkg {
				t.Errorf("package manager is %q, want %q", d.PackageManager, tc.wantPkg)
			}
			if got := d.SupportsMultiVersion(); got != tc.wantSupported {
				t.Fatalf("SupportsMultiVersion() = %v, want %v", got, tc.wantSupported)
			}

			if tc.wantSupported {
				if d.Repository == "" {
					t.Error("a supported distribution must name the repository that makes it work")
				}
				if d.RefusalReason != "" {
					t.Errorf("a supported distribution must not carry a refusal reason: %q", d.RefusalReason)
				}
				return
			}

			// A refusal has to explain itself. "Not supported" sends an
			// operator to read source code.
			if !strings.Contains(d.RefusalReason, tc.wantReason) {
				t.Errorf("the refusal reason %q does not contain %q", d.RefusalReason, tc.wantReason)
			}
			if !strings.Contains(d.RefusalReason, "will manage") {
				t.Error("the refusal must say what the panel WILL still do on this host, not " +
					"only what it will not")
			}
		})
	}
}

// TestARefusedDistributionRefusesInstallationRatherThanDoingSomethingElse
// proves the refusal is enforced, not merely reported. openSUSE's zypper would
// happily install "php8-fpm" when asked for 8.3, and the panel would then have
// a database row claiming PHP 8.3 on a host running whatever Leap ships.
func TestARefusedDistributionRefusesInstallationRatherThanDoingSomethingElse(t *testing.T) {
	for _, id := range []string{"opensuse-leap", "amzn"} {
		t.Run(id, func(t *testing.T) {
			distro := classify(id, "15.6", id, "")
			runner := newFakeRunner()
			manager, err := New(Options{Distro: &distro, Runner: runner, RootDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}

			err = manager.InstallVersion(nil, "8.3", nil)
			var unsupported *ErrMultiVersionUnsupported
			if !errors.As(err, &unsupported) {
				t.Fatalf("InstallVersion returned %v, want ErrMultiVersionUnsupported", err)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("a refused host still ran commands: %v", runner.calls)
			}
			if !strings.Contains(err.Error(), "multi-version PHP is not supported on") {
				t.Fatalf("the error does not name the capability that is missing: %v", err)
			}
		})
	}
}

// TestPoolManagementStillWorksOnARefusedDistribution is the other half of
// honesty: refusing to INSTALL a second PHP is not the same as refusing to
// manage the PHP that is there. A Leap host still gets pool files.
func TestPoolManagementStillWorksOnARefusedDistribution(t *testing.T) {
	distro := classify("opensuse-leap", "15.6", "openSUSE Leap 15.6", "suse")
	root := t.TempDir()
	runner := newFakeRunner()
	manager, err := New(Options{Distro: &distro, Runner: runner, RootDir: root, SystemctlPath: "systemctl"})
	if err != nil {
		t.Fatal(err)
	}
	layout, err := manager.Layout("8.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{layout.PoolDir, filepath.Dir(layout.Binary)} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.ApplyPool(nil, testSpec("example-com", "8.3"), ""); err != nil {
		t.Fatalf("a Leap host could not have a pool file written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "/etc/php8/fpm/php-fpm.d/example-com.conf")); err != nil {
		t.Fatalf("the pool file was not written where openSUSE looks for it: %v", err)
	}
}

// TestLayoutsDifferPerFamily is the assertion that stops somebody "unifying"
// the two layouts. A Debian pool file written into a RHEL host's directory is a
// file FPM never reads and a panel that reports success.
func TestLayoutsDifferPerFamily(t *testing.T) {
	debian, err := LayoutFor(classify("ubuntu", "24.04", "Ubuntu", "debian"), "8.3")
	if err != nil {
		t.Fatal(err)
	}
	rhel, err := LayoutFor(classify("almalinux", "9", "AlmaLinux 9", "rhel"), "8.3")
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name                 string
		debian, rhel         string
		wantDebian, wantRHEL string
	}{
		{"pool directory", debian.PoolDir, rhel.PoolDir,
			"/etc/php/8.3/fpm/pool.d", "/etc/opt/remi/php83/php-fpm.d"},
		{"fpm binary", debian.Binary, rhel.Binary,
			"/usr/sbin/php-fpm8.3", "/opt/remi/php83/root/usr/sbin/php-fpm"},
		{"systemd unit", debian.Service, rhel.Service,
			"php8.3-fpm", "php83-php-fpm"},
		{"fpm package", debian.Package, rhel.Package,
			"php8.3-fpm", "php83-php-fpm"},
	}
	for _, check := range checks {
		if check.debian != check.wantDebian {
			t.Errorf("Debian %s is %q, want %q", check.name, check.debian, check.wantDebian)
		}
		if check.rhel != check.wantRHEL {
			t.Errorf("RHEL %s is %q, want %q", check.name, check.rhel, check.wantRHEL)
		}
		if check.debian == check.rhel {
			t.Errorf("the Debian and RHEL %s are identical (%q); one of the two families is "+
				"being written the other's layout", check.name, check.debian)
		}
	}

	if debian.ExtensionPackage("redis") != "php8.3-redis" {
		t.Errorf("Debian extension package is %q", debian.ExtensionPackage("redis"))
	}
	if rhel.ExtensionPackage("redis") != "php83-php-redis" {
		t.Errorf("RHEL extension package is %q", rhel.ExtensionPackage("redis"))
	}
}

// TestAVersionCanNeverBecomeAPathSegment is the injection test for the one
// value that is concatenated straight into /etc.
func TestAVersionCanNeverBecomeAPathSegment(t *testing.T) {
	distro := classify("ubuntu", "24.04", "Ubuntu", "debian")
	for _, version := range []string{
		"../../../etc/cron.d", "8.3/../../etc", "8.3\n", "8.3;rm -rf /", "-8.3",
		"8.3 ", "", "latest", "8", "8.3.1", "../8.3",
	} {
		if _, err := LayoutFor(distro, version); err == nil {
			t.Errorf("LayoutFor accepted the version %q and would build a path from it", version)
		}
	}
	// The whole supported range must still be accepted.
	for _, version := range SupportedVersions() {
		if _, err := LayoutFor(distro, version); err != nil {
			t.Errorf("LayoutFor refused the supported version %q: %v", version, err)
		}
	}
}

// TestDetectDistroReadsOsRelease proves the real reader parses the file the
// installer reads, quotes and all.
func TestDetectDistroReadsOsRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	content := "NAME=\"Rocky Linux\"\nVERSION=\"9.4 (Blue Onyx)\"\nID=\"rocky\"\n" +
		"ID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\n" +
		"PRETTY_NAME=\"Rocky Linux 9.4 (Blue Onyx)\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := detectDistroFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "rocky" || d.Family != FamilyRHEL || !d.SupportsMultiVersion() {
		t.Fatalf("Rocky Linux 9.4 was classified as %+v", d)
	}
	if d.Pretty != "Rocky Linux 9.4 (Blue Onyx)" {
		t.Fatalf("PRETTY_NAME was not used: %q", d.Pretty)
	}
}

// TestAnUnidentifiableHostIsAnErrorNotAGuess: a container with no
// /etc/os-release must not be quietly treated as Debian.
func TestAnUnidentifiableHostIsAnErrorNotAGuess(t *testing.T) {
	if _, err := detectDistroFrom(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a host with no /etc/os-release was classified anyway")
	}
}

// TestTheReportCoversExactlyTheNineSupportedOperatingSystems keeps the
// operator-facing matrix in step with deploy/README.md's table.
func TestTheReportCoversExactlyTheNineSupportedOperatingSystems(t *testing.T) {
	report := SupportedFamiliesReport()
	if len(report) != 9 {
		t.Fatalf("the support matrix has %d rows, want 9 - one per operating system the "+
			"installer supports", len(report))
	}
	supported := 0
	for _, row := range report {
		if row.Mechanism == "" {
			t.Errorf("%s has no mechanism recorded", row.Distribution)
		}
		if row.Supported {
			supported++
		}
	}
	if supported != 7 {
		t.Fatalf("%d of the nine are reported as supporting multi-version PHP, want 7 "+
			"(openSUSE Leap and Amazon Linux 2023 are the two that cannot)", supported)
	}
}
