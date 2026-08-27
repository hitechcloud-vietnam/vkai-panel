package webserver

// Filesystem layout for the vhosts the adapters generate.
//
// Every adapter used to embed its distro's default directories (/var/www,
// /var/log/nginx, /var/log/caddy, ...) directly into the configuration it
// wrote. That made a site's document root and its logs depend on which web
// server happened to be installed, and it put customer data outside the panel
// installation root, where nothing backs it up. All of it now comes from the
// single layout declared in internal/config.

import (
	"fmt"
	"path/filepath"

	panelpaths "github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// SiteRoot is the document root of one site: /vkai-panel/www/domains/<domain>.
// The domain is validated, so the result is always one segment below the web
// root.
func SiteRoot(domain string) (string, error) {
	return panelpaths.SiteRoot(domain)
}

// SiteLogRoot is the parent of every per-site web server log directory:
// /vkai-panel/logs/sites.
func SiteLogRoot() string {
	return panelpaths.SiteLogRoot()
}

// SiteLogDir is the log directory of one site: /vkai-panel/logs/sites/<domain>.
func SiteLogDir(domain string) (string, error) {
	return panelpaths.SiteLogDir(domain)
}

// siteLogDirFor is the per-site log directory below an adapter's log root.
func siteLogDirFor(logRoot, domain string) (string, error) {
	if err := panelpaths.ValidateDomain(domain); err != nil {
		return "", err
	}
	return filepath.Join(logRoot, panelpaths.NormalizeDomain(domain)), nil
}

// siteLogFile builds one log file path below logRoot. logRoot is the adapter's
// configurable log root so an operator can still point a web server at its
// distro directory; the domain segment is validated either way, because the
// domain reaches here straight from an API request.
func siteLogFile(logRoot, domain, name string) (string, error) {
	if err := panelpaths.ValidateDomain(domain); err != nil {
		return "", err
	}
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid log file name %q", name)
	}
	return filepath.Join(logRoot, panelpaths.NormalizeDomain(domain), name), nil
}
