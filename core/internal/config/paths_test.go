package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// SiteRoot is the only thing standing between an API request body and
// os.MkdirAll, so every payload that would escape the web root has to be
// refused rather than cleaned up into something plausible.
func TestSiteRootRejectsTraversal(t *testing.T) {
	bad := []string{
		"",
		" ",
		".",
		"..",
		"../../etc/cron.d/pwn",
		"a/../../etc",
		"example.com/../../../root",
		"/etc/passwd",
		"..example.com",
		".example.com",
		"-example.com",
		"example.com/",
		"example.com/sub",
		"exam ple.com",
		"example.com\x00",
		"example.com\n",
		"exa`id`mple.com",
		"exa$(id)mple.com",
		"exam;ple.com",
		"exam..ple.com",
		"exam\\ple.com",
		strings.Repeat("a", 254) + ".com",
	}
	for _, d := range bad {
		if got, err := SiteRoot(d); err == nil {
			t.Errorf("SiteRoot(%q) accepted, produced %q", d, got)
		}
	}
}

func TestSiteRootAcceptsRealDomains(t *testing.T) {
	for _, d := range []string{"example.com", "sub.example.co.uk", "my-site123.org", "Example.COM"} {
		got, err := SiteRoot(d)
		if err != nil {
			t.Fatalf("SiteRoot(%q) rejected a legitimate domain: %v", d, err)
		}
		want := filepath.Join(WebRoot(), strings.ToLower(d))
		if got != want {
			t.Errorf("SiteRoot(%q) = %q, want %q", d, got, want)
		}
	}
}

// Case is normalised so "Example.com" and "example.com" can never become two
// directories holding two different copies of one customer's site.
func TestSiteRootNormalisesCase(t *testing.T) {
	upper, err := SiteRoot("EXAMPLE.com")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := SiteRoot("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if upper != lower {
		t.Errorf("case produced two roots: %q vs %q", upper, lower)
	}
}

func TestLayoutFollowsPanelRoot(t *testing.T) {
	t.Setenv(EnvPanelRoot, "/srv/panel")
	for name, got := range map[string]string{
		"PanelRoot":   PanelRoot(),
		"WebRoot":     WebRoot(),
		"BackupRoot":  BackupRoot(),
		"LogRoot":     LogRoot(),
		"SiteLogRoot": SiteLogRoot(),
		"EtcRoot":     EtcRoot(),
		"SSLRoot":     SSLRoot(),
		"TmpRoot":     TmpRoot(),
		"DefaultSite": DefaultSite(),
	} {
		if !strings.HasPrefix(got, "/srv/panel") {
			t.Errorf("%s = %q, expected it to follow %s", name, got, EnvPanelRoot)
		}
	}
}

func TestSubtreeOverrideWins(t *testing.T) {
	t.Setenv(EnvPanelRoot, "/srv/panel")
	t.Setenv(EnvBackupRoot, "/mnt/backup-volume")
	if got := BackupRoot(); got != "/mnt/backup-volume" {
		t.Errorf("BackupRoot() = %q, want the explicit override", got)
	}
	if got := WebRoot(); got != "/srv/panel/www/domains" {
		t.Errorf("WebRoot() = %q, want it to still follow the panel root", got)
	}
}

// A relative root would silently re-anchor every site directory onto whatever
// directory the process happens to start in, so it is ignored, not honoured.
func TestRelativeRootIsIgnored(t *testing.T) {
	t.Setenv(EnvWebRoot, "relative/www")
	if got := WebRoot(); got != filepath.Join(DefaultPanelRoot, "www", "domains") {
		t.Errorf("WebRoot() = %q, want the built-in default", got)
	}
}

// The defaults are the documented install layout; a typo here would move every
// customer's site on the next deploy.
func TestDefaultLayout(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{PanelRoot(), "/vkai-panel"},
		{WebRoot(), "/vkai-panel/www/domains"},
		{BackupRoot(), "/vkai-panel/www/backup"},
		{DefaultSite(), "/vkai-panel/www/default"},
		{LogRoot(), "/vkai-panel/logs"},
		{SiteLogRoot(), "/vkai-panel/logs/sites"},
		{EtcRoot(), "/vkai-panel/etc"},
		{SSLRoot(), "/vkai-panel/ssl"},
		{TmpRoot(), "/vkai-panel/tmp"},
		{ConfigFile(), "/vkai-panel/etc/config.yaml"},
		{EnvFile(), "/vkai-panel/etc/.env"},
		{PanelStateFile(), "/vkai-panel/etc/panel_access.json"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
