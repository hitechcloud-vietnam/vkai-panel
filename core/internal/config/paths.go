package config

// System path layout for VKAI Panel (HiTech Cloud).
//
// Every absolute path the panel writes to or reads from is declared here and
// nowhere else. A literal "/var/www" or "/opt/vkai-panel" left in a service is
// a path the operator cannot move and the security tests cannot reason about,
// so the rule is: no filesystem literal outside this file.
//
// The layout is rooted at a single directory so an operator can relocate the
// whole installation with one variable:
//
//	/vkai-panel/                      PanelRoot   (VKAI_PANEL_ROOT)
//	/vkai-panel/core/                 API code and binaries
//	/vkai-panel/panel/                built UI
//	/vkai-panel/www/domains/<domain>  WebRoot     (VKAI_WEB_ROOT)   - customer sites
//	/vkai-panel/www/backup/           BackupRoot  (VKAI_BACKUP_ROOT)
//	/vkai-panel/www/default/          DefaultSite - catch-all vhost
//	/vkai-panel/logs/                 LogRoot     (VKAI_LOG_ROOT)
//	/vkai-panel/logs/sites/<domain>/  SiteLogRoot - web server logs per site
//	/vkai-panel/etc/                  EtcRoot     - .env, config.yaml, state files
//	/vkai-panel/ssl/                  SSLRoot     - certificates
//	/vkai-panel/tmp/                  TmpRoot     - staging for backups/uploads
//
// Overriding VKAI_PANEL_ROOT moves every derived path with it. Overriding one
// of the specific variables moves only that subtree, which is how a separate
// backup volume or log partition is mounted in.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultPanelRoot is the installation root of the panel. Everything else in
// this file is derived from it unless the operator says otherwise.
const DefaultPanelRoot = "/vkai-panel"

// Names of the environment variables that move the layout. Legacy names are
// still honoured so an upgrade from an older deployment keeps working.
const (
	EnvPanelRoot  = "VKAI_PANEL_ROOT"
	EnvWebRoot    = "VKAI_WEB_ROOT"
	EnvBackupRoot = "VKAI_BACKUP_ROOT"
	EnvLogRoot    = "VKAI_LOG_ROOT"
	EnvEtcRoot    = "VKAI_ETC_ROOT"
	EnvSSLRoot    = "VKAI_SSL_ROOT"
	EnvTmpRoot    = "VKAI_TMP_ROOT"
)

// PanelRoot is the installation root: /vkai-panel unless VKAI_PANEL_ROOT says
// otherwise.
func PanelRoot() string {
	return rootFromEnv(EnvPanelRoot, DefaultPanelRoot, "PANEL_ROOT")
}

// CoreRoot holds the API binary and its assets (formerly backend/).
func CoreRoot() string { return filepath.Join(PanelRoot(), "core") }

// UIRoot holds the built Next.js UI (formerly frontend/).
func UIRoot() string { return filepath.Join(PanelRoot(), "panel") }

// WWWRoot is the parent of the per-domain site trees, the backups and the
// default vhost. Nothing writes here directly.
func WWWRoot() string { return filepath.Join(PanelRoot(), "www") }

// WebRoot is the only tree a customer site document root may live under.
// Replaces the historical /var/www.
func WebRoot() string {
	return rootFromEnv(EnvWebRoot, filepath.Join(WWWRoot(), "domains"), "WEB_ROOT")
}

// BackupRoot is the only tree backups may be written to.
func BackupRoot() string {
	return rootFromEnv(EnvBackupRoot, filepath.Join(WWWRoot(), "backup"), "BACKUP_ROOT")
}

// DefaultSite is the catch-all document root served for unmatched hosts.
func DefaultSite() string { return filepath.Join(WWWRoot(), "default") }

// LogRoot holds the panel's own logs.
func LogRoot() string {
	return rootFromEnv(EnvLogRoot, filepath.Join(PanelRoot(), "logs"), "LOG_ROOT")
}

// SiteLogRoot holds the web server access/error logs, one directory per site.
func SiteLogRoot() string { return filepath.Join(LogRoot(), "sites") }

// EtcRoot holds the panel configuration: .env, config.yaml, panel_access.json.
func EtcRoot() string {
	return rootFromEnv(EnvEtcRoot, filepath.Join(PanelRoot(), "etc"), "ETC_ROOT")
}

// SSLRoot holds certificates issued for the panel and for the hosted sites.
func SSLRoot() string {
	return rootFromEnv(EnvSSLRoot, filepath.Join(PanelRoot(), "ssl"), "SSL_ROOT")
}

// TmpRoot is scratch space owned by the panel. It is deliberately not /tmp:
// staging a backup in a world-writable directory is a symlink race.
func TmpRoot() string {
	return rootFromEnv(EnvTmpRoot, filepath.Join(PanelRoot(), "tmp"), "TMP_ROOT")
}

// ConfigFile is the panel's YAML configuration file.
func ConfigFile() string { return filepath.Join(EtcRoot(), "config.yaml") }

// EnvFile is the panel's environment file, read by the systemd units.
func EnvFile() string { return filepath.Join(EtcRoot(), ".env") }

// PanelStateFile is where the panel access gate persists its port and entrance.
func PanelStateFile() string { return filepath.Join(EtcRoot(), "panel_access.json") }

// PanelSSLDir holds the panel's own certificate, kept apart from the customer
// certificates so a site renewal can never overwrite the panel's key.
func PanelSSLDir() string { return filepath.Join(SSLRoot(), "panel") }

// DatabaseBackupDir is where database dumps land.
func DatabaseBackupDir() string { return filepath.Join(BackupRoot(), "databases") }

// maxDomainLength is the maximum length of a DNS name, and therefore of the
// single path segment a site root may add to WebRoot.
const maxDomainLength = 253

// domainNameRe accepts exactly the characters that are safe as one path
// segment: lowercase letters, digits, dots and hyphens, starting and ending on
// an alphanumeric. Everything a path traversal needs - "/", "\", "..", NUL,
// a leading dot or hyphen - is outside this set.
var domainNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// NormalizeDomain lowercases and trims a domain so that "Example.COM" and
// "example.com" can never become two different directories on disk.
func NormalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}

// ValidateDomain rejects anything that must not be used as a directory name.
// It is the gate in front of every path built from caller-supplied input; the
// callers below return its error rather than building a path they would then
// have to re-check.
func ValidateDomain(domain string) error {
	// Checked on the raw value, before trimming: TrimSpace would otherwise
	// launder "example.com\n" into a valid domain, leaving the stored domain
	// and the directory on disk disagreeing about what the site is called.
	if strings.ContainsFunc(domain, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return fmt.Errorf("ten mien chua ky tu dieu khien khong hop le")
	}

	d := NormalizeDomain(domain)
	if d == "" {
		return fmt.Errorf("ten mien khong duoc de trong")
	}
	if len(d) > maxDomainLength {
		return fmt.Errorf("ten mien qua dai (toi da %d ky tu)", maxDomainLength)
	}
	if strings.Contains(d, "..") {
		return fmt.Errorf("ten mien %q khong hop le: chua '..'", domain)
	}
	if strings.HasPrefix(d, ".") || strings.HasPrefix(d, "-") {
		return fmt.Errorf("ten mien %q khong hop le: bat dau bang '.' hoac '-'", domain)
	}
	if !domainNameRe.MatchString(d) {
		return fmt.Errorf("ten mien %q khong hop le: chi cho phep chu thuong, so, '.' va '-'", domain)
	}
	return nil
}

// SiteRoot returns the document root of one site: WebRoot/<domain>. The domain
// is validated first, so the returned path is always exactly one segment below
// the web root and can never escape it.
func SiteRoot(domain string) (string, error) {
	return siteSubdir(WebRoot(), domain)
}

// SiteLogDir returns the web server log directory of one site.
func SiteLogDir(domain string) (string, error) {
	return siteSubdir(SiteLogRoot(), domain)
}

// SiteSSLDir returns the certificate directory of one site.
func SiteSSLDir(domain string) (string, error) {
	return siteSubdir(SSLRoot(), domain)
}

// SiteBackupDir returns the backup directory of one site.
func SiteBackupDir(domain string) (string, error) {
	return siteSubdir(BackupRoot(), domain)
}

// siteSubdir joins a validated domain onto a root and proves the result did not
// leave that root. The containment check is redundant given ValidateDomain, and
// that is the point: it holds even if the character set is ever widened.
func siteSubdir(root, domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	base := filepath.Clean(root)
	joined := filepath.Clean(filepath.Join(base, NormalizeDomain(domain)))
	if joined != base && !strings.HasPrefix(joined, base+string(filepath.Separator)) {
		return "", fmt.Errorf("ten mien %q tao ra duong dan nam ngoai %s", domain, base)
	}
	return joined, nil
}

// WithinWebRoot checks that a caller-supplied document root stays inside the
// web root, for the case where the caller supplies a full path instead of a
// domain.
func WithinWebRoot(path string) (string, error) {
	return within(WebRoot(), path, "goc website")
}

// WithinBackupRoot checks that a caller-supplied backup destination stays
// inside the backup root.
func WithinBackupRoot(path string) (string, error) {
	return within(BackupRoot(), path, "thu muc sao luu")
}

func within(root, path, what string) (string, error) {
	if strings.ContainsAny(path, "\x00\n\r") {
		return "", fmt.Errorf("%s %q chua ky tu khong hop le", what, path)
	}
	base := filepath.Clean(root)
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		clean = filepath.Clean(filepath.Join(base, clean))
	}
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q nam ngoai %s", what, path, base)
	}
	return clean, nil
}

// rootFromEnv resolves one path variable. A value that is not an absolute path
// is ignored rather than honoured: a relative root would silently re-anchor
// every site directory onto the process working directory.
func rootFromEnv(primary, fallback string, legacy ...string) string {
	names := append([]string{primary}, legacy...)
	for _, name := range names {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" || !filepath.IsAbs(v) {
			continue
		}
		return filepath.Clean(v)
	}
	return filepath.Clean(fallback)
}
