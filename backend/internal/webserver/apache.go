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

// ApacheAdapter implements WebServerAdapter for Apache
type ApacheAdapter struct {
	ConfigDir string
	LogDir    string
	Binary    string
}

func NewApacheAdapter() *ApacheAdapter {
	return &ApacheAdapter{
		ConfigDir: "/etc/apache2",
		LogDir:    "/var/log/apache2",
		Binary:    "apache2ctl",
	}
}

func (a *ApacheAdapter) Name() string {
	return "apache"
}

func (a *ApacheAdapter) CreateSite(ctx context.Context, config *SiteConfig) error {
	if err := a.CreateVirtualHost(ctx, config); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "a2ensite", config.Domain+".conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable site: %w", err)
	}

	return a.Reload(ctx)
}

func (a *ApacheAdapter) DeleteSite(ctx context.Context, domain string) error {
	cmd := exec.CommandContext(ctx, "a2dissite", domain+".conf")
	cmd.Run()

	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete site config: %w", err)
	}

	return a.Reload(ctx)
}

func (a *ApacheAdapter) EnableSite(ctx context.Context, domain string) error {
	cmd := exec.CommandContext(ctx, "a2ensite", domain+".conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable site: %w", err)
	}
	return a.Reload(ctx)
}

func (a *ApacheAdapter) DisableSite(ctx context.Context, domain string) error {
	cmd := exec.CommandContext(ctx, "a2dissite", domain+".conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disable site: %w", err)
	}
	return a.Reload(ctx)
}

func (a *ApacheAdapter) CreateVirtualHost(ctx context.Context, config *SiteConfig) error {
	tmpl := `<VirtualHost *:80>
    ServerName {{.Domain}}
    DocumentRoot {{.RootDir}}

    <Directory {{.RootDir}}>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/{{.Domain}}_error.log
    CustomLog ${APACHE_LOG_DIR}/{{.Domain}}_access.log combined

{{if .PHPVersion}}
    <FilesMatch \.php$>
        SetHandler "proxy:unix:/run/php/php{{.PHPVersion}}-fpm.sock|fcgi://localhost"
    </FilesMatch>
{{end}}

{{if .ProxyTarget}}
    ProxyPass / {{.ProxyTarget}}/
    ProxyPassReverse / {{.ProxyTarget}}/
{{end}}
</VirtualHost>
`

	if config.Index == "" {
		config.Index = "index.php index.html index.htm"
	}

	t, err := template.New("vhost").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	configPath := filepath.Join(a.ConfigDir, "sites-available", config.Domain+".conf")
	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	return t.Execute(f, config)
}

func (a *ApacheAdapter) ConfigureSSL(ctx context.Context, domain, certPath, keyPath string) error {
	sslConfig := fmt.Sprintf(`<VirtualHost *:443>
    ServerName %s
    DocumentRoot /var/www/%s

    SSLEngine on
    SSLCertificateFile %s
    SSLCertificateKeyFile %s
    SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1

    <Directory /var/www/%s>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/%s_ssl_error.log
    CustomLog ${APACHE_LOG_DIR}/%s_ssl_access.log combined
</VirtualHost>
`, domain, domain, certPath, keyPath, domain, domain, domain)

	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+"_ssl.conf")
	if err := os.WriteFile(configPath, []byte(sslConfig), 0644); err != nil {
		return fmt.Errorf("failed to write SSL config: %w", err)
	}

	cmd := exec.CommandContext(ctx, "a2enmod", "ssl")
	cmd.Run()

	cmd = exec.CommandContext(ctx, "a2ensite", domain+"_ssl.conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable SSL site: %w", err)
	}

	return a.Reload(ctx)
}

func (a *ApacheAdapter) ConfigurePHP(ctx context.Context, domain, phpVersion string) error {
	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	updated := strings.ReplaceAll(
		string(content),
		"php-fpm.sock",
		fmt.Sprintf("php%s-fpm.sock", phpVersion),
	)

	return os.WriteFile(configPath, []byte(updated), 0644)
}

func (a *ApacheAdapter) ConfigureProxy(ctx context.Context, domain, target string) error {
	return nil
}

func (a *ApacheAdapter) ConfigureHeaders(ctx context.Context, domain string, headers map[string]string) error {
	return nil
}

func (a *ApacheAdapter) ConfigureRedirect(ctx context.Context, domain, targetURL string, code int) error {
	redirectConfig := fmt.Sprintf(`<VirtualHost *:80>
    ServerName %s
    Redirect %d %s
</VirtualHost>
`, domain, code, targetURL)

	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	return os.WriteFile(configPath, []byte(redirectConfig), 0644)
}

func (a *ApacheAdapter) ConfigureRewrite(ctx context.Context, domain string, rules []RewriteRule) error {
	return nil
}

func (a *ApacheAdapter) Reload(ctx context.Context) error {
	if err := a.TestConfig(ctx); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "reload", "apache2")
	return cmd.Run()
}

func (a *ApacheAdapter) Restart(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "apache2")
	return cmd.Run()
}

func (a *ApacheAdapter) TestConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.Binary, "configtest")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apache config test failed: %s", string(output))
	}
	return nil
}

func (a *ApacheAdapter) GetAccessLog(ctx context.Context, domain string, lines int) ([]string, error) {
	logPath := filepath.Join(a.LogDir, domain+"_access.log")
	return tailFile(logPath, lines)
}

func (a *ApacheAdapter) GetErrorLog(ctx context.Context, domain string, lines int) ([]string, error) {
	logPath := filepath.Join(a.LogDir, domain+"_error.log")
	return tailFile(logPath, lines)
}

func (a *ApacheAdapter) IsInstalled(ctx context.Context) bool {
	_, err := exec.LookPath(a.Binary)
	return err == nil
}

func (a *ApacheAdapter) GetVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, a.Binary, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
