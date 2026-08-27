package webserver

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// WebServerAdapter defines the interface for web server management.
// Each web server (Nginx, Apache, OpenLiteSpeed, etc.) implements this interface.
type WebServerAdapter interface {
	// Name returns the web server name
	Name() string

	// Site management
	CreateSite(ctx context.Context, config *SiteConfig) error
	DeleteSite(ctx context.Context, domain string) error
	EnableSite(ctx context.Context, domain string) error
	DisableSite(ctx context.Context, domain string) error

	// Configuration
	CreateVirtualHost(ctx context.Context, config *SiteConfig) error
	ConfigureSSL(ctx context.Context, domain, certPath, keyPath string) error
	ConfigurePHP(ctx context.Context, domain, phpVersion string) error
	ConfigureProxy(ctx context.Context, domain, target string) error
	ConfigureHeaders(ctx context.Context, domain string, headers map[string]string) error
	ConfigureRedirect(ctx context.Context, domain, targetURL string, code int) error
	ConfigureRewrite(ctx context.Context, domain string, rules []RewriteRule) error

	// Service management
	Reload(ctx context.Context) error
	Restart(ctx context.Context) error
	TestConfig(ctx context.Context) error

	// Logs
	GetAccessLog(ctx context.Context, domain string, lines int) ([]string, error)
	GetErrorLog(ctx context.Context, domain string, lines int) ([]string, error)

	// Status
	IsInstalled(ctx context.Context) bool
	GetVersion(ctx context.Context) (string, error)
}

// SiteConfig represents a website configuration
type SiteConfig struct {
	Domain      string
	RootDir     string
	PHPVersion  string
	SSLEnabled  bool
	CertPath    string
	KeyPath     string
	ProxyTarget string
	LogDir      string
	Index       string
	Port        int
}

// RewriteRule represents a URL rewrite rule
type RewriteRule struct {
	Pattern     string
	Replacement string
	Flags       string
}

// Registry holds all registered web server adapters
type Registry struct {
	adapters map[string]WebServerAdapter
}

// NewRegistry creates a new adapter registry
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]WebServerAdapter),
	}
}

// Register adds a web server adapter to the registry
func (r *Registry) Register(adapter WebServerAdapter) {
	r.adapters[adapter.Name()] = adapter
}

// Get returns an adapter by name
func (r *Registry) Get(name string) (WebServerAdapter, bool) {
	adapter, ok := r.adapters[name]
	return adapter, ok
}

// List returns all registered adapter names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

// Domain names reach these adapters from HTTP request bodies and are then
// concatenated into vhost file paths and rendered into configuration files.
// Every adapter entry point validates the domain first: filepath.Join happily
// normalises "../../etc/cron.d/x" into a path outside the config directory, so
// the only safe answer is to reject anything that is not a plain host name.

var siteDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*\.[a-z]{2,63}$`)

// ValidateSiteDomain rejects any domain that could escape a config directory or
// break out of a configuration file line.
func ValidateSiteDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if len(domain) > 253 {
		return fmt.Errorf("domain is too long")
	}
	if strings.ContainsAny(domain, "/\\\x00\n\r") || strings.Contains(domain, "..") {
		return fmt.Errorf("invalid domain %q", domain)
	}
	if !siteDomainRe.MatchString(strings.ToLower(domain)) {
		return fmt.Errorf("invalid domain %q", domain)
	}
	return nil
}

// ValidateSiteConfig validates every caller-controlled field of a SiteConfig.
func ValidateSiteConfig(config *SiteConfig) error {
	if config == nil {
		return fmt.Errorf("site config is required")
	}
	if err := ValidateSiteDomain(config.Domain); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"root_dir":  config.RootDir,
		"cert_path": config.CertPath,
		"key_path":  config.KeyPath,
		"log_dir":   config.LogDir,
	} {
		if value == "" {
			continue
		}
		if strings.ContainsAny(value, "\x00\n\r") || strings.Contains(value, "..") {
			return fmt.Errorf("invalid %s", name)
		}
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	if config.PHPVersion != "" && !phpVersionRe.MatchString(config.PHPVersion) {
		return fmt.Errorf("invalid php version")
	}
	return nil
}

var phpVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

// SiteConfigPath builds a path under configDir and proves it did not escape it.
func SiteConfigPath(configDir, subdir, name string) (string, error) {
	if err := ValidateSiteDomain(strings.TrimSuffix(strings.TrimSuffix(name, ".conf"), "_ssl")); err != nil {
		return "", err
	}
	base := filepath.Clean(configDir)
	full := filepath.Clean(filepath.Join(base, subdir, name))
	if !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", fmt.Errorf("resolved path escapes the config directory")
	}
	return full, nil
}
