package twofactor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

const testPassword = "correct horse battery staple"

// auditEntry is one captured audit call.
type auditEntry struct {
	Action  string
	Status  string
	Details models.JSONMap
}

type captureAudit struct {
	mu      sync.Mutex
	entries []auditEntry
}

func (a *captureAudit) Log(_ context.Context, _ uuid.UUID, _ *uuid.UUID, action, _ string,
	_ *uuid.UUID, details models.JSONMap, _, _, status string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, auditEntry{Action: action, Status: status, Details: details})
	return nil
}

func (a *captureAudit) count(action string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	total := 0
	for _, entry := range a.entries {
		if entry.Action == action {
			total++
		}
	}
	return total
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

// harness is a service wired to in-memory storage and a controllable clock.
type harness struct {
	t       *testing.T
	service *Service
	store   *MemoryStore
	audit   *captureAudit
	userID  uuid.UUID
	clock   time.Time
	mu      sync.Mutex
}

func newHarness(t *testing.T, opts Options) *harness {
	t.Helper()

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i + 1)
	}
	box, err := NewSecretBox(master)
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}

	hash, err := utils.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	store := NewMemoryStore()
	userID := uuid.New()
	store.AddAccount(Account{
		UserID:       userID,
		TenantID:     uuid.New(),
		Username:     "admin",
		Email:        "admin@example.test",
		PasswordHash: hash,
	})

	h := &harness{
		t:      t,
		store:  store,
		audit:  &captureAudit{},
		userID: userID,
		clock:  time.Unix(1700000000, 0).UTC(),
	}

	opts.Now = h.now
	// A generous limiter by default: the limiter has its own test.
	h.service = NewService(store, box, h.audit, NewMemoryLimiter(10000, time.Minute), nil, opts)
	return h
}

func (h *harness) now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(d)
}

func (h *harness) request(code string) Request {
	return Request{
		UserID:    h.userID,
		IPAddress: "203.0.113.10",
		UserAgent: "test-agent",
		Password:  testPassword,
		Code:      code,
	}
}

// currentCode returns the code the user's authenticator would show now.
func (h *harness) currentCode() string {
	h.t.Helper()
	enrolment, err := h.store.Get(context.Background(), h.userID)
	if err != nil || enrolment == nil {
		h.t.Fatalf("no enrolment stored: %v", err)
	}
	secret := h.openSecret(enrolment.SecretCiphertext)
	return CodeAt(secret, h.now(), enrolment.DigitCount(), enrolment.Period())
}

func (h *harness) codeAtStepOffset(offset int) string {
	h.t.Helper()
	enrolment, err := h.store.Get(context.Background(), h.userID)
	if err != nil || enrolment == nil {
		h.t.Fatalf("no enrolment stored: %v", err)
	}
	secret := h.openSecret(enrolment.SecretCiphertext)
	step := Step(h.now(), enrolment.Period())
	return HOTP(secret, uint64(int64(step)+int64(offset)), enrolment.DigitCount())
}

func (h *harness) openSecret(ciphertext string) []byte {
	h.t.Helper()
	secret, err := h.service.box.Open(ciphertext)
	if err != nil {
		h.t.Fatalf("Open sealed secret: %v", err)
	}
	return secret
}

// enrol runs the full enable path and returns the recovery codes.
func (h *harness) enrol() []string {
	h.t.Helper()
	ctx := context.Background()

	if _, err := h.service.StartEnrolment(ctx, h.request("")); err != nil {
		h.t.Fatalf("StartEnrolment: %v", err)
	}
	set, err := h.service.ConfirmEnrolment(ctx, h.request(h.currentCode()))
	if err != nil {
		h.t.Fatalf("ConfirmEnrolment: %v", err)
	}

	// Confirmation spends the proving step, so move the clock past it: a test
	// that verified immediately would be exercising the replay guard by
	// accident rather than the case it means to check.
	h.advance(2 * DefaultPeriod)
	return set.Codes
}

// ---------------------------------------------------------------------------
// Enrolment
// ---------------------------------------------------------------------------

// TestEnrolmentDoesNotEnableWithoutAProvenCode is the requirement that keeps
// people out of the support queue: generating a secret must not turn the second
// factor on, because an app that failed to take the secret would lock the owner
// out of their own panel.
func TestEnrolmentDoesNotEnableWithoutAProvenCode(t *testing.T) {
	h := newHarness(t, Options{})
	ctx := context.Background()

	start, err := h.service.StartEnrolment(ctx, h.request(""))
	if err != nil {
		t.Fatalf("StartEnrolment: %v", err)
	}
	if start.Secret == "" || !strings.HasPrefix(start.OTPAuthURI, "otpauth://totp/") {
		t.Fatalf("enrolment did not return usable material: %+v", start)
	}

	status, err := h.service.Status(ctx, h.userID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Enabled {
		t.Fatal("two-factor reported as enabled straight after generating a secret")
	}
	if !status.PendingEnrolment {
		t.Fatal("a started enrolment is not reported as pending")
	}
	if h.store.UserFlag(h.userID) {
		t.Fatal("users.mfa_enabled was set before a code was proved")
	}

	// A wrong code must not enable it either.
	wrong := "000000"
	if wrong == h.currentCode() {
		wrong = "111111"
	}
	if _, err := h.service.ConfirmEnrolment(ctx, h.request(wrong)); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("ConfirmEnrolment with a wrong code returned %v, want ErrVerificationFailed", err)
	}

	status, _ = h.service.Status(ctx, h.userID)
	if status.Enabled {
		t.Fatal("a failed confirmation enabled two-factor")
	}
	if !h.audit.has(ActionVerificationFailed, "failure") {
		t.Fatal("a failed confirmation was not audited")
	}

	// The correct code enables it, once.
	set, err := h.service.ConfirmEnrolment(ctx, h.request(h.currentCode()))
	if err != nil {
		t.Fatalf("ConfirmEnrolment with the right code: %v", err)
	}
	if len(set.Codes) != DefaultRecoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(set.Codes), DefaultRecoveryCodeCount)
	}

	status, _ = h.service.Status(ctx, h.userID)
	if !status.Enabled {
		t.Fatal("two-factor is not enabled after a proven code")
	}
	if !h.store.UserFlag(h.userID) {
		t.Fatal("users.mfa_enabled was not mirrored on enable")
	}
	if !h.audit.has(ActionEnabled, "success") {
		t.Fatal("enabling two-factor was not audited")
	}
}

// TestStartEnrolmentRequiresPassword: a session left open must not be enough to
// move the second factor to somebody else's phone.
func TestStartEnrolmentRequiresPassword(t *testing.T) {
	h := newHarness(t, Options{})
	req := h.request("")
	req.Password = "wrong password"

	if _, err := h.service.StartEnrolment(context.Background(), req); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("StartEnrolment with a wrong password returned %v, want ErrVerificationFailed", err)
	}
	if enrolment, _ := h.store.Get(context.Background(), h.userID); enrolment != nil {
		t.Fatal("a secret was generated for a caller who could not prove the password")
	}
}

// TestEnrolmentCannotOverwriteALiveFactor: re-enrolling must go through
// disable, which needs the password and a current code.
func TestEnrolmentCannotOverwriteALiveFactor(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()

	if _, err := h.service.StartEnrolment(context.Background(), h.request("")); !errors.Is(err, ErrAlreadyEnabled) {
		t.Fatalf("StartEnrolment on an enabled account returned %v, want ErrAlreadyEnabled", err)
	}
}

// TestPendingEnrolmentExpires: an abandoned secret does not stay usable.
func TestPendingEnrolmentExpires(t *testing.T) {
	h := newHarness(t, Options{EnrolmentTTL: 10 * time.Minute})
	ctx := context.Background()

	if _, err := h.service.StartEnrolment(ctx, h.request("")); err != nil {
		t.Fatalf("StartEnrolment: %v", err)
	}
	code := h.currentCode()
	h.advance(11 * time.Minute)

	if _, err := h.service.ConfirmEnrolment(ctx, h.request(code)); !errors.Is(err, ErrNoPendingEnrolment) {
		t.Fatalf("ConfirmEnrolment after expiry returned %v, want ErrNoPendingEnrolment", err)
	}
}

// ---------------------------------------------------------------------------
// Verification, drift and replay
// ---------------------------------------------------------------------------

// TestCodeCannotBeReplayedInsideItsWindow is the property that makes a stolen
// code useless: it is valid for ninety seconds, but only once.
func TestCodeCannotBeReplayedInsideItsWindow(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()
	ctx := context.Background()

	code := h.currentCode()
	if _, err := h.service.Verify(ctx, h.request(code)); err != nil {
		t.Fatalf("first use of a valid code failed: %v", err)
	}

	// Same code, same step, still inside its validity window.
	if _, err := h.service.Verify(ctx, h.request(code)); !errors.Is(err, ErrCodeReplayed) {
		t.Fatalf("replayed code returned %v, want ErrCodeReplayed", err)
	}

	// Ten seconds later the code is still nominally valid, and still refused.
	h.advance(10 * time.Second)
	if _, err := h.service.Verify(ctx, h.request(code)); !errors.Is(err, ErrCodeReplayed) {
		t.Fatalf("replayed code inside its window returned %v, want ErrCodeReplayed", err)
	}
	if !h.audit.has(ActionVerificationFailed, "failure") {
		t.Fatal("a replayed code was not audited as a failed verification")
	}
}

// TestOlderStepRejectedAfterUse: accepting drift must not reopen a step that
// has already been spent.
func TestOlderStepRejectedAfterUse(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()
	ctx := context.Background()

	if _, err := h.service.Verify(ctx, h.request(h.currentCode())); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// The previous step is inside the drift window but behind the spent step.
	if _, err := h.service.Verify(ctx, h.request(h.codeAtStepOffset(-1))); !errors.Is(err, ErrCodeReplayed) {
		t.Fatalf("an already-passed step was accepted: %v", err)
	}

	// The next step is ahead of it and is accepted.
	if _, err := h.service.Verify(ctx, h.request(h.codeAtStepOffset(1))); err != nil {
		t.Fatalf("the next step was refused: %v", err)
	}
}

// TestDriftWindowAcceptedThroughTheService checks the service applies the same
// window the algorithm tests pin, including the step just outside it.
func TestDriftWindowAcceptedThroughTheService(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()
	ctx := context.Background()

	if _, err := h.service.Verify(ctx, h.request(h.codeAtStepOffset(-1))); err != nil {
		t.Fatalf("a code one step behind was refused: %v", err)
	}

	h.advance(2 * time.Minute)
	if _, err := h.service.Verify(ctx, h.request(h.codeAtStepOffset(2))); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("a code two steps ahead was accepted: %v", err)
	}
}

// TestFailedVerificationsAreAuditedAndLockOut: six digits is a million
// combinations, so failures must both be visible and eventually stop.
func TestFailedVerificationsAreAuditedAndLockOut(t *testing.T) {
	h := newHarness(t, Options{FailureThreshold: 3, LockoutDuration: 15 * time.Minute})
	h.enrol()
	ctx := context.Background()

	before := h.audit.count(ActionVerificationFailed)
	for i := 0; i < 3; i++ {
		if _, err := h.service.Verify(ctx, h.request("000001")); err == nil {
			t.Fatal("a wrong code was accepted")
		}
	}
	if got := h.audit.count(ActionVerificationFailed) - before; got != 3 {
		t.Fatalf("audited %d failed verifications, want 3", got)
	}
	if !h.audit.has(ActionLockedOut, "failure") {
		t.Fatal("the lockout was not audited")
	}

	// Even the right code is refused while locked out.
	if _, err := h.service.Verify(ctx, h.request(h.currentCode())); !errors.Is(err, ErrLockedOut) {
		t.Fatalf("verification during lockout returned %v, want ErrLockedOut", err)
	}

	h.advance(16 * time.Minute)
	if _, err := h.service.Verify(ctx, h.request(h.currentCode())); err != nil {
		t.Fatalf("verification after the lockout expired failed: %v", err)
	}
}

// TestRateLimiterStopsGuessing: the limiter is consulted before any credential
// is compared, and a limiter that errors is treated as a refusal.
func TestRateLimiterStopsGuessing(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()
	ctx := context.Background()

	h.service.limiter = NewMemoryLimiter(2, time.Minute)
	for i := 0; i < 2; i++ {
		_, _ = h.service.Verify(ctx, h.request("000000"))
	}
	if _, err := h.service.Verify(ctx, h.request(h.currentCode())); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Verify past the limit returned %v, want ErrRateLimited", err)
	}

	h.service.limiter = failingLimiter{}
	if _, err := h.service.Verify(ctx, h.request(h.currentCode())); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("a limiter error returned %v, want ErrRateLimited", err)
	}
}

type failingLimiter struct{}

func (failingLimiter) Allow(context.Context, string) (bool, error) {
	return true, errors.New("limiter backend unavailable")
}

// ---------------------------------------------------------------------------
// Recovery codes
// ---------------------------------------------------------------------------

// TestRecoveryCodeIsSingleUse: a recovery code is a bypass of the second
// factor, so it must work exactly once.
func TestRecoveryCodeIsSingleUse(t *testing.T) {
	h := newHarness(t, Options{})
	codes := h.enrol()
	ctx := context.Background()

	result, err := h.service.Verify(ctx, h.request(codes[0]))
	if err != nil {
		t.Fatalf("first use of a recovery code failed: %v", err)
	}
	if result.Method != MethodRecoveryCode {
		t.Fatalf("method is %q, want %q", result.Method, MethodRecoveryCode)
	}
	if result.RecoveryCodesRemaining != DefaultRecoveryCodeCount-1 {
		t.Fatalf("%d codes remain, want %d", result.RecoveryCodesRemaining, DefaultRecoveryCodeCount-1)
	}

	if _, err := h.service.Verify(ctx, h.request(codes[0])); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("a spent recovery code was accepted: %v", err)
	}

	// A different code still works, and typing is forgiving.
	if _, err := h.service.Verify(ctx, h.request(strings.ToLower(codes[1]))); err != nil {
		t.Fatalf("a lower-case recovery code was refused: %v", err)
	}

	if !h.audit.has(ActionRecoveryCodeUsed, "success") {
		t.Fatal("using a recovery code was not audited")
	}
}

// TestRecoveryCodesWarnWhenFewRemain: running out silently leaves the account
// one lost phone away from an out-of-band recovery process.
func TestRecoveryCodesWarnWhenFewRemain(t *testing.T) {
	h := newHarness(t, Options{RecoveryCodeCount: 4, LowRecoveryThreshold: 3})
	codes := h.enrol()
	ctx := context.Background()

	status, _ := h.service.Status(ctx, h.userID)
	if status.RecoveryCodesLow {
		t.Fatal("a full set of recovery codes is reported as low")
	}

	if _, err := h.service.Verify(ctx, h.request(codes[0])); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	status, _ = h.service.Status(ctx, h.userID)
	if !status.RecoveryCodesLow {
		t.Fatal("three remaining codes are not reported as low")
	}
	if status.RecoveryCodesRemaining != 3 || status.RecoveryCodesTotal != 4 {
		t.Fatalf("counts are %d/%d, want 3/4", status.RecoveryCodesRemaining, status.RecoveryCodesTotal)
	}
}

// TestRegenerateRecoveryCodesInvalidatesTheOldSet.
func TestRegenerateRecoveryCodesInvalidatesTheOldSet(t *testing.T) {
	h := newHarness(t, Options{RecoveryCodeCount: 3})
	old := h.enrol()
	ctx := context.Background()

	set, err := h.service.RegenerateRecoveryCodes(ctx, h.request(h.currentCode()))
	if err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}
	if len(set.Codes) != 3 {
		t.Fatalf("got %d codes, want 3", len(set.Codes))
	}

	h.advance(1 * time.Minute)
	if _, err := h.service.Verify(ctx, h.request(old[0])); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("an old recovery code still works: %v", err)
	}
	if _, err := h.service.Verify(ctx, h.request(set.Codes[0])); err != nil {
		t.Fatalf("a new recovery code was refused: %v", err)
	}
}

// TestRegenerateRequiresPasswordAndCode.
func TestRegenerateRequiresPasswordAndCode(t *testing.T) {
	h := newHarness(t, Options{RecoveryCodeCount: 3})
	h.enrol()
	ctx := context.Background()

	req := h.request(h.currentCode())
	req.Password = "wrong password"
	if _, err := h.service.RegenerateRecoveryCodes(ctx, req); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("regeneration without the password returned %v", err)
	}

	if _, err := h.service.RegenerateRecoveryCodes(ctx, h.request("000000")); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("regeneration without a valid code returned %v", err)
	}
}

// ---------------------------------------------------------------------------
// Disabling
// ---------------------------------------------------------------------------

// TestDisableRequiresPasswordAndCode: turning the second factor off is the
// first thing an attacker does with a stolen session.
func TestDisableRequiresPasswordAndCode(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()
	ctx := context.Background()

	wrongPassword := h.request(h.currentCode())
	wrongPassword.Password = "wrong password"
	if err := h.service.Disable(ctx, wrongPassword); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("Disable without the password returned %v", err)
	}

	if err := h.service.Disable(ctx, h.request("000000")); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("Disable without a valid code returned %v", err)
	}

	status, _ := h.service.Status(ctx, h.userID)
	if !status.Enabled {
		t.Fatal("a failed disable turned two-factor off")
	}

	h.advance(1 * time.Minute)
	if err := h.service.Disable(ctx, h.request(h.currentCode())); err != nil {
		t.Fatalf("Disable with both credentials failed: %v", err)
	}

	status, _ = h.service.Status(ctx, h.userID)
	if status.Enabled {
		t.Fatal("two-factor is still enabled after a successful disable")
	}
	if h.store.UserFlag(h.userID) {
		t.Fatal("users.mfa_enabled was not cleared on disable")
	}
	if !h.audit.has(ActionDisabled, "success") {
		t.Fatal("disabling two-factor was not audited")
	}

	// Every credential is gone with it.
	unused, total, _ := h.store.CountRecoveryCodes(ctx, h.userID)
	if unused != 0 || total != 0 {
		t.Fatalf("recovery codes survived a disable: %d/%d", unused, total)
	}
}

// TestVerifyRequiresAnEnabledFactor.
func TestVerifyRequiresAnEnabledFactor(t *testing.T) {
	h := newHarness(t, Options{})
	if _, err := h.service.Verify(context.Background(), h.request("000000")); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("Verify without an enrolment returned %v, want ErrNotEnrolled", err)
	}
}

// TestStoredSecretIsNotPlaintext: a database dump must not hand over the second
// factors it contains.
func TestStoredSecretIsNotPlaintext(t *testing.T) {
	h := newHarness(t, Options{})
	ctx := context.Background()

	start, err := h.service.StartEnrolment(ctx, h.request(""))
	if err != nil {
		t.Fatalf("StartEnrolment: %v", err)
	}

	enrolment, err := h.store.Get(ctx, h.userID)
	if err != nil || enrolment == nil {
		t.Fatalf("stored enrolment: %v", err)
	}
	if strings.Contains(enrolment.SecretCiphertext, start.Secret) {
		t.Fatal("the base32 secret appears in the stored record")
	}
	secret, err := DecodeSecret(start.Secret)
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	if strings.Contains(enrolment.SecretCiphertext, string(secret)) {
		t.Fatal("the raw secret appears in the stored record")
	}
	if enrolment.KeyVersion != CurrentKeyVersion {
		t.Fatalf("key version is %d, want %d", enrolment.KeyVersion, CurrentKeyVersion)
	}
}

// TestRequiredReportsConfirmedFactorsOnly is what the login path asks: a
// started-but-unconfirmed enrolment must not make the panel demand a code the
// user cannot yet produce.
func TestRequiredReportsConfirmedFactorsOnly(t *testing.T) {
	h := newHarness(t, Options{})
	ctx := context.Background()

	required, err := h.service.Required(ctx, h.userID)
	if err != nil {
		t.Fatalf("Required: %v", err)
	}
	if required {
		t.Fatal("an account with no enrolment is reported as requiring a second factor")
	}

	if _, err := h.service.StartEnrolment(ctx, h.request("")); err != nil {
		t.Fatalf("StartEnrolment: %v", err)
	}
	if required, _ = h.service.Required(ctx, h.userID); required {
		t.Fatal("a pending enrolment is reported as requiring a second factor")
	}

	if _, err := h.service.ConfirmEnrolment(ctx, h.request(h.currentCode())); err != nil {
		t.Fatalf("ConfirmEnrolment: %v", err)
	}
	if required, _ = h.service.Required(ctx, h.userID); !required {
		t.Fatal("a confirmed enrolment is not reported as requiring a second factor")
	}
}

// TestStoreRefusesToOverwriteAConfirmedEnrolment guards the store itself, not
// just the service check above it: a race between two enrolment requests must
// not be able to replace a live second factor.
func TestStoreRefusesToOverwriteAConfirmedEnrolment(t *testing.T) {
	h := newHarness(t, Options{})
	h.enrol()

	err := h.store.Save(context.Background(), &Enrolment{
		UserID:           h.userID,
		SecretCiphertext: "replacement",
		Digits:           DefaultDigits,
		PeriodSeconds:    30,
	})
	if !errors.Is(err, ErrAlreadyEnabled) {
		t.Fatalf("Save over a confirmed enrolment returned %v, want ErrAlreadyEnabled", err)
	}
}
