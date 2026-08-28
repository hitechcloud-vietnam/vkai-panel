package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/twofactor"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

const (
	testPassword = "correct horse battery staple"
	testIP       = "203.0.113.10"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeUserRepo is the slice of the user repository the auth service uses.
type fakeUserRepo struct {
	mu    sync.Mutex
	users map[string]*models.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[string]*models.User)}
}

func (r *fakeUserRepo) add(user *models.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[user.Username] = user
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*models.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[username]
	if !ok {
		return nil, errors.New("user not found")
	}
	copied := *user
	return &copied, nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, user := range r.users {
		if user.ID == id {
			copied := *user
			return &copied, nil
		}
	}
	return nil, errors.New("user not found")
}

func (r *fakeUserRepo) UpdateLastLogin(context.Context, uuid.UUID, string) error { return nil }

func (r *fakeUserRepo) GetRoleNames(context.Context, uuid.UUID) ([]string, error) {
	return []string{"admin"}, nil
}

func (r *fakeUserRepo) GetPermissionNames(context.Context, uuid.UUID) ([]string, error) {
	return []string{"server:read"}, nil
}

// stubVerifier stands in for the two-factor service where the test cares about
// the gate's decision rather than about the codes.
type stubVerifier struct {
	required    bool
	requiredErr error
	verifyErr   error
}

func (s *stubVerifier) Required(context.Context, uuid.UUID) (bool, error) {
	return s.required, s.requiredErr
}

func (s *stubVerifier) Verify(context.Context, twofactor.Request) (*twofactor.VerifyResult, error) {
	if s.verifyErr != nil {
		return nil, s.verifyErr
	}
	return &twofactor.VerifyResult{Method: twofactor.MethodTOTP}, nil
}

// auditEntry is one captured audit call.
type auditEntry struct {
	Action string
	Status string
}

type captureAudit struct {
	mu      sync.Mutex
	entries []auditEntry
}

func (a *captureAudit) Log(_ context.Context, _ uuid.UUID, _ *uuid.UUID, action, _ string,
	_ *uuid.UUID, _ models.JSONMap, _, _, status string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, auditEntry{Action: action, Status: status})
	return nil
}

func (a *captureAudit) has(action, status string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, entry := range a.entries {
		if entry.Action == action && entry.Status == status {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type loginHarness struct {
	t          *testing.T
	service    *AuthService
	repo       *fakeUserRepo
	user       *models.User
	jwt        *auth.JWTManager
	twoFactor  *twofactor.Service
	tfStore    *twofactor.MemoryStore
	audit      *captureAudit
	recovery   []string
	challengeT time.Duration
}

// newLoginHarness builds an auth service over a fake user repository and, when
// asked for, a real two-factor service over an in-memory store. Real, not
// stubbed: the point of these tests is that the login path and the two-factor
// service agree about codes, replays and recovery codes.
func newLoginHarness(t *testing.T, withTwoFactor bool) *loginHarness {
	t.Helper()

	hash, err := utils.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	user := &models.User{
		ID:           uuid.New(),
		TenantID:     uuid.New(),
		Username:     "admin",
		Email:        "admin@example.test",
		PasswordHash: hash,
		Status:       "active",
	}

	repo := newFakeUserRepo()
	repo.add(user)

	jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute, 24*time.Hour, "vkai-panel-test")

	h := &loginHarness{
		t:    t,
		repo: repo,
		user: user,
		jwt:  jwtManager,
	}

	h.service = &AuthService{
		userRepo:   repo,
		jwtManager: jwtManager,
		logger:     zap.NewNop(),
		failures:   newLoginFailureTracker(),
	}

	if withTwoFactor {
		master := make([]byte, 32)
		for i := range master {
			master[i] = byte(i + 1)
		}
		box, err := twofactor.NewSecretBox(master)
		if err != nil {
			t.Fatalf("NewSecretBox: %v", err)
		}

		store := twofactor.NewMemoryStore()
		store.AddAccount(twofactor.Account{
			UserID:       user.ID,
			TenantID:     user.TenantID,
			Username:     user.Username,
			Email:        user.Email,
			PasswordHash: hash,
		})

		h.audit = &captureAudit{}
		h.tfStore = store
		h.twoFactor = twofactor.NewService(store, box, h.audit,
			twofactor.NewMemoryLimiter(10000, time.Minute), zap.NewNop(),
			// Three recovery codes rather than ten: each one costs a bcrypt
			// hash, and the tests need two.
			twofactor.Options{RecoveryCodeCount: 3})
		h.service.SetTwoFactor(h.twoFactor)
	}

	return h
}

// enrol takes the account all the way through enrolment, as the settings page
// would, so the login tests start from a genuinely protected account.
func (h *loginHarness) enrol() {
	h.t.Helper()
	ctx := context.Background()

	if _, err := h.twoFactor.StartEnrolment(ctx, twofactor.Request{
		UserID:   h.user.ID,
		Password: testPassword,
	}); err != nil {
		h.t.Fatalf("StartEnrolment: %v", err)
	}

	set, err := h.twoFactor.ConfirmEnrolment(ctx, twofactor.Request{
		UserID: h.user.ID,
		Code:   h.currentCode(),
	})
	if err != nil {
		h.t.Fatalf("ConfirmEnrolment: %v", err)
	}
	h.recovery = set.Codes

	// The store mirrors the flag onto the user record; the fake repository is
	// not wired to it, so mirror it here as the database would.
	h.user.MFAEnabled = h.tfStore.UserFlag(h.user.ID)
	h.repo.add(h.user)
}

// currentCode is the code the user's authenticator app would be showing.
func (h *loginHarness) currentCode() string {
	h.t.Helper()

	enrolment, err := h.tfStore.Get(context.Background(), h.user.ID)
	if err != nil || enrolment == nil {
		h.t.Fatalf("no enrolment stored: %v", err)
	}

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i + 1)
	}
	box, err := twofactor.NewSecretBox(master)
	if err != nil {
		h.t.Fatalf("NewSecretBox: %v", err)
	}
	secret, err := box.Open(enrolment.SecretCiphertext)
	if err != nil {
		h.t.Fatalf("open secret: %v", err)
	}

	return twofactor.CodeAt(secret, time.Now(), enrolment.DigitCount(), enrolment.Period())
}

// nextCode returns a code from the following time step, for the tests that
// need a second distinct code without waiting thirty seconds.
func (h *loginHarness) nextCode() string {
	h.t.Helper()

	enrolment, err := h.tfStore.Get(context.Background(), h.user.ID)
	if err != nil || enrolment == nil {
		h.t.Fatalf("no enrolment stored: %v", err)
	}

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i + 1)
	}
	box, err := twofactor.NewSecretBox(master)
	if err != nil {
		h.t.Fatalf("NewSecretBox: %v", err)
	}
	secret, err := box.Open(enrolment.SecretCiphertext)
	if err != nil {
		h.t.Fatalf("open secret: %v", err)
	}

	return twofactor.CodeAt(secret, time.Now().Add(enrolment.Period()), enrolment.DigitCount(), enrolment.Period())
}

func (h *loginHarness) login() (*models.LoginResponse, error) {
	h.t.Helper()
	return h.service.Login(context.Background(), models.LoginRequest{
		Username: h.user.Username,
		Password: testPassword,
	}, testIP)
}

func (h *loginHarness) exchange(challenge, code string) (*models.LoginResponse, error) {
	h.t.Helper()
	return h.service.CompleteTwoFactorLogin(context.Background(), TwoFactorExchange{
		ChallengeToken: challenge,
		Code:           code,
		IP:             testIP,
		UserAgent:      "test-agent",
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// An account with no second factor signs in exactly as it did before.
func TestLoginWithoutTwoFactorIsUnchanged(t *testing.T) {
	h := newLoginHarness(t, false)

	resp, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.TwoFactorRequired {
		t.Fatal("an account with no second factor was asked for one")
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.User == nil {
		t.Fatal("login did not return a session")
	}
	if _, err := h.jwt.ValidateAccessToken(resp.AccessToken); err != nil {
		t.Fatalf("access token does not validate: %v", err)
	}
}

// The requirement: password alone yields no usable token when two-factor is on.
func TestPasswordAloneYieldsNoUsableToken(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	resp, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if !resp.TwoFactorRequired {
		t.Fatal("login did not demand a second factor")
	}
	if resp.AccessToken != "" || resp.RefreshToken != "" {
		t.Fatal("the password step minted a token pair for an account with two-factor enabled")
	}
	if resp.User != nil {
		t.Fatal("the password step disclosed the user record before the second factor")
	}
	if resp.ChallengeToken == "" {
		t.Fatal("no challenge was issued")
	}

	// The challenge is not a credential anywhere.
	if _, err := h.jwt.ValidateToken(resp.ChallengeToken); err == nil {
		t.Fatal("the challenge was accepted as an access token")
	}
	if _, err := h.jwt.ValidateAccessToken(resp.ChallengeToken); err == nil {
		t.Fatal("the challenge was accepted by ValidateAccessToken")
	}
	if _, err := h.service.RefreshToken(context.Background(), resp.ChallengeToken); err == nil {
		t.Fatal("the challenge was accepted as a refresh token")
	}
}

// A challenge is short lived: minutes, not hours.
func TestChallengeIsShortLived(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	resp, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.ChallengeExpiresIn <= 0 || resp.ChallengeExpiresIn > 600 {
		t.Fatalf("challenge lives for %d seconds; it must be minutes", resp.ChallengeExpiresIn)
	}
}

// The whole flow: password, then code, then a session.
func TestExchangeIssuesTheRealTokenPair(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// The code that confirmed enrolment was spent by the confirmation, so the
	// user's app has moved on a step by the time they sign in.
	resp, err := h.exchange(first.ChallengeToken, h.nextCode())
	if err != nil {
		t.Fatalf("CompleteTwoFactorLogin: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.User == nil {
		t.Fatal("the exchange did not return a session")
	}
	if _, err := h.jwt.ValidateAccessToken(resp.AccessToken); err != nil {
		t.Fatalf("access token does not validate: %v", err)
	}
	if !h.audit.has(twofactor.ActionVerified, "success") {
		t.Fatal("a successful second factor was not audited")
	}
}

// A challenge buys one attempt. Replaying a challenge that already produced a
// session must not produce a second one.
func TestChallengeCannotBeReplayed(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := h.exchange(first.ChallengeToken, h.nextCode()); err != nil {
		t.Fatalf("first exchange: %v", err)
	}

	resp, err := h.exchange(first.ChallengeToken, h.nextCode())
	if !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("replay error = %v, want ErrChallengeInvalid", err)
	}
	if resp != nil {
		t.Fatal("a replayed challenge produced a response")
	}
}

// An expired challenge is refused and sends the caller back to the password.
func TestExpiredChallengeIsRefused(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	h.jwt.SetTwoFactorChallengeTTL(time.Second)

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	if _, err := h.exchange(first.ChallengeToken, h.currentCode()); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("expired challenge error = %v, want ErrChallengeInvalid", err)
	}
}

// A wrong code spends the challenge it came with, hands back a replacement
// bounded by the original deadline, and is audited.
func TestWrongCodeSpendsTheChallengeAndOffersARetry(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	resp, err := h.exchange(first.ChallengeToken, "000000")
	if resp != nil {
		t.Fatal("a wrong code produced a response")
	}
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("wrong code error = %v, want ErrVerificationFailed", err)
	}

	var retry *TwoFactorRetryError
	if !errors.As(err, &retry) {
		t.Fatalf("wrong code did not offer a retry challenge: %v", err)
	}
	if retry.ChallengeToken == "" || retry.ChallengeToken == first.ChallengeToken {
		t.Fatal("the retry challenge is missing or is the spent one")
	}
	if retry.ChallengeExpiresIn > int(auth.DefaultTwoFactorChallengeTTL.Seconds()) {
		t.Fatal("the retry challenge extended the window")
	}
	if !h.audit.has(twofactor.ActionVerificationFailed, "failure") {
		t.Fatal("a failed second factor was not audited")
	}

	// The spent challenge is dead even though the attempt failed.
	if _, err := h.exchange(first.ChallengeToken, h.nextCode()); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("spent challenge error = %v, want ErrChallengeInvalid", err)
	}

	// The replacement works.
	session, err := h.exchange(retry.ChallengeToken, h.nextCode())
	if err != nil {
		t.Fatalf("retry exchange: %v", err)
	}
	if session.AccessToken == "" {
		t.Fatal("the retry did not produce a session")
	}
}

// A recovery code is accepted in place of a TOTP code, and only once.
func TestRecoveryCodeWorksOnce(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	if len(h.recovery) == 0 {
		t.Fatal("enrolment issued no recovery codes")
	}
	code := h.recovery[0]

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	resp, err := h.exchange(first.ChallengeToken, code)
	if err != nil {
		t.Fatalf("recovery code exchange: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("a recovery code did not produce a session")
	}
	if !h.audit.has(twofactor.ActionRecoveryCodeUsed, "success") {
		t.Fatal("recovery code use was not audited")
	}

	// The same code again, on a fresh challenge, is refused.
	second, err := h.login()
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if _, err := h.exchange(second.ChallengeToken, code); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("reused recovery code error = %v, want ErrVerificationFailed", err)
	}

	// A different code from the same set still works.
	third, err := h.login()
	if err != nil {
		t.Fatalf("third Login: %v", err)
	}
	if _, err := h.exchange(third.ChallengeToken, h.recovery[1]); err != nil {
		t.Fatalf("second recovery code: %v", err)
	}
}

// Refresh cannot be used to skip the gate: the password step issues no refresh
// token for a protected account, and the challenge is not one.
func TestRefreshCannotSkipTheGate(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if first.RefreshToken != "" {
		t.Fatal("the password step handed out a refresh token")
	}
	if _, err := h.service.RefreshToken(context.Background(), first.ChallengeToken); err == nil {
		t.Fatal("a challenge was refreshed into a token pair")
	}

	// Only after the second factor does refresh work at all.
	session, err := h.exchange(first.ChallengeToken, h.nextCode())
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if _, err := h.service.RefreshToken(context.Background(), session.RefreshToken); err != nil {
		t.Fatalf("refresh after a complete sign-in: %v", err)
	}
}

// The audit's dangerous state: the account says two-factor is on and nothing is
// wired to check it. Sign-in must be refused, not completed.
func TestEnrolledAccountIsRefusedWhenNoVerifierIsWired(t *testing.T) {
	h := newLoginHarness(t, false)
	h.user.MFAEnabled = true
	h.repo.add(h.user)

	resp, err := h.login()
	if !errors.Is(err, ErrTwoFactorUnavailable) {
		t.Fatalf("login error = %v, want ErrTwoFactorUnavailable", err)
	}
	if resp != nil {
		t.Fatal("a session was issued for an account whose second factor cannot be checked")
	}

	// The same refusal on refresh, so a live session cannot be extended past a
	// gate the process can no longer enforce.
	if _, err := h.service.RefreshToken(context.Background(), "not-a-token"); err == nil {
		t.Fatal("an invalid refresh token was accepted")
	}
}

// A lookup failure must not fall open.
func TestEnrolmentLookupFailureFailsClosed(t *testing.T) {
	h := newLoginHarness(t, false)
	h.service.SetTwoFactor(&stubVerifier{requiredErr: errors.New("database is down")})

	if _, err := h.login(); !errors.Is(err, ErrTwoFactorUnavailable) {
		t.Fatalf("login error = %v, want ErrTwoFactorUnavailable", err)
	}
}

// A nil *twofactor.Service in the interface must not panic on the first login.
func TestSetTwoFactorRejectsATypedNil(t *testing.T) {
	h := newLoginHarness(t, false)

	var service *twofactor.Service
	h.service.SetTwoFactor(service)

	if _, err := h.login(); err != nil {
		t.Fatalf("Login with an unwired verifier: %v", err)
	}
}

// The password step must answer identically whether or not the account has a
// second factor, to a caller who does not know the password.
func TestWrongPasswordDoesNotDiscloseEnrolment(t *testing.T) {
	protected := newLoginHarness(t, true)
	protected.enrol()

	plain := newLoginHarness(t, false)

	_, protectedErr := protected.service.Login(context.Background(), models.LoginRequest{
		Username: protected.user.Username,
		Password: "wrong password",
	}, testIP)
	_, plainErr := plain.service.Login(context.Background(), models.LoginRequest{
		Username: plain.user.Username,
		Password: "wrong password",
	}, testIP)

	if protectedErr == nil || plainErr == nil {
		t.Fatal("a wrong password was accepted")
	}
	if protectedErr.Error() != plainErr.Error() {
		t.Fatalf("enrolment is observable from a failed password: %q vs %q",
			protectedErr.Error(), plainErr.Error())
	}

	// And an unknown user answers the same way again.
	_, unknownErr := protected.service.Login(context.Background(), models.LoginRequest{
		Username: "nobody",
		Password: "wrong password",
	}, testIP)
	if unknownErr == nil || unknownErr.Error() != protectedErr.Error() {
		t.Fatalf("an unknown user is distinguishable: %v", unknownErr)
	}
}

// The exchange says only that verification failed. It must not name the code,
// the user, or the password.
func TestExchangeFailureSaysNothingExtra(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	first, err := h.login()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	_, err = h.exchange(first.ChallengeToken, "000000")
	if err == nil {
		t.Fatal("a wrong code was accepted")
	}
	message := strings.ToLower(err.Error())
	for _, leak := range []string{"password", "username", "user", h.user.Username} {
		if strings.Contains(message, strings.ToLower(leak)) {
			t.Fatalf("exchange failure mentions %q: %q", leak, err.Error())
		}
	}
}

// Repeated failures on the exchange lock the account out, keyed by user id, so
// spreading the attempt over many addresses does not evade it.
func TestExchangeFailuresAreRateLimited(t *testing.T) {
	h := newLoginHarness(t, true)
	h.enrol()

	var lastErr error
	for i := 0; i < loginAttemptLimit+1; i++ {
		resp, err := h.login()
		if err != nil {
			lastErr = err
			break
		}
		_, lastErr = h.exchange(resp.ChallengeToken, "000000")
	}

	if !errors.Is(lastErr, ErrTooManyAttempts) {
		t.Fatalf("after %d failed exchanges the error was %v, want ErrTooManyAttempts",
			loginAttemptLimit+1, lastErr)
	}
	if !h.audit.has(twofactor.ActionLockedOut, "failure") {
		t.Fatal("the two-factor lockout was not audited")
	}
}
