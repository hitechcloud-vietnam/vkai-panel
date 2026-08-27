package webserver

import "context"

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
