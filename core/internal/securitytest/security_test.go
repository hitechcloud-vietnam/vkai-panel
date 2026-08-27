// Package securitytest holds regression tests for the security properties the
// panel depends on: the file manager jail, token type separation and
// revocation, permission enforcement, and the input validators that keep
// caller-controlled text out of shells, systemd units and SQL.
package securitytest

import (
	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustSiteRoot is the document root the panel would create for a domain. Tests
// build their paths from it so that moving the layout cannot leave a test
// asserting on a directory the panel no longer uses.
func mustSiteRoot(domain string) string {
	root, err := config.SiteRoot(domain)
	if err != nil {
		panic(err)
	}
	return root
}

func TestConfigFailFast(t *testing.T) {
	// config.Load() searches the working directory for config.yaml, so the test
	// runs from an empty directory to exercise the defaults-only path.
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil {
		t.Fatal("expected failure with no secret")
	} else {
		t.Log("no-secret error:", err)
	}

	os.Setenv("VKAI_JWT_SECRET", "change-me-in-production-aaaaaaaaaaaaaaaa")
	os.Setenv("VKAI_DB_PASSWORD", "hunter2hunter2")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected failure with placeholder secret")
	} else {
		t.Log("placeholder error:", err)
	}

	os.Setenv("VKAI_JWT_SECRET", "9f3a7c1e5b2d8046af91c3e7d5b208146a9e3f7c1b5d8042ae96c3f7d1b58204")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal("expected success, got:", err)
	}
	t.Log("ok, sslmode:", cfg.Database.SSLMode, "secret set:", cfg.JWT.Secret != "")
}

func TestFileManagerJail(t *testing.T) {
	fm := service.NewFileManager("/")
	t.Log("base:", fm.BasePath())
	for _, bad := range []string{"/etc/shadow", "../../etc/passwd", "/root/.ssh/id_rsa", config.ConfigFile()} {
		if p, err := fm.ResolvePath(bad); err == nil {
			t.Logf("RESOLVED %q -> %q (must stay in jail)", bad, p)
			if p == bad {
				t.Errorf("escaped jail: %q", bad)
			}
		} else {
			t.Logf("rejected %q: %v", bad, err)
		}
	}
	inJail := filepath.Join(config.WebRoot(), "site", "index.html")
	if p, err := fm.ResolvePath(inJail); err != nil || p != inJail {
		t.Errorf("in-jail absolute path broke: %q %v", p, err)
	}
	if p, err := fm.ResolvePath("site/index.html"); err != nil || p != inJail {
		t.Errorf("relative path broke: %q %v", p, err)
	}
}

func TestTokenTypeSeparation(t *testing.T) {
	m := auth.NewJWTManager("a-very-long-random-secret-value-0123456789", 15*time.Minute, 168*time.Hour, "vkai-panel")
	uid, tid := uuid.New(), uuid.New()

	pair, err := m.GenerateTokenPairWithPermissions(uid, tid, "u", "e@x.io", []string{"viewer"}, []string{"website.read"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.ValidateAccessToken(pair.RefreshToken); err == nil {
		t.Error("refresh token was accepted as an access token")
	} else {
		t.Log("refresh-as-access rejected:", err)
	}
	if _, err := m.ValidateRefreshToken(pair.AccessToken); err == nil {
		t.Error("access token was accepted as a refresh token")
	} else {
		t.Log("access-as-refresh rejected:", err)
	}

	claims, err := m.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	m.Revoke(claims)
	if _, err := m.ValidateAccessToken(pair.AccessToken); err == nil {
		t.Error("revoked token still valid")
	} else {
		t.Log("revoked token rejected:", err)
	}

	// Cross-secret forgery
	other := auth.NewJWTManager("another-very-long-random-secret-0123456789", time.Minute, time.Hour, "vkai-panel")
	if _, err := m.ValidateAccessToken(mustAccess(t, other, uid, tid)); err == nil {
		t.Error("token signed with a different secret was accepted")
	}
}

func mustAccess(t *testing.T, m *auth.JWTManager, uid, tid uuid.UUID) string {
	p, err := m.GenerateTokenPair(uid, tid, "u", "e", nil)
	if err != nil {
		t.Fatal(err)
	}
	return p.AccessToken
}

func TestPermissionChecks(t *testing.T) {
	viewer := &auth.TokenClaims{RoleIDs: []string{"viewer"}, Permissions: []string{"website.read"}}
	admin := &auth.TokenClaims{RoleIDs: []string{"super_admin"}}

	cases := []struct {
		claims   *auth.TokenClaims
		res, act string
		want     bool
	}{
		{viewer, "website", "read", true},
		{viewer, "website", "write", false},
		{viewer, "terminal", "execute", false},
		{viewer, "user", "write", false},
		{admin, "terminal", "execute", true},
		{nil, "website", "read", false},
	}
	for _, c := range cases {
		if got := middleware.HasPermission(c.claims, c.res, c.act); got != c.want {
			t.Errorf("HasPermission(%s.%s)=%v want %v", c.res, c.act, got, c.want)
		}
	}
}

func TestInjectionValidators(t *testing.T) {
	bad := []string{
		"a; COPY (SELECT 1) TO PROGRAM 'sh'; --",
		"a'; DROP DATABASE x; --",
		"`evil`",
		"x\nUser=root",
	}
	for _, v := range bad {
		if err := utils.ValidateSQLIdentifier(v, "name"); err == nil {
			t.Errorf("SQL identifier accepted: %q", v)
		}
		if err := utils.ValidateNodeStartScript(v, "start_script"); err == nil {
			t.Errorf("start script accepted: %q", v)
		}
	}
	if err := utils.ValidateSQLIdentifier("app_db1", "name"); err != nil {
		t.Errorf("legitimate identifier rejected: %v", err)
	}
	if err := utils.ValidateNodeStartScript("server.js", "s"); err != nil {
		t.Errorf("legitimate start script rejected: %v", err)
	}
	if err := utils.ValidateNodeStartScript("npm start", "s"); err != nil {
		t.Errorf("npm start rejected: %v", err)
	}

	if err := utils.ValidateCronSchedule("* * * * *"); err != nil {
		t.Errorf("valid cron rejected: %v", err)
	}
	for _, v := range []string{"* * * * * root sh", "*/5 * * * *\nx", "'; sh; '"} {
		if err := utils.ValidateCronSchedule(v); err == nil {
			t.Errorf("cron schedule accepted: %q", v)
		}
	}

	if _, err := utils.QuoteSQLLiteral("pa'ss"); err != nil {
		t.Errorf("quotable literal rejected: %v", err)
	} else {
		q, _ := utils.QuoteSQLLiteral("pa'ss")
		if q != "'pa''ss'" {
			t.Errorf("bad quoting: %s", q)
		}
	}
	if _, err := utils.QuoteSQLLiteral(`back\slash`); err == nil {
		t.Error("backslash literal was quoted instead of rejected")
	}
}

func TestDomainValidation(t *testing.T) {
	bad := []string{
		"../../../etc/cron.d/pwn",
		"a/b.com",
		"evil.com\nserver_name x",
		"",
		"..",
		"exam..ple.com",
	}
	for _, d := range bad {
		if err := webserver.ValidateSiteDomain(d); err == nil {
			t.Errorf("domain accepted: %q", d)
		}
	}
	for _, d := range []string{"example.com", "sub.example.co.uk", "my-site123.org"} {
		if err := webserver.ValidateSiteDomain(d); err != nil {
			t.Errorf("legitimate domain rejected %q: %v", d, err)
		}
	}
}

func TestNginxRejectsTraversalDomain(t *testing.T) {
	a := webserver.NewNginxAdapter()
	cfg := &webserver.SiteConfig{Domain: "../../../etc/cron.d/pwn", RootDir: mustSiteRoot("x.example.com")}
	if err := a.CreateSite(nil, cfg); err == nil {
		t.Error("nginx adapter accepted a traversal domain")
	} else {
		t.Log("nginx rejected:", err)
	}
	if err := a.DeleteSite(nil, "../../etc/passwd"); err == nil {
		t.Error("nginx DeleteSite accepted a traversal domain")
	}
}
