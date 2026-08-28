package main

import (
	"strings"
	"testing"
	"time"
)

// loadConfig is the only place the agent's behaviour can be changed from
// outside, so the values it refuses matter as much as the ones it accepts.

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("VKAI_PANEL_URL", "https://panel.example.com")
	t.Setenv("VKAI_AGENT_HOSTNAME", "node-1")
	// Nothing else is set: every test starts from the defaults.
	for _, name := range []string{
		"VKAI_AGENT_STATUS_INTERVAL", "VKAI_AGENT_STATUS_JITTER", "VKAI_AGENT_BUFFER_SAMPLES",
		"VKAI_AGENT_AUDIT_LOG", "VKAI_AGENT_LOG_ROOTS", "VKAI_AGENT_DISK_ROOTS",
		"VKAI_AGENT_PORT", "VKAI_AGENT_BIND", "VKAI_AGENT_STATE_DIR",
	} {
		t.Setenv(name, "")
	}
}

func TestTheDefaultsAreACadenceWithJitterAndABoundedBuffer(t *testing.T) {
	baseEnv(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("the default configuration was refused: %v", err)
	}
	if cfg.StatusInterval != 30*time.Second {
		t.Fatalf("the default cadence is %s, want 30s", cfg.StatusInterval)
	}
	if cfg.StatusJitter <= 0 {
		t.Fatal("the default cadence has no jitter, so a fleet reports in lockstep")
	}
	if cfg.BufferSamples <= 0 {
		t.Fatal("the default buffer holds nothing, so a sample that cannot be delivered is lost")
	}
	if !strings.HasPrefix(cfg.AuditPath, "/") {
		t.Fatalf("the operation record defaults to %q, which is not an absolute path", cfg.AuditPath)
	}
}

func TestAnUnreasonableCadenceIsRefused(t *testing.T) {
	for _, bad := range []struct{ name, value string }{
		{"VKAI_AGENT_STATUS_INTERVAL", "1s"}, // faster than the minimum
		{"VKAI_AGENT_STATUS_INTERVAL", "not a time"},
		{"VKAI_AGENT_STATUS_JITTER", "0.9"}, // more jitter than cadence
		{"VKAI_AGENT_STATUS_JITTER", "-0.1"},
		{"VKAI_AGENT_STATUS_JITTER", "half"},
		{"VKAI_AGENT_BUFFER_SAMPLES", "0"},
		{"VKAI_AGENT_BUFFER_SAMPLES", "-5"},
		{"VKAI_AGENT_BUFFER_SAMPLES", "10000000"}, // would be gigabytes of retained JSON
	} {
		t.Run(bad.name+"="+bad.value, func(t *testing.T) {
			baseEnv(t)
			t.Setenv(bad.name, bad.value)
			if _, err := loadConfig(); err == nil {
				t.Fatalf("%s=%q was accepted", bad.name, bad.value)
			}
		})
	}
}

// A configuration mistake must not be able to turn "read a log file" back into
// "read any file on this host".
func TestSlashIsRefusedAsALogOrDiskRoot(t *testing.T) {
	for _, name := range []string{"VKAI_AGENT_LOG_ROOTS", "VKAI_AGENT_DISK_ROOTS"} {
		t.Run(name, func(t *testing.T) {
			baseEnv(t)
			t.Setenv(name, "/var/log:/")
			_, err := loadConfig()
			if err == nil {
				t.Fatalf("%s accepted \"/\" as a root", name)
			}
			if !strings.Contains(err.Error(), "every file") {
				t.Fatalf("the refusal does not explain the consequence: %v", err)
			}
		})
	}
}

func TestRootsAreReadAsAColonSeparatedListLikePath(t *testing.T) {
	baseEnv(t)
	t.Setenv("VKAI_AGENT_LOG_ROOTS", "/var/log: /srv/logs :")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("a valid root list was refused: %v", err)
	}
	if len(cfg.LogRoots) != 2 || cfg.LogRoots[0] != "/var/log" || cfg.LogRoots[1] != "/srv/logs" {
		t.Fatalf("the roots were read as %v", cfg.LogRoots)
	}
}

func TestARelativeAuditPathIsRefused(t *testing.T) {
	baseEnv(t)
	t.Setenv("VKAI_AGENT_AUDIT_LOG", "operations.log")
	if _, err := loadConfig(); err == nil {
		t.Fatal("a relative path was accepted for the operation record")
	}
}

func TestThePanelURLIsRequired(t *testing.T) {
	baseEnv(t)
	t.Setenv("VKAI_PANEL_URL", "")
	t.Setenv("PANEL_URL", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("the agent started without knowing which panel it belongs to")
	}
}
