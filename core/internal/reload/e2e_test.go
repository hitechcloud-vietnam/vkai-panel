package reload_test

// End-to-end tests for the reload path.
//
// These deliberately drive the panel the way an operator does: a real HTTP
// request, over a real socket, to the real settings handler, mounted on the
// real listener the reload package manages. Nothing here calls a reload
// function directly.
//
// That is the point. The defect this work exists to remove is a component that
// is written, tested and merged while connected to nothing - and a test that
// calls Apply() and asserts it returned nil proves exactly that component
// works, which was never in doubt. What has to be proven is the wiring: that a
// PUT arriving on the panel's own port moves the panel's own port, and that the
// panel is still answering afterwards.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/handler"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/reload"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// panelUnderTest is a running panel: the same objects the API entry point wires,
// assembled the same way.
type panelUnderTest struct {
	t         *testing.T
	cfg       *config.PanelAccessConfig
	sup       *reload.Supervisor
	rebinder  *reload.Rebinder
	stateFile string
}

func startPanel(t *testing.T) *panelUnderTest {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := config.DefaultPanelAccess()
	cfg.Enabled = true
	cfg.Bind = "127.0.0.1"
	cfg.Port = freePort(t)
	cfg.Entrance = "/vkai_test_door"
	cfg.EntranceEnabled = true
	cfg.SessionTTLSeconds = 3600
	cfg.AllowedIPs = []string{}
	cfg.TrustedProxies = []string{}
	cfg.TLS.Enabled = false
	cfg.StateFile = dir + "/panel_access.json"

	logger := zap.NewNop()

	sup := reload.New(reload.Options{Config: cfg, Logger: logger})
	reload.SetDefault(sup)
	t.Cleanup(func() { reload.SetDefault(nil) })

	// The settings service finds the supervisor the way it does in the running
	// panel: through the process-wide handoff, because the router builds it.
	svc := service.NewPanelSettingsService(cfg, nil, logger)
	settings := handler.NewPanelSettingsHandler(svc, logger)

	engine := gin.New()
	engine.GET("/api/v1/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/api/v1/panel/settings", settings.Get)
	engine.PUT("/api/v1/panel/settings", settings.Update)

	guard, err := reload.NewGuardSwitch(cfg, "test-secret-that-is-long-enough-1234", engine, logger)
	if err != nil {
		t.Fatalf("access gate: %v", err)
	}

	rebinder, err := reload.NewRebinder(reload.RebinderOptions{
		Handler:      guard,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  10 * time.Second,
		DrainTimeout: 2 * time.Second,
		Address:      func(c *config.PanelAccessConfig) string { return c.ListenAddr() },
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("listener: %v", err)
	}

	sup.Register(reload.NewStateFile(logger))
	sup.Register(guard)
	sup.Register(rebinder)
	sup.SetProbe(reload.Probes{rebinder, guard})

	if err := rebinder.Start(cfg); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = rebinder.Shutdown(contextWithTimeout(2 * time.Second)) })

	return &panelUnderTest{t: t, cfg: cfg, sup: sup, rebinder: rebinder, stateFile: cfg.StateFile}
}

// put sends a real request to the panel's own port, through its own entrance.
func (p *panelUnderTest) put(port int, body string) (int, map[string]any) {
	p.t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d%s/api/v1/panel/settings", port, p.cfg.Entrance)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		p.t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		p.t.Fatalf("PUT %s: %v", url, err)
	}
	defer response.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		p.t.Fatalf("decode response: %v", err)
	}
	return response.StatusCode, decoded
}

// serving reports whether the panel answers on a port at all.
func serving(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", port))
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// waitUntil polls a condition, because a listener stops accepting a moment
// after the response that announced it.
func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func contextWithTimeout(d time.Duration) contextCompat {
	return contextCompat{deadline: time.Now().Add(d)}
}

// TestPortChangeThroughTheAPIMovesTheListener is the test that matters: a
// change made through the HTTP endpoint reaches the running server.
func TestPortChangeThroughTheAPIMovesTheListener(t *testing.T) {
	panel := startPanel(t)
	oldPort := panel.cfg.Port
	newPort := freePort(t)

	if !serving(oldPort) {
		t.Fatalf("the panel is not answering on its own port %d before the change", oldPort)
	}

	status, body := panel.put(oldPort, fmt.Sprintf(`{"port":%d}`, newPort))
	if status != http.StatusOK {
		t.Fatalf("expected 200 from the settings endpoint, got %d: %v", status, body)
	}

	data, _ := body["data"].(map[string]any)
	if applied, _ := data["applied"].(bool); !applied {
		t.Fatalf("the endpoint answered without applying the change: %v", data)
	}

	waitUntil(t, "the new port to serve", func() bool { return serving(newPort) })
	waitUntil(t, "the old port to stop", func() bool { return !serving(oldPort) })

	// The panel is not merely listening on the new port: it is the same process,
	// serving the same routes, with the entrance still enforced.
	status, _ = panel.put(newPort, `{"session_ttl_seconds":1800}`)
	if status != http.StatusOK {
		t.Fatalf("the panel does not serve its own API on the new port: status %d", status)
	}

	// And what is on disk agrees with what is running.
	if got := storedPort(t, panel.stateFile); got != newPort {
		t.Fatalf("the state file says port %d, the panel is on %d", got, newPort)
	}
}

// TestPortAlreadyInUseLeavesThePanelReachable is the failure this whole design
// is shaped around: the panel must never close the door it has for one it
// cannot open.
func TestPortAlreadyInUseLeavesThePanelReachable(t *testing.T) {
	panel := startPanel(t)
	oldPort := panel.cfg.Port

	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot open the squatting listener: %v", err)
	}
	defer squatter.Close()
	taken := squatter.Addr().(*net.TCPAddr).Port

	status, body := panel.put(oldPort, fmt.Sprintf(`{"port":%d}`, taken))
	if status != http.StatusConflict {
		t.Fatalf("expected 409 when the port is taken, got %d: %v", status, body)
	}

	apiError, _ := body["error"].(map[string]any)
	if code, _ := apiError["code"].(string); code != "NOT_APPLIED" {
		t.Fatalf("expected the NOT_APPLIED code, got %q: %v", code, body)
	}
	if details, _ := apiError["details"].(string); !strings.Contains(details, "already listening") {
		t.Fatalf("the response does not say why the port could not be bound: %q", details)
	}

	if !serving(oldPort) {
		t.Fatal("the panel stopped answering on its original port after a failed rebind")
	}
	if got := storedPort(t, panel.stateFile); got != 0 && got != oldPort {
		t.Fatalf("the state file was left describing port %d, which the panel is not serving", got)
	}
}

// TestLockoutIsRolledBackAutomatically proves the confirmation is not the
// protection: even with the caller insisting, a change that makes the panel
// unreachable is undone by the panel itself.
func TestLockoutIsRolledBackAutomatically(t *testing.T) {
	panel := startPanel(t)
	port := panel.cfg.Port

	status, body := panel.put(port, `{"allowed_ips":["203.0.113.0/24"],"confirm":true}`)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 for a self-locking allow list, got %d: %v", status, body)
	}

	apiError, _ := body["error"].(map[string]any)
	if code, _ := apiError["code"].(string); code != "ROLLED_BACK" {
		t.Fatalf("expected the ROLLED_BACK code, got %q: %v", code, body)
	}

	if !serving(port) {
		t.Fatal("the panel is unreachable after a change that should have been rolled back")
	}

	// The rollback is not cosmetic: the gate has to admit a real request again.
	status, _ = panel.put(port, `{"session_ttl_seconds":2400}`)
	if status != http.StatusOK {
		t.Fatalf("the access gate did not return to the configuration that works: status %d", status)
	}

	live := panel.sup.Current()
	if len(live.AllowedIPs) != 0 {
		t.Fatalf("the rolled-back allow list is still live: %v", live.AllowedIPs)
	}
}

// TestEntranceChangeAppliesToTheNextRequest covers the settings that were
// already claimed to be live, so the claim is now checked rather than asserted.
func TestEntranceChangeAppliesToTheNextRequest(t *testing.T) {
	panel := startPanel(t)
	port := panel.cfg.Port

	status, body := panel.put(port, `{"entrance":"/vkai_moved_door","confirm":true}`)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}

	client := &http.Client{Timeout: 2 * time.Second}

	old, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/vkai_test_door/api/v1/panel/settings", port))
	if err != nil {
		t.Fatalf("request through the old entrance: %v", err)
	}
	defer old.Body.Close()
	if old.StatusCode != http.StatusNotFound {
		t.Fatalf("the old entrance still works: status %d", old.StatusCode)
	}

	fresh, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/vkai_moved_door/api/v1/panel/settings", port))
	if err != nil {
		t.Fatalf("request through the new entrance: %v", err)
	}
	defer fresh.Body.Close()
	if fresh.StatusCode != http.StatusOK {
		t.Fatalf("the new entrance does not work: status %d", fresh.StatusCode)
	}
}

// TestInFlightRequestSurvivesTheRebind checks the promise made about draining:
// a request already running on the old listener finishes on it.
func TestInFlightRequestSurvivesTheRebind(t *testing.T) {
	panel := startPanel(t)
	oldPort := panel.cfg.Port
	newPort := freePort(t)

	// The request that asks for the change is itself in flight on the old
	// listener when the listener is retired. If draining were wrong, this
	// response would never arrive - which is exactly how the deadlock in the
	// notification path first showed itself.
	done := make(chan int, 1)
	go func() {
		status, _ := panel.put(oldPort, fmt.Sprintf(`{"port":%d}`, newPort))
		done <- status
	}()

	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Fatalf("the request that moved the port did not get its answer: status %d", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the request that moved the port never got a response: the old listener did not drain it")
	}

	waitUntil(t, "the new port to serve", func() bool { return serving(newPort) })
}

func storedPort(t *testing.T, path string) int {
	t.Helper()
	raw, err := readFile(path)
	if err != nil {
		return 0
	}
	var stored struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("the state file is not valid JSON: %v", err)
	}
	return stored.Port
}
