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

// NginxAdapter implements WebServerAdapter for Nginx
type NginxAdapter struct {
	ConfigDir string
	LogDir    string
	Binary    string
}

func NewNginxAdapter() *NginxAdapter {
	return &NginxAdapter{
		ConfigDir: "/etc/nginx",
		LogDir:    SiteLogRoot(),
		Binary:    "nginx",
	}
}

func (a *NginxAdapter) Name() string {
	return "nginx"
}

func (a *NginxAdapter) CreateSite(ctx context.Context, config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	if err := a.CreateVirtualHost(ctx, config); err != nil {
		return err
	}

	// Create symlink to sites-enabled
	sitesEnabled := filepath.Join(a.ConfigDir, "sites-enabled", config.Domain+".conf")
	siteAvailable := filepath.Join(a.ConfigDir, "sites-available", config.Domain+".conf")

	if err := os.Symlink(siteAvailable, sitesEnabled); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to enable site: %w", err)
	}

	return a.Reload(ctx)
}

func (a *NginxAdapter) DeleteSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	// Remove from sites-enabled
	sitesEnabled := filepath.Join(a.ConfigDir, "sites-enabled", domain+".conf")
	os.Remove(sitesEnabled)

	// Remove from sites-available
	siteAvailable := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	if err := os.Remove(siteAvailable); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete site config: %w", err)
	}

	return a.Reload(ctx)
}

func (a *NginxAdapter) EnableSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	siteAvailable := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	sitesEnabled := filepath.Join(a.ConfigDir, "sites-enabled", domain+".conf")

	if _, err := os.Stat(siteAvailable); os.IsNotExist(err) {
		return fmt.Errorf("site config not found: %s", domain)
	}

	if err := os.Symlink(siteAvailable, sitesEnabled); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to enable site: %w", err)
	}

	return a.Reload(ctx)
}

func (a *NginxAdapter) DisableSite(ctx context.Context, domain string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	sitesEnabled := filepath.Join(a.ConfigDir, "sites-enabled", domain+".conf")
	if err := os.Remove(sitesEnabled); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to disable site: %w", err)
	}
	return a.Reload(ctx)
}

func (a *NginxAdapter) CreateVirtualHost(ctx context.Context, config *SiteConfig) error {
	if err := ValidateSiteConfig(config); err != nil {
		return err
	}
	tmpl := `server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};

    root {{.RootDir}};
    index {{.Index}};

    access_log {{.LogDir}}/access.log;
    error_log {{.LogDir}}/error.log;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

{{if .PHPVersion}}
    location ~ \.php$ {
        fastcgi_pass unix:/run/php/php{{.PHPVersion}}-fpm.sock;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
{{end}}

{{if .ProxyTarget}}
    location / {
        proxy_pass {{.ProxyTarget}};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
{{end}}

    location ~ /\.ht {
        deny all;
    }
}
`

	if config.Index == "" {
		config.Index = "index.php index.html index.htm"
	}
	// nginx creates log files but not their directory: without this the vhost
	// is syntactically valid and the service still refuses to start.
	logDir, err := siteLogDirFor(a.LogDir, config.Domain)
	if err != nil {
		return err
	}
	if config.LogDir == "" {
		config.LogDir = logDir
	}
	if err := os.MkdirAll(config.LogDir, 0o750); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	t, err := template.New("vhost").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	siteAvailable := filepath.Join(a.ConfigDir, "sites-available", config.Domain+".conf")
	f, err := os.Create(siteAvailable)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	return t.Execute(f, config)
}

func (a *NginxAdapter) ConfigureSSL(ctx context.Context, domain, certPath, keyPath string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	siteRoot, err := SiteRoot(domain)
	if err != nil {
		return err
	}
	logDir, err := siteLogDirFor(a.LogDir, domain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	sslConfig := fmt.Sprintf(`
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name %s;

    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    root %s;
    index index.php index.html index.htm;

    access_log %s/ssl_access.log;
    error_log %s/ssl_error.log;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        fastcgi_pass unix:/run/php/php-fpm.sock;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
}

server {
    listen 80;
    listen [::]:80;
    server_name %s;
    return 301 https://$host$request_uri;
}
`, domain, certPath, keyPath, siteRoot, logDir, logDir, domain)

	sslConfPath := filepath.Join(a.ConfigDir, "sites-available", domain+"_ssl.conf")
	if err := os.WriteFile(sslConfPath, []byte(sslConfig), 0644); err != nil {
		return fmt.Errorf("failed to write SSL config: %w", err)
	}

	// Enable SSL config
	sitesEnabled := filepath.Join(a.ConfigDir, "sites-enabled", domain+"_ssl.conf")
	os.Symlink(sslConfPath, sitesEnabled)

	return a.Reload(ctx)
}

func (a *NginxAdapter) ConfigurePHP(ctx context.Context, domain, phpVersion string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	// Update PHP-FPM socket in site config
	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Replace PHP version in fastcgi_pass
	updated := strings.ReplaceAll(
		string(content),
		"php-fpm.sock",
		fmt.Sprintf("php%s-fpm.sock", phpVersion),
	)

	return os.WriteFile(configPath, []byte(updated), 0644)
}

func (a *NginxAdapter) ConfigureProxy(ctx context.Context, domain, target string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	// Implementation for reverse proxy configuration
	return nil
}

func (a *NginxAdapter) ConfigureHeaders(ctx context.Context, domain string, headers map[string]string) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	// Implementation for custom headers
	return nil
}

func (a *NginxAdapter) ConfigureRedirect(ctx context.Context, domain, targetURL string, code int) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	redirectConfig := fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %s;
    return %d %s;
}
`, domain, code, targetURL)

	configPath := filepath.Join(a.ConfigDir, "sites-available", domain+".conf")
	return os.WriteFile(configPath, []byte(redirectConfig), 0644)
}

func (a *NginxAdapter) ConfigureRewrite(ctx context.Context, domain string, rules []RewriteRule) error {
	if err := ValidateSiteDomain(domain); err != nil {
		return err
	}
	// Implementation for rewrite rules
	return nil
}

func (a *NginxAdapter) Reload(ctx context.Context) error {
	if err := a.TestConfig(ctx); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "reload", "nginx")
	return cmd.Run()
}

func (a *NginxAdapter) Restart(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "nginx")
	return cmd.Run()
}

func (a *NginxAdapter) TestConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.Binary, "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(output))
	}
	return nil
}

func (a *NginxAdapter) GetAccessLog(ctx context.Context, domain string, lines int) ([]string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return nil, err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "access.log")
	if err != nil {
		return nil, err
	}
	return tailFile(logPath, lines)
}

func (a *NginxAdapter) GetErrorLog(ctx context.Context, domain string, lines int) ([]string, error) {
	if err := ValidateSiteDomain(domain); err != nil {
		return nil, err
	}
	logPath, err := siteLogFile(a.LogDir, domain, "error.log")
	if err != nil {
		return nil, err
	}
	return tailFile(logPath, lines)
}

func (a *NginxAdapter) IsInstalled(ctx context.Context) bool {
	_, err := exec.LookPath(a.Binary)
	return err == nil
}

func (a *NginxAdapter) GetVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, a.Binary, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func tailFile(path string, lines int) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	allLines := strings.Split(string(content), "\n")
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}

	return allLines, nil
}
