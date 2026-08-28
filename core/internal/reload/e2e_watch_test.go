package reload_test

// The file and signal origins, driven through the same running panel.
//
// An operator who edits /vkai-panel/etc/panel_access.json expects it to matter.
// Before this, it mattered at the next restart - a restart they have to
// remember to perform, on the machine they administer over the network, using
// the panel that the restart interrupts.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/reload"
)

// writeState rewrites the state file the way an editor would.
func writeState(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the state file: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("parse the state file: %v", err)
	}
	mutate(stored)

	updated, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatalf("write the state file: %v", err)
	}
}

// TestEditingTheStateFileMovesTheRunningPanel is the file origin end to end.
func TestEditingTheStateFileMovesTheRunningPanel(t *testing.T) {
	panel := startPanel(t)

	// Establish the file: the panel writes it as part of applying a change, and
	// an operator edits what is there.
	if status, body := panel.put(panel.cfg.Port, `{"session_ttl_seconds":1800}`); status != http.StatusOK {
		t.Fatalf("seeding the state file failed: %d %v", status, body)
	}

	watcher := reload.NewWatcher(panel.sup, reload.WatcherOptions{
		StateFile:    panel.stateFile,
		PollInterval: 20 * time.Millisecond,
		QuietPeriod:  60 * time.Millisecond,
		Logger:       zap.NewNop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx)

	oldPort := panel.cfg.Port
	newPort := freePort(t)
	writeState(t, panel.stateFile, func(stored map[string]any) { stored["port"] = newPort })

	waitUntil(t, "the port from the file to be serving", func() bool { return serving(newPort) })
	waitUntil(t, "the old port to stop", func() bool { return !serving(oldPort) })

	if live := panel.sup.Current(); live.Port != newPort {
		t.Fatalf("the live configuration says port %d, the file said %d", live.Port, newPort)
	}
}

// TestABrokenStateFileChangesNothing is the debounce and the validation
// together: a file that is being written, or is simply wrong, must never reach
// the running panel.
func TestABrokenStateFileChangesNothing(t *testing.T) {
	panel := startPanel(t)
	port := panel.cfg.Port

	if status, _ := panel.put(port, `{"session_ttl_seconds":1800}`); status != http.StatusOK {
		t.Fatal("seeding the state file failed")
	}

	watcher := reload.NewWatcher(panel.sup, reload.WatcherOptions{
		StateFile:    panel.stateFile,
		PollInterval: 20 * time.Millisecond,
		QuietPeriod:  60 * time.Millisecond,
		Logger:       zap.NewNop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx)

	// Half a file, exactly as a truncate-then-write editor leaves it.
	if err := os.WriteFile(panel.stateFile, []byte(`{"port": 45`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	if !serving(port) {
		t.Fatal("a half-written state file took the panel off its port")
	}

	// A complete file that is nevertheless refused: 443 belongs to the hosted
	// websites and the panel must never take it.
	if err := os.WriteFile(panel.stateFile,
		[]byte(`{"enabled":true,"bind":"127.0.0.1","port":443,"entrance":"/vkai_test_door","entrance_enabled":true,"session_ttl_seconds":1800}`),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	if !serving(port) {
		t.Fatal("an invalid state file took the panel off its port")
	}
	if live := panel.sup.Current(); live.Port != port {
		t.Fatalf("an invalid state file reached the live configuration: port %d", live.Port)
	}
}

// TestReloadFromDiskAppliesImmediately is the SIGHUP path. The signal itself is
// delivered by the operating system; what is checked here is what the handler
// does, which is everything that can be wrong.
func TestReloadFromDiskAppliesImmediately(t *testing.T) {
	panel := startPanel(t)

	if status, _ := panel.put(panel.cfg.Port, `{"session_ttl_seconds":1800}`); status != http.StatusOK {
		t.Fatal("seeding the state file failed")
	}

	watcher := reload.NewWatcher(panel.sup, reload.WatcherOptions{
		StateFile: panel.stateFile,
		Logger:    zap.NewNop(),
	})

	newPort := freePort(t)
	writeState(t, panel.stateFile, func(stored map[string]any) { stored["port"] = newPort })

	watcher.Reload(context.Background(), reload.Request{Origin: reload.OriginSignal, Actor: "SIGHUP"})

	waitUntil(t, "the reloaded port to serve", func() bool { return serving(newPort) })
}

// TestEnvironmentChangesThatCannotApplyAreReported is the honest half: the
// panel must say what it could not do, not report success for it.
func TestEnvironmentChangesThatCannotApplyAreReported(t *testing.T) {
	panel := startPanel(t)

	dir := t.TempDir()
	envFile := dir + "/.env"
	if err := os.WriteFile(envFile, []byte("VKAI_DB_PASSWORD=one\nVKAI_REDIS_HOST=127.0.0.1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	watcher := reload.NewWatcher(panel.sup, reload.WatcherOptions{
		StateFile: panel.stateFile,
		EnvFile:   envFile,
		Logger:    zap.NewNop(),
	})

	if err := os.WriteFile(envFile, []byte("VKAI_DB_PASSWORD=two\nVKAI_REDIS_HOST=10.0.0.9\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	watcher.Reload(context.Background(), reload.Request{Origin: reload.OriginSignal, Actor: "SIGHUP"})

	pending := panel.sup.RestartRequired()
	if len(pending) == 0 {
		t.Fatal("a changed database password was not reported as needing a restart")
	}

	joined := fmt.Sprint(pending)
	for _, want := range []string{"VKAI_DB_PASSWORD", "VKAI_REDIS_HOST"} {
		if !contains(joined, want) {
			t.Fatalf("%s is missing from the restart-required report: %v", want, pending)
		}
	}
	// The value must never appear, only the name and the reason.
	if contains(joined, "two") {
		t.Fatalf("the new value of a secret leaked into the report: %v", pending)
	}
}

// TestEnvironmentPinnedSettingsAreRefused: the environment wins at every load,
// so a change the API could make and the next load would undo is refused with
// the variable named.
func TestEnvironmentPinnedSettingsAreRefused(t *testing.T) {
	panel := startPanel(t)

	// Pin the port the way the installer's .env does, and re-derive.
	live := panel.sup.Current()
	pinned := reload.Clone(live)
	pinned.EnvOverrides = append(pinned.EnvOverrides, "port")
	panel.sup.Adopt(pinned)

	status, body := panel.put(panel.cfg.Port, fmt.Sprintf(`{"port":%d}`, freePort(t)))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for a setting the environment pins, got %d: %v", status, body)
	}

	apiError, _ := body["error"].(map[string]any)
	message, _ := apiError["message"].(string)
	if !contains(message, config.PanelEnvVariable("port")) {
		t.Fatalf("the refusal does not name the variable to remove: %q", message)
	}
	if !serving(panel.cfg.Port) {
		t.Fatal("the panel stopped serving after a refused change")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
