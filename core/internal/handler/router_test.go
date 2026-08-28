package handler

// The test that was missing.
//
// Four security features - two-factor authentication, the agent mTLS channel,
// the layered credential limiter and the upgrade engine - were written, tested
// and merged while none of them was reachable. Every one of those tests built
// its own gin engine, registered the routes it wanted, and then proved they
// worked. That proves a route CAN be mounted. It says nothing about whether it
// IS mounted, and for four features in a row the answer was no.
//
// So this file tests the one thing those tests could not: the engine that
// cmd/api/main.go actually serves. It builds a router through the real
// NewRouter, with the same wiring main.go uses, and asserts against the route
// table gin resolved and against responses driven through the whole middleware
// chain. A future deletion - of the two-factor mount, of the agent PKI mount,
// of the credential guard - fails here, and it fails by name.
//
// Two rules for anything added below:
//
//   - Assert through the real NewRouter. A route registered by the test itself
//     is exactly the mistake this file exists to catch.
//   - Assert behaviour, not variables. "The guard is installed" means a request
//     driven through the engine is refused, not that a field is non-nil.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
)

// buildRouter builds the panel engine the way cmd/api/main.go builds it.
//
// The handlers that need a database are passed as nil: registration takes a
// method value, never a call, so a nil handler still produces exactly the same
// route table as a live one. The three that this file is about are real:
//
//   - the two-factor handler, built by the same constructor main.go uses
//     (NewTwoFactorHandler, which NewTwoFactorHandlerFromDB ends in), so the
//     two-factor paths in the table come from RegisterTwoFactorRoutes and not
//     from the 503 fallback;
//   - the agent PKI handler, with a real certificate authority in a temporary
//     directory, so the agent routes come from RegisterAgentPKIRoutes;
//   - the credential limiter, installed process-wide before the router is
//     built, because ProtectCredentialEndpoints captures it once.
//
// The limiter policy is the shipped one with the numbers turned down, so a
// test does not have to sit through the real progressive delay. The delay
// itself is asserted in internal/middleware.
func buildRouter(t *testing.T, policy ratelimit.Policy) *gin.Engine {
	t.Helper()
	return buildRouterWith(t, ratelimit.New(ratelimit.NewMemoryStore(), policy))
}

// buildRouterWith builds the engine around a limiter the caller holds, so a
// test can drive that limiter directly.
func buildRouterWith(t *testing.T, guard *ratelimit.Guard) *gin.Engine {
	t.Helper()
	return buildEngine(t, guard, NewTwoFactorHandler(nil, zap.NewNop()))
}

// buildEngine is the builder proper. The two-factor handler is the caller's to
// choose, so a test can also build the degraded panel - the one whose master
// key is missing - and check what that serves.
func buildEngine(t *testing.T, guard *ratelimit.Guard, twoFactor *TwoFactorHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Nothing in this file may write to the panel's real log tree.
	t.Setenv(middleware.EnvAuthLog, filepath.Join(t.TempDir(), "auth.log"))
	// The constant-time floor pads every failure to 250ms. It is asserted in
	// internal/middleware; here it would only make the file slow.
	t.Setenv(middleware.EnvAuthMinResponse, "0")

	logger := zap.NewNop()
	jwtManager := auth.NewJWTManager("router-test-secret", time.Hour, 24*time.Hour, "vkai-router-test")

	// Installed before the router is built: ProtectCredentialEndpoints
	// captures the limiter once, when the middleware is constructed.
	middleware.SetCredentialLimiter(guard)

	authority, err := agentpki.New(agentpki.Options{Dir: t.TempDir(), Logger: logger})
	if err != nil {
		t.Fatalf("could not open a certificate authority for the test: %v", err)
	}

	router := NewRouter(
		NewAuthHandler(nil, logger), // real handler, nil service: Login binds before it calls anything
		nil, nil, nil,
		NewHealthHandler(logger),
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		twoFactor,
		NewAgentPKIHandler(authority, jwtManager, logger),
		jwtManager,
		logger,
	)

	return router.Setup()
}

// testPolicy is the shipped policy with the counts and the delay turned down.
func testPolicy() ratelimit.Policy {
	policy := ratelimit.DefaultPolicy()
	policy.PairFreeAttempts = 1
	policy.PairLockThreshold = 3
	policy.BaseDelay = time.Millisecond
	policy.MaxDelay = 2 * time.Millisecond
	return policy
}

// routeTable is method+path for every route gin resolved, as a set.
func routeTable(engine *gin.Engine) map[string]bool {
	table := make(map[string]bool)
	for _, route := range engine.Routes() {
		table[route.Method+" "+route.Path] = true
	}
	return table
}

// TestNewRouterMountsTheDefences is the route table assertion, one case per
// route. A deletion anywhere in Setup fails the case that names the route.
func TestNewRouterMountsTheDefences(t *testing.T) {
	table := routeTable(buildRouter(t, testPolicy()))

	cases := []struct {
		name   string
		method string
		path   string
		want   bool
		why    string
	}{
		// Two-factor authentication. These are the four the settings page
		// calls, plus the two it will call, and all six 404ed while the
		// feature was merged but unmounted.
		{"two-factor status", http.MethodGet, "/api/v1/two-factor/status", true,
			"the settings page reads the badge from here; a 404 makes it say the feature does not exist"},
		{"two-factor enrolment start", http.MethodPost, "/api/v1/two-factor/enroll", true,
			"nobody can enrol without it"},
		{"two-factor enrolment confirmation", http.MethodPost, "/api/v1/two-factor/enroll/verify", true,
			"this is the only call that turns two-factor on"},
		{"two-factor verification", http.MethodPost, "/api/v1/two-factor/verify", true,
			"the login gate has nothing to call without it"},
		{"two-factor recovery codes", http.MethodPost, "/api/v1/two-factor/recovery-codes", true,
			"an operator who loses their phone needs a way back in"},
		{"two-factor removal", http.MethodPost, "/api/v1/two-factor/disable", true,
			"a second factor that cannot be turned off is a lockout waiting to happen"},

		// The agent mTLS channel.
		{"agent PKI CA certificate", http.MethodGet, "/api/v1/agent-pki/ca", true,
			"an agent needs the trust anchor before it can talk to the panel"},
		{"agent PKI enrolment", http.MethodPost, "/api/v1/agent-pki/enrol", true,
			"this is the door that replaces the static agent token"},
		{"agent PKI renewal", http.MethodPost, "/api/v1/agent-pki/renew", true,
			"without it every agent certificate expires and the fleet goes silent"},
		{"agent PKI status", http.MethodPost, "/api/v1/agent-pki/status", true,
			"this is the signed heartbeat that replaced the shared-secret one"},
		{"agent PKI enrolment minting", http.MethodPost, "/api/v1/agent-pki/enrolments", true,
			"an operator mints the one-time token here"},
		{"agent PKI agent list", http.MethodGet, "/api/v1/agent-pki/agents", true,
			"an operator has to be able to see what is enrolled"},
		{"agent PKI revocation", http.MethodPost, "/api/v1/agent-pki/agents/:agent_id/revoke", true,
			"revocation is the only answer to a compromised agent"},
		{"agent PKI deny list", http.MethodGet, "/api/v1/agent-pki/deny-list", true,
			"an operator has to be able to see what is refused"},

		// The stubs. These were unauthenticated, outside every middleware, and
		// their bodies were a TODO comment. They must not come back: a route
		// that accepts anything from anybody and answers 200 having done
		// nothing reads as implemented.
		{"the unauthenticated agent registration stub", http.MethodPost, "/api/v1/agent/register", false,
			"it was an empty unauthenticated handler; the real door is POST /api/v1/agent-pki/enrol"},
		{"the unauthenticated agent heartbeat stub", http.MethodPost, "/api/v1/agent/heartbeat", false,
			"it was an empty unauthenticated handler; the real one is POST /api/v1/agent-pki/status"},

		// Controls: if these ever fail, the whole table is being read wrong.
		{"login", http.MethodPost, "/api/v1/auth/login", true, "the panel has to be usable"},
		{"health", http.MethodGet, "/health", true, "the process has to be probeable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.method + " " + tc.path
			got := table[key]
			switch {
			case tc.want && !got:
				t.Fatalf("%s is not mounted by NewRouter, and it must be: %s.\n"+
					"Registering it in a test's own gin engine is not mounting it.", key, tc.why)
			case !tc.want && got:
				t.Fatalf("%s is mounted by NewRouter, and it must not be: %s.", key, tc.why)
			}
		})
	}
}

// TestTwoFactorRoutesSitBehindAuthentication proves the two-factor group was
// mounted on the authenticated group rather than beside it. Every route acts on
// the caller's own account and takes the user id from the validated token, so a
// route reachable without one would let a stranger read - or disable - an
// administrator's second factor.
func TestTwoFactorRoutesSitBehindAuthentication(t *testing.T) {
	engine := buildRouter(t, testPolicy())

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/two-factor/status"},
		{http.MethodPost, "/api/v1/two-factor/enroll"},
		{http.MethodPost, "/api/v1/two-factor/enroll/verify"},
		{http.MethodPost, "/api/v1/two-factor/recovery-codes"},
		{http.MethodPost, "/api/v1/two-factor/disable"},
	}

	// One source address per case. The credential guard covers this group -
	// which the next test asserts - so cases sharing an address would walk it
	// into a lockout and this test would be reporting that instead.
	for i, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = fmt.Sprintf("198.51.100.%d:44000", 11+i)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			switch recorder.Code {
			case http.StatusUnauthorized:
				// What it must be: the route exists and the session gate ran.
			case http.StatusNotFound:
				t.Fatalf("%s %s answered 404: the two-factor routes are not mounted",
					tc.method, tc.path)
			default:
				t.Fatalf("%s %s answered %d without a token, want 401: the route is mounted "+
					"outside the authenticated group", tc.method, tc.path, recorder.Code)
			}
		})
	}
}

// TestCredentialGuardRefusesGuessingOnTheRealEngine drives real requests
// through the engine until the limiter stops them. It asserts the guard is
// installed by making it act, not by reading a variable off the router: the
// bug this file exists to catch was a defence that was constructed, tested and
// never reached by a request.
func TestCredentialGuardRefusesGuessingOnTheRealEngine(t *testing.T) {
	policy := testPolicy()
	engine := buildRouter(t, policy)

	// The two-factor group is guarded by an entry the router passes to
	// ProtectCredentialEndpoints: this panel serves the second factor at
	// /api/v1/two-factor, which the default table does not cover.
	//
	// Each attempt is refused 401 by the session gate, which the guard
	// classifies as a failed credential attempt - so repeated attempts walk
	// the pair counter up to the lock exactly as guessed codes would.
	attempt := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/two-factor/verify",
			strings.NewReader(`{"username":"operator","code":"000000"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.31:41000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder.Code
	}

	for i := 0; i < policy.PairLockThreshold; i++ {
		if code := attempt(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d on /api/v1/two-factor/verify answered %d, want 401", i+1, code)
		}
	}
	if code := attempt(); code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d on /api/v1/two-factor/verify answered %d, want 429: the credential "+
			"guard is not installed on the engine NewRouter builds, or it does not cover "+
			"/api/v1/two-factor", policy.PairLockThreshold+1, code)
	}
}

// TestCredentialGuardCoversTheLoginRoute proves the guard sits in front of
// /api/v1/auth/login on the real engine, without depending on the login
// handler: the pair is locked directly on the process-wide limiter, and the
// request that follows must be refused before it reaches anything.
func TestCredentialGuardCoversTheLoginRoute(t *testing.T) {
	policy := testPolicy()

	// The engine is built around this limiter, so the test can lock a pair on
	// it directly rather than having to make the login handler fail.
	guard := ratelimit.New(ratelimit.NewMemoryStore(), policy)
	engine := buildRouterWith(t, guard)

	subject := ratelimit.NewSubject("192.0.2.77", "operator")
	for i := 0; i <= policy.PairLockThreshold; i++ {
		if _, err := guard.RecordFailure(context.Background(), middleware.ScopeLogin, subject); err != nil {
			t.Fatalf("could not record a failure on the limiter: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"operator","password":"guess"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.77:52000"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("POST /api/v1/auth/login answered %d for a locked pair, want 429: the credential "+
			"guard is not in front of the login route on the engine NewRouter builds", recorder.Code)
	}
	if retry := recorder.Header().Get("Retry-After"); retry == "" {
		t.Error("the refusal carried no Retry-After header, so a legitimate client cannot tell " +
			"when to come back")
	}
}

// TestAuthRateLimitIsNotInstalledOnTheAuthGroup pins the removal of the second
// limiter. AuthRateLimit was a hard cutoff at five REQUESTS per address per
// fifteen minutes: with it in place the sixth call below would be refused
// whatever it contained, which is both a denial of service against a shared
// address and a ceiling the layered limiter's behaviour could never be seen
// through.
//
// A malformed body is used deliberately. It is rejected by the handler's
// binding with 400, which the guard classifies as neither success nor failure,
// so nothing here is counted by the limiter that is supposed to be there - and
// anything that answers 429 can only be a request counter that should not be.
func TestAuthRateLimitIsNotInstalledOnTheAuthGroup(t *testing.T) {
	engine := buildRouter(t, testPolicy())

	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"not":"a login"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.90:53000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)

		if recorder.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d to /api/v1/auth/login was refused 429 without a single failed "+
				"credential attempt: a request-counting limiter (middleware.AuthRateLimit) is "+
				"installed alongside the credential guard", i+1)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("request %d to /api/v1/auth/login answered %d, want 400", i+1, recorder.Code)
		}
	}
}

// TestOrdinaryRoutesAreUntouchedByTheGuard is the other half of the credential
// guard's contract: it costs a map lookup on everything that is not a
// credential endpoint. A guard that counted ordinary traffic would lock
// operators out of their own panel, which is how a defence gets switched off.
func TestOrdinaryRoutesAreUntouchedByTheGuard(t *testing.T) {
	engine := buildRouter(t, testPolicy())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.120:54000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d to /health answered %d, want 200: the credential guard is "+
				"counting ordinary traffic", i+1, recorder.Code)
		}
	}
}

// TestTwoFactorPathsAnswerWhenTheServiceCannotBeBuilt covers the degraded
// panel: no VKAI_SECRET_KEY, so no secret box, so no two-factor service.
//
// The paths must still be mounted. A 404 tells the settings page that this
// panel has no such feature, and an operator who is told that concludes their
// account does not need a second factor; a 503 naming the missing key is a
// configuration error somebody can fix.
func TestTwoFactorPathsAnswerWhenTheServiceCannotBeBuilt(t *testing.T) {
	engine := buildEngine(t, ratelimit.New(ratelimit.NewMemoryStore(), testPolicy()), nil)

	table := routeTable(engine)
	if !table["POST /api/v1/two-factor/*path"] || !table["GET /api/v1/two-factor/*path"] {
		t.Fatalf("a panel that cannot build the two-factor service serves no two-factor paths " +
			"at all, so the settings page gets 404s that read as \"this panel has no two-factor\"")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/two-factor/status", nil)
	req.RemoteAddr = "198.51.100.200:45000"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("GET /api/v1/two-factor/status answered 404 on a panel without the master key")
	}
}
