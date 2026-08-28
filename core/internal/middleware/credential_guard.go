package middleware

// CredentialGuard is the middleware that makes credential guessing against the
// panel impractical. It sits in front of every endpoint that accepts a secret -
// the login form, token refresh, password reset, API key authentication,
// two-factor verification and agent enrolment - and does four things that the
// handler behind it cannot do for itself:
//
//  1. Consults the layered limiter in internal/ratelimit before the credential
//     is checked, and refuses the attempt outright when a limit is reached.
//  2. Applies the progressive delay the limiter asks for, so the cost of
//     guessing grows with the guessing rather than stopping dead at N.
//  3. Equalises the failure paths. Every rejected attempt leaves with the same
//     body and after the same minimum time, so "no such user" and "wrong
//     password" cannot be told apart. Without this the endpoint is a user
//     enumeration oracle and an attacker gets the account list for free before
//     they start guessing passwords.
//  4. Emits one structured line per attempt for fail2ban and for the operator.
//
// It is deliberately independent of the handler. The handler keeps returning
// whatever it returns; the guard reads the status code, records the outcome
// and rewrites the response. That is what lets the same guard cover endpoints
// owned by different packages, including ones that do not exist yet.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Credential scopes. Each names one kind of secret. The limiter keeps a
// separate address-account budget per scope, so fumbling a two-factor code
// cannot lock the password form, while the per-address and per-account
// dimensions are shared across all of them so an attacker cannot get a fresh
// allowance by moving from one form to the next.
const (
	ScopeLogin         = "login"
	ScopeRefresh       = "refresh"
	ScopePasswordReset = "password_reset"
	ScopeAPIKey        = "api_key"
	ScopeTwoFactor     = "two_factor"
	ScopeAgentEnrol    = "agent_enrol"
)

// EnvAuthMinResponse overrides the constant-time floor, as a Go duration.
// Setting it to "0" disables the floor and is only appropriate in a test.
const EnvAuthMinResponse = "VKAI_AUTH_MIN_RESPONSE"

// EnvAuthAllowlist names addresses or CIDR blocks that are never limited or
// locked. It exists so an operator cannot be shut out of their own panel by
// the defence, which is the failure mode that gets a defence switched off.
// Allow-listed attempts are still logged, so fail2ban and the audit trail
// still see them.
const EnvAuthAllowlist = "VKAI_AUTH_ALLOWLIST"

// defaultMinResponse is the floor every failed authentication is padded to.
//
// It has to sit comfortably above the slowest legitimate failure path. The
// panel verifies passwords with bcrypt at the default cost, which is roughly
// 60-100ms on current server hardware; the "no such user" path does no hashing
// at all and returns in microseconds. 250ms swallows the difference with room
// to spare on a loaded machine, and is short enough that a person logging in
// does not perceive it.
const defaultMinResponse = 250 * time.Millisecond

// DisableConstantTimeFloor is the explicit way to switch the floor off, since
// a zero MinResponse means "unset" and takes the default. It exists for tests.
// A credential endpoint served without the floor is a user enumeration oracle.
const DisableConstantTimeFloor = -1 * time.Nanosecond

// AccountFunc extracts the account identifier an attempt is aimed at.
// Returning an empty string is allowed: the attempt is then limited by address
// alone.
type AccountFunc func(c *gin.Context) string

// CredentialGuardOptions configures one guarded endpoint.
type CredentialGuardOptions struct {
	// Scope names the credential kind. Required.
	Scope string

	// Account extracts the target account. Defaults to reading the usual JSON
	// login fields.
	Account AccountFunc

	// Guard is the limiter. A nil Guard means every attempt is refused, which
	// is the fail-closed answer for a panel wired without a counter store.
	Guard *ratelimit.Guard

	// Log receives one event per attempt.
	Log *AuthLogger

	// MinResponse is the constant-time floor for failed attempts.
	MinResponse time.Duration

	// Allowlist holds addresses or CIDR blocks exempt from limiting.
	Allowlist []string

	// Logger receives operational problems - a limiter that cannot be reached,
	// a handler slower than the floor.
	Logger *zap.Logger

	// Resolver maps a request to a source address. Defaults to the
	// trusted-proxy-aware resolver.
	Resolver *ClientIPResolver

	// sleep is the pause primitive, replaced in tests.
	sleep func(ctx context.Context, d time.Duration)
}

func (o CredentialGuardOptions) normalize() CredentialGuardOptions {
	if strings.TrimSpace(o.Scope) == "" {
		o.Scope = ScopeLogin
	}
	if o.Account == nil {
		o.Account = AccountFromLoginBody()
	}
	if o.MinResponse == 0 {
		o.MinResponse = configuredMinResponse()
	}
	if o.MinResponse < 0 {
		o.MinResponse = 0
	}
	if o.Resolver == nil {
		o.Resolver = DefaultClientIPResolver()
	}
	if len(o.Allowlist) == 0 {
		o.Allowlist = configuredAllowlist()
	}
	if o.sleep == nil {
		o.sleep = sleepContext
	}
	return o
}

func configuredMinResponse() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvAuthMinResponse))
	if raw == "" {
		return defaultMinResponse
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return defaultMinResponse
	}
	return d
}

func configuredAllowlist() []string {
	raw := strings.TrimSpace(os.Getenv(EnvAuthAllowlist))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func sleepContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	if ctx == nil {
		<-timer.C
		return
	}
	select {
	case <-timer.C:
	case <-ctx.Done():
		// The client hung up. Nothing is gained by holding the goroutine: the
		// attempt never reaches the credential check, and the counters that
		// produced this delay outlive the connection.
	}
}

// CredentialGuard builds the middleware.
func CredentialGuard(opts CredentialGuardOptions) gin.HandlerFunc {
	opts = opts.normalize()
	allowlist := NewClientIPResolver(opts.Allowlist)
	hasAllowlist := len(opts.Allowlist) > 0

	return func(c *gin.Context) {
		address := opts.Resolver.Resolve(c.Request)
		account := ""
		if opts.Account != nil {
			account = opts.Account(c)
		}
		subject := ratelimit.NewSubject(address, account)

		event := AuthEvent{
			// The address as it actually is, not the bucket the limiter counts
			// it in. fail2ban has to be able to ban what this field names, and
			// it cannot ban a /64.
			IP:        address,
			Account:   account,
			Scope:     opts.Scope,
			Path:      c.FullPath(),
			RequestID: c.GetString("request_id"),
		}

		if hasAllowlist && allowlist.Trusted(address) {
			runGuarded(c, opts, subject, event, false)
			return
		}

		ctx := c.Request.Context()

		if opts.Guard == nil {
			opts.refuse(c, event, ratelimit.Decision{
				Allow:      false,
				RetryAfter: 30 * time.Second,
				Outcome:    ratelimit.OutcomeUnavailable,
				Dimension:  ratelimit.DimensionStore,
			})
			return
		}

		decision, err := opts.Guard.Check(ctx, opts.Scope, subject)
		if err != nil && opts.Logger != nil {
			opts.Logger.Error("credential limiter store error",
				zap.String("scope", opts.Scope),
				zap.Bool("allowed", decision.Allow),
				zap.Error(err))
		}
		if !decision.Allow {
			opts.refuse(c, event, decision)
			return
		}

		if decision.Delay > 0 {
			opts.sleep(ctx, decision.Delay)
			if ctx.Err() != nil {
				c.Abort()
				return
			}
		}

		runGuarded(c, opts, subject, event, true)
	}
}

// refuse answers an attempt the limiter stopped. The body is the same for all
// three dimensions and for a store outage: which counter tripped is exactly
// the information an attacker would use to decide how to route around it.
func (o CredentialGuardOptions) refuse(c *gin.Context, event AuthEvent, decision ratelimit.Decision) {
	event.Outcome = AuthOutcomeBlocked
	event.Dimension = string(decision.Dimension)
	switch decision.Outcome {
	case ratelimit.OutcomeLocked:
		event.Reason = ReasonLocked
	case ratelimit.OutcomeUnavailable:
		event.Reason = ReasonLimiterUnavailable
	default:
		event.Reason = ReasonThrottled
	}
	o.Log.Log(event)

	retry := decision.RetryAfter
	if retry <= 0 {
		retry = time.Second
	}
	c.Header("Retry-After", strconv.Itoa(int((retry+time.Second-1)/time.Second)))

	status := http.StatusTooManyRequests
	if decision.Outcome == ratelimit.OutcomeUnavailable {
		// Honest about the cause: the panel is refusing because it cannot
		// verify how many attempts have been made, not because this caller
		// made too many.
		status = http.StatusServiceUnavailable
	}

	c.AbortWithStatusJSON(status, utils.APIResponse{
		Success: false,
		Error: &utils.APIError{
			Code:    "TOO_MANY_ATTEMPTS",
			Message: "Too many authentication attempts. Try again later.",
		},
		RequestID: c.GetString("request_id"),
	})
}

// runGuarded executes the handler with the response buffered, then equalises
// the failure path and records the outcome.
func runGuarded(c *gin.Context, opts CredentialGuardOptions, subject ratelimit.Subject, event AuthEvent, count bool) {
	original := c.Writer
	capture := newCaptureWriter(original)
	c.Writer = capture

	// The writer is restored from a defer so that a panicking handler cannot
	// leave the buffer installed. If it did, the Recovery middleware's 500
	// would be written into a buffer nobody flushes and the caller would get
	// an empty response to a panic - a failure that is far harder to diagnose
	// than the panic itself.
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		c.Writer = original
		capture.flush()
	}
	defer restore()

	start := time.Now()
	c.Next()
	elapsed := time.Since(start)

	outcome := classify(c, capture.Status())

	switch outcome {
	case AuthOutcomeFailure:
		event.Outcome = AuthOutcomeFailure
		event.Reason = ReasonInvalidCredentials

		if count && opts.Guard != nil {
			// context.Background, not the request context: the attempt has
			// already happened and must be counted even if the client hangs
			// up the moment it sees the failure. Otherwise an attacker just
			// aborts every request and the counters never move.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			decision, err := opts.Guard.RecordFailure(ctx, opts.Scope, subject)
			cancel()
			if err != nil && opts.Logger != nil {
				opts.Logger.Error("credential limiter could not record a failure",
					zap.String("scope", opts.Scope), zap.Error(err))
			}
			event.Dimension = string(decision.Dimension)
		}

		// Every failure leaves with the same body, whatever the handler said.
		// "Unknown user", "wrong password", "account disabled" and "account
		// locked" all become one answer.
		capture.replaceWithCanonicalFailure(c.GetString("request_id"))

		if opts.MinResponse > 0 && elapsed < opts.MinResponse {
			opts.sleep(c.Request.Context(), opts.MinResponse-elapsed)
		} else if opts.MinResponse > 0 && opts.Logger != nil {
			// The floor only equalises the paths while the slowest of them
			// stays under it. Once it does not, the difference is measurable
			// again, so say so rather than let it rot.
			opts.Logger.Warn("failed authentication took longer than the constant-time floor",
				zap.String("scope", opts.Scope),
				zap.Duration("elapsed", elapsed),
				zap.Duration("floor", opts.MinResponse))
		}

	case AuthOutcomeSuccess:
		event.Outcome = AuthOutcomeSuccess
		event.Reason = ReasonOK
		if count && opts.Guard != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := opts.Guard.RecordSuccess(ctx, opts.Scope, subject); err != nil && opts.Logger != nil {
				opts.Logger.Error("credential limiter could not record a success",
					zap.String("scope", opts.Scope), zap.Error(err))
			}
			cancel()
		}

	default:
		// Neither: a malformed body, a validation error, a server fault. It is
		// not a credential attempt, so it is not counted and not logged as one.
	}

	restore()

	if outcome != "" {
		opts.Log.Log(event)
	}
}

// classify decides what the handler's response means. A handler may state the
// outcome directly with MarkAuthOutcome; otherwise the status code speaks.
func classify(c *gin.Context, status int) string {
	if explicit, ok := c.Get(authOutcomeKey); ok {
		if s, isString := explicit.(string); isString {
			return s
		}
	}
	switch {
	case status >= 200 && status < 300:
		return AuthOutcomeSuccess
	case status == http.StatusUnauthorized:
		return AuthOutcomeFailure
	default:
		return ""
	}
}

const authOutcomeKey = "vkai_auth_outcome"

// MarkAuthOutcome lets a handler state the result of a credential check
// explicitly, for the cases where the status code alone is ambiguous - an
// endpoint that answers 200 to a wrong password on purpose, for instance,
// which a password reset form should do so that it does not disclose which
// addresses have accounts.
func MarkAuthOutcome(c *gin.Context, outcome string) {
	c.Set(authOutcomeKey, outcome)
}

// captureWriter buffers a response so the guard can rewrite it and hold it
// back until the constant-time floor has elapsed.
type captureWriter struct {
	gin.ResponseWriter
	status      int
	body        bytes.Buffer
	wroteHeader bool
	// overflowed marks a response too large to buffer. Credential endpoints
	// answer in a few kilobytes; anything past the cap is written straight
	// through, giving up the rewrite rather than the request.
	overflowed bool
}

const maxCapturedBody = 1 << 20

func newCaptureWriter(w gin.ResponseWriter) *captureWriter {
	return &captureWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *captureWriter) WriteHeader(status int) {
	if w.overflowed {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
}

func (w *captureWriter) WriteHeaderNow() {}

func (w *captureWriter) Write(data []byte) (int, error) {
	if w.overflowed {
		return w.ResponseWriter.Write(data)
	}
	if w.body.Len()+len(data) > maxCapturedBody {
		w.spill()
		return w.ResponseWriter.Write(data)
	}
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.body.Write(data)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *captureWriter) Status() int { return w.status }

func (w *captureWriter) Size() int { return w.body.Len() }

func (w *captureWriter) Written() bool { return w.wroteHeader }

// Flush is suppressed while buffering: letting a handler flush would send the
// unequalised response before the guard could rewrite it.
func (w *captureWriter) Flush() {}

// spill gives up buffering and sends what has accumulated so far.
func (w *captureWriter) spill() {
	if w.overflowed {
		return
	}
	w.overflowed = true
	w.ResponseWriter.WriteHeader(w.status)
	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
		w.body.Reset()
	}
}

func (w *captureWriter) replaceWithCanonicalFailure(requestID string) {
	if w.overflowed {
		return
	}
	body, err := json.Marshal(utils.APIResponse{
		Success: false,
		Error: &utils.APIError{
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid credentials",
		},
		RequestID: requestID,
	})
	if err != nil {
		return
	}
	w.status = http.StatusUnauthorized
	w.body.Reset()
	w.body.Write(body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Del("Content-Length")
}

func (w *captureWriter) flush() {
	if w.overflowed {
		return
	}
	if !w.wroteHeader && w.body.Len() == 0 {
		return
	}
	w.ResponseWriter.WriteHeader(w.status)
	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
	}
}

// ---------------------------------------------------------------------------
// Account extraction
// ---------------------------------------------------------------------------

// maxAccountBodyPeek bounds how much of a request body is read to find the
// account name, so a large body cannot be used to make the guard itself
// expensive.
const maxAccountBodyPeek = 64 << 10

// loginAccountFields are the JSON field names the panel's credential forms use
// for the account identifier.
var loginAccountFields = []string{"username", "email", "login", "account", "identifier", "user"}

// AccountFromLoginBody reads the account identifier out of a JSON request body
// and puts the body back so the handler still sees it.
func AccountFromLoginBody(fields ...string) AccountFunc {
	if len(fields) == 0 {
		fields = loginAccountFields
	}
	return func(c *gin.Context) string {
		if account := accountFromJSONBody(c, fields); account != "" {
			return account
		}
		for _, field := range fields {
			if v := strings.TrimSpace(c.Query(field)); v != "" {
				return v
			}
		}
		return ""
	}
}

func accountFromJSONBody(c *gin.Context, fields []string) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(c.ContentType()), "json") {
		return ""
	}

	limited := io.LimitReader(c.Request.Body, maxAccountBodyPeek)
	peeked, err := io.ReadAll(limited)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(peeked))
		return ""
	}
	// Put the body back exactly as it was, including anything past the peek
	// limit, so the handler binds the same request the client sent.
	c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peeked), c.Request.Body))

	var payload map[string]json.RawMessage
	if json.Unmarshal(peeked, &payload) != nil {
		return ""
	}
	for _, field := range fields {
		raw, ok := payload[field]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) != nil {
			continue
		}
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// AccountFromHeader reads the account identifier from a request header.
func AccountFromHeader(name string) AccountFunc {
	return func(c *gin.Context) string {
		return strings.TrimSpace(c.GetHeader(name))
	}
}

// AccountFromSecretHeader derives a stable, non-reversible identifier from a
// secret carried in a header - an API key, an agent token, a refresh token.
//
// The secret itself must never become the account identifier: it would end up
// in a Redis key and in the authentication log, which is to say in a backup and
// on an operator's screen. The digest still distinguishes one key from another,
// which is all the limiter needs.
func AccountFromSecretHeader(names ...string) AccountFunc {
	return func(c *gin.Context) string {
		for _, name := range names {
			value := strings.TrimSpace(c.GetHeader(name))
			if value == "" {
				continue
			}
			if strings.EqualFold(name, "Authorization") {
				parts := strings.SplitN(value, " ", 2)
				if len(parts) == 2 {
					value = strings.TrimSpace(parts[1])
				}
			}
			if value == "" {
				continue
			}
			return CredentialFingerprint(value)
		}
		return ""
	}
}

// AccountFromBodySecret derives the same kind of digest from a JSON body field
// that carries a secret rather than a name - a refresh token, an enrolment
// token, a reset token.
func AccountFromBodySecret(fields ...string) AccountFunc {
	return func(c *gin.Context) string {
		if value := accountFromJSONBody(c, fields); value != "" {
			return CredentialFingerprint(value)
		}
		return ""
	}
}

// AccountFirstOf tries several extractors in order.
func AccountFirstOf(funcs ...AccountFunc) AccountFunc {
	return func(c *gin.Context) string {
		for _, f := range funcs {
			if f == nil {
				continue
			}
			if v := f(c); v != "" {
				return v
			}
		}
		return ""
	}
}

// CredentialFingerprint is a short, stable, one-way identifier for a secret.
func CredentialFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "fp_" + hex.EncodeToString(sum[:8])
}

// ---------------------------------------------------------------------------
// Process-wide wiring
// ---------------------------------------------------------------------------

var (
	sharedGuardMu   sync.Mutex
	sharedGuardSet  bool
	sharedGuardInst *ratelimit.Guard
)

// SetCredentialLimiter installs the process-wide limiter. The API entry point
// should call this with a store built on the Redis connection it already
// holds; when it does not, ProtectCredentials builds one from configuration on
// first use.
func SetCredentialLimiter(guard *ratelimit.Guard) {
	sharedGuardMu.Lock()
	defer sharedGuardMu.Unlock()
	sharedGuardInst = guard
	sharedGuardSet = true
}

// CredentialLimiter returns the process-wide limiter, building it from
// configuration on first use.
func CredentialLimiter(logger *zap.Logger) *ratelimit.Guard {
	sharedGuardMu.Lock()
	defer sharedGuardMu.Unlock()
	if sharedGuardSet {
		return sharedGuardInst
	}
	sharedGuardInst = buildCredentialLimiter(logger)
	sharedGuardSet = true
	return sharedGuardInst
}

// ProtectCredentials is the one-line form used from the router. It wires the
// process-wide limiter, the authentication log and the default account
// extractor for the scope.
//
//	authGroup.Use(middleware.ProtectCredentials(middleware.ScopeLogin, r.logger))
func ProtectCredentials(scope string, logger *zap.Logger) gin.HandlerFunc {
	return CredentialGuard(CredentialGuardOptions{
		Scope:   scope,
		Account: DefaultAccountFor(scope),
		Guard:   CredentialLimiter(logger),
		Log:     DefaultAuthLogger(logger),
		Logger:  logger,
	})
}

// DefaultAccountFor picks the account extractor that suits a scope.
func DefaultAccountFor(scope string) AccountFunc {
	switch scope {
	case ScopeRefresh:
		return AccountFromBodySecret("refresh_token", "token")
	case ScopeAPIKey:
		return AccountFromSecretHeader("X-API-Key", "Authorization")
	case ScopeAgentEnrol:
		return AccountFirstOf(
			AccountFromSecretHeader("X-Agent-Token"),
			AccountFromBodySecret("token", "enrollment_token", "agent_token"),
		)
	case ScopeTwoFactor:
		// The account is known from the half-authenticated session, not from
		// the code: limiting by code would give every wrong code its own
		// budget, which is no limit at all.
		return AccountFirstOf(
			AccountFromLoginBody("username", "email", "user_id"),
			AccountFromSecretHeader("X-2FA-Session", "X-MFA-Token"),
		)
	case ScopePasswordReset:
		return AccountFirstOf(
			AccountFromLoginBody("email", "username", "login"),
			AccountFromBodySecret("token", "reset_token"),
		)
	default:
		return AccountFromLoginBody()
	}
}
