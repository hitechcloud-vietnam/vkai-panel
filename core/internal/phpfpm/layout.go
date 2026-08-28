package phpfpm

// Where a PHP version lives, per family.
//
// Every path in this file is built from a version string that has been through
// ValidateVersion, which accepts exactly `<major>.<minor>` with one or two
// digits each. That is the whole defence: a version reaches here from an HTTP
// request body, is concatenated into /etc/php/<ver>/fpm/pool.d, and a version
// of "../../../etc/cron.d" would otherwise write a root cron job. There is no
// second layer to fall back on, so the regexp is anchored and total.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// versionRe accepts 5.6, 7.4, 8.0 ... 8.4 and nothing else. No slashes, no
// dots beyond the one separator, no leading dash (which would turn a version
// into an option when it reaches a package manager argv).
var versionRe = regexp.MustCompile(`^[0-9]\.[0-9]{1,2}$`)

// MinSupportedVersion / MaxSupportedVersion bound what the panel will install.
// Below 7.4 the security support is gone from both repositories; above the
// maximum the panel has not been tested and the pool directives differ.
const (
	MinSupportedVersion = "7.4"
	MaxSupportedVersion = "8.4"
)

// ValidateVersion is the only way a version string becomes a path segment.
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("php version is required")
	}
	if !versionRe.MatchString(version) {
		return fmt.Errorf("invalid php version %q: expected a major.minor version such as 8.3", version)
	}
	if compareVersion(version, MinSupportedVersion) < 0 || compareVersion(version, MaxSupportedVersion) > 0 {
		return fmt.Errorf("php version %q is outside the supported range %s-%s",
			version, MinSupportedVersion, MaxSupportedVersion)
	}
	return nil
}

// compareVersion orders two already well-formed major.minor strings. It is
// numeric, so 8.10 sorts after 8.9 rather than before it the way a string
// comparison would.
func compareVersion(a, b string) int {
	aMajor, aMinor := splitVersion(a)
	bMajor, bMinor := splitVersion(b)
	switch {
	case aMajor != bMajor:
		return aMajor - bMajor
	case aMinor != bMinor:
		return aMinor - bMinor
	}
	return 0
}

func splitVersion(v string) (int, int) {
	major, minor, _ := strings.Cut(v, ".")
	m, _ := strconv.Atoi(major)
	n, _ := strconv.Atoi(minor)
	return m, n
}

// SupportedVersions lists the versions the panel will install, newest first.
func SupportedVersions() []string {
	return []string{"8.4", "8.3", "8.2", "8.1", "8.0", "7.4"}
}

// Layout is the set of filesystem and systemd names for one PHP version on one
// distribution. Every field is an absolute path or a unit name; nothing here is
// a shell fragment.
type Layout struct {
	Version string
	// PoolDir is the directory FPM scans for pool files.
	PoolDir string
	// ConfDir is the directory PHP scans for extension .ini files. Extensions
	// are a property of the FPM master process, not of a pool - see the note
	// on PoolSpec.Extensions - so this is where an extension is really turned
	// on or off.
	ConfDir string
	// MainConfig is the fpm.conf / php-fpm.conf the binary is tested against.
	MainConfig string
	// Binary is the php-fpm executable for this version.
	Binary string
	// CLIBinary is the php executable for this version. WP-CLI runs under it.
	CLIBinary string
	// Service is the systemd unit that owns this version's master process.
	Service string
	// SocketDir is where this version's per-pool unix sockets are created.
	SocketDir string
	// Package is the name of the FPM package for this version.
	Package string
	// ExtensionPackagePrefix turns an extension name into a package name.
	ExtensionPackagePrefix string
}

// SocketPath is the listen socket for one pool of this version. The pool name
// has been through ValidatePoolName, so it is one safe path segment.
func (l Layout) SocketPath(poolName string) string {
	return filepath.Join(l.SocketDir, poolName+".sock")
}

// PoolPath is the pool file for one pool of this version.
func (l Layout) PoolPath(poolName string) string {
	return filepath.Join(l.PoolDir, poolName+".conf")
}

// ExtensionPackage is the distribution package that provides one extension for
// this version, e.g. php8.3-redis or php83-php-redis.
func (l Layout) ExtensionPackage(extension string) string {
	return l.ExtensionPackagePrefix + extension
}

// LayoutFor resolves the layout for a version on a distribution.
//
// The two supported families lay PHP out completely differently, and that
// difference is the whole reason this function exists rather than a format
// string somewhere:
//
//	Debian/Ubuntu (sury)  /etc/php/8.3/fpm/pool.d/      php8.3-fpm      /usr/sbin/php-fpm8.3
//	RHEL family (remi)    /etc/opt/remi/php83/php-fpm.d/ php83-php-fpm  /opt/remi/php83/root/usr/sbin/php-fpm
//
// A caller that writes a Debian pool file on an AlmaLinux host produces a file
// that FPM never reads, and a panel that reports success. That is precisely the
// class of failure this repository has already paid for, so the layout is
// resolved once, from the detected distribution, and never assumed.
func LayoutFor(d Distro, version string) (Layout, error) {
	if err := ValidateVersion(version); err != nil {
		return Layout{}, err
	}

	switch d.Family {
	case FamilyDebian:
		return Layout{
			Version:                version,
			PoolDir:                filepath.Join("/etc/php", version, "fpm/pool.d"),
			ConfDir:                filepath.Join("/etc/php", version, "fpm/conf.d"),
			MainConfig:             filepath.Join("/etc/php", version, "fpm/php-fpm.conf"),
			Binary:                 "/usr/sbin/php-fpm" + version,
			CLIBinary:              "/usr/bin/php" + version,
			Service:                "php" + version + "-fpm",
			SocketDir:              filepath.Join("/run/php"),
			Package:                "php" + version + "-fpm",
			ExtensionPackagePrefix: "php" + version + "-",
		}, nil

	case FamilyRHEL:
		// Remi collapses the dot: 8.3 becomes php83.
		short := strings.ReplaceAll(version, ".", "")
		return Layout{
			Version:                version,
			PoolDir:                filepath.Join("/etc/opt/remi/php"+short, "php-fpm.d"),
			ConfDir:                filepath.Join("/etc/opt/remi/php"+short, "php.d"),
			MainConfig:             filepath.Join("/etc/opt/remi/php"+short, "php-fpm.conf"),
			Binary:                 filepath.Join("/opt/remi/php"+short, "root/usr/sbin/php-fpm"),
			CLIBinary:              filepath.Join("/opt/remi/php"+short, "root/usr/bin/php"),
			Service:                "php" + short + "-php-fpm",
			SocketDir:              filepath.Join("/var/opt/remi/php"+short, "run/php-fpm"),
			Package:                "php" + short + "-php-fpm",
			ExtensionPackagePrefix: "php" + short + "-php-",
		}, nil

	case FamilySUSE:
		// Leap has exactly one PHP, installed unversioned. The layout is given
		// so that an existing pool can still be read, written and reloaded;
		// installing a second version is refused elsewhere, by name.
		return Layout{
			Version:                version,
			PoolDir:                "/etc/php8/fpm/php-fpm.d",
			ConfDir:                "/etc/php8/conf.d",
			MainConfig:             "/etc/php8/fpm/php-fpm.conf",
			Binary:                 "/usr/sbin/php-fpm",
			CLIBinary:              "/usr/bin/php",
			Service:                "php-fpm",
			SocketDir:              "/run/php-fpm",
			Package:                "php8-fpm",
			ExtensionPackagePrefix: "php8-",
		}, nil

	default:
		return Layout{}, fmt.Errorf("no PHP layout is known for %s: %s", d.Pretty, d.RefusalReason)
	}
}
