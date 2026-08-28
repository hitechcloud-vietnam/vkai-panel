package config

// The environment file, which a reload has to read for itself.
//
// Editing /vkai-panel/etc/.env does not change os.Environ of a process that is
// already running. A reload that consulted the process environment would report
// "nothing changed" for a file the operator had just edited, which is the
// silent no-op this work exists to remove.

import "testing"

func TestParseEnvFileReadsWhatAnInstallerWrites(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"

	content := "" +
		"# VKAI Panel environment\n" +
		"\n" +
		"VKAI_PANEL_PORT=8899\n" +
		"export VKAI_PANEL_DOMAIN=\"panel.example.com\"\n" +
		"VKAI_DB_PASSWORD='a secret with spaces'\n" +
		"  VKAI_PANEL_ALLOWED_IPS=203.0.113.0/24, 198.51.100.7  \n" +
		"a line with no equals sign\n" +
		"=novalue\n"

	if err := writeFile(path, content); err != nil {
		t.Fatalf("write: %v", err)
	}

	vars, err := ParseEnvFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for key, want := range map[string]string{
		"VKAI_PANEL_PORT":        "8899",
		"VKAI_PANEL_DOMAIN":      "panel.example.com",
		"VKAI_DB_PASSWORD":       "a secret with spaces",
		"VKAI_PANEL_ALLOWED_IPS": "203.0.113.0/24, 198.51.100.7",
	} {
		if got := vars[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if len(vars) != 4 {
		t.Errorf("unparseable lines were not skipped: %v", vars)
	}
}

func TestParseEnvFileTreatsAMissingFileAsEmpty(t *testing.T) {
	vars, err := ParseEnvFile(t.TempDir() + "/does-not-exist")
	if err != nil {
		t.Fatalf("a missing environment file is not an error: %v", err)
	}
	if len(vars) != 0 {
		t.Fatalf("expected no variables, got %v", vars)
	}
}

func TestLoadPanelAccessFromUsesTheSuppliedEnvironment(t *testing.T) {
	dir := t.TempDir()

	vars := map[string]string{
		"VKAI_PANEL_PORT":     "8899",
		"VKAI_PANEL_ENTRANCE": "/vkai_from_file",
	}

	cfg, err := LoadPanelAccessFrom(PanelAccessSource{
		Env:       EnvFileLookup(vars, func(string) (string, bool) { return "", false }),
		StateFile: dir + "/panel_access.json",
		NoPersist: true,
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Port != 8899 {
		t.Errorf("port = %d, want 8899", cfg.Port)
	}
	if cfg.Entrance != "/vkai_from_file" {
		t.Errorf("entrance = %q", cfg.Entrance)
	}
	if !cfg.IsEnvOverridden("port") {
		t.Error("the port was not marked as pinned by the environment")
	}
	if _, err := osStat(dir + "/panel_access.json"); err == nil {
		t.Error("NoPersist wrote the state file anyway")
	}
}

func TestRestartRequiredEnvKeysCoverTheConnectionsHeldAtStartup(t *testing.T) {
	keys := RestartRequiredEnvKeys()

	for _, name := range []string{
		"VKAI_DB_PASSWORD", "VKAI_DATABASE_HOST", "VKAI_REDIS_HOST",
		"VKAI_JWT_SECRET", "VKAI_PANEL_ROOT", "VKAI_UI_UPSTREAM",
	} {
		if keys[name] == "" {
			t.Errorf("%s is not reported as needing a restart", name)
		}
	}

	// The panel access settings are hot-applied, so none of them belongs here:
	// listing one would tell an operator to restart for a change that is
	// already live.
	for _, name := range []string{
		"VKAI_PANEL_PORT", "VKAI_PANEL_ENTRANCE", "VKAI_PANEL_ALLOWED_IPS",
		"VKAI_PANEL_DOMAIN", "VKAI_PANEL_TLS_MODE",
	} {
		if keys[name] != "" {
			t.Errorf("%s is claimed to need a restart, but it is applied live", name)
		}
	}
}

func writeFile(path, content string) error { return osWriteFile(path, []byte(content), 0o600) }
