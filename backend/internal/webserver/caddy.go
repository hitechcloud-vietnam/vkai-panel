package webserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// CaddyAdapter implements WebServerAdapter for Caddy
type CaddyAdapter struct {
	ConfigPath string
	BinPath    string
}

func NewCaddyAdapter() *CaddyAdapter {
	return &CaddyAdapter{
		ConfigPath: "/etc/caddy",
		BinPath:    "/usr/bin/caddy",
	}
}

func (a *CaddyAdapter) Name() string {
	return "caddy"
}

func (a *CaddyAdapter) IsInstalled() bool {
	_, err := os.Stat(a.BinPath)
	return err == nil
}

func (a *CaddyAdapter) GetVersion() (string, error) {
	out, err := exec.Command(a.BinPath, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get Caddy version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *CaddyAdapter) CreateSite(config *SiteConfig) error {
	tmpl := `{{.Domain}} {
    root * /var/www/{{.Domain}}
    encode gzip

    {{if .SSLEnabled}}
    tls {{.SSLCertPath}} {{.SSLKeyPath}}
    {{else}}
    tls internal
    {{end}}

    {{if .PHPEnabled}}
    php_fastcgi unix//run/php/php{{.PHPVersion}}-fpm.sock
    {{end}}

    file_server
}
`

	t, err := template.New("site").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, config); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	sitePath := filepath.Join(a.ConfigPath, "sites", config.Domain+".conf")
	if err := os.MkdirAll(filepath.Dir(sitePath), 0755); err != nil {
		return fmt.Errorf("failed to create sites directory: %w", err)
	}

	if err := os.WriteFile(sitePath, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("failed to write site config: %w", err)
	}

	return nil
}

func (a *CaddyAdapter) DeleteSite(domain string) error {
	sitePath := filepath.Join(a.ConfigPath, "sites", domain+".conf")
	return os.Remove(sitePath)
}

func (a *CaddyAdapter) EnableSite(domain string) error {
	return nil // Caddy doesn't have enable/disable
}

func (a *CaddyAdapter) DisableSite(domain string) error {
	return nil
}

func (a *CaddyAdapter) Reload() error {
	cmd := exec.Command("systemctl", "reload", "caddy")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload Caddy: %s: %w", string(output), err)
	}
	return nil
}

func (a *CaddyAdapter) Restart() error {
	cmd := exec.Command("systemctl", "restart", "caddy")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart Caddy: %s: %w", string(output), err)
	}
	return nil
}

func (a *CaddyAdapter) TestConfig() error {
	cmd := exec.Command(a.BinPath, "validate", "--config", filepath.Join(a.ConfigPath, "Caddyfile"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("config test failed: %s: %w", string(output), err)
	}
	return nil
}

func (a *CaddyAdapter) GetSiteConfig(domain string) (string, error) {
	sitePath := filepath.Join(a.ConfigPath, "sites", domain+".conf")
	data, err := os.ReadFile(sitePath)
	if err != nil {
		return "", fmt.Errorf("failed to read site config: %w", err)
	}
	return string(data), nil
}

func (a *CaddyAdapter) UpdateSiteConfig(domain string, config *SiteConfig) error {
	return a.CreateSite(config)
}

func (a *CaddyAdapter) AddRewriteRule(domain string, rule *RewriteRule) error {
	return nil // TODO: Implement rewrite rules for Caddy
}

func (a *CaddyAdapter) RemoveRewriteRule(domain string, ruleID string) error {
	return nil
}

func (a *CaddyAdapter) GetAccessLog(domain string) (string, error) {
	logPath := filepath.Join("/var/log/caddy", domain+"-access.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read access log: %w", err)
	}
	return string(data), nil
}

func (a *CaddyAdapter) GetErrorLog(domain string) (string, error) {
	logPath := filepath.Join("/var/log/caddy", domain+"-error.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read error log: %w", err)
	}
	return string(data), nil
}

func (a *CaddyAdapter) ClearAccessLog(domain string) error {
	logPath := filepath.Join("/var/log/caddy", domain+"-access.log")
	return os.WriteFile(logPath, []byte(""), 0644)
}

func (a *CaddyAdapter) ClearErrorLog(domain string) error {
	logPath := filepath.Join("/var/log/caddy", domain+"-error.log")
	return os.WriteFile(logPath, []byte(""), 0644)
}
