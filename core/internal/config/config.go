package config

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Agent    AgentConfig    `mapstructure:"agent"`
	Log      LogConfig      `mapstructure:"log"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Paths    PathsConfig    `mapstructure:"paths"`
	UI       UIConfig       `mapstructure:"ui"`
}

// UIConfig locates the Next.js process that renders the panel interface.
//
// The API serves the interface by forwarding to it (see internal/uiproxy), so
// that the security entrance guards the pages and the login form and not just
// /api. The upstream is always loopback: nginx publishes one port, and that
// port reaches this process, never Next.js directly.
//
// An empty upstream means no interface is attached, which is a valid API-only
// deployment; unknown paths then get the API's own 404.
type UIConfig struct {
	Upstream string `mapstructure:"upstream"`
}

// PathsConfig is the resolved filesystem layout. It exists so a handler can be
// handed the layout instead of reaching for a package-level helper, and so
// "vkai config show" can print what the process actually resolved.
type PathsConfig struct {
	PanelRoot   string `mapstructure:"panel_root"`
	WebRoot     string `mapstructure:"web_root"`
	BackupRoot  string `mapstructure:"backup_root"`
	LogRoot     string `mapstructure:"log_root"`
	SiteLogRoot string `mapstructure:"site_log_root"`
	EtcRoot     string `mapstructure:"etc_root"`
	SSLRoot     string `mapstructure:"ssl_root"`
	TmpRoot     string `mapstructure:"tmp_root"`
	DefaultSite string `mapstructure:"default_site"`
}

// CORSConfig holds the browser origins allowed to call the API. There is no
// wildcard: an empty list means no cross-origin browser access at all.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxOpen  int    `mapstructure:"max_open"`
	MaxIdle  int    `mapstructure:"max_idle"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret          string        `mapstructure:"secret"`
	AccessTokenTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTokenTTL time.Duration `mapstructure:"refresh_ttl"`
	Issuer          string        `mapstructure:"issuer"`
}

type AgentConfig struct {
	TokenHeader string `mapstructure:"token_header"`
	CertPath    string `mapstructure:"cert_path"`
	KeyPath     string `mapstructure:"key_path"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	// The installed location wins over the legacy one: a stale /etc/vkai left
	// behind by an older build must not shadow the file the operator edits.
	viper.AddConfigPath(EtcRoot())
	viper.AddConfigPath("/etc/vkai")

	// Defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 30110)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "vkai")
	// No default password: an unset database password must fail, not fall back
	// to a value that is published in this repository.
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "vkai_panel")
	viper.SetDefault("database.sslmode", "require")
	viper.SetDefault("database.max_open", 25)
	viper.SetDefault("database.max_idle", 5)

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	// No default JWT secret. A hard-coded fallback here is equivalent to
	// publishing an admin token generator, so Load() fails instead.
	viper.SetDefault("jwt.secret", "")
	viper.SetDefault("jwt.access_ttl", "15m")
	viper.SetDefault("jwt.refresh_ttl", "168h")
	viper.SetDefault("jwt.issuer", "vkai-panel")

	// Filesystem layout. These mirror the helpers in paths.go so that a config
	// file may pin a path, while VKAI_PANEL_ROOT and friends still move the
	// whole tree when nothing is pinned.
	viper.SetDefault("paths.panel_root", PanelRoot())
	viper.SetDefault("paths.web_root", WebRoot())
	viper.SetDefault("paths.backup_root", BackupRoot())
	viper.SetDefault("paths.log_root", LogRoot())
	viper.SetDefault("paths.site_log_root", SiteLogRoot())
	viper.SetDefault("paths.etc_root", EtcRoot())
	viper.SetDefault("paths.ssl_root", SSLRoot())
	viper.SetDefault("paths.tmp_root", TmpRoot())
	viper.SetDefault("paths.default_site", DefaultSite())

	viper.SetDefault("agent.token_header", "X-Agent-Token")

	// The Next.js service installed by deploy/install.sh, on loopback. It has
	// to have a default: an unset value must not silently take the interface
	// out of the panel's front door.
	viper.SetDefault("ui.upstream", "http://127.0.0.1:3000")

	viper.SetDefault("cors.allowed_origins", []string{})

	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.max_size", 100)
	viper.SetDefault("log.max_backups", 3)
	viper.SetDefault("log.max_age", 28)
	viper.SetDefault("log.compress", true)

	// Environment variables. The replacer is what makes VKAI_JWT_SECRET map onto
	// the nested key "jwt.secret"; without it no nested key could ever be set
	// from the environment.
	viper.SetEnvPrefix("VKAI")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Explicit bindings for the short names used in .env / deployment units.
	// Without these, VKAI_DB_PASSWORD would be silently ignored (AutomaticEnv
	// would only look for VKAI_DATABASE_PASSWORD) and the operator would end up
	// running on whatever the config file happened to contain.
	// Each entry lists the VKAI_ name first and the pre-whitelabel names after
	// it: viper takes the first variable that is actually set, so an existing
	// deployment keeps booting on its old .env while new installs use VKAI_.
	for key, names := range map[string][]string{
		"server.host":       {"VKAI_SERVER_HOST", "PANEL_SERVER_HOST", "SERVER_HOST"},
		"server.port":       {"VKAI_SERVER_PORT", "PANEL_SERVER_PORT", "SERVER_PORT"},
		"database.host":     {"VKAI_DATABASE_HOST", "VKAI_DB_HOST", "DB_HOST"},
		"database.port":     {"VKAI_DATABASE_PORT", "VKAI_DB_PORT", "DB_PORT"},
		"database.user":     {"VKAI_DATABASE_USER", "VKAI_DB_USER", "DB_USER"},
		"database.password": {"VKAI_DATABASE_PASSWORD", "VKAI_DB_PASSWORD", "DB_PASSWORD"},
		"database.dbname":   {"VKAI_DATABASE_DBNAME", "VKAI_DB_NAME", "DB_NAME"},
		"database.sslmode":  {"VKAI_DATABASE_SSLMODE", "VKAI_DB_SSLMODE", "DB_SSLMODE"},
		"redis.host":        {"VKAI_REDIS_HOST", "REDIS_HOST"},
		"redis.port":        {"VKAI_REDIS_PORT", "REDIS_PORT"},
		"redis.password":    {"VKAI_REDIS_PASSWORD", "REDIS_PASSWORD"},
		"redis.db":          {"VKAI_REDIS_DB", "REDIS_DB"},
		"jwt.secret":        {"VKAI_JWT_SECRET", "JWT_SECRET"},
		"jwt.issuer":        {"VKAI_JWT_ISSUER", "JWT_ISSUER"},
		"log.level":         {"VKAI_LOG_LEVEL", "LOG_LEVEL"},
		"ui.upstream":       {"VKAI_UI_UPSTREAM", "UI_UPSTREAM"},
		"paths.panel_root":  {EnvPanelRoot, "PANEL_ROOT"},
		"paths.web_root":    {EnvWebRoot, "WEB_ROOT"},
		"paths.backup_root": {EnvBackupRoot, "BACKUP_ROOT"},
		"paths.log_root":    {EnvLogRoot, "LOG_ROOT"},
		"paths.etc_root":    {EnvEtcRoot, "ETC_ROOT"},
		"paths.ssl_root":    {EnvSSLRoot, "SSL_ROOT"},
		"paths.tmp_root":    {EnvTmpRoot, "TMP_ROOT"},
	} {
		args := append([]string{key}, names...)
		_ = viper.BindEnv(args...)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		log.Println("No config file found, using defaults and environment variables")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if strings.TrimSpace(cfg.Paths.SiteLogRoot) == "" {
		cfg.Paths.SiteLogRoot = filepath.Join(cfg.Paths.LogRoot, "sites")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// weakSecretMarkers are substrings that appear in every placeholder secret
// shipped in this repository, its docs and its compose files.
var weakSecretMarkers = []string{
	"change", "changeme", "secret-key", "your-secret", "placeholder",
	"example", "todo", "password", "vkai_secret",
}

// Validate refuses to start with a configuration that would leave the panel
// trivially compromisable. Every check here fails the process at boot rather
// than degrading silently at runtime.
func (c *Config) Validate() error {
	secret := strings.TrimSpace(c.JWT.Secret)
	if secret == "" {
		return fmt.Errorf("jwt.secret is not set: set VKAI_JWT_SECRET to a random value of at least 32 characters")
	}
	if len([]byte(secret)) < 32 {
		return fmt.Errorf("jwt.secret must be at least 32 bytes long (got %d)", len([]byte(secret)))
	}
	lowered := strings.ToLower(secret)
	for _, marker := range weakSecretMarkers {
		if strings.Contains(lowered, marker) {
			return fmt.Errorf("jwt.secret still contains the placeholder text %q: generate a random secret", marker)
		}
	}

	if strings.TrimSpace(c.Database.Password) == "" {
		return fmt.Errorf("database.password is not set: set VKAI_DB_PASSWORD")
	}
	if strings.ToLower(strings.TrimSpace(c.Database.Password)) == "vkai_secret" {
		return fmt.Errorf("database.password is still the published default value")
	}

	switch c.Database.SSLMode {
	case "require", "verify-ca", "verify-full":
	case "disable", "allow", "prefer":
		// Only tolerated when the database is local; otherwise credentials and
		// customer data cross the network in the clear.
		if !isLocalHost(c.Database.Host) {
			return fmt.Errorf("database.sslmode=%q is not allowed for remote host %q", c.Database.SSLMode, c.Database.Host)
		}
	default:
		return fmt.Errorf("database.sslmode %q is not a valid value", c.Database.SSLMode)
	}

	return nil
}

func isLocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1", "":
		return true
	}
	return false
}
