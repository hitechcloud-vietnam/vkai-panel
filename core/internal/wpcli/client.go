package wpcli

// The WordPress operations customers expect, each one built from typed
// arguments and run as the site's own user.
//
// Every method here returns parsed data, not a blob of text: WP-CLI is asked
// for --format=json and the result is unmarshalled, so a caller gets a typed
// list rather than something it has to scrape. A command whose JSON does not
// parse is an error, not an empty list - "no plugins" and "the command did not
// work" must never look the same to an operator.

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Client is the typed WP-CLI surface.
type Client struct {
	runner *Runner
	logger *zap.Logger
}

// NewClient builds a client over a runner.
func NewClient(runner *Runner, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{runner: runner, logger: logger}
}

// Runner exposes the underlying runner, so a caller can ask Available().
func (c *Client) Runner() *Runner { return c.runner }

// Site identifies one WordPress installation and the identity that owns it.
type Site struct {
	// Dir is the WordPress root, e.g. /vkai-panel/www/domains/example.com.
	Dir string
	// Identity is the non-root user every command for this site runs as.
	Identity Identity
	// URL is the site's canonical URL.
	URL string
}

// ---------------------------------------------------------------------------
// Plugins and themes
// ---------------------------------------------------------------------------

// Plugin is one row of `wp plugin list --format=json`.
type Plugin struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Update    string `json:"update"`
	Version   string `json:"version"`
	UpdateVer string `json:"update_version"`
	AutoUpd   string `json:"auto_update"`
}

// Theme is one row of `wp theme list --format=json`.
type Theme struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Update    string `json:"update"`
	Version   string `json:"version"`
	UpdateVer string `json:"update_version"`
}

// ListPlugins returns the plugins actually installed in the site directory.
// This is the live answer, not the panel's record of what it once installed -
// the two drift the moment a customer installs a plugin from wp-admin.
func (c *Client) ListPlugins(ctx context.Context, site Site) ([]Plugin, error) {
	result, err := c.run(ctx, site, 2*time.Minute,
		"plugin", "list", "--format=json",
		"--fields=name,status,update,version,update_version,auto_update")
	if err != nil {
		return nil, err
	}
	var plugins []Plugin
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &plugins); err != nil {
		return nil, fmt.Errorf("wp plugin list returned output that is not JSON: %w", err)
	}
	return plugins, nil
}

// ListThemes returns the themes actually installed in the site directory.
func (c *Client) ListThemes(ctx context.Context, site Site) ([]Theme, error) {
	result, err := c.run(ctx, site, 2*time.Minute,
		"theme", "list", "--format=json",
		"--fields=name,status,update,version,update_version")
	if err != nil {
		return nil, err
	}
	var themes []Theme
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &themes); err != nil {
		return nil, fmt.Errorf("wp theme list returned output that is not JSON: %w", err)
	}
	return themes, nil
}

// UpdatePlugins updates the named plugins, or every plugin when slugs is empty.
func (c *Client) UpdatePlugins(ctx context.Context, site Site, slugs []string) (*Result, error) {
	args := []string{"plugin", "update"}
	if len(slugs) == 0 {
		args = append(args, "--all")
	} else {
		for _, raw := range slugs {
			slug, err := Slug(raw)
			if err != nil {
				return nil, err
			}
			args = append(args, slug)
		}
	}
	args = append(args, "--format=json")
	return c.run(ctx, site, 15*time.Minute, args...)
}

// UpdateThemes updates the named themes, or every theme when slugs is empty.
func (c *Client) UpdateThemes(ctx context.Context, site Site, slugs []string) (*Result, error) {
	args := []string{"theme", "update"}
	if len(slugs) == 0 {
		args = append(args, "--all")
	} else {
		for _, raw := range slugs {
			slug, err := Slug(raw)
			if err != nil {
				return nil, err
			}
			args = append(args, slug)
		}
	}
	args = append(args, "--format=json")
	return c.run(ctx, site, 15*time.Minute, args...)
}

// InstallPlugin installs one plugin, optionally activating it.
func (c *Client) InstallPlugin(ctx context.Context, site Site, rawSlug, rawVersion string, activate bool) (*Result, error) {
	slug, err := Slug(rawSlug)
	if err != nil {
		return nil, err
	}
	args := []string{"plugin", "install", slug}
	if rawVersion != "" && rawVersion != "latest" {
		version, err := CoreVersion(rawVersion)
		if err != nil {
			return nil, fmt.Errorf("plugin version: %w", err)
		}
		args = append(args, "--version="+version)
	}
	if activate {
		args = append(args, "--activate")
	}
	return c.run(ctx, site, 10*time.Minute, args...)
}

// InstallTheme installs one theme, optionally activating it.
func (c *Client) InstallTheme(ctx context.Context, site Site, rawSlug, rawVersion string, activate bool) (*Result, error) {
	slug, err := Slug(rawSlug)
	if err != nil {
		return nil, err
	}
	args := []string{"theme", "install", slug}
	if rawVersion != "" && rawVersion != "latest" {
		version, err := CoreVersion(rawVersion)
		if err != nil {
			return nil, fmt.Errorf("theme version: %w", err)
		}
		args = append(args, "--version="+version)
	}
	if activate {
		args = append(args, "--activate")
	}
	return c.run(ctx, site, 10*time.Minute, args...)
}

// ---------------------------------------------------------------------------
// Core
// ---------------------------------------------------------------------------

// CoreVersionOf reports the installed WordPress version.
func (c *Client) CoreVersionOf(ctx context.Context, site Site) (string, error) {
	result, err := c.run(ctx, site, time.Minute, "core", "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// UpdateCore updates WordPress core, then runs the database update that a core
// update requires. Skipping the second step leaves a site showing the
// "database update required" screen to every visitor.
func (c *Client) UpdateCore(ctx context.Context, site Site, rawVersion string) (*Result, error) {
	args := []string{"core", "update"}
	if rawVersion != "" && rawVersion != "latest" {
		version, err := CoreVersion(rawVersion)
		if err != nil {
			return nil, err
		}
		args = append(args, "--version="+version)
	}
	result, err := c.run(ctx, site, 20*time.Minute, args...)
	if err != nil {
		return result, err
	}
	if _, err := c.run(ctx, site, 10*time.Minute, "core", "update-db"); err != nil {
		return result, fmt.Errorf("WordPress core updated but the database update failed, so the "+
			"site will show the update screen to visitors: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Migration
// ---------------------------------------------------------------------------

// SearchReplaceReport is the parsed result of a search-replace.
type SearchReplaceReport struct {
	// DryRun records whether anything was actually written.
	DryRun bool `json:"dry_run"`
	// Replacements is the total number of rows changed.
	Replacements int `json:"replacements"`
	// Tables is the per-table breakdown as WP-CLI reported it.
	Tables []SearchReplaceTable `json:"tables"`
}

// SearchReplaceTable is one row of `wp search-replace --format=json`.
type SearchReplaceTable struct {
	Table        string `json:"Table"`
	Column       string `json:"Column"`
	Replacements any    `json:"Replacements"`
	Type         string `json:"Type"`
}

// SearchReplace rewrites URLs across the database.
//
// --precise and --all-tables-with-prefix are not optional extras. A WordPress
// database stores serialised PHP arrays whose element lengths are encoded in
// the string; a plain SQL REPLACE of a longer URL for a shorter one corrupts
// every one of them, and the symptom is a theme's options silently reverting.
// --precise makes WP-CLI unserialise, replace and reserialise instead.
//
// dryRun is the caller's to choose and the panel's default: this is the one
// operation in the toolkit that rewrites every row of a customer's database.
func (c *Client) SearchReplace(ctx context.Context, site Site, rawFrom, rawTo string, dryRun bool) (*SearchReplaceReport, error) {
	from, err := FreeText("search-replace from", rawFrom, 2000)
	if err != nil {
		return nil, err
	}
	to, err := FreeText("search-replace to", rawTo, 2000)
	if err != nil {
		return nil, err
	}
	if from == to {
		return nil, fmt.Errorf("search-replace from and to are identical (%q); nothing would change", from)
	}

	args := []string{"search-replace", from, to,
		"--precise", "--recurse-objects", "--all-tables-with-prefix",
		"--skip-columns=guid", "--format=json"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	result, err := c.run(ctx, site, 30*time.Minute, args...)
	if err != nil {
		return nil, err
	}

	report := &SearchReplaceReport{DryRun: dryRun}
	trimmed := strings.TrimSpace(result.Stdout)
	if trimmed != "" && strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &report.Tables); err != nil {
			return nil, fmt.Errorf("wp search-replace returned output that is not JSON: %w", err)
		}
	}
	for _, table := range report.Tables {
		switch value := table.Replacements.(type) {
		case float64:
			report.Replacements += int(value)
		case string:
			var n int
			fmt.Sscanf(value, "%d", &n)
			report.Replacements += n
		}
	}
	return report, nil
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// ResetUserPassword sets a new password for one WordPress user.
//
// The password is generated here when the caller does not supply one, and it is
// passed with `wp user update --user_pass=` as a single argv element. It is
// never interpolated into a shell string, so it may contain any character; and
// because there is no shell, it never appears in a shell history either. It
// does appear in this process's argv for the lifetime of the command, which is
// why the generated one is single-use and returned to the operator rather than
// stored.
func (c *Client) ResetUserPassword(ctx context.Context, site Site, rawLogin, newPassword string) (string, error) {
	login, err := Login(rawLogin)
	if err != nil {
		return "", err
	}
	password := newPassword
	if password == "" {
		if password, err = GeneratePassword(24); err != nil {
			return "", err
		}
	} else if strings.ContainsRune(password, 0) {
		return "", fmt.Errorf("the new password contains a NUL byte")
	} else if len(password) < 8 {
		return "", fmt.Errorf("the new password is shorter than 8 characters")
	} else if len(password) > 200 {
		return "", fmt.Errorf("the new password is longer than 200 characters")
	}

	if _, err := c.run(ctx, site, 2*time.Minute,
		"user", "update", login, "--user_pass="+password, "--skip-email"); err != nil {
		return "", err
	}
	c.logger.Info("WordPress user password reset",
		zap.String("site", site.Dir),
		zap.String("login", login),
		zap.String("ran_as", site.Identity.String()))
	return password, nil
}

// ---------------------------------------------------------------------------
// Installation
// ---------------------------------------------------------------------------

// InstallRequest is everything needed to turn an empty directory into a working
// WordPress site.
type InstallRequest struct {
	Site Site

	DBName     string
	DBUser     string
	DBPassword string
	DBHost     string
	DBPrefix   string

	AdminUser     string
	AdminPassword string
	AdminEmail    string
	SiteTitle     string

	CoreVersion string
	Locale      string
}

// Install performs a real WordPress installation: download core, write
// wp-config.php with freshly generated salts, create the tables, then set
// ownership and modes.
//
// Every step runs as the site user. The panel is root, so it *could* untar into
// the directory itself and chown afterwards - and that is exactly the shortcut
// that leaves a file owned by root inside a customer's tree, which the customer
// then cannot update from wp-admin. Downloading as the site user means every
// file is created with the right owner from the start.
func (c *Client) Install(ctx context.Context, req InstallRequest) error {
	site := req.Site
	dbName, err := Identifier("database name", req.DBName)
	if err != nil {
		return err
	}
	dbUser, err := Identifier("database user", req.DBUser)
	if err != nil {
		return err
	}
	dbHost := req.DBHost
	if dbHost == "" {
		dbHost = "localhost"
	}
	if err := validateDBHost(dbHost); err != nil {
		return err
	}
	prefix, err := TablePrefix(defaultString(req.DBPrefix, "wp_"))
	if err != nil {
		return err
	}
	adminUser, err := Login(req.AdminUser)
	if err != nil {
		return err
	}
	if _, err := FreeText("admin email", req.AdminEmail, 254); err != nil {
		return err
	}
	title, err := FreeText("site title", defaultString(req.SiteTitle, "WordPress"), 200)
	if err != nil {
		return err
	}
	siteURL, err := SiteURL(site.URL)
	if err != nil {
		return err
	}
	coreVersion, err := CoreVersion(req.CoreVersion)
	if err != nil {
		return err
	}
	if req.AdminPassword == "" {
		return fmt.Errorf("an administrator password is required")
	}
	if strings.ContainsRune(req.DBPassword, 0) || strings.ContainsRune(req.AdminPassword, 0) {
		return fmt.Errorf("a password contains a NUL byte")
	}
	locale := defaultString(req.Locale, "en_US")
	if !localeRe.MatchString(locale) {
		return fmt.Errorf("invalid locale %q", locale)
	}

	// 1. Core files, downloaded as the site user.
	downloadArgs := []string{"core", "download", "--locale=" + locale, "--force"}
	if coreVersion != "latest" {
		downloadArgs = append(downloadArgs, "--version="+coreVersion)
	}
	if _, err := c.run(ctx, site, 20*time.Minute, downloadArgs...); err != nil {
		return fmt.Errorf("downloading WordPress: %w", err)
	}

	// 2. wp-config.php. --skip-check because the database may not accept
	// connections until the grants land; --force because a retried install
	// must overwrite a half-written config rather than stop.
	//
	// The salts: `wp config create` fetches them from the WordPress.org secret
	// key service, which means a host with no outbound access silently gets
	// the placeholder values shipped in wp-config-sample.php - a site whose
	// auth cookies can be forged by anyone who knows those constants. So the
	// salts are generated locally with crypto/rand and written afterwards,
	// and the fetch is skipped entirely.
	configArgs := []string{
		"config", "create",
		"--dbname=" + dbName,
		"--dbuser=" + dbUser,
		"--dbpass=" + req.DBPassword,
		"--dbhost=" + dbHost,
		"--dbprefix=" + prefix,
		"--locale=" + locale,
		"--skip-check",
		"--skip-salts",
		"--force",
	}
	if _, err := c.run(ctx, site, 5*time.Minute, configArgs...); err != nil {
		return fmt.Errorf("writing wp-config.php: %w", err)
	}
	if err := c.writeSalts(ctx, site); err != nil {
		return err
	}

	// 3. Create the tables and the first administrator.
	installArgs := []string{
		"core", "install",
		"--url=" + siteURL,
		"--title=" + title,
		"--admin_user=" + adminUser,
		"--admin_password=" + req.AdminPassword,
		"--admin_email=" + req.AdminEmail,
		"--skip-email",
	}
	if _, err := c.run(ctx, site, 10*time.Minute, installArgs...); err != nil {
		return fmt.Errorf("running the WordPress installer: %w", err)
	}

	c.logger.Info("WordPress installed",
		zap.String("dir", site.Dir),
		zap.String("url", siteURL),
		zap.String("version", coreVersion),
		zap.String("ran_as", site.Identity.String()))
	return nil
}

// writeSalts replaces the eight authentication constants with values from
// crypto/rand. Each is 64 characters from a 90-character alphabet, which is
// what the WordPress.org service returns.
func (c *Client) writeSalts(ctx context.Context, site Site) error {
	constants := []string{
		"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY",
		"AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT",
	}
	for _, name := range constants {
		salt, err := GenerateSalt(64)
		if err != nil {
			return fmt.Errorf("generating %s: %w", name, err)
		}
		if _, err := c.run(ctx, site, time.Minute,
			"config", "set", name, salt, "--type=constant"); err != nil {
			return fmt.Errorf("writing %s into wp-config.php: %w", name, err)
		}
	}
	return nil
}

// IsInstalled reports whether the directory holds a WordPress that answers.
func (c *Client) IsInstalled(ctx context.Context, site Site) bool {
	_, err := c.run(ctx, site, time.Minute, "core", "is-installed")
	return err == nil
}

// ---------------------------------------------------------------------------
// Random material
// ---------------------------------------------------------------------------

const saltAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
	"!@#%^&*()-_=+[]{}<>~"

const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789" +
	"!@#%^&*()-_=+"

// GenerateSalt returns n characters of cryptographically random material for a
// wp-config.php constant.
func GenerateSalt(n int) (string, error) { return randomString(n, saltAlphabet) }

// GeneratePassword returns n characters of cryptographically random material
// for a WordPress password. Its alphabet omits the characters an operator
// misreads when copying by hand (l, I, 1, O, 0).
func GeneratePassword(n int) (string, error) { return randomString(n, passwordAlphabet) }

// randomString draws from crypto/rand with rejection-free modulo-safe indexing
// via rand.Int, which is uniform over the alphabet.
func randomString(n int, alphabet string) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("length must be positive")
	}
	out := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("no randomness available: %w", err)
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

var localeRe = regexp.MustCompile(`^[a-z]{2}(_[A-Z]{2})?$`)

func validateDBHost(host string) error {
	if strings.ContainsAny(host, "`$&|;<>()[]{}!*?'\"\\ \t\n\r\x00") {
		return fmt.Errorf("invalid database host %q", host)
	}
	if strings.HasPrefix(host, "-") {
		return fmt.Errorf("invalid database host %q: it may not start with a dash", host)
	}
	if len(host) > 255 {
		return fmt.Errorf("database host is too long")
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// run is the single funnel every operation above goes through.
func (c *Client) run(ctx context.Context, site Site, timeout time.Duration, args ...string) (*Result, error) {
	result, err := c.runner.Run(ctx, Command{
		Identity: site.Identity,
		Dir:      site.Dir,
		Args:     args,
		Timeout:  timeout,
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

// EnsureDir creates the site directory owned by the site user, so that the
// first WP-CLI command has somewhere to write. It is the one operation in this
// package that runs as the panel (root), because creating a directory under
// /vkai-panel/www and giving it away is precisely a root operation.
func EnsureDir(dir string, identity Identity) error {
	cleaned, err := Path("wordpress directory", dir)
	if err != nil {
		return err
	}
	if identity.UID == 0 || identity.GID == 0 {
		return &ErrWouldRunAsRoot{Requested: identity.Name, UID: identity.UID, GID: identity.GID}
	}
	if err := os.MkdirAll(cleaned, 0o750); err != nil {
		return fmt.Errorf("cannot create %s: %w", cleaned, err)
	}
	if err := os.Chown(cleaned, int(identity.UID), int(identity.GID)); err != nil {
		return fmt.Errorf("cannot give %s to %s: %w", cleaned, identity.Name, err)
	}
	cacheDir := filepath.Join(cleaned, ".wp-cli", "cache")
	if err := os.MkdirAll(cacheDir, 0o750); err == nil {
		_ = os.Chown(filepath.Join(cleaned, ".wp-cli"), int(identity.UID), int(identity.GID))
		_ = os.Chown(cacheDir, int(identity.UID), int(identity.GID))
	}
	return nil
}
