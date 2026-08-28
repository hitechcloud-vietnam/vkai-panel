package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// TestEveryCredentialAcceptingRouteIsCovered is the list this task exists to
// close. Each entry is an endpoint that takes a secret from a caller who is
// not yet trusted; a route missing from the table is a route with no brute
// force protection and no authentication log line.
func TestEveryCredentialAcceptingRouteIsCovered(t *testing.T) {
	cases := map[string]string{
		// The login form, as registered in internal/handler/router.go.
		"/api/v1/auth/login": ScopeLogin,

		// Token refresh: a refresh token is a week-long credential.
		"/api/v1/auth/refresh": ScopeRefresh,

		// Two-factor verification, under any of the spellings the roadmap and
		// the UI might land on.
		"/api/v1/auth/2fa/verify":          ScopeTwoFactor,
		"/api/v1/auth/two-factor/verify":   ScopeTwoFactor,
		"/api/v1/auth/mfa/verify":          ScopeTwoFactor,
		"/api/v1/auth/otp/verify":          ScopeTwoFactor,
		"/api/v1/auth/backup-code/consume": ScopeTwoFactor,

		// Password reset, both halves.
		"/api/v1/auth/forgot-password":       ScopePasswordReset,
		"/api/v1/auth/reset-password":        ScopePasswordReset,
		"/api/v1/auth/password-reset/verify": ScopePasswordReset,

		// Agent enrolment: a guessed token here is root on a customer's
		// server, not merely a session on the panel.
		"/api/v1/agent/register":  ScopeAgentEnrol,
		"/api/v1/agent/enroll":    ScopeAgentEnrol,
		"/api/v1/agent/heartbeat": ScopeAgentEnrol,
		"/api/v1/nodes/register":  ScopeAgentEnrol,
	}

	for path, wantScope := range cases {
		scope, ok := resolveCredentialScope(defaultCredentialRoutes, path)
		if !ok {
			t.Errorf("%s is not covered by the credential route table", path)
			continue
		}
		if scope != wantScope {
			t.Errorf("%s resolved to scope %q, want %q", path, scope, wantScope)
		}
	}
}

func TestOrdinaryRoutesAreNotGuarded(t *testing.T) {
	// Guarding these would put a Redis round trip in front of every request
	// and would count an expired session as a credential guess.
	ordinary := []string{
		"/api/v1/auth/me",
		"/api/v1/auth/logout",
		"/api/v1/websites",
		"/api/v1/servers/:id/metrics",
		"/health",
		"",
	}
	for _, path := range ordinary {
		if scope, ok := resolveCredentialScope(defaultCredentialRoutes, path); ok {
			t.Errorf("%s should not be guarded, but resolved to %q", path, scope)
		}
	}
}

func TestExtraRoutesOverrideTheDefaults(t *testing.T) {
	routes := append([]CredentialRoute{
		{Path: "/api/v1/auth/login", Scope: ScopeTwoFactor},
		{Path: "/api/v1/custom/sso", Prefix: true, Scope: ScopeLogin},
	}, defaultCredentialRoutes...)

	if scope, _ := resolveCredentialScope(routes, "/api/v1/auth/login"); scope != ScopeTwoFactor {
		t.Errorf("an explicitly registered route should win, got %q", scope)
	}
	if scope, ok := resolveCredentialScope(routes, "/api/v1/custom/sso/callback"); !ok || scope != ScopeLogin {
		t.Errorf("a new route should be coverable without editing the table, got %q ok=%v", scope, ok)
	}
}

func TestCarriesAPIKey(t *testing.T) {
	cases := []struct {
		name, header, value string
		want                bool
	}{
		{"dedicated header", APIKeyHeader, "vkai_key", true},
		{"ApiKey scheme", "Authorization", "ApiKey vkai_key", true},
		{"api-key scheme", "Authorization", "api-key vkai_key", true},
		{"bearer is the session token, not an API key", "Authorization", "Bearer eyJhbGci", false},
		{"basic", "Authorization", "Basic dXNlcjpwYXNz", false},
		{"nothing", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			var got bool
			engine.GET("/", func(c *gin.Context) { got = carriesAPIKey(c) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			engine.ServeHTTP(httptest.NewRecorder(), req)
			if got != tc.want {
				t.Errorf("carriesAPIKey = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOneLineOfWiringProtectsTheLoginRoute exercises the composition the
// router actually installs: one engine-level middleware, no per-route changes.
func TestOneLineOfWiringProtectsTheLoginRoute(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "auth.log")
	t.Setenv(EnvAuthLog, logPath)

	policy := ratelimit.DefaultPolicy()
	// Small numbers so the test does not have to sit through the real
	// progressive delay; the delay itself is asserted elsewhere.
	policy.PairFreeAttempts = 2
	policy.PairLockThreshold = 3
	SetCredentialLimiter(ratelimit.New(ratelimit.NewMemoryStore(), policy))
	t.Cleanup(func() {
		sharedGuardMu.Lock()
		sharedGuardInst, sharedGuardSet = nil, false
		sharedGuardMu.Unlock()
	})

	engine := gin.New()
	// This is the line that goes into internal/handler/router.go.
	engine.Use(ProtectCredentialEndpoints(nil))
	engine.POST("/api/v1/auth/login", func(c *gin.Context) {
		utils.Unauthorized(c, "user not found")
	})
	engine.GET("/api/v1/websites", func(c *gin.Context) {
		utils.Success(c, gin.H{"websites": []string{}})
	})

	attempt := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"username":"operator","password":"guess"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.240:5000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder.Code
	}

	for i := 0; i < policy.PairLockThreshold; i++ {
		if code := attempt(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, code)
		}
	}
	if code := attempt(); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the pair is locked", code)
	}

	// An ordinary route is untouched and answers at full speed.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/websites", nil)
	req.RemoteAddr = "203.0.113.240:5000"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("an ordinary route got %d, want 200", recorder.Code)
	}

	// And the whole thing landed in the file the fail2ban jail reads.
	written, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("the authentication log was not written to %s: %v", logPath, err)
	}
	lines := strings.Split(strings.TrimRight(string(written), "\n"), "\n")
	if len(lines) != policy.PairLockThreshold+1 {
		t.Fatalf("wrote %d lines, want %d:\n%s", len(lines), policy.PairLockThreshold+1, written)
	}

	failregex, _ := loadFail2banRegexes(t)
	for _, line := range lines {
		if !failregex.MatchString(line) {
			t.Fatalf("the shipped fail2ban filter does not match a line written through the "+
				"real wiring: %q", line)
		}
	}
	if !strings.Contains(lines[len(lines)-1], "outcome=blocked") {
		t.Fatalf("the lockout was not recorded: %q", lines[len(lines)-1])
	}
}
