package webserver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TraefikAdapter implements WebServerAdapter for Traefik
type TraefikAdapter struct {
	ConfigPath string
	BinPath    string
	LogDir     string
}

func NewTraefikAdapter() *TraefikAdapter {
	return &TraefikAdapter{
		ConfigPath: "/etc/traefik",
		BinPath:    "/usr/bin/traefik",
		LogDir:     SiteLogRoot(),
	}
}

func (a *TraefikAdapter) Name() string {
	return "traefik"
}

func (a *TraefikAdapter) IsInstalled() bool {
	_, err := os.Stat(a.BinPath)
	return err == nil
}

func (a *TraefikAdapter) GetVersion() (string, error) {
	out, err := exec.Command(a.BinPath, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get Traefik version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

type traefikDynamicConfig struct {
	HTTP *traefikHTTP `json:"http,omitempty"`
	TLS  *traefikTLS  `json:"tls,omitempty"`
}

type traefikHTTP struct {
	Routers  map[string]*traefikRouter  `json:"routers,omitempty"`
	Services map[string]*traefikService `json:"services,omitempty"`
}

type traefikRouter struct {
	Rule        string            `json:"rule"`
	Service     string            `json:"service"`
	TLS         *traefikRouterTLS `json:"tls,omitempty"`
	EntryPoints []string          `json:"entryPoints,omitempty"`
}

type traefikRouterTLS struct {
	CertResolver string `json:"certResolver,omitempty"`
}

type traefikService struct {
	LoadBalancer *traefikLoadBalancer `json:"loadBalancer,omitempty"`
}

type traefikLoadBalancer struct {
	Servers []traefikServer `json:"servers"`
}

type traefikServer struct {
	URL string `json:"url"`
}

type traefikTLS struct {
	Certificates []traefikCertificate `json:"certificates,omitempty"`
}

type traefikCertificate struct {
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}

func (a *TraefikAdapter) CreateSite(config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	dynamicConfig := &traefikDynamicConfig{
		HTTP: &traefikHTTP{
			Routers: map[string]*traefikRouter{
				config.Domain: {
					Rule:        fmt.Sprintf("Host(`%s`)", config.Domain),
					Service:     config.Domain,
					EntryPoints: []string{"websecure"},
				},
			},
			Services: map[string]*traefikService{
				config.Domain: {
					LoadBalancer: &traefikLoadBalancer{
						Servers: []traefikServer{
							{URL: fmt.Sprintf("http://127.0.0.1:%d", config.Port)},
						},
					},
				},
			},
		},
	}

	if config.SSLEnabled {
		dynamicConfig.HTTP.Routers[config.Domain].TLS = &traefikRouterTLS{
			CertResolver: "letsencrypt",
		}
	}

	data, err := json.MarshalIndent(dynamicConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	sitePath := filepath.Join(a.ConfigPath, "dynamic", config.Domain+".json")
	if err := os.MkdirAll(filepath.Dir(sitePath), 0755); err != nil {
		return fmt.Errorf("failed to create dynamic directory: %w", err)
	}

	if err := os.WriteFile(sitePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write site config: %w", err)
	}

	return nil
}

func (a *TraefikAdapter) DeleteSite(domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	sitePath := filepath.Join(a.ConfigPath, "dynamic", domain+".json")
	return os.Remove(sitePath)
}

func (a *TraefikAdapter) EnableSite(domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil // Traefik doesn't have enable/disable
}

func (a *TraefikAdapter) DisableSite(domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *TraefikAdapter) Reload() error {
	cmd := exec.Command("systemctl", "reload", "traefik")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload Traefik: %s: %w", string(output), err)
	}
	return nil
}

func (a *TraefikAdapter) Restart() error {
	cmd := exec.Command("systemctl", "restart", "traefik")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart Traefik: %s: %w", string(output), err)
	}
	return nil
}

func (a *TraefikAdapter) TestConfig() error {
	cmd := exec.Command(a.BinPath, "healthcheck")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("config test failed: %s: %w", string(output), err)
	}
	return nil
}

func (a *TraefikAdapter) GetSiteConfig(domain string) (string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return "", err
	}
	sitePath := filepath.Join(a.ConfigPath, "dynamic", domain+".json")
	data, err := os.ReadFile(sitePath)
	if err != nil {
		return "", fmt.Errorf("failed to read site config: %w", err)
	}
	return string(data), nil
}

func (a *TraefikAdapter) UpdateSiteConfig(domain string, config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	return a.CreateSite(config)
}

func (a *TraefikAdapter) AddRewriteRule(domain string, rule *RewriteRule) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil // TODO: Implement middleware for Traefik
}

func (a *TraefikAdapter) RemoveRewriteRule(domain string, ruleID string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *TraefikAdapter) GetAccessLog(domain string) (string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return "", err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "access.log")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read access log: %w", err)
	}
	return string(data), nil
}

func (a *TraefikAdapter) GetErrorLog(domain string) (string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return "", err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "error.log")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read error log: %w", err)
	}
	return string(data), nil
}

func (a *TraefikAdapter) ClearAccessLog(domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "access.log")
	if err != nil {
		return err
	}
	return os.WriteFile(logPath, []byte(""), 0644)
}

func (a *TraefikAdapter) ClearErrorLog(domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "error.log")
	if err != nil {
		return err
	}
	return os.WriteFile(logPath, []byte(""), 0644)
}
