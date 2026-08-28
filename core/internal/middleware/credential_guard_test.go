package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

func init() { gin.SetMode(gin.TestMode) }

// sleepRecorder replaces the real pause so a test can assert what the guard
// asked for without waiting for it.
type sleepRecorder struct {
	mu     sync.Mutex
	slept  []time.Duration
	actual bool
}

func (s *sleepRecorder) sleep(ctx context.Context, d time.Duration) {
	s.mu.Lock()
	s.slept = append(s.slept, d)
	actual := s.actual
	s.mu.Unlock()
	if actual {
		sleepContext(ctx, d)
	}
}

func (s *sleepRecorder) durations() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Duration, len(s.slept))
	copy(out, s.slept)
	return out
}

func (s *sleepRecorder) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slept = nil
}

type guardHarness struct {
	engine  *gin.Engine
	guard   *ratelimit.Guard
	store   ratelimit.Store
	log     *bytes.Buffer
	sleeper *sleepRecorder
}

// newHarness wires one guarded POST /login route over an in-memory limiter.
func newHarness(t *testing.T, policy ratelimit.Policy, minResponse time.Duration, handler gin.HandlerFunc) *guardHarness {
	t.Helper()

	store := ratelimit.NewMemoryStore()
	return newHarnessWithStore(t, store, policy, minResponse, handler)
}

func newHarnessWithStore(t *testing.T, store ratelimit.Store, policy ratelimit.Policy, minResponse time.Duration, handler gin.HandlerFunc) *guardHarness {
	t.Helper()

	guard := ratelimit.New(store, policy)
	logBuffer := &bytes.Buffer{}
	sleeper := &sleepRecorder{}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("request_id", "test-request")
		c.Next()
	})
	engine.POST("/login", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeLogin,
		Guard:       guard,
		Log:         NewAuthLogger(logBuffer, nil),
		MinResponse: minResponse,
		Resolver:    NewClientIPResolver(nil),
		sleep:       sleeper.sleep,
	}), handler)

	return &guardHarness{engine: engine, guard: guard, store: store, log: logBuffer, sleeper: sleeper}
}

func (h *guardHarness) post(t *testing.T, remoteAddr, username, password string) (*httptest.ResponseRecorder, time.Duration) {
	t.Helper()

	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr

	recorder := httptest.NewRecorder()
	start := time.Now()
	h.engine.ServeHTTP(recorder, req)
	return recorder, time.Since(start)
}

func (h *guardHarness) lines() []string {
	trimmed := strings.TrimRight(h.log.String(), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// rejectAll is a login handler that never accepts anything.
func rejectAll(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&body)
	utils.Unauthorized(c, "Invalid credentials")
}

// --------------------------------------------------------------------------
// Constant time and constant body
// --------------------------------------------------------------------------

// TestUnknownAndKnownAccountsAreIndistinguishable is the user-enumeration
// test. An attacker who can tell "no such user" from "wrong password" gets the
// panel's account list before they start guessing passwords, which is most of
// the work done for them.
func TestUnknownAndKnownAccountsAreIndistinguishable(t *testing.T) {
	const floor = 150 * time.Millisecond

	// The realistic asymmetry: the "no such user" branch returns without
	// hashing anything, while the "wrong password" branch pays for a bcrypt
	// verification.
	handler := func(c *gin.Context) {
		var body struct {
			Username string `json:"username"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Username == "known" {
			time.Sleep(45 * time.Millisecond) // stands in for bcrypt
			utils.Unauthorized(c, "invalid credentials")
			return
		}
		utils.Unauthorized(c, "user not found")
	}

	harness := newHarness(t, ratelimit.DefaultPolicy(), floor, handler)
	harness.sleeper.actual = true

	unknown, unknownTime := harness.post(t, "203.0.113.5:5000", "no-such-user", "x")
	known, knownTime := harness.post(t, "203.0.113.6:5000", "known", "x")

	if unknown.Code != known.Code {
		t.Fatalf("status differs: unknown=%d known=%d", unknown.Code, known.Code)
	}
	if unknown.Body.String() != known.Body.String() {
		t.Fatalf("body differs, which is a user enumeration oracle:\n unknown: %s\n known:   %s",
			unknown.Body.String(), known.Body.String())
	}
	if known.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", known.Code)
	}

	for name, elapsed := range map[string]time.Duration{"unknown": unknownTime, "known": knownTime} {
		if elapsed < floor {
			t.Fatalf("%s account answered in %v, below the constant-time floor %v", name, elapsed, floor)
		}
	}

	difference := unknownTime - knownTime
	if difference < 0 {
		difference = -difference
	}
	// The tolerance is scheduler noise, not signal: the underlying handlers
	// differ by 45ms, and what is being asserted is that the difference no
	// longer reaches the response.
	if difference > 40*time.Millisecond {
		t.Fatalf("timing still distinguishes the two accounts: unknown=%v known=%v (difference %v)",
			unknownTime, knownTime, difference)
	}
}

// TestEveryFailureReasonGetsTheSameAnswer covers the other half: the handler
// may say anything it likes, the caller always sees one answer.
func TestEveryFailureReasonGetsTheSameAnswer(t *testing.T) {
	reasons := []string{
		"user not found",
		"invalid password",
		"account is disabled",
		"account is locked, try again later",
		"tenant suspended",
	}

	var bodies []string
	for i, reason := range reasons {
		reason := reason
		harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, func(c *gin.Context) {
			utils.Unauthorized(c, reason)
		})
		recorder, _ := harness.post(t, fmt.Sprintf("203.0.113.%d:5000", i+20), "someone", "x")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
		bodies = append(bodies, recorder.Body.String())

		if strings.Contains(recorder.Body.String(), reason) {
			t.Fatalf("the handler's reason %q reached the caller: %s", reason, recorder.Body.String())
		}
	}

	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Fatalf("failure bodies differ:\n %s\n %s", bodies[0], bodies[i])
		}
	}

	var payload utils.APIResponse
	if err := json.Unmarshal([]byte(bodies[0]), &payload); err != nil {
		t.Fatalf("the canonical failure body is not valid JSON: %v", err)
	}
	if payload.Success || payload.Error == nil || payload.Error.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("unexpected canonical body: %s", bodies[0])
	}
	if payload.RequestID != "test-request" {
		t.Fatalf("the request id was lost in normalisation: %s", bodies[0])
	}
}

func TestSuccessfulResponseIsPassedThroughUntouched(t *testing.T) {
	harness := newHarness(t, ratelimit.DefaultPolicy(), 100*time.Millisecond, func(c *gin.Context) {
		utils.Success(c, gin.H{"access_token": "a-token-value"})
	})
	harness.sleeper.actual = true

	recorder, elapsed := harness.post(t, "203.0.113.30:5000", "operator", "correct")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "a-token-value") {
		t.Fatalf("the successful body was altered: %s", recorder.Body.String())
	}
	// A success is not padded: the floor exists to hide which failure happened,
	// and a success is not a failure.
	if elapsed > 60*time.Millisecond {
		t.Fatalf("a successful login was delayed by %v; the floor should only apply to failures", elapsed)
	}
}

// --------------------------------------------------------------------------
// Layered limiting through the middleware
// --------------------------------------------------------------------------

func TestProgressiveDelayIsAppliedBeforeTheHandler(t *testing.T) {
	harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, rejectAll)

	// Attempt N is charged the delay earned by the N-1 failures before it.
	// The first three failures are free - that is the human typo budget - so
	// the first four attempts cost nothing and the delay doubles from there
	// until it reaches the cap.
	want := []time.Duration{
		0, 0, 0, 0,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
	}

	for attempt, expected := range want {
		harness.sleeper.reset()
		recorder, _ := harness.post(t, "203.0.113.40:5000", "operator", "wrong")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", attempt+1, recorder.Code)
		}
		slept := harness.sleeper.durations()
		var total time.Duration
		for _, d := range slept {
			total += d
		}
		if total != expected {
			t.Fatalf("attempt %d: delayed %v, want %v (all pauses: %v)", attempt+1, total, expected, slept)
		}
	}
}

func TestLockedPairIsRefusedWithRetryAfter(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	harness := newHarness(t, policy, DisableConstantTimeFloor, rejectAll)

	for i := 0; i < policy.PairLockThreshold; i++ {
		if recorder, _ := harness.post(t, "203.0.113.50:5000", "operator", "wrong"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, recorder.Code)
		}
	}

	recorder, _ := harness.post(t, "203.0.113.50:5000", "operator", "wrong")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the pair is locked", recorder.Code)
	}

	retryAfter := recorder.Header().Get("Retry-After")
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil || seconds <= 0 {
		t.Fatalf("Retry-After = %q, want a positive number of seconds", retryAfter)
	}
	if want := int(policy.LockSteps[0].Seconds()); seconds > want {
		t.Fatalf("Retry-After = %ds, longer than the first lock step %ds", seconds, want)
	}

	// The refusal says nothing about which counter tripped.
	body := recorder.Body.String()
	for _, leak := range []string{"pair", "account", "address", "lock"} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Fatalf("the refusal names the dimension that tripped (%q), telling an attacker "+
				"what to route around: %s", leak, body)
		}
	}

	lines := harness.lines()
	last := lines[len(lines)-1]
	if !strings.Contains(last, "outcome=blocked") || !strings.Contains(last, "reason=locked") {
		t.Fatalf("the block was not logged in the fail2ban format: %q", last)
	}
	if !strings.Contains(last, "ip=203.0.113.50") {
		t.Fatalf("the log line does not carry the source address: %q", last)
	}
}

func TestSprayAcrossAccountsIsStoppedByTheAddressDimension(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	policy.AddressLimit = 6
	harness := newHarness(t, policy, DisableConstantTimeFloor, rejectAll)

	for i := 0; i < policy.AddressLimit; i++ {
		recorder, _ := harness.post(t, "198.51.100.60:5000", fmt.Sprintf("victim-%d", i), "wrong")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, recorder.Code)
		}
	}

	// A brand new account from the same address: no pair has failed more than
	// once, so only the address dimension can stop this.
	recorder, _ := harness.post(t, "198.51.100.60:5000", "victim-fresh", "wrong")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 from the address dimension", recorder.Code)
	}

	// A different address is unaffected.
	other, _ := harness.post(t, "198.51.100.61:5000", "victim-fresh", "wrong")
	if other.Code != http.StatusUnauthorized {
		t.Fatalf("an unrelated address got %d, want 401", other.Code)
	}
}

func TestDistributedAttackIsStoppedByTheAccountDimension(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	policy.AccountLimit = 5
	policy.AddressLimit = 1000
	harness := newHarness(t, policy, DisableConstantTimeFloor, rejectAll)

	for i := 0; i < policy.AccountLimit; i++ {
		recorder, _ := harness.post(t, fmt.Sprintf("192.0.2.%d:5000", i+1), "operator", "wrong")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, recorder.Code)
		}
	}

	recorder, _ := harness.post(t, "192.0.2.200:5000", "operator", "wrong")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 from the account dimension", recorder.Code)
	}

	// Another account from the same fresh address is untouched: the account
	// dimension must not become a way to lock out the whole panel.
	other, _ := harness.post(t, "192.0.2.200:5000", "someone-else", "wrong")
	if other.Code != http.StatusUnauthorized {
		t.Fatalf("an unrelated account got %d, want 401", other.Code)
	}
}

func TestSuccessClearsTheCountersForThatPair(t *testing.T) {
	policy := ratelimit.DefaultPolicy()

	accept := false
	harness := newHarness(t, policy, DisableConstantTimeFloor, func(c *gin.Context) {
		if accept {
			utils.Success(c, gin.H{"ok": true})
			return
		}
		utils.Unauthorized(c, "invalid credentials")
	})

	for i := 0; i < policy.PairLockThreshold-1; i++ {
		harness.post(t, "203.0.113.70:5000", "operator", "wrong")
	}

	accept = true
	recorder, _ := harness.post(t, "203.0.113.70:5000", "operator", "correct")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	accept = false
	harness.sleeper.reset()
	recorder, _ = harness.post(t, "203.0.113.70:5000", "operator", "wrong")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	for _, d := range harness.sleeper.durations() {
		if d != 0 {
			t.Fatalf("the pair still carries a delay of %v after a successful login", d)
		}
	}
}

func TestAMalformedRequestIsNotCountedAsAGuess(t *testing.T) {
	harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, func(c *gin.Context) {
		utils.BadRequest(c, "Invalid request body")
	})

	for i := 0; i < 30; i++ {
		recorder, _ := harness.post(t, "203.0.113.80:5000", "operator", "wrong")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
	}

	if lines := harness.lines(); len(lines) != 0 {
		t.Fatalf("malformed requests were logged as authentication attempts:\n%s", strings.Join(lines, "\n"))
	}
}

// --------------------------------------------------------------------------
// Failing closed
// --------------------------------------------------------------------------

type brokenStore struct{}

func (brokenStore) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("connection refused")
}
func (brokenStore) Get(context.Context, string) (int64, bool, error) {
	return 0, false, errors.New("connection refused")
}
func (brokenStore) Set(context.Context, string, int64, time.Duration) error {
	return errors.New("connection refused")
}
func (brokenStore) TTL(context.Context, string) (time.Duration, error) {
	return 0, errors.New("connection refused")
}
func (brokenStore) Delete(context.Context, ...string) error {
	return errors.New("connection refused")
}

func TestAuthenticationIsRefusedWhenTheLimiterCannotCount(t *testing.T) {
	harness := newHarnessWithStore(t, brokenStore{}, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, func(c *gin.Context) {
		t.Error("the handler must not run while the limiter is blind: " +
			"an attacker who can take Redis down would otherwise have taken the protection down with it")
		utils.Success(c, gin.H{})
	})

	recorder, _ := harness.post(t, "203.0.113.90:5000", "operator", "wrong")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Fatal("a refusal should tell the client when to come back")
	}

	lines := harness.lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "reason=limiter_unavailable") {
		t.Fatalf("the outage was not recorded in the authentication log: %v", lines)
	}
}

func TestNoLimiterMeansNoAuthentication(t *testing.T) {
	engine := gin.New()
	logBuffer := &bytes.Buffer{}
	engine.POST("/login", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeLogin,
		Guard:       nil,
		Log:         NewAuthLogger(logBuffer, nil),
		MinResponse: DisableConstantTimeFloor,
		Resolver:    NewClientIPResolver(nil),
		sleep:       func(context.Context, time.Duration) {},
	}), func(c *gin.Context) {
		t.Error("a guard with no limiter must not run the handler")
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.91:5000"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

// --------------------------------------------------------------------------
// Allow list
// --------------------------------------------------------------------------

func TestAllowlistedAddressIsNeverBlockedButIsStillLogged(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	store := ratelimit.NewMemoryStore()
	logBuffer := &bytes.Buffer{}

	engine := gin.New()
	engine.POST("/login", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeLogin,
		Guard:       ratelimit.New(store, policy),
		Log:         NewAuthLogger(logBuffer, nil),
		MinResponse: DisableConstantTimeFloor,
		Allowlist:   []string{"10.20.0.0/16"},
		Resolver:    NewClientIPResolver(nil),
		sleep:       func(context.Context, time.Duration) {},
	}), rejectAll)

	for i := 0; i < policy.PairLockThreshold*3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"operator","password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.20.30.40:5000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401 - an allow-listed operator must never "+
				"be locked out of their own panel", i+1, recorder.Code)
		}
	}

	// Still logged, so the jail and the audit trail are not blinded by the
	// allow list.
	lines := strings.Split(strings.TrimRight(logBuffer.String(), "\n"), "\n")
	if len(lines) != policy.PairLockThreshold*3 {
		t.Fatalf("allow-listed attempts were logged %d times, want %d", len(lines), policy.PairLockThreshold*3)
	}
}

// --------------------------------------------------------------------------
// Account extraction
// --------------------------------------------------------------------------

func TestAccountExtractionLeavesTheBodyIntactForTheHandler(t *testing.T) {
	var seen struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Extra    string `json:"extra"`
	}

	harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, func(c *gin.Context) {
		if err := c.ShouldBindJSON(&seen); err != nil {
			t.Errorf("the handler could not read the body the guard peeked at: %v", err)
		}
		utils.Unauthorized(c, "invalid credentials")
	})

	body := `{"username":"operator","password":"s3cret","extra":"kept"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.100:5000"
	harness.engine.ServeHTTP(httptest.NewRecorder(), req)

	if seen.Username != "operator" || seen.Password != "s3cret" || seen.Extra != "kept" {
		t.Fatalf("the handler saw a different body than the client sent: %+v", seen)
	}
}

func TestAccountIsRecordedInTheLogButTheSecretIsNot(t *testing.T) {
	harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, rejectAll)
	harness.post(t, "203.0.113.101:5000", "operator", "hunter2-the-real-password")

	line := harness.lines()[0]
	if !strings.Contains(line, "account=operator") {
		t.Fatalf("the account is missing from the log line: %q", line)
	}
	if strings.Contains(line, "hunter2") {
		t.Fatalf("the password reached the authentication log: %q", line)
	}
}

func TestSecretShapedCredentialsAreFingerprintedNotLogged(t *testing.T) {
	const secret = "vkai_live_0123456789abcdef"

	engine := gin.New()
	var captured string
	engine.POST("/refresh", func(c *gin.Context) {
		captured = DefaultAccountFor(ScopeRefresh)(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/refresh",
		strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, secret)))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), req)

	if captured == "" {
		t.Fatal("no account identifier was derived from the refresh token")
	}
	if strings.Contains(captured, secret) {
		t.Fatalf("the token itself became the limiter key: %q", captured)
	}
	if captured != CredentialFingerprint(secret) {
		t.Fatalf("fingerprint = %q, want %q", captured, CredentialFingerprint(secret))
	}
	// Two different tokens must not collapse onto one budget.
	if CredentialFingerprint(secret) == CredentialFingerprint(secret+"x") {
		t.Fatal("different secrets produced the same fingerprint")
	}
}

func TestDefaultAccountExtractorExistsForEveryScope(t *testing.T) {
	scopes := []string{ScopeLogin, ScopeRefresh, ScopePasswordReset, ScopeAPIKey, ScopeTwoFactor, ScopeAgentEnrol}
	for _, scope := range scopes {
		if DefaultAccountFor(scope) == nil {
			t.Fatalf("scope %q has no account extractor, so it would be limited by address alone", scope)
		}
	}
}

// --------------------------------------------------------------------------
// Source address resolution
// --------------------------------------------------------------------------

// TestForwardedHeadersFromUntrustedPeersAreIgnored is the difference between a
// limiter and a suggestion. gin's own ClientIP trusts every proxy on this
// engine, so an attacker could put a fresh address in X-Forwarded-For on every
// request and never hit any per-address limit.
func TestForwardedHeadersFromUntrustedPeersAreIgnored(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	policy.AddressLimit = 4
	harness := newHarness(t, policy, DisableConstantTimeFloor, rejectAll)

	for i := 0; i < policy.AddressLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login",
			strings.NewReader(fmt.Sprintf(`{"username":"victim-%d","password":"x"}`, i)))
		req.Header.Set("Content-Type", "application/json")
		// A different forged address every time.
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		req.RemoteAddr = "203.0.113.200:5000"
		recorder := httptest.NewRecorder()
		harness.engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, recorder.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"victim-x","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.250")
	req.RemoteAddr = "203.0.113.200:5000"
	recorder := httptest.NewRecorder()
	harness.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: a forged X-Forwarded-For must not reset the counter", recorder.Code)
	}
}

func TestForwardedHeadersFromATrustedProxyAreBelieved(t *testing.T) {
	resolver := NewClientIPResolver([]string{"127.0.0.0/8"})

	cases := []struct {
		name       string
		remoteAddr string
		forwarded  string
		realIP     string
		want       string
	}{
		{"untrusted peer wins over the header", "203.0.113.1:5000", "198.51.100.9", "", "203.0.113.1"},
		{"trusted proxy passes the client through", "127.0.0.1:5000", "198.51.100.9", "", "198.51.100.9"},
		{"rightmost untrusted entry is taken", "127.0.0.1:5000", "198.51.100.9, 203.0.113.7", "", "203.0.113.7"},
		{"trusted entries are skipped", "127.0.0.1:5000", "198.51.100.9, 127.0.0.5", "", "198.51.100.9"},
		{"forged unparseable entry stops the walk", "127.0.0.1:5000", "198.51.100.9, not-an-ip", "", "127.0.0.1"},
		{"X-Real-IP is the fallback", "127.0.0.1:5000", "", "198.51.100.11", "198.51.100.11"},
		{"no headers at all", "127.0.0.1:5000", "", "", "127.0.0.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := resolver.Resolve(req); got != tc.want {
				t.Fatalf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultTrustListIsLoopbackOnly(t *testing.T) {
	resolver := NewClientIPResolver(defaultTrustedProxies)
	if resolver.Trusted("10.0.0.5") || resolver.Trusted("192.168.1.5") || resolver.Trusted("172.16.0.5") {
		t.Fatal("private ranges must not be trusted by default: a container network or a " +
			"hostile neighbour on the same LAN is not a reverse proxy")
	}
	if !resolver.Trusted("127.0.0.1") || !resolver.Trusted("::1") {
		t.Fatal("loopback should be trusted: the panel's own nginx sits there")
	}
}

func TestUnparseableProxyEntriesAreDroppedNotTrusted(t *testing.T) {
	resolver := NewClientIPResolver([]string{"127.0.0.1", "definitely not a cidr", ""})
	if resolver.Trusted("203.0.113.1") {
		t.Fatal("a typo in the trusted proxy list must not widen it to everything")
	}
	if !resolver.Trusted("127.0.0.1") {
		t.Fatal("the valid entry should still be honoured")
	}
}

// --------------------------------------------------------------------------
// API key authentication
// --------------------------------------------------------------------------

func TestAPIKeyAuthAnswersEveryRejectionTheSameWay(t *testing.T) {
	validator := func(ctx context.Context, rawKey string) (*APIKeyPrincipal, error) {
		switch rawKey {
		case "vkai_valid_key":
			return &APIKeyPrincipal{
				KeyID:    uuid.New(),
				UserID:   uuid.New(),
				TenantID: uuid.New(),
			}, nil
		case "vkai_expired_key":
			return nil, errors.New("API key has expired")
		default:
			return nil, errors.New("API key not found")
		}
	}

	engine := gin.New()
	logBuffer := &bytes.Buffer{}
	engine.GET("/things", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeAPIKey,
		Account:     DefaultAccountFor(ScopeAPIKey),
		Guard:       ratelimit.New(ratelimit.NewMemoryStore(), ratelimit.DefaultPolicy()),
		Log:         NewAuthLogger(logBuffer, nil),
		MinResponse: DisableConstantTimeFloor,
		Resolver:    NewClientIPResolver(nil),
		sleep:       func(context.Context, time.Duration) {},
	}), APIKeyAuth(validator), func(c *gin.Context) {
		utils.Success(c, gin.H{"ok": true})
	})

	request := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/things", nil)
		req.RemoteAddr = "203.0.113.210:5000"
		if key != "" {
			req.Header.Set(APIKeyHeader, key)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	expired := request("vkai_expired_key")
	unknown := request("vkai_unknown_key")
	if expired.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("statuses: expired=%d unknown=%d, want 401 for both", expired.Code, unknown.Code)
	}
	if expired.Body.String() != unknown.Body.String() {
		t.Fatalf("an expired key is distinguishable from an unknown one:\n %s\n %s",
			expired.Body.String(), unknown.Body.String())
	}

	ok := request("vkai_valid_key")
	if ok.Code != http.StatusOK {
		t.Fatalf("a valid key got %d, want 200", ok.Code)
	}

	// The key itself never reaches the log.
	if strings.Contains(logBuffer.String(), "vkai_expired_key") || strings.Contains(logBuffer.String(), "vkai_valid_key") {
		t.Fatalf("an API key was written to the authentication log:\n%s", logBuffer.String())
	}
	if !strings.Contains(logBuffer.String(), "scope=api_key") {
		t.Fatalf("API key attempts are not attributed to their own scope:\n%s", logBuffer.String())
	}
}

func TestMissingCredentialIsNotCountedAsAGuess(t *testing.T) {
	engine := gin.New()
	logBuffer := &bytes.Buffer{}
	engine.GET("/things", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeAPIKey,
		Account:     DefaultAccountFor(ScopeAPIKey),
		Guard:       ratelimit.New(ratelimit.NewMemoryStore(), ratelimit.DefaultPolicy()),
		Log:         NewAuthLogger(logBuffer, nil),
		MinResponse: DisableConstantTimeFloor,
		Resolver:    NewClientIPResolver(nil),
		sleep:       func(context.Context, time.Duration) {},
	}), APIKeyAuth(func(context.Context, string) (*APIKeyPrincipal, error) {
		return nil, errors.New("no")
	}))

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/things", nil)
		req.RemoteAddr = "203.0.113.211:5000"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	}

	if logBuffer.Len() != 0 {
		t.Fatalf("a client that offered no credential at all was counted as guessing:\n%s", logBuffer.String())
	}
}

func TestExtractAPIKey(t *testing.T) {
	cases := []struct {
		header, value, want string
	}{
		{APIKeyHeader, "vkai_key", "vkai_key"},
		{"Authorization", "ApiKey vkai_key", "vkai_key"},
		{"Authorization", "Bearer vkai_key", "vkai_key"},
		{"Authorization", "Basic dXNlcjpwYXNz", ""},
		{"Authorization", "vkai_key", ""},
	}
	for _, tc := range cases {
		engine := gin.New()
		var got string
		engine.GET("/", func(c *gin.Context) { got = extractAPIKey(c) })
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(tc.header, tc.value)
		engine.ServeHTTP(httptest.NewRecorder(), req)
		if got != tc.want {
			t.Errorf("%s: %q -> %q, want %q", tc.header, tc.value, got, tc.want)
		}
	}
}

// --------------------------------------------------------------------------
// Every failed authentication is logged
// --------------------------------------------------------------------------

func TestEveryFailedAuthenticationProducesOneLogLine(t *testing.T) {
	harness := newHarness(t, ratelimit.DefaultPolicy(), DisableConstantTimeFloor, rejectAll)

	const attempts = 5
	for i := 0; i < attempts; i++ {
		harness.post(t, "203.0.113.220:5000", "operator", "wrong")
	}

	lines := harness.lines()
	if len(lines) != attempts {
		t.Fatalf("%d failed attempts produced %d log lines", attempts, len(lines))
	}

	failregex, _ := loadFail2banRegexes(t)
	for _, line := range lines {
		if !strings.Contains(line, "outcome=failure") {
			t.Fatalf("line does not record a failure: %q", line)
		}
		match := failregex.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("a line the panel produced at runtime is not matched by the shipped "+
				"fail2ban filter: %q", line)
		}
		if host := hostFrom(failregex, match); host != "203.0.113.220" {
			t.Fatalf("filter captured %q, want 203.0.113.220, from %q", host, line)
		}
	}
}

// TestTheLoggedAddressIsTheOneFail2banCanBan separates the two jobs the source
// address does. The limiter counts an IPv6 caller by its /64, because an
// attacker is handed the whole allocation and counting single addresses would
// be the same as not counting. The log names the individual address, because
// that is what fail2ban has to hand to the firewall - a /64 is not something
// <HOST> matches or that an iptables rule from this jail would ban.
func TestTheLoggedAddressIsTheOneFail2banCanBan(t *testing.T) {
	policy := ratelimit.DefaultPolicy()
	policy.PairFreeAttempts = 0
	policy.PairLockThreshold = 2
	harness := newHarness(t, policy, DisableConstantTimeFloor, rejectAll)

	first, _ := harness.post(t, "[2001:db8:1:1::5]:5000", "operator", "wrong")
	second, _ := harness.post(t, "[2001:db8:1:1::6]:5000", "operator", "wrong")
	third, _ := harness.post(t, "[2001:db8:1:1::7]:5000", "operator", "wrong")

	if first.Code != http.StatusUnauthorized || second.Code != http.StatusUnauthorized {
		t.Fatalf("statuses: %d %d, want 401 for both", first.Code, second.Code)
	}
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: moving inside one /64 must not reset the counter", third.Code)
	}

	lines := harness.lines()
	wantAddresses := []string{"2001:db8:1:1::5", "2001:db8:1:1::6", "2001:db8:1:1::7"}
	for i, want := range wantAddresses {
		if !strings.Contains(lines[i], "ip="+want+" ") {
			t.Fatalf("line %d names %q, want the individual address ip=%s: %q",
				i, lines[i], want, lines[i])
		}
		if strings.Contains(lines[i], "/64") {
			t.Fatalf("the log line names a prefix rather than a bannable address: %q", lines[i])
		}
	}

	failregex, _ := loadFail2banRegexes(t)
	for _, line := range lines {
		match := failregex.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("the fail2ban filter does not match an IPv6 line: %q", line)
		}
		if host := hostFrom(failregex, match); !strings.Contains(host, ":") {
			t.Fatalf("filter captured %q, which is not the IPv6 address in %q", host, line)
		}
	}
}

// TestAPanickingHandlerStillProducesAResponse checks that the buffered writer
// is handed back on the way out. Without it the Recovery middleware writes its
// 500 into a buffer that is never flushed, and a panic on the login route
// becomes an empty reply that looks like a network fault.
func TestAPanickingHandlerStillProducesAResponse(t *testing.T) {
	var recovered any

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				recovered = r
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
			}
		}()
		c.Next()
	})
	engine.POST("/login", CredentialGuard(CredentialGuardOptions{
		Scope:       ScopeLogin,
		Guard:       ratelimit.New(ratelimit.NewMemoryStore(), ratelimit.DefaultPolicy()),
		Log:         NewAuthLogger(&bytes.Buffer{}, nil),
		MinResponse: DisableConstantTimeFloor,
		Resolver:    NewClientIPResolver(nil),
		sleep:       func(context.Context, time.Duration) {},
	}), func(c *gin.Context) {
		panic("the user repository is nil")
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"operator","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.250:5000"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recovered == nil {
		t.Fatal("the panic did not reach the recovery middleware")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("the recovery response was swallowed by the guard's buffer: %q", recorder.Body.String())
	}
}
