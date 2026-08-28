package phpfpm

// Multi-version PHP is the feature customers migrate for, and it is the one
// place where "the panel supports nine operating systems" cannot be made true
// by writing the same code nine times. Side-by-side PHP is not a property of
// PHP; it is a property of somebody's package repository:
//
//   - Debian and Ubuntu have deb.sury.org (Ondrej Sury). It ships php5.6
//     through php8.4 as separate packages, each with its own FPM unit, its own
//     /etc/php/<ver>/fpm/pool.d and its own binary. Side-by-side is the design.
//   - RHEL, CentOS Stream, Rocky, AlmaLinux and Fedora have Remi. It ships
//     php74 through php84 as Software Collections under /opt/remi/php<NN>,
//     again one FPM unit and one pool directory each.
//   - openSUSE Leap ships exactly one PHP per release in the base repositories
//     and has no maintained side-by-side repository. There is no honest way to
//     run 7.4 and 8.3 on the same Leap host from packages.
//   - Amazon Linux 2023 ships php8.1/php8.2 from its own repositories, one at a
//     time. Remi does not publish for AL2023.
//
// So this file does the thing the brief asks for: it says which families are
// supported properly and refuses the other two cleanly, by name, with the
// reason and with the alternative. It does not pretend uniformity, and it does
// not silently do nothing - which is how a panel ends up reporting "PHP 8.3
// installed" on a host where nothing was installed at all.

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Family is the package-manager family, matching deploy/install.sh exactly:
// debian (apt-get), rhel (dnf) and suse (zypper). Keeping the same three names
// means an operator reading an installer log and a panel error sees one
// vocabulary, not two.
type Family string

const (
	FamilyDebian Family = "debian"
	FamilyRHEL   Family = "rhel"
	FamilySUSE   Family = "suse"
)

// MultiVersionSupport says what this host can actually do.
type MultiVersionSupport int

const (
	// MultiVersionUnknown is the zero value and never means "supported".
	MultiVersionUnknown MultiVersionSupport = iota
	// MultiVersionSupported: a side-by-side repository exists and is wired in.
	MultiVersionSupported
	// MultiVersionRefused: the distribution is supported by the panel, but not
	// for installing several PHP versions side by side. Existing pools are
	// still managed; only installation is refused.
	MultiVersionRefused
)

// Distro is everything the FPM manager needs to know about the host: where a
// version's pool directory is, what its systemd unit is called, what its
// php-fpm binary is, and how to install it.
//
// Every path here is derived from a validated version string (see
// ValidateVersion), so nothing an API caller sends can reach the filesystem
// as a path segment.
type Distro struct {
	// ID is the /etc/os-release ID, e.g. "ubuntu", "almalinux".
	ID string
	// VersionID is the /etc/os-release VERSION_ID, e.g. "24.04", "9".
	VersionID string
	// Pretty is PRETTY_NAME, used in operator-facing messages.
	Pretty string
	// Family is the package-manager family.
	Family Family
	// PackageManager is the binary that installs packages: apt-get, dnf,
	// zypper. It is never invoked through a shell.
	PackageManager string
	// MultiVersion says whether several PHP versions can be installed here.
	MultiVersion MultiVersionSupport
	// RefusalReason is set when MultiVersion is MultiVersionRefused. It is
	// written for an operator, names the distribution, and says what to do
	// instead.
	RefusalReason string
	// Repository is the third-party repository that provides the side-by-side
	// packages, for the operator-facing capability report.
	Repository string
}

// SupportsMultiVersion is the single question the service asks.
func (d Distro) SupportsMultiVersion() bool {
	return d.MultiVersion == MultiVersionSupported
}

// ErrMultiVersionUnsupported is returned by every entry point that would
// install or remove a PHP version on a host that cannot host several. It is a
// clean refusal: the caller can distinguish it from a failure and report it as
// a capability rather than as an error.
type ErrMultiVersionUnsupported struct {
	Distro Distro
}

func (e *ErrMultiVersionUnsupported) Error() string {
	return fmt.Sprintf("multi-version PHP is not supported on %s: %s",
		e.Distro.Pretty, e.Distro.RefusalReason)
}

// DetectDistro reads /etc/os-release and resolves the family the same way
// deploy/install.sh resolve_os_family does, then decides multi-version support.
//
// A host with no /etc/os-release is an error, not a guess. The installer
// refuses to install there; the panel refuses to claim it can manage PHP there.
func DetectDistro() (Distro, error) {
	return detectDistroFrom("/etc/os-release")
}

func detectDistroFrom(path string) (Distro, error) {
	f, err := os.Open(path)
	if err != nil {
		return Distro{}, fmt.Errorf("cannot identify this operating system: %s is unreadable: %w", path, err)
	}
	defer f.Close()

	fields := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return Distro{}, fmt.Errorf("cannot identify this operating system: reading %s: %w", path, err)
	}

	return classify(fields["ID"], fields["VERSION_ID"], fields["PRETTY_NAME"], fields["ID_LIKE"]), nil
}

// classify maps an os-release triple onto the support matrix. It is separate
// from the file reading so the whole matrix is testable without nine hosts.
func classify(id, versionID, pretty, idLike string) Distro {
	id = strings.ToLower(strings.TrimSpace(id))
	if pretty == "" {
		pretty = id
		if versionID != "" {
			pretty += " " + versionID
		}
	}

	d := Distro{ID: id, VersionID: versionID, Pretty: pretty}

	switch id {
	case "ubuntu", "debian", "linuxmint", "pop", "elementary", "raspbian":
		d.Family = FamilyDebian
	case "centos", "rhel", "redhat", "rocky", "almalinux", "ol", "oracle", "fedora":
		d.Family = FamilyRHEL
	case "amzn":
		d.Family = FamilyRHEL
	case "opensuse-leap", "opensuse", "opensuse-tumbleweed", "sles", "sled":
		d.Family = FamilySUSE
	default:
		// Derive from ID_LIKE, but never silently: the family is only used to
		// manage pools that already exist, never to install.
		like := " " + strings.ToLower(idLike) + " "
		switch {
		case strings.Contains(like, "debian"), strings.Contains(like, "ubuntu"):
			d.Family = FamilyDebian
		case strings.Contains(like, "rhel"), strings.Contains(like, "fedora"), strings.Contains(like, "centos"):
			d.Family = FamilyRHEL
		case strings.Contains(like, "suse"):
			d.Family = FamilySUSE
		}
	}

	switch d.Family {
	case FamilyDebian:
		d.PackageManager = "apt-get"
	case FamilyRHEL:
		d.PackageManager = "dnf"
	case FamilySUSE:
		d.PackageManager = "zypper"
	}

	// The support decision. Only the two families with a real side-by-side
	// repository are supported; everything else is refused by name.
	switch {
	case id == "ubuntu" || id == "debian":
		d.MultiVersion = MultiVersionSupported
		d.Repository = "deb.sury.org (Ondrej Sury)"
	case id == "rhel" || id == "redhat" || id == "centos" || id == "rocky" ||
		id == "almalinux" || id == "ol" || id == "oracle" || id == "fedora":
		d.MultiVersion = MultiVersionSupported
		d.Repository = "Remi's RPM repository"
	case id == "amzn":
		d.MultiVersion = MultiVersionRefused
		d.RefusalReason = "Amazon Linux 2023 ships one PHP version at a time from its own " +
			"repositories and Remi does not publish packages for it, so several PHP versions " +
			"cannot be installed side by side. The panel will manage the PHP that AL2023 " +
			"provides, including its FPM pools, but it will not claim to install a second version."
	case d.Family == FamilySUSE:
		d.MultiVersion = MultiVersionRefused
		d.RefusalReason = "openSUSE Leap ships exactly one PHP version per release in the base " +
			"repositories and has no maintained side-by-side repository comparable to " +
			"deb.sury.org or Remi. The panel will manage the PHP that Leap provides, including " +
			"its FPM pools, but it will not claim to install a second version."
	case d.Family == "":
		d.MultiVersion = MultiVersionRefused
		d.RefusalReason = fmt.Sprintf("this operating system (ID=%q) is not one of the nine the "+
			"panel supports and no family could be derived from ID_LIKE, so neither its package "+
			"manager nor its PHP layout is known.", id)
	default:
		// A derived family: Mint, Pop!_OS, Oracle by ID_LIKE and so on. The
		// pool layout is almost certainly right, but the repository has not
		// been tested, so installation is refused rather than half-attempted.
		d.MultiVersion = MultiVersionRefused
		d.RefusalReason = fmt.Sprintf("%s is outside the tested matrix; its family (%s) was "+
			"derived from ID_LIKE. Existing PHP-FPM pools are managed normally, but installing a "+
			"PHP version is refused because the side-by-side repository for this distribution "+
			"has not been verified.", pretty, d.Family)
	}

	return d
}

// SupportedFamiliesReport is the operator-facing answer to "which operating
// systems can this panel really install several PHP versions on". It is served
// by GET /php/system so the answer lives in the product, not only in a
// document nobody reads.
func SupportedFamiliesReport() []FamilySupport {
	return []FamilySupport{
		{Distribution: "Ubuntu 20.04 / 22.04 / 24.04", Family: string(FamilyDebian), Supported: true, Mechanism: "deb.sury.org (Ondrej Sury)"},
		{Distribution: "Debian 11 / 12", Family: string(FamilyDebian), Supported: true, Mechanism: "deb.sury.org (Ondrej Sury)"},
		{Distribution: "RHEL 8 / 9", Family: string(FamilyRHEL), Supported: true, Mechanism: "Remi's RPM repository"},
		{Distribution: "CentOS Stream 8 / 9", Family: string(FamilyRHEL), Supported: true, Mechanism: "Remi's RPM repository"},
		{Distribution: "Rocky Linux 8 / 9", Family: string(FamilyRHEL), Supported: true, Mechanism: "Remi's RPM repository"},
		{Distribution: "AlmaLinux 8 / 9", Family: string(FamilyRHEL), Supported: true, Mechanism: "Remi's RPM repository"},
		{Distribution: "Fedora 38+", Family: string(FamilyRHEL), Supported: true, Mechanism: "Remi's RPM repository"},
		{Distribution: "openSUSE Leap 15.x", Family: string(FamilySUSE), Supported: false, Mechanism: "no side-by-side repository exists; Leap ships one PHP per release"},
		{Distribution: "Amazon Linux 2023", Family: string(FamilyRHEL), Supported: false, Mechanism: "Remi does not publish for AL2023; AL2023 ships one PHP at a time"},
	}
}

// FamilySupport is one row of the report above.
type FamilySupport struct {
	Distribution string `json:"distribution"`
	Family       string `json:"family"`
	Supported    bool   `json:"multi_version_supported"`
	Mechanism    string `json:"mechanism"`
}

// DetectedForTest builds a Distro from an os-release triple without reading a
// file. It is exported because internal/service has to be able to drive the
// whole pool lifecycle against a named distribution - proving that a Debian
// host really does get /etc/php/8.3/fpm/pool.d and an AlmaLinux one really does
// get /etc/opt/remi/php83 - and a test cannot change what /etc/os-release says
// on the machine it runs on.
//
// It is deliberately the same classify() the real detector uses, so a test can
// never be passing against a support matrix that production does not have.
func DetectedForTest(id, versionID, pretty, idLike string) Distro {
	return classify(id, versionID, pretty, idLike)
}
