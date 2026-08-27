package config

import (
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPanelAccessGeneratesAndPersistsAnEntrance(t *testing.T) {
	state := filepath.Join(t.TempDir(), "panel_access.json")
	t.Setenv("VKAI_PANEL_CONFIG_FILE", state)

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}

	if cfg.Port != DefaultPanelPort {
		t.Fatalf("Port = %d, want the aaPanel-compatible default %d", cfg.Port, DefaultPanelPort)
	}
	if !strings.HasPrefix(cfg.Entrance, "/vkai_") {
		t.Fatalf("Entrance = %q, want a generated /vkai_ prefix", cfg.Entrance)
	}
	if len(cfg.Generated) == 0 {
		t.Fatal("Generated is empty: the banner would not tell the operator to write the entrance down")
	}

	info, err := os.Stat(state)
	if err != nil {
		t.Fatalf("the generated entrance was not persisted: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %o, want 0600: the entrance is a secret", perm)
	}

	// A second load must reuse the persisted entrance, otherwise every restart
	// would change the panel URL.
	again, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("second LoadPanelAccess: %v", err)
	}
	if again.Entrance != cfg.Entrance {
		t.Fatalf("entrance changed across restarts: %q -> %q", cfg.Entrance, again.Entrance)
	}
	if len(again.Generated) != 0 {
		t.Fatalf("Generated = %v on the second load, want nothing regenerated", again.Generated)
	}
}

func TestEnvironmentOverridesTheStateFile(t *testing.T) {
	state := filepath.Join(t.TempDir(), "panel_access.json")
	t.Setenv("VKAI_PANEL_CONFIG_FILE", state)
	t.Setenv("VKAI_PANEL_PORT", "9001")
	t.Setenv("VKAI_PANEL_ENTRANCE", "cong_quan_tri")
	t.Setenv("VKAI_PANEL_ALLOWED_IPS", "203.0.113.7, 10.0.0.0/8")
	t.Setenv("VKAI_PANEL_DOMAIN", "Panel.Example.COM")

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}

	if cfg.Port != 9001 {
		t.Fatalf("Port = %d, want 9001", cfg.Port)
	}
	if cfg.Entrance != "/cong_quan_tri" {
		t.Fatalf("Entrance = %q, want the normalised /cong_quan_tri", cfg.Entrance)
	}
	if len(cfg.AllowedIPs) != 2 {
		t.Fatalf("AllowedIPs = %v, want two entries", cfg.AllowedIPs)
	}
	if cfg.Domain != "panel.example.com" {
		t.Fatalf("Domain = %q, want it lower-cased", cfg.Domain)
	}
	if !cfg.IsEnvOverridden("port") {
		t.Fatal("port is not marked as environment-pinned, so the CLI would not warn")
	}
}

func TestPanelRefusesTheCustomerWebsitePorts(t *testing.T) {
	for _, port := range []int{80, 443} {
		cfg := DefaultPanelAccess()
		cfg.Port = port
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("port %d was accepted: the panel must never take a customer website port", port)
		}
		if !strings.Contains(err.Error(), "cong rieng") {
			t.Fatalf("port %d: error = %v, want it to point at the dedicated panel port", port, err)
		}
	}

	// The same rule applies to the port a reverse proxy publishes.
	cfg := DefaultPanelAccess()
	cfg.Entrance = "/vkai_a1b2c3d4"
	cfg.PublicPort = 443
	if err := cfg.Validate(); err == nil {
		t.Fatal("public port 443 was accepted")
	}
}

func TestValidateEntranceRejectsSystemPathsAndJunk(t *testing.T) {
	for _, entrance := range []string{"", "/", "/api", "/api/v1", "/health", "/ws", "/ab", "/a b", "/x/../y"} {
		if err := ValidateEntrance(NormalizeEntrance(entrance)); err == nil {
			t.Fatalf("entrance %q was accepted", entrance)
		}
	}
	for _, entrance := range []string{"/vkai_a1b2c3d4", "/cong_quan_tri", "/a/b/c"} {
		if err := ValidateEntrance(NormalizeEntrance(entrance)); err != nil {
			t.Fatalf("entrance %q was rejected: %v", entrance, err)
		}
	}
}

func TestNormalizeEntranceIsCanonical(t *testing.T) {
	for input, want := range map[string]string{
		"vkai_x":       "/vkai_x",
		"/vkai_x/":     "/vkai_x",
		"//vkai_x//y/": "/vkai_x/y",
		"  /vkai_x  ":  "/vkai_x",
		"":             "",
		"/":            "",
	} {
		if got := NormalizeEntrance(input); got != want {
			t.Fatalf("NormalizeEntrance(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRandomPanelPortStaysInRangeAndAvoidsReservedPorts(t *testing.T) {
	for i := 0; i < 200; i++ {
		port, err := RandomPanelPort()
		if err != nil {
			t.Fatalf("RandomPanelPort: %v", err)
		}
		if port < PanelRandomPortMin || port > PanelRandomPortMax {
			t.Fatalf("port %d is outside %d-%d", port, PanelRandomPortMin, PanelRandomPortMax)
		}
		if _, reserved := reservedPanelPorts[port]; reserved {
			t.Fatalf("port %d is reserved", port)
		}
	}
}

func TestSelfSignedTLSMaterialIsGeneratedAndUsable(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultPanelAccess()
	cfg.Entrance = "/vkai_a1b2c3d4"
	cfg.Domain = "panel.example.com"
	cfg.TLS = PanelTLSConfig{
		Enabled:    true,
		SelfSigned: true,
		CertFile:   filepath.Join(dir, "panel.crt"),
		KeyFile:    filepath.Join(dir, "panel.key"),
	}

	certFile, keyFile, err := cfg.EnsureTLSMaterial()
	if err != nil {
		t.Fatalf("EnsureTLSMaterial: %v", err)
	}

	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("the generated pair does not load: %v", err)
	}

	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("private key mode = %o, want 0600", perm)
	}

	if cfg.Scheme() != "https" {
		t.Fatalf("Scheme() = %q, want https once TLS is on", cfg.Scheme())
	}
}

func TestAccessURLUsesThePublicPortWhenProxied(t *testing.T) {
	cfg := DefaultPanelAccess()
	cfg.Bind = "127.0.0.1"
	cfg.Port = 30110
	cfg.PublicPort = 8888
	cfg.PublicScheme = "https"
	cfg.Entrance = "/vkai_a1b2c3d4"
	cfg.Domain = "panel.example.com"

	if got, want := cfg.AccessURL(), "https://panel.example.com:8888/vkai_a1b2c3d4/"; got != want {
		t.Fatalf("AccessURL() = %q, want %q", got, want)
	}
	if !cfg.IsProxied() {
		t.Fatal("IsProxied() = false, want true")
	}
	if cfg.ListenAddr() != "127.0.0.1:30110" {
		t.Fatalf("ListenAddr() = %q, want the internal address", cfg.ListenAddr())
	}
	if !strings.Contains(cfg.Banner(), "ufw allow 8888/tcp") {
		t.Fatal("the banner must advise opening the port operators actually connect to")
	}
}

func TestTLSModeDefaultsToSelfSigned(t *testing.T) {
	state := filepath.Join(t.TempDir(), "panel_access.json")
	t.Setenv("VKAI_PANEL_CONFIG_FILE", state)

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}

	if cfg.TLSMode() != TLSModeSelfSigned {
		t.Fatalf("TLSMode() = %q, want %q", cfg.TLSMode(), TLSModeSelfSigned)
	}
	if !cfg.TLS.Enabled {
		t.Fatal("TLS is off by default: the first login would travel in cleartext")
	}
	if !cfg.TLS.ACME.UseStaging {
		t.Fatal("ACME staging is off by default: a misconfigured install would burn production rate limits")
	}
	// The mode has to be written back concretely, so the next build never has
	// to guess what an older state file meant.
	if cfg.TLS.Mode != TLSModeSelfSigned {
		t.Fatalf("persisted TLS.Mode = %q, want it resolved to %q", cfg.TLS.Mode, TLSModeSelfSigned)
	}
}

func TestLegacyStateFileWithoutAModeKeepsWorking(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		stored   string
		wantMode string
	}{
		{
			name:     "self signed",
			stored:   `{"tls":{"enabled":true,"self_signed":true,"cert_file":"/x.crt","key_file":"/x.key"}}`,
			wantMode: TLSModeSelfSigned,
		},
		{
			// Written by a build that predates modes: self_signed=false meant
			// "the operator supplied their own files". Reading it as
			// self-signed would overwrite that certificate on the next start.
			name:     "operator supplied files",
			stored:   `{"tls":{"enabled":true,"self_signed":false,"cert_file":"/x.crt","key_file":"/x.key"}}`,
			wantMode: TLSModeCustom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(state, []byte(tc.stored), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("VKAI_PANEL_CONFIG_FILE", state)

			cfg, err := LoadPanelAccess()
			if err != nil {
				t.Fatalf("LoadPanelAccess: %v", err)
			}
			if cfg.TLSMode() != tc.wantMode {
				t.Fatalf("TLSMode() = %q, want %q", cfg.TLSMode(), tc.wantMode)
			}
		})
	}
}

func TestACMEEnvironmentVariables(t *testing.T) {
	state := filepath.Join(t.TempDir(), "panel_access.json")
	t.Setenv("VKAI_PANEL_CONFIG_FILE", state)
	t.Setenv("VKAI_PANEL_TLS_MODE", "letsencrypt")
	t.Setenv("VKAI_PANEL_ACME_EMAIL", "ops@hitechcloud.vn")
	t.Setenv("VKAI_PANEL_ACME_STAGING", "false")
	t.Setenv("VKAI_PANEL_ACME_PROFILE", "tlsserver")

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}

	if cfg.TLSMode() != TLSModeLetsEncrypt {
		t.Fatalf("TLSMode() = %q, want %q", cfg.TLSMode(), TLSModeLetsEncrypt)
	}
	if !cfg.UsesACME() {
		t.Fatal("UsesACME() = false in letsencrypt mode")
	}
	if cfg.TLS.ACME.Email != "ops@hitechcloud.vn" {
		t.Fatalf("Email = %q", cfg.TLS.ACME.Email)
	}
	if cfg.TLS.ACME.UseStaging {
		t.Fatal("staging was not switched off by VKAI_PANEL_ACME_STAGING=false")
	}
	if cfg.TLS.ACME.Profile != ACMEProfileTLSServer {
		t.Fatalf("Profile = %q, want %q", cfg.TLS.ACME.Profile, ACMEProfileTLSServer)
	}
	if !cfg.IsEnvOverridden("tls_mode") {
		t.Fatal("tls_mode is not marked as environment-pinned")
	}
	// A CA-issued certificate still needs somewhere to live.
	if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
		t.Fatal("no certificate path was derived for letsencrypt mode")
	}
}

func TestLegacySelfSignedFlagStillSelectsTheMode(t *testing.T) {
	state := filepath.Join(t.TempDir(), "panel_access.json")
	t.Setenv("VKAI_PANEL_CONFIG_FILE", state)
	t.Setenv("VKAI_PANEL_TLS_SELF_SIGNED", "true")

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}
	if cfg.TLSMode() != TLSModeSelfSigned {
		t.Fatalf("TLSMode() = %q, want the legacy flag to still work", cfg.TLSMode())
	}
}

func TestExplicitCertificateFilesSwitchToCustomMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VKAI_PANEL_CONFIG_FILE", filepath.Join(dir, "panel_access.json"))
	t.Setenv("VKAI_PANEL_TLS_CERT", filepath.Join(dir, "mine.crt"))
	t.Setenv("VKAI_PANEL_TLS_KEY", filepath.Join(dir, "mine.key"))

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}
	if cfg.TLSMode() != TLSModeCustom {
		t.Fatalf("TLSMode() = %q, want %q so the supplied pair is never overwritten", cfg.TLSMode(), TLSModeCustom)
	}
}

func TestModeEnvironmentBeatsTheOlderFlags(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VKAI_PANEL_CONFIG_FILE", filepath.Join(dir, "panel_access.json"))
	t.Setenv("VKAI_PANEL_TLS_CERT", filepath.Join(dir, "mine.crt"))
	t.Setenv("VKAI_PANEL_TLS_SELF_SIGNED", "true")
	t.Setenv("VKAI_PANEL_TLS_MODE", "letsencrypt")

	cfg, err := LoadPanelAccess()
	if err != nil {
		t.Fatalf("LoadPanelAccess: %v", err)
	}
	if cfg.TLSMode() != TLSModeLetsEncrypt {
		t.Fatalf("TLSMode() = %q, want the explicit mode to win", cfg.TLSMode())
	}
}

func TestInvalidTLSModeAndACMESettingsAreRejected(t *testing.T) {
	t.Run("mode", func(t *testing.T) {
		t.Setenv("VKAI_PANEL_CONFIG_FILE", filepath.Join(t.TempDir(), "panel_access.json"))
		t.Setenv("VKAI_PANEL_TLS_MODE", "vault")
		if _, err := LoadPanelAccess(); err == nil {
			t.Fatal("an unknown TLS mode was accepted")
		}
	})

	t.Run("email", func(t *testing.T) {
		t.Setenv("VKAI_PANEL_CONFIG_FILE", filepath.Join(t.TempDir(), "panel_access.json"))
		t.Setenv("VKAI_PANEL_TLS_MODE", "letsencrypt")
		t.Setenv("VKAI_PANEL_ACME_EMAIL", "not-an-address")
		if _, err := LoadPanelAccess(); err == nil {
			t.Fatal("an unparseable ACME contact address was accepted")
		}
	})

	t.Run("profile", func(t *testing.T) {
		t.Setenv("VKAI_PANEL_CONFIG_FILE", filepath.Join(t.TempDir(), "panel_access.json"))
		t.Setenv("VKAI_PANEL_TLS_MODE", "letsencrypt")
		t.Setenv("VKAI_PANEL_ACME_PROFILE", "short lived/../x")
		if _, err := LoadPanelAccess(); err == nil {
			t.Fatal("a malformed ACME profile name was accepted")
		}
	})
}

func TestACMEIdentifierPrefersThePinnedDomain(t *testing.T) {
	cfg := DefaultPanelAccess()
	cfg.Domain = "Panel.Example.COM"

	id, err := cfg.ACMEIdentifier()
	if err != nil {
		t.Fatalf("ACMEIdentifier: %v", err)
	}
	if id.Type != ACMEIdentifierDNS || id.Value != "panel.example.com" {
		t.Fatalf("identifier = %+v, want a lower-cased dns identifier", id)
	}
	if got := cfg.ACMEProfileFor(id.Type); got != ACMEProfileTLSServer {
		t.Fatalf("profile for a dns identifier = %q, want %q", got, ACMEProfileTLSServer)
	}
}

func TestIPIdentifiersUseTheShortLivedProfile(t *testing.T) {
	// Certificates for a bare IP address are only issued under "shortlived",
	// which is about six days. Anything else is refused by the CA.
	if got := DefaultACMEProfile(ACMEIdentifierIP); got != ACMEProfileShortLived {
		t.Fatalf("DefaultACMEProfile(ip) = %q, want %q", got, ACMEProfileShortLived)
	}
	if got := DefaultACMEProfile(ACMEIdentifierDNS); got != ACMEProfileTLSServer {
		t.Fatalf("DefaultACMEProfile(dns) = %q, want %q", got, ACMEProfileTLSServer)
	}

	cfg := DefaultPanelAccess()
	cfg.TLS.ACME.Profile = "classic"
	if got := cfg.ACMEProfileFor(ACMEIdentifierIP); got != ACMEProfileClassic {
		t.Fatalf("an operator-pinned profile was ignored: %q", got)
	}
}

func TestUnroutableAddressesAreNeverOrdered(t *testing.T) {
	// Ordering one of these does not merely fail: it spends the per-account
	// failed-validation rate limit, which then blocks every later order.
	unroutable := []string{
		"127.0.0.1",       // loopback
		"10.1.2.3",        // RFC1918
		"172.16.0.9",      // RFC1918
		"172.31.255.254",  // RFC1918, top of the block
		"192.168.1.1",     // RFC1918
		"169.254.10.10",   // link-local
		"100.64.0.1",      // CGNAT, RFC6598
		"100.127.255.255", // CGNAT, top of the block
		"0.0.0.0",
		"224.0.0.1",
		"255.255.255.255",
	}
	for _, raw := range unroutable {
		if IsPubliclyRoutableIPv4(net.ParseIP(raw)) {
			t.Fatalf("%s is treated as publicly routable", raw)
		}
	}

	routable := []string{"116.118.2.44", "8.8.8.8", "172.32.0.1", "100.128.0.1", "9.255.255.255"}
	for _, raw := range routable {
		if !IsPubliclyRoutableIPv4(net.ParseIP(raw)) {
			t.Fatalf("%s is treated as unroutable, so a valid install could never get a certificate", raw)
		}
	}
}

func TestBannerNamesTheCertificateSource(t *testing.T) {
	cfg := DefaultPanelAccess()
	cfg.Entrance = "/vkai_a1b2c3d4"
	cfg.Domain = "panel.example.com"
	cfg.TLS.Mode = TLSModeLetsEncrypt

	// Before the TLS manager reports in, the banner falls back to the mode.
	if !strings.Contains(cfg.Banner(), "letsencrypt (staging)") {
		t.Fatalf("the banner does not name the requested source:\n%s", cfg.Banner())
	}

	// Once it does, the banner names what is actually on the wire - here, the
	// fallback an unreachable CA left behind.
	cfg.CertSource = "self-signed (fallback)"
	banner := cfg.Banner()
	if !strings.Contains(banner, "self-signed (fallback)") {
		t.Fatalf("the banner hides the fallback:\n%s", banner)
	}
}

func TestEnsureTLSMaterialDoesNotOverwriteAnIssuedCertificate(t *testing.T) {
	dir := t.TempDir()

	cfg := DefaultPanelAccess()
	cfg.Domain = "panel.example.com"
	cfg.TLS.Mode = TLSModeLetsEncrypt
	cfg.TLS.CertFile = filepath.Join(dir, "panel.crt")
	cfg.TLS.KeyFile = filepath.Join(dir, "panel.key")

	// First call bootstraps a local pair, because the panel must answer HTTPS
	// before any ACME order can finish.
	certFile, keyFile, err := cfg.EnsureTLSMaterial()
	if err != nil {
		t.Fatalf("EnsureTLSMaterial: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("the bootstrap pair does not load: %v", err)
	}

	// Stand in for what the TLS manager writes after a successful order. The
	// second call must leave it exactly as it is: regenerating here would throw
	// away a certificate that cost a rate limit to obtain.
	marker := []byte("-----BEGIN CERTIFICATE-----\nissued\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(certFile, marker, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := cfg.EnsureTLSMaterial(); err != nil {
		t.Fatalf("second EnsureTLSMaterial: %v", err)
	}
	got, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(marker) {
		t.Fatal("the issued certificate was overwritten by the self-signed generator")
	}
}

func TestCustomModeWithoutFilesIsRejectedAtLoad(t *testing.T) {
	cfg := DefaultPanelAccess()
	cfg.TLS.Mode = TLSModeCustom
	cfg.TLS.SelfSigned = false
	cfg.TLS.CertFile = ""
	cfg.TLS.KeyFile = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("custom TLS mode was accepted with no certificate paths")
	}
}

func TestRecordACMEResult(t *testing.T) {
	cfg := DefaultPanelAccess()

	cfg.RecordACMEResult(time.Time{}, errors.New("connection refused"))
	if cfg.TLS.ACME.LastError == "" {
		t.Fatal("the failure was not recorded, so the UI could not explain the fallback")
	}

	at := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	cfg.RecordACMEResult(at, nil)
	if !cfg.TLS.ACME.LastIssuedAt.Equal(at) {
		t.Fatalf("LastIssuedAt = %s, want %s", cfg.TLS.ACME.LastIssuedAt, at)
	}
	if cfg.TLS.ACME.LastError != "" {
		t.Fatal("a successful issuance left the previous error behind")
	}
}
