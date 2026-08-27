package config

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
