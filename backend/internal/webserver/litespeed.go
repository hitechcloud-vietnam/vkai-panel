package webserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// LiteSpeedAdapter implements WebServerAdapter for LiteSpeed Enterprise
type LiteSpeedAdapter struct {
	ConfigPath string
	BinPath    string
}

func NewLiteSpeedAdapter() *LiteSpeedAdapter {
	return &LiteSpeedAdapter{
		ConfigPath: "/usr/local/lsws/conf",
		BinPath:    "/usr/local/lsws/bin/lswsctrl",
	}
}

func (a *LiteSpeedAdapter) Name() string {
	return "litespeed"
}

func (a *LiteSpeedAdapter) IsInstalled() bool {
	_, err := os.Stat(a.BinPath)
	return err == nil
}

func (a *LiteSpeedAdapter) GetVersion() (string, error) {
	out, err := exec.Command(a.BinPath, "-v").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get LiteSpeed version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *LiteSpeedAdapter) CreateSite(config *SiteConfig) error {
	tmpl := `vhDomain {{.Domain}}
vhRoot /var/www/vhosts/{{.Domain}}
configFile $VH_CONF_DIR/{{.Domain}}/vhconf.conf
allowSymbolLink 1
enableScript 1
restrained 0
setUIDMode 2
`

	t, err := template.New("site").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, config); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	vhostPath := filepath.Join(a.ConfigPath, "vhosts", config.Domain)
	if err := os.MkdirAll(vhostPath, 0755); err != nil {
		return fmt.Errorf("failed to create vhost directory: %w", err)
	}

	confFile := filepath.Join(vhostPath, "vhconf.conf")
	if err := os.WriteFile(confFile, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("failed to write vhost config: %w", err)
	}

	return nil
}

func (a *LiteSpeedAdapter) DeleteSite(domain string) error {
	vhostPath := filepath.Join(a.ConfigPath, "vhosts", domain)
	return os.RemoveAll(vhostPath)
}

func (a *LiteSpeedAdapter) EnableSite(domain string) error {
	return nil // LiteSpeed doesn't have enable/disable like Apache
}

func (a *LiteSpeedAdapter) DisableSite(domain string) error {
	return nil
}

func (a *LiteSpeedAdapter) Reload() error {
	cmd := exec.Command(a.BinPath, "restart")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload LiteSpeed: %s: %w", string(output), err)
	}
	return nil
}

func (a *LiteSpeedAdapter) Restart() error {
	cmd := exec.Command(a.BinPath, "restart")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart LiteSpeed: %s: %w", string(output), err)
	}
	return nil
}

func (a *LiteSpeedAdapter) TestConfig() error {
	cmd := exec.Command(a.BinPath, "restart")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("config test failed: %s: %w", string(output), err)
	}
	return nil
}

func (a *LiteSpeedAdapter) GetSiteConfig(domain string) (string, error) {
	confFile := filepath.Join(a.ConfigPath, "vhosts", domain, "vhconf.conf")
	data, err := os.ReadFile(confFile)
	if err != nil {
		return "", fmt.Errorf("failed to read site config: %w", err)
	}
	return string(data), nil
}

func (a *LiteSpeedAdapter) UpdateSiteConfig(domain string, config *SiteConfig) error {
	return a.CreateSite(config)
}

func (a *LiteSpeedAdapter) AddRewriteRule(domain string, rule *RewriteRule) error {
	return nil // TODO: Implement rewrite rules for LiteSpeed
}

func (a *LiteSpeedAdapter) RemoveRewriteRule(domain string, ruleID string) error {
	return nil
}

func (a *LiteSpeedAdapter) GetAccessLog(domain string) (string, error) {
	logPath := filepath.Join("/var/log/lsws", domain, "access.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read access log: %w", err)
	}
	return string(data), nil
}

func (a *LiteSpeedAdapter) GetErrorLog(domain string) (string, error) {
	logPath := filepath.Join("/var/log/lsws", domain, "error.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("failed to read error log: %w", err)
	}
	return string(data), nil
}

func (a *LiteSpeedAdapter) ClearAccessLog(domain string) error {
	logPath := filepath.Join("/var/log/lsws", domain, "access.log")
	return os.WriteFile(logPath, []byte(""), 0644)
}

func (a *LiteSpeedAdapter) ClearErrorLog(domain string) error {
	logPath := filepath.Join("/var/log/lsws", domain, "error.log")
	return os.WriteFile(logPath, []byte(""), 0644)
}
