package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/version"
)

// TestHealthPathsAreRegistered guards the promise the access gate makes.
//
// internal/middleware.PanelGuard answers both /health and /api/v1/health
// without the security entrance, so both must exist: a probe path that is let
// through the gate only to hit a 404 is worse than no promise at all.
func TestHealthPathsAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerHealthRoutes(engine, NewHealthHandler(nil))

	for _, path := range []string{HealthPath, HealthAliasPath} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, rec.Code)
		}

		var body struct {
			Data struct {
				Status  string `json:"status"`
				Version string `json:"version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: %v (body %s)", path, err, rec.Body.String())
		}
		if body.Data.Status != "healthy" {
			t.Fatalf("GET %s: status = %q, want healthy (body %s)", path, body.Data.Status, rec.Body.String())
		}

		// The version used to be the literal "1.0.0", so a panel built from
		// VERSION 0.5.0 reported a release that did not exist and the upgrade
		// check compared against it.
		if body.Data.Version != version.Version {
			t.Fatalf("GET %s: version = %q, want the version of this build (%q)",
				path, body.Data.Version, version.Version)
		}
	}
}
