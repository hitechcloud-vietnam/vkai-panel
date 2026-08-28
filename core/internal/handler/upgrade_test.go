package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// The upgrade routes, exercised end to end through gin: the version endpoint's
// three-field contract, the 503 a build with no release engine gives, the 409
// that stops a second upgrade, and the version string being rejected before it
// could reach a shell.
func TestUpgradeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("VKAI_ETC_ROOT", t.TempDir())

	mount := func(svc *service.UpgradeService) *gin.Engine {
		h := NewUpgradeHandler(svc, nil)
		r := gin.New()
		r.GET("/api/v1/version", h.Version)
		r.GET("/api/v1/upgrade/status", h.Status)
		r.POST("/api/v1/upgrade/check", h.Check)
		r.POST("/api/v1/upgrade/start", h.Start)
		r.GET("/api/v1/upgrade/progress/:id", h.Progress)
		return r
	}

	do := func(r *gin.Engine, method, path, body string) (int, map[string]any) {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// --- no engine wired -----------------------------------------------
	r := mount(service.NewUpgradeService(nil, nil, nil))

	code, body := do(r, "GET", "/api/v1/version", "")
	if code != 200 {
		t.Fatalf("version: %d", code)
	}
	data, _ := body["data"].(map[string]any)
	if len(data) != 3 {
		t.Fatalf("version leaks extra fields: %v", data)
	}
	for _, k := range []string{"version", "commit", "build_date"} {
		if _, ok := data[k]; !ok {
			t.Fatalf("version missing %q: %v", k, data)
		}
	}
	t.Logf("version payload: %v", data)

	if code, body = do(r, "GET", "/api/v1/upgrade/status", ""); code != 200 {
		t.Fatalf("status: %d %v", code, body)
	}
	t.Logf("status payload: %v", body["data"])

	if code, _ = do(r, "POST", "/api/v1/upgrade/check", ""); code != 503 {
		t.Fatalf("check without engine: %d", code)
	}
	if code, _ = do(r, "POST", "/api/v1/upgrade/start", `{"version":"1.2.3"}`); code != 503 {
		t.Fatalf("start without engine: %d", code)
	}
	if code, _ = do(r, "GET", "/api/v1/upgrade/progress/nope", ""); code != 404 {
		t.Fatalf("progress unknown: %d", code)
	}

	// --- engine wired ---------------------------------------------------
	t.Setenv("VKAI_ETC_ROOT", t.TempDir())
	engine := service.UpgradeEngineFuncs{
		CheckFunc: func(context.Context) (service.UpgradeStatus, error) {
			return service.UpgradeStatus{
				Current: "0.3.0", Latest: "0.4.0", UpdateAvailable: true,
				Changelog: "- a change",
			}, nil
		},
		StartFunc: func(_ context.Context, _ string) (string, error) { return "job-42", nil },
		ProgressFunc: func(context.Context, string) (service.UpgradeProgress, error) {
			return service.UpgradeProgress{Step: "download", Percent: 20, Message: "Downloading"}, nil
		},
	}
	r = mount(service.NewUpgradeService(engine, nil, nil))

	if code, body = do(r, "POST", "/api/v1/upgrade/check", ""); code != 200 {
		t.Fatalf("check: %d %v", code, body)
	}
	checked, _ := body["data"].(map[string]any)
	if checked["latest"] != "0.4.0" || checked["update_available"] != true {
		t.Fatalf("check payload: %v", checked)
	}

	// A version a shell would find interesting is rejected before it reaches
	// the release script.
	if code, _ = do(r, "POST", "/api/v1/upgrade/start", `{"version":"0.4.0; id"}`); code != 400 {
		t.Fatalf("injection version: %d", code)
	}

	if code, body = do(r, "POST", "/api/v1/upgrade/start", ""); code != 202 {
		t.Fatalf("start: %d %v", code, body)
	}
	started, _ := body["data"].(map[string]any)
	if started["job_id"] != "job-42" || started["to_version"] != "0.4.0" {
		t.Fatalf("start payload: %v", started)
	}

	// A second upgrade while one is running is refused, not queued.
	if code, _ = do(r, "POST", "/api/v1/upgrade/start", ""); code != 409 {
		t.Fatalf("second start: %d", code)
	}

	if code, body = do(r, "GET", "/api/v1/upgrade/progress/job-42", ""); code != 200 {
		t.Fatalf("progress: %d %v", code, body)
	}
	progress, _ := body["data"].(map[string]any)
	if progress["step_key"] != "download" || progress["running"] != true {
		t.Fatalf("progress payload: %v", progress)
	}
	t.Logf("progress payload: %v", progress)

	// While a job runs, no upgrade is on offer.
	if code, body = do(r, "GET", "/api/v1/upgrade/status", ""); code != 200 {
		t.Fatalf("status: %d", code)
	}
	live, _ := body["data"].(map[string]any)
	if live["update_available"] != false {
		t.Fatalf("update offered during a job: %v", live)
	}
}
