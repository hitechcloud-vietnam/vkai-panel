package handler

// Scoped API keys, key rotation, key revocation and session binding, driven
// through the engine the panel actually serves.
//
// Every assertion below goes through the real NewRouter, the real middleware
// chain, the real registration function and the real services. A test that
// registers its own routes on its own engine proves the code CAN work; this
// codebase has shipped four features that could work and did not, each with a
// green test of exactly that kind.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

const (
	boundAddress   = "203.0.113.10"
	nearbyAddress  = "203.0.113.240"
	remoteAddress  = "198.51.100.4"
	browserAgent   = "Mozilla/5.0 (X11; Linux x86_64) Chrome/126.0.6478.127 Safari/537.36"
	operatorSecret = "correct horse battery staple"
)

// accessPanel is the whole running surface for one test.
type accessPanel struct {
	t        *testing.T
	handler  http.Handler
	router   *Router
	keys     *service.APIKeyService
	sessions *service.SessionService
	store    *fakeSessionStore
	jwt      *auth.JWTManager
	tenantID uuid.UUID
	userID   uuid.UUID
	token    string
	adminTok string
	clock    time.Time
}

// newAccessPanel builds the panel the way cmd/api/main.go builds it: the real
// router, the access routes mounted through RegisterAccessRoutes, and the
// engine wrapped in the session binding gate.
func newAccessPanel(t *testing.T) *accessPanel {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Nothing here may write to the panel's real log tree, and the
	// constant-time floor would only make this file slow.
	t.Setenv(middleware.EnvAuthLog, filepath.Join(t.TempDir(), "auth.log"))
	t.Setenv(middleware.EnvAuthMinResponse, "0")
	t.Setenv("VKAI_SECRET_KEY", strings.Repeat("7c", 32))

	logger := zap.NewNop()
	policy := ratelimit.DefaultPolicy()
	policy.PairFreeAttempts = 50
	policy.PairLockThreshold = 500
	policy.AddressLimit = 5000
	policy.AccountLimit = 5000
	policy.BaseDelay = 0
	policy.MaxDelay = 0
	middleware.SetCredentialLimiter(ratelimit.New(ratelimit.NewMemoryStore(), policy))

	tenantID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	keyService := service.NewAPIKeyServiceWithStore(newFakeKeyStore(), &fakeAuthority{
		roles: []string{"operator"},
		permissions: []string{
			"website.read", "website.write",
			"database.read", "database.write",
			"dns.read",
			"ssl.read",
			// Deliberately NOT dns.write: it is what
			// TestKeyCannotExceedTheAuthorityOfItsOwner turns on.
		},
	}, nil, logger)
	keyService.SetClock(func() time.Time { return now })

	passwordHash, err := utils.HashPassword(operatorSecret)
	if err != nil {
		t.Fatalf("hash the operator password: %v", err)
	}

	sessionStore := newFakeSessionStore()
	sessionService := service.NewSessionServiceWithStore(sessionStore,
		&fakeUsers{user: &models.User{ID: userID, TenantID: tenantID, PasswordHash: passwordHash}},
		nil, logger)
	sessionService.SetPolicy(auth.DefaultBindingPolicy())

	sessionHandler := NewSessionHandler(sessionService, logger)
	SetSessionHandler(sessionHandler)
	t.Cleanup(func() { SetSessionHandler(nil) })

	jwtManager := auth.NewJWTManager("access-routes-test-secret", time.Hour, 24*time.Hour, "vkai-access-test")

	router := NewRouter(
		NewAuthHandler(nil, logger),
		nil, nil, nil,
		NewHealthHandler(logger),
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		NewAPIKeyHandler(keyService, logger),
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil,
		jwtManager,
		logger,
	)

	engine := router.Setup()

	// The line cmd/api/main.go carries, and the line router.go must carry.
	router.RegisterAccessRoutes()

	panel := &accessPanel{
		t:        t,
		router:   router,
		keys:     keyService,
		sessions: sessionService,
		store:    sessionStore,
		jwt:      jwtManager,
		tenantID: tenantID,
		userID:   userID,
		clock:    now,
		handler: middleware.BindSessions(engine, middleware.SessionGuardOptions{
			Evaluator: sessionService,
			JWT:       jwtManager,
			Logger:    logger,
		}),
	}

	panel.token = panel.mintToken(userID, []string{"operator"},
		[]string{"user.read", "user.write"})
	panel.adminTok = panel.mintToken(uuid.New(), []string{"admin"}, nil)

	return panel
}

func (p *accessPanel) mintToken(userID uuid.UUID, roles, permissions []string) string {
	p.t.Helper()
	pair, err := p.jwt.GenerateTokenPairWithPermissions(userID, p.tenantID,
		"operator", "operator@example.test", roles, permissions)
	if err != nil {
		p.t.Fatalf("mint a token: %v", err)
	}
	return pair.AccessToken
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

type requestOption func(*http.Request)

func withJWT(token string) requestOption {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func withKey(key string) requestOption {
	return func(r *http.Request) { r.Header.Set(middleware.APIKeyHeader, key) }
}

func fromAddress(address string) requestOption {
	return func(r *http.Request) { r.RemoteAddr = address + ":54321" }
}

func withUserAgent(agent string) requestOption {
	return func(r *http.Request) { r.Header.Set("User-Agent", agent) }
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details string `json:"details"`
	} `json:"error"`
}

func (p *accessPanel) do(method, path string, body any, opts ...requestOption) (*httptest.ResponseRecorder, envelope) {
	p.t.Helper()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			p.t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	// A default that every case can override: one browser, one address.
	req.RemoteAddr = boundAddress + ":54321"
	req.Header.Set("User-Agent", browserAgent)
	for _, opt := range opts {
		opt(req)
	}

	recorder := httptest.NewRecorder()
	p.handler.ServeHTTP(recorder, req)

	var out envelope
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &out)
	}
	return recorder, out
}

// createKey mints a key through the HTTP endpoint the panel actually exposes.
func (p *accessPanel) createKey(name string, scopes []string, extra map[string]any) (id uuid.UUID, raw string) {
	p.t.Helper()

	body := map[string]any{"name": name, "scopes": scopes}
	for k, v := range extra {
		body[k] = v
	}

	recorder, env := p.do(http.MethodPost, "/api/v1/api-keys", body, withJWT(p.token))
	if recorder.Code != http.StatusCreated {
		p.t.Fatalf("create key %q: status %d, body %s", name, recorder.Code, recorder.Body.String())
	}

	var created struct {
		ID  uuid.UUID `json:"id"`
		Key string    `json:"key"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		p.t.Fatalf("decode the created key: %v (%s)", err, env.Data)
	}
	if created.Key == "" {
		p.t.Fatal("the created key was not returned; it is only ever shown once and this was that once")
	}
	return created.ID, created.Key
}

// ---------------------------------------------------------------------------
// The routes exist on the engine the panel serves
// ---------------------------------------------------------------------------

func TestAccessRoutesAreMountedOnTheRealRouter(t *testing.T) {
	panel := newAccessPanel(t)

	table := make(map[string]bool)
	for _, route := range panel.router.Engine().Routes() {
		table[route.Method+" "+route.Path] = true
	}

	for _, want := range []struct{ method, path, why string }{
		{http.MethodGet, "/api/v1/sessions",
			"an operator cannot see where they are signed in"},
		{http.MethodDelete, "/api/v1/sessions/:id",
			"an operator cannot end a session, which is the entire feature"},
		{http.MethodPost, "/api/v1/sessions/current/reauthenticate",
			"a session that moved network could never be repaired, only replaced by signing in again"},
		{http.MethodDelete, "/api/v1/sessions/user/:id",
			"an administrator cannot sign a compromised account out"},
		{http.MethodPost, "/api/v1/api-keys/:id/rotate",
			"rotation without downtime is unreachable, so keys never get rotated"},
		{http.MethodPost, "/api/v1/api-keys/:id/revoke",
			"revocation is unreachable"},
		{http.MethodGet, "/api/v1/access/scopes",
			"an interface has no way to offer a scope picker"},
		{http.MethodGet, "/api/v1/integration/whoami",
			"an API key can authenticate to nothing at all"},
		{http.MethodGet, "/api/v1/integration/websites",
			"the scoped surface is empty"},
		{http.MethodDelete, "/api/v1/integration/databases/:id",
			"the scoped surface has no write route to refuse"},
	} {
		if !table[want.method+" "+want.path] {
			t.Errorf("%s %s is not mounted: %s", want.method, want.path, want.why)
		}
	}
}

// TestRegisterAccessRoutesIsIdempotent matters because the call lives in
// cmd/api/main.go today and belongs in router.go. Both may be present at once
// during the move, and gin panics on a duplicate route.
func TestRegisterAccessRoutesIsIdempotent(t *testing.T) {
	panel := newAccessPanel(t)
	before := len(panel.router.Engine().Routes())

	panel.router.RegisterAccessRoutes()
	panel.router.RegisterAccessRoutes()

	if after := len(panel.router.Engine().Routes()); after != before {
		t.Fatalf("a second registration added %d routes; it must be a no-op", after-before)
	}
}

// TestAccessRoutesAreMountedByMain fails if the line that makes any of this
// reachable is deleted from the entry point.
//
// If the mount moves into internal/handler/router.go, replace the assertion
// with one against that file - do not delete it.
func TestAccessRoutesAreMountedByMain(t *testing.T) {
	src := mainSource(t)

	for _, wiring := range []struct{ needle, why string }{
		{"router.RegisterAccessRoutes()",
			"the scoped API key surface, key rotation, key revocation and every session endpoint are not reachable in the running panel"},
		{"handler.SetSessionHandler(sessionHandler)",
			"the session routes would answer 503: no session handler is installed"},
		{"repository.NewPanelSessionRepository(db.DB)",
			"there is no session store, so no session can be ended before its token expires"},
		{"middleware.BindSessions(engine,",
			"session binding is not enforced on any request: a stolen token is usable from anywhere until it expires"},
		{"API:      apiHandler,",
			"the engine is served unwrapped, so the session gate is built and then bypassed"},
	} {
		if !strings.Contains(src, wiring.needle) {
			t.Errorf("cmd/api/main.go no longer contains %q: %s", wiring.needle, wiring.why)
		}
	}
}

// ---------------------------------------------------------------------------
// Scopes
// ---------------------------------------------------------------------------

func TestScopedKeyIsRefusedOutsideItsScope(t *testing.T) {
	panel := newAccessPanel(t)
	_, key := panel.createKey("website reader", []string{"website:read"}, nil)

	// It authenticates, and it can say what it is.
	recorder, env := panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(key))
	if recorder.Code != http.StatusOK {
		t.Fatalf("whoami: status %d, body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(string(env.Data), "website:read") {
		t.Fatalf("whoami did not report the key's scopes: %s", env.Data)
	}

	// Inside its scope it gets past authorisation. The website handler is not
	// wired in this test - nothing behind the gate is - so what is asserted is
	// that the request was NOT refused by the scope gate.
	recorder, env = panel.do(http.MethodGet, "/api/v1/integration/websites", nil, withKey(key))
	if recorder.Code == http.StatusForbidden || recorder.Code == http.StatusUnauthorized {
		t.Fatalf("a key scoped website:read was refused on GET /integration/websites: %d %s",
			recorder.Code, recorder.Body.String())
	}

	// A different module: this is the case the whole feature exists for.
	recorder, env = panel.do(http.MethodGet, "/api/v1/integration/databases", nil, withKey(key))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a key scoped website:read reached the databases endpoint: %d %s",
			recorder.Code, recorder.Body.String())
	}
	if env.Error == nil || env.Error.Code != "SCOPE_REQUIRED" {
		t.Fatalf("the refusal did not name the missing scope: %s", recorder.Body.String())
	}

	// A write with a read-only scope, on the module the key IS scoped for.
	recorder, env = panel.do(http.MethodDelete,
		"/api/v1/integration/websites/"+uuid.New().String(), nil, withKey(key))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a read-only key deleted through a write route: %d %s",
			recorder.Code, recorder.Body.String())
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "website:write") {
		t.Fatalf("the refusal did not name website:write: %s", recorder.Body.String())
	}
}

// TestEveryIntegrationRouteDeclaresAScope walks the route table and drives
// every scoped route with a key holding one scope that no route uses. Anything
// that answers something other than a scope refusal is authenticated but
// unscoped - which is a route an API key can reach with no limit at all.
func TestEveryIntegrationRouteDeclaresAScope(t *testing.T) {
	panel := newAccessPanel(t)
	_, key := panel.createKey("unrelated scope", []string{"terminal:read"}, nil)

	prefix := "/api/v1" + IntegrationPrefix
	checked := 0

	for _, route := range panel.router.Engine().Routes() {
		if !strings.HasPrefix(route.Path, prefix) {
			continue
		}
		if route.Path == prefix+"/whoami" {
			// Deliberately unscoped: an integration being refused has to be
			// able to find out what it holds.
			continue
		}

		path := strings.NewReplacer(
			":id", uuid.New().String(),
			":domainId", uuid.New().String(),
		).Replace(route.Path)

		recorder, env := panel.do(route.Method, path, map[string]any{}, withKey(key))
		checked++

		if recorder.Code != http.StatusForbidden || env.Error == nil || env.Error.Code != "SCOPE_REQUIRED" {
			t.Errorf("%s %s does not declare a scope: a key holding only terminal:read got %d %s",
				route.Method, route.Path, recorder.Code, recorder.Body.String())
		}
	}

	if checked == 0 {
		t.Fatal("no scoped integration routes were found; the surface is not mounted")
	}
}

func TestKeyWithNoUsableScopesAuthorisesNothing(t *testing.T) {
	panel := newAccessPanel(t)

	// The creation endpoint refuses an empty grant outright.
	recorder, _ := panel.do(http.MethodPost, "/api/v1/api-keys",
		map[string]any{"name": "empty", "scopes": []string{}}, withJWT(panel.token))
	if recorder.Code == http.StatusCreated {
		t.Fatal("a key with no scopes was created; such a key must not exist")
	}

	// And an unknown module is refused rather than silently ignored.
	recorder, _ = panel.do(http.MethodPost, "/api/v1/api-keys",
		map[string]any{"name": "typo", "scopes": []string{"websites:read"}}, withJWT(panel.token))
	if recorder.Code == http.StatusCreated {
		t.Fatal("a scope naming a module that does not exist was accepted")
	}
}

// TestKeyCannotExceedTheAuthorityOfItsOwner is the second half of the check.
//
// The account this key belongs to holds dns.read and not dns.write. A key
// scoped dns:write therefore reads DNS and cannot change it - and nobody had to
// re-scope the key when the account lost the permission, which is the point: an
// account demoted today does not leave keys behind that are still promoted.
func TestKeyCannotExceedTheAuthorityOfItsOwner(t *testing.T) {
	panel := newAccessPanel(t)
	_, key := panel.createKey("over-scoped", []string{"dns:write"}, nil)

	recorder, _ := panel.do(http.MethodGet, "/api/v1/integration/dns/zones", nil, withKey(key))
	if recorder.Code == http.StatusForbidden || recorder.Code == http.StatusUnauthorized {
		t.Fatalf("dns:write should still permit a read the owner is entitled to: %d %s",
			recorder.Code, recorder.Body.String())
	}

	recorder, env := panel.do(http.MethodPost, "/api/v1/integration/dns/zones",
		map[string]any{}, withKey(key))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a key wrote DNS through an account that may not: %d %s",
			recorder.Code, recorder.Body.String())
	}
	if env.Error == nil || env.Error.Code != "FORBIDDEN" {
		t.Fatalf("the refusal came from the scope check rather than from the owner's authority: %s",
			recorder.Body.String())
	}
	if !strings.Contains(env.Error.Message, "dns.write") {
		t.Fatalf("the refusal did not name the permission the account lacks: %s", recorder.Body.String())
	}
}

func TestKeyPinnedToANetworkIsRefusedFromElsewhere(t *testing.T) {
	panel := newAccessPanel(t)
	_, key := panel.createKey("pinned", []string{"website:read"},
		map[string]any{"allowed_cidrs": []string{boundAddress + "/32"}})

	recorder, _ := panel.do(http.MethodGet, "/api/v1/integration/whoami", nil,
		withKey(key), fromAddress(boundAddress))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the key was refused from the address it is pinned to: %d %s",
			recorder.Code, recorder.Body.String())
	}

	recorder, _ = panel.do(http.MethodGet, "/api/v1/integration/whoami", nil,
		withKey(key), fromAddress(remoteAddress))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("the key worked from an address outside its allow list: %d %s",
			recorder.Code, recorder.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Rotation
// ---------------------------------------------------------------------------

func TestRotationKeepsBothKeysValidThroughTheOverlap(t *testing.T) {
	panel := newAccessPanel(t)
	id, oldKey := panel.createKey("deploy robot", []string{"website:read"}, nil)

	// The key works before anything happens to it.
	if recorder, _ := panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(oldKey)); recorder.Code != http.StatusOK {
		t.Fatalf("the key did not work before rotation: %d", recorder.Code)
	}

	recorder, env := panel.do(http.MethodPost,
		"/api/v1/api-keys/"+id.String()+"/rotate",
		map[string]any{"overlap_hours": 2}, withJWT(panel.token))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("rotate: status %d, body %s", recorder.Code, recorder.Body.String())
	}

	var rotated struct {
		ID                    uuid.UUID  `json:"id"`
		Key                   string     `json:"key"`
		Scopes                []string   `json:"scopes"`
		ReplacedKeyID         *uuid.UUID `json:"replaced_key_id"`
		ReplacedKeyValidUntil *time.Time `json:"replaced_key_valid_until"`
	}
	if err := json.Unmarshal(env.Data, &rotated); err != nil {
		t.Fatalf("decode the replacement: %v (%s)", err, env.Data)
	}
	if rotated.Key == "" || rotated.Key == oldKey {
		t.Fatal("rotation did not produce a new key")
	}
	if rotated.ReplacedKeyID == nil || *rotated.ReplacedKeyID != id {
		t.Fatal("the response does not say which key was replaced")
	}
	if rotated.ReplacedKeyValidUntil == nil {
		t.Fatal("the response does not say how long the old key has left, which is the only thing the operator needs")
	}
	if len(rotated.Scopes) != 1 || rotated.Scopes[0] != "website:read" {
		t.Fatalf("the replacement did not inherit the grant: %v", rotated.Scopes)
	}

	// The overlap. This is the property that makes rotation something an
	// operator will actually do: twelve machines can be updated over an
	// afternoon instead of in the same second.
	for name, key := range map[string]string{"the retiring key": oldKey, "the replacement": rotated.Key} {
		recorder, _ := panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(key))
		if recorder.Code != http.StatusOK {
			t.Fatalf("during the overlap %s did not work: %d %s", name, recorder.Code, recorder.Body.String())
		}
	}

	// Walk past the deadline.
	panel.keys.SetClock(func() time.Time { return panel.clock.Add(3 * time.Hour) })

	recorder, _ = panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(oldKey))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("the retired key still worked after the overlap ended: %d %s",
			recorder.Code, recorder.Body.String())
	}

	recorder, _ = panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(rotated.Key))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the replacement stopped working when the old key expired: %d %s",
			recorder.Code, recorder.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Revocation
// ---------------------------------------------------------------------------

func TestRevocationTakesEffectOnTheNextRequest(t *testing.T) {
	panel := newAccessPanel(t)
	id, key := panel.createKey("leaked", []string{"website:read"}, nil)

	if recorder, _ := panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(key)); recorder.Code != http.StatusOK {
		t.Fatalf("the key did not work before revocation: %d", recorder.Code)
	}

	recorder, _ := panel.do(http.MethodPost, "/api/v1/api-keys/"+id.String()+"/revoke",
		map[string]any{"reason": "found in a public repository"}, withJWT(panel.token))
	if recorder.Code != http.StatusOK {
		t.Fatalf("revoke: status %d, body %s", recorder.Code, recorder.Body.String())
	}

	// The very next request. Not at the next expiry, not after a cache
	// timeout.
	recorder, _ = panel.do(http.MethodGet, "/api/v1/integration/whoami", nil, withKey(key))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a revoked key still authenticated: %d %s", recorder.Code, recorder.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func TestSessionIsEstablishedAndListed(t *testing.T) {
	panel := newAccessPanel(t)

	recorder, env := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list sessions: %d %s", recorder.Code, recorder.Body.String())
	}

	var listed struct {
		Sessions []models.SessionView `json:"sessions"`
		Policy   struct {
			IPBinding     string `json:"ip_binding"`
			DeviceBinding bool   `json:"device_binding"`
		} `json:"policy"`
	}
	if err := json.Unmarshal(env.Data, &listed); err != nil {
		t.Fatalf("decode the session list: %v (%s)", err, env.Data)
	}
	if len(listed.Sessions) != 1 {
		t.Fatalf("expected exactly the session making the request, got %d", len(listed.Sessions))
	}
	if !listed.Sessions[0].Current {
		t.Fatal("the session making the request was not marked as the current one; an operator would not know which one they are about to end")
	}
	if listed.Sessions[0].OriginIP != boundAddress {
		t.Fatalf("the session was bound to %q, not to the address it was used from", listed.Sessions[0].OriginIP)
	}
	if listed.Policy.IPBinding != auth.IPBindingNetwork || !listed.Policy.DeviceBinding {
		t.Fatalf("the policy reported to the operator is %+v", listed.Policy)
	}
}

func TestSessionFromADifferentAddress(t *testing.T) {
	panel := newAccessPanel(t)

	// Establish the session where it belongs.
	if recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token)); recorder.Code != http.StatusOK {
		t.Fatalf("establish: %d", recorder.Code)
	}

	t.Run("a neighbouring address in the same block is nothing", func(t *testing.T) {
		recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil,
			withJWT(panel.token), fromAddress(nearbyAddress))
		if recorder.Code != http.StatusOK {
			t.Fatalf("a NAT pool moving the client one address along ended the session: %d %s",
				recorder.Code, recorder.Body.String())
		}
	})

	t.Run("reading from a different network continues", func(t *testing.T) {
		recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil,
			withJWT(panel.token), fromAddress(remoteAddress))
		if recorder.Code != http.StatusOK {
			t.Fatalf("a phone moving from wifi to mobile data lost the dashboard: %d %s",
				recorder.Code, recorder.Body.String())
		}
	})

	t.Run("writing from a different network waits for the password", func(t *testing.T) {
		recorder, env := panel.do(http.MethodDelete, "/api/v1/sessions/"+uuid.New().String(), nil,
			withJWT(panel.token), fromAddress(remoteAddress))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("a state-changing request from a moved session was allowed: %d %s",
				recorder.Code, recorder.Body.String())
		}
		if env.Error == nil || env.Error.Code != middleware.SessionReauthCode {
			t.Fatalf("the refusal did not ask for a password: %s", recorder.Body.String())
		}
		if env.Error != nil && !strings.Contains(env.Error.Details, middleware.ReauthenticatePath) {
			t.Fatalf("the refusal did not say where to prove the password: %s", recorder.Body.String())
		}
	})

	t.Run("a different device is refused outright", func(t *testing.T) {
		fresh := newAccessPanel(t)
		if recorder, _ := fresh.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(fresh.token)); recorder.Code != http.StatusOK {
			t.Fatalf("establish: %d", recorder.Code)
		}

		recorder, env := fresh.do(http.MethodGet, "/api/v1/sessions", nil,
			withJWT(fresh.token), withUserAgent("curl/8.5.0"))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("the token was replayed by different tooling and worked: %d %s",
				recorder.Code, recorder.Body.String())
		}
		if env.Error == nil || env.Error.Code != middleware.SessionEndedCode {
			t.Fatalf("the refusal did not say the session is over: %s", recorder.Body.String())
		}

		// And the session is over for the real device too: a token seen on two
		// devices is a token that has been copied.
		recorder, _ = fresh.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(fresh.token))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("the session survived a device change: %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("strict mode refuses any address change", func(t *testing.T) {
		strict := newAccessPanel(t)
		strict.sessions.SetPolicy(auth.BindingPolicy{IPMode: auth.IPBindingStrict, DeviceBinding: true})

		if recorder, _ := strict.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(strict.token)); recorder.Code != http.StatusOK {
			t.Fatalf("establish: %d", recorder.Code)
		}

		recorder, env := strict.do(http.MethodGet, "/api/v1/sessions", nil,
			withJWT(strict.token), fromAddress(nearbyAddress))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("strict mode tolerated an address change: %d %s", recorder.Code, recorder.Body.String())
		}
		if env.Error == nil || env.Error.Code != middleware.SessionEndedCode {
			t.Fatalf("strict mode did not end the session: %s", recorder.Body.String())
		}
	})
}

func TestReauthenticationRebindsAMovedSessionInsteadOfEndingIt(t *testing.T) {
	panel := newAccessPanel(t)

	if recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token)); recorder.Code != http.StatusOK {
		t.Fatalf("establish: %d", recorder.Code)
	}

	// Move, and be told to prove the password.
	recorder, _ := panel.do(http.MethodDelete, "/api/v1/sessions/"+uuid.New().String(), nil,
		withJWT(panel.token), fromAddress(remoteAddress))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected the write to be held: %d", recorder.Code)
	}

	// A wrong password changes nothing.
	recorder, _ = panel.do(http.MethodPost, middleware.ReauthenticatePath,
		map[string]any{"password": "not the password"},
		withJWT(panel.token), fromAddress(remoteAddress))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong password was accepted: %d %s", recorder.Code, recorder.Body.String())
	}

	// The right one re-binds the session to where it is now.
	recorder, _ = panel.do(http.MethodPost, middleware.ReauthenticatePath,
		map[string]any{"password": operatorSecret},
		withJWT(panel.token), fromAddress(remoteAddress))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the password was refused: %d %s", recorder.Code, recorder.Body.String())
	}

	// The same session - the same token - can now write from the new address.
	recorder, env := panel.do(http.MethodDelete, "/api/v1/sessions/"+uuid.New().String(), nil,
		withJWT(panel.token), fromAddress(remoteAddress))
	if recorder.Code == http.StatusForbidden && env.Error != nil && env.Error.Code == middleware.SessionReauthCode {
		t.Fatalf("the session was not re-bound: %s", recorder.Body.String())
	}
}

func TestSessionTerminationTakesEffectAtOnce(t *testing.T) {
	panel := newAccessPanel(t)

	recorder, env := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	if recorder.Code != http.StatusOK {
		t.Fatalf("establish: %d", recorder.Code)
	}
	var listed struct {
		Sessions []models.SessionView `json:"sessions"`
	}
	if err := json.Unmarshal(env.Data, &listed); err != nil || len(listed.Sessions) != 1 {
		t.Fatalf("could not read the session list: %v (%s)", err, env.Data)
	}
	sessionID := listed.Sessions[0].ID

	// Ending the session the request is being made with is allowed, and the
	// answer says so.
	recorder, env = panel.do(http.MethodDelete, "/api/v1/sessions/"+sessionID.String(), nil, withJWT(panel.token))
	if recorder.Code != http.StatusOK {
		t.Fatalf("terminate: %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(string(env.Data), `"current_session":true`) {
		t.Fatalf("the answer did not tell the operator they had just signed themselves out: %s", env.Data)
	}

	// The token is still cryptographically valid and still inside its
	// lifetime. It must nonetheless be refused, from the next request onwards.
	// This is the thing a stateless JWT cannot do on its own.
	if _, err := panel.jwt.ValidateAccessToken(panel.token); err != nil {
		t.Fatalf("the token stopped validating on its own, so this proves nothing: %v", err)
	}

	recorder, env = panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a terminated session kept working: %d %s", recorder.Code, recorder.Body.String())
	}
	if env.Error == nil || env.Error.Code != middleware.SessionEndedCode {
		t.Fatalf("the refusal did not name the cause: %s", recorder.Body.String())
	}

	// And it stays ended: the next request must not quietly establish a fresh
	// session for the same token.
	recorder, _ = panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("the ended session came back: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestOneOperatorCannotEndAnothersSession(t *testing.T) {
	panel := newAccessPanel(t)

	if recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token)); recorder.Code != http.StatusOK {
		t.Fatalf("establish: %d", recorder.Code)
	}
	recorder, env := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	var listed struct {
		Sessions []models.SessionView `json:"sessions"`
	}
	_ = json.Unmarshal(env.Data, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("could not read the session list: %s", env.Data)
	}

	// A different account, holding an administrator role, using the
	// self-service endpoint. Self-service is scoped to the caller's own
	// sessions whatever their role.
	recorder, _ = panel.do(http.MethodDelete, "/api/v1/sessions/"+listed.Sessions[0].ID.String(), nil,
		withJWT(panel.adminTok), fromAddress(boundAddress))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("another account ended a session through the self-service endpoint: %d %s",
			recorder.Code, recorder.Body.String())
	}
}

func TestAdministratorCanEndEverySessionOfAnAccount(t *testing.T) {
	panel := newAccessPanel(t)

	if recorder, _ := panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token)); recorder.Code != http.StatusOK {
		t.Fatalf("establish: %d", recorder.Code)
	}

	recorder, _ := panel.do(http.MethodDelete, "/api/v1/sessions/user/"+panel.userID.String(), nil,
		withJWT(panel.adminTok))
	if recorder.Code != http.StatusOK {
		t.Fatalf("administrator termination: %d %s", recorder.Code, recorder.Body.String())
	}

	recorder, _ = panel.do(http.MethodGet, "/api/v1/sessions", nil, withJWT(panel.token))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("the account's session survived: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestScopeCatalogueIsServed(t *testing.T) {
	panel := newAccessPanel(t)

	recorder, env := panel.do(http.MethodGet, "/api/v1/access/scopes", nil, withJWT(panel.token))
	if recorder.Code != http.StatusOK {
		t.Fatalf("scope catalogue: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{"website", "database", "read", "write"} {
		if !strings.Contains(string(env.Data), fmt.Sprintf("%q", want)) {
			t.Errorf("the catalogue does not mention %q: %s", want, env.Data)
		}
	}
}
