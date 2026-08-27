package webserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// OpenLiteSpeedAdapter implements WebServerAdapter for OpenLiteSpeed
type OpenLiteSpeedAdapter struct {
	ConfigDir string
	LogDir    string
	Binary    string
}

func NewOpenLiteSpeedAdapter() *OpenLiteSpeedAdapter {
	return &OpenLiteSpeedAdapter{
		ConfigDir: "/usr/local/lsws/conf",
		LogDir:    SiteLogRoot(),
		Binary:    "/usr/local/lsws/bin/lswsctrl",
	}
}

func (a *OpenLiteSpeedAdapter) Name() string {
	return "openlitespeed"
}

func (a *OpenLiteSpeedAdapter) CreateSite(ctx context.Context, config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	if err := a.CreateVirtualHost(ctx, config); err != nil {
		return err
	}
	return a.Reload(ctx)
}

func (a *OpenLiteSpeedAdapter) DeleteSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	vhostDir := filepath.Join(a.ConfigDir, "vhosts", domain)
	if err := os.RemoveAll(vhostDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete vhost: %w", err)
	}
	return a.Reload(ctx)
}

func (a *OpenLiteSpeedAdapter) EnableSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return a.Reload(ctx)
}

func (a *OpenLiteSpeedAdapter) DisableSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return a.Reload(ctx)
}

func (a *OpenLiteSpeedAdapter) CreateVirtualHost(ctx context.Context, config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	vhostDir := filepath.Join(a.ConfigDir, "vhosts", config.Domain)
	if err := os.MkdirAll(vhostDir, 0755); err != nil {
		return fmt.Errorf("failed to create vhost dir: %w", err)
	}

	tmpl := `docRoot                   $VH_ROOT/html/
vhDomain                  {{.Domain}}
vhAliases                 *
adminEmails               admin@{{.Domain}}
enableGzip                1

errorlog $VH_ROOT/logs/error.log {
  useServer               0
  logLevel                WARN
  rollingSize             10M
}

accesslog $VH_ROOT/logs/access.log {
  useServer               0
  logFormat               "%h %l %u %t \"%r\" %>s %b"
  rollingSize             10M
}

index  {
  useServer               0
  indexFiles              index.php, index.html
}

context / {
  type                    NULL
  location                $VH_ROOT/html/
  allowBrowse             1

  rewrite  {
    enable                1
    rules                 <<<END_RULES
rewritecond %{HTTP_HOST} ^{{.Domain}} [NC]
rewriterule ^(.*)$ https://%{HTTP_HOST}$1 [R=301,L]
END_RULES
  }
}

extprocessor php{{.PHPVersion}} {
  type                    lsapi
  address                 UDS://tmp/lshttpd/php{{.PHPVersion}}.sock
  maxConns                35
  env                     LSAPI_CHILDREN=35
  initTimeout             60
  retryTimeout            0
  persistConn             1
  pcKeepAliveTimeout      1
  respBuffer              0
  autoStart               1
  path                    /usr/local/lsws/lsphp{{.PHPVersion}}/bin/lsphp
  backlog                 100
  instances               1
  priority                0
  memSoftLimit            2047M
  runOnStartUp            1
}

scripthandler {
  add                     lsapi:php{{.PHPVersion}} php
}
`

	if config.PHPVersion == "" {
		config.PHPVersion = "84"
	}

	t, err := template.New("vhost").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	configPath := filepath.Join(vhostDir, "vhost.conf")
	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create vhost config: %w", err)
	}
	defer f.Close()

	// Create directories
	os.MkdirAll(filepath.Join(vhostDir, "html"), 0755)
	os.MkdirAll(filepath.Join(vhostDir, "logs"), 0755)

	return t.Execute(f, config)
}

func (a *OpenLiteSpeedAdapter) ConfigureSSL(ctx context.Context, domain, certPath, keyPath string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	sslConfig := fmt.Sprintf(`listener DefaultSSL {
  address                 *:443
  secure                  1
  keyFile                 %s
  certFile                %s
  certChain               1
  sslProtocol             24
  ciphers                 EECDH+AESGCM:EDH+AESGCM:AES256+EECDH:AES256+EDH
  enableECDHE             1
  renegProtection         1
  sslSessionCache         1
  enableSpdy              15
  map                     %s %s
}
`, keyPath, certPath, domain, domain)

	configPath := filepath.Join(a.ConfigDir, "listeners", domain+"_ssl.conf")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	if err := os.WriteFile(configPath, []byte(sslConfig), 0644); err != nil {
		return fmt.Errorf("failed to write SSL config: %w", err)
	}

	return a.Reload(ctx)
}

func (a *OpenLiteSpeedAdapter) ConfigurePHP(ctx context.Context, domain, phpVersion string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) ConfigureProxy(ctx context.Context, domain, target string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) ConfigureHeaders(ctx context.Context, domain string, headers map[string]string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) ConfigureRedirect(ctx context.Context, domain, targetURL string, code int) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) ConfigureRewrite(ctx context.Context, domain string, rules []RewriteRule) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) Reload(ctx context.Context) error {
	if err := a.TestConfig(ctx); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, a.Binary, "reload")
	return cmd.Run()
}

func (a *OpenLiteSpeedAdapter) Restart(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.Binary, "restart")
	return cmd.Run()
}

func (a *OpenLiteSpeedAdapter) TestConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.Binary, "configtest")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openlitespeed config test failed: %s", string(output))
	}
	return nil
}

func (a *OpenLiteSpeedAdapter) GetAccessLog(ctx context.Context, domain string, lines int) ([]string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return nil, err
	}
	logPath := filepath.Join(a.ConfigDir, "vhosts", domain, "logs", "access.log")
	return tailFile(logPath, lines)
}

func (a *OpenLiteSpeedAdapter) GetErrorLog(ctx context.Context, domain string, lines int) ([]string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return nil, err
	}
	logPath := filepath.Join(a.ConfigDir, "vhosts", domain, "logs", "error.log")
	return tailFile(logPath, lines)
}

func (a *OpenLiteSpeedAdapter) IsInstalled(ctx context.Context) bool {
	_, err := os.Stat(a.Binary)
	return err == nil
}

func (a *OpenLiteSpeedAdapter) GetVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, a.Binary, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
