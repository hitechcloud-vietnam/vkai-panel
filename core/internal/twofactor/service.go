package twofactor

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Audit actions. They share the "two_factor" resource so one audit query shows
// the whole life cycle of an account's second factor.
const (
	AuditResource = "two_factor"

	ActionEnrolmentStarted    = "two_factor.enrolment_started"
	ActionEnabled             = "two_factor.enabled"
	ActionDisabled            = "two_factor.disabled"
	ActionVerified            = "two_factor.verified"
	ActionVerificationFailed  = "two_factor.verification_failed"
	ActionRecoveryCodeUsed    = "two_factor.recovery_code_used"
	ActionRecoveryCodesIssued = "two_factor.recovery_codes_issued"
	ActionLockedOut           = "two_factor.locked_out"

	auditStatusSuccess = "success"
	auditStatusFailure = "failure"
)

// AuditLogger is the audit sink. The signature is exactly
// service.AuditService.Log, so the panel's audit service satisfies it without
// an adapter.
type AuditLogger interface {
	Log(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, action, resource string,
		resourceID *uuid.UUID, details models.JSONMap, ipAddress, userAgent, status string) error
}

// Options tunes the service. The zero value is filled in with the defaults
// below, all of which are stated in the provisioning URI where an authenticator
// app needs to agree with them.
type Options struct {
	Issuer               string
	Digits               int
	Period               time.Duration
	Skew                 int
	RecoveryCodeCount    int
	LowRecoveryThreshold int

	// FailureThreshold and LockoutDuration are the per-account lockout. Unlike
	// the rate limiter, which is keyed by source address, this one follows the
	// account, so spreading an attack over many addresses does not evade it.
	FailureThreshold int
	LockoutDuration  time.Duration

	// EnrolmentTTL is how long an unconfirmed enrolment stays usable. A secret
	// generated and then abandoned should not sit there forever.
	EnrolmentTTL time.Duration

	// Now is injectable so tests can drive the clock.
	Now func() time.Time
}

func (o Options) withDefaults() Options {
	if o.Issuer == "" {
		o.Issuer = "VKAI Panel"
	}
	if o.Digits <= 0 {
		o.Digits = DefaultDigits
	}
	if o.Period <= 0 {
		o.Period = DefaultPeriod
	}
	if o.Skew < 0 {
		o.Skew = DefaultSkew
	}
	if o.Skew == 0 {
		o.Skew = DefaultSkew
	}
	if o.RecoveryCodeCount <= 0 {
		o.RecoveryCodeCount = DefaultRecoveryCodeCount
	}
	if o.LowRecoveryThreshold <= 0 {
		o.LowRecoveryThreshold = DefaultLowRecoveryThreshold
	}
	if o.FailureThreshold <= 0 {
		o.FailureThreshold = 5
	}
	if o.LockoutDuration <= 0 {
		o.LockoutDuration = 15 * time.Minute
	}
	if o.EnrolmentTTL <= 0 {
		o.EnrolmentTTL = 15 * time.Minute
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Now().UTC() }
	}
	return o
}

// Service is the two-factor life cycle: enrol, confirm, verify, disable.
type Service struct {
	store   Store
	box     *SecretBox
	audit   AuditLogger
	limiter Limiter
	logger  *zap.Logger
	opts    Options
}

// NewService builds the service. A nil limiter falls back to the in-process
// limiter rather than to no limit at all.
func NewService(store Store, box *SecretBox, audit AuditLogger, limiter Limiter, logger *zap.Logger, opts Options) *Service {
	if limiter == nil {
		limiter = NewMemoryLimiter(DefaultVerifyLimit, DefaultVerifyWindow)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		store:   store,
		box:     box,
		audit:   audit,
		limiter: limiter,
		logger:  logger,
		opts:    opts.withDefaults(),
	}
}

// ---------------------------------------------------------------------------
// Requests and results
// ---------------------------------------------------------------------------

// Request carries the caller identity and the request context every operation
// needs for auditing.
type Request struct {
	UserID    uuid.UUID
	IPAddress string
	UserAgent string
	Password  string
	Code      string
}

// Status is what the settings screen renders.
type Status struct {
	Enabled                bool       `json:"enabled"`
	PendingEnrolment       bool       `json:"pending_enrolment"`
	ConfirmedAt            *time.Time `json:"confirmed_at"`
	LastUsedAt             *time.Time `json:"last_used_at"`
	LockedUntil            *time.Time `json:"locked_until"`
	RecoveryCodesRemaining int        `json:"recovery_codes_remaining"`
	RecoveryCodesTotal     int        `json:"recovery_codes_total"`
	RecoveryCodesLow       bool       `json:"recovery_codes_low"`
	Algorithm              string     `json:"algorithm"`
	Digits                 int        `json:"digits"`
	PeriodSeconds          int        `json:"period_seconds"`
}

// EnrolmentStart is the material an authenticator app needs. The secret is
// returned in the clear here and only here, to the authenticated owner of the
// account, because there is no other way for a phone to learn it.
type EnrolmentStart struct {
	Secret        string    `json:"secret"`
	OTPAuthURI    string    `json:"otpauth_uri"`
	Issuer        string    `json:"issuer"`
	Account       string    `json:"account"`
	Algorithm     string    `json:"algorithm"`
	Digits        int       `json:"digits"`
	PeriodSeconds int       `json:"period_seconds"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// RecoveryCodeSet is shown exactly once. The panel cannot show it again: only
// bcrypt hashes are kept.
type RecoveryCodeSet struct {
	Codes       []string  `json:"codes"`
	Count       int       `json:"count"`
	GeneratedAt time.Time `json:"generated_at"`
}

// VerifyResult reports which factor was accepted and how much recovery budget
// is left.
type VerifyResult struct {
	Method                 string `json:"method"`
	RecoveryCodesRemaining int    `json:"recovery_codes_remaining"`
	RecoveryCodesLow       bool   `json:"recovery_codes_low"`
}

const (
	// MethodTOTP and MethodRecoveryCode name the accepted factor.
	MethodTOTP         = "totp"
	MethodRecoveryCode = "recovery_code"
)

// ---------------------------------------------------------------------------
// Life cycle
// ---------------------------------------------------------------------------

// Status returns the current state of an account's second factor.
func (s *Service) Status(ctx context.Context, userID uuid.UUID) (*Status, error) {
	enrolment, err := s.store.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	status := &Status{
		Algorithm:     AlgorithmSHA1,
		Digits:        s.opts.Digits,
		PeriodSeconds: int(s.opts.Period / time.Second),
	}
	if enrolment == nil {
		return status, nil
	}

	status.Enabled = enrolment.Enabled
	status.PendingEnrolment = !enrolment.Enabled && !s.enrolmentExpired(enrolment)
	status.ConfirmedAt = enrolment.ConfirmedAt
	status.LastUsedAt = enrolment.LastUsedAt
	status.Algorithm = enrolment.Algorithm
	status.Digits = enrolment.DigitCount()
	status.PeriodSeconds = int(enrolment.Period() / time.Second)

	if enrolment.LockedUntil != nil && enrolment.LockedUntil.After(s.now()) {
		status.LockedUntil = enrolment.LockedUntil
	}

	if enrolment.Enabled {
		unused, total, err := s.store.CountRecoveryCodes(ctx, userID)
		if err != nil {
			return nil, err
		}
		status.RecoveryCodesRemaining = unused
		status.RecoveryCodesTotal = total
		status.RecoveryCodesLow = unused <= s.opts.LowRecoveryThreshold
	}

	return status, nil
}

// StartEnrolment generates a secret and returns it for the user's
// authenticator app. It does NOT enable two-factor: an account is only
// protected once ConfirmEnrolment has seen a working code. Enabling at
// generation time locks people out of their own panel whenever the app fails to
// take the secret.
func (s *Service) StartEnrolment(ctx context.Context, req Request) (*EnrolmentStart, error) {
	if err := s.checkLimit(ctx, "enrol", req); err != nil {
		return nil, err
	}

	account, err := s.store.Account(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	// Re-authentication: a session left open on an unlocked laptop must not be
	// enough to move the second factor to an attacker's phone.
	if !utils.CheckPassword(req.Password, account.PasswordHash) {
		s.record(ctx, account, ActionVerificationFailed, auditStatusFailure, req, models.JSONMap{
			"stage":  "enrolment_start",
			"reason": "password_mismatch",
		})
		return nil, ErrVerificationFailed
	}

	existing, err := s.store.Get(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Enabled {
		return nil, ErrAlreadyEnabled
	}

	secret, err := GenerateSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.Seal(secret)
	if err != nil {
		return nil, err
	}

	now := s.now()
	enrolment := &Enrolment{
		UserID:           account.UserID,
		TenantID:         account.TenantID,
		KeyVersion:       s.box.Version(),
		SecretCiphertext: sealed,
		Algorithm:        AlgorithmSHA1,
		Digits:           s.opts.Digits,
		PeriodSeconds:    int(s.opts.Period / time.Second),
		Enabled:          false,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.Save(ctx, enrolment); err != nil {
		return nil, err
	}

	s.record(ctx, account, ActionEnrolmentStarted, auditStatusSuccess, req, nil)

	return &EnrolmentStart{
		Secret:        EncodeSecret(secret),
		OTPAuthURI:    ProvisioningURI(s.opts.Issuer, account.Username, secret, s.opts.Digits, s.opts.Period),
		Issuer:        s.opts.Issuer,
		Account:       account.Username,
		Algorithm:     AlgorithmSHA1,
		Digits:        s.opts.Digits,
		PeriodSeconds: int(s.opts.Period / time.Second),
		ExpiresAt:     now.Add(s.opts.EnrolmentTTL),
	}, nil
}

// ConfirmEnrolment enables two-factor once, and only once, the user has proved
// a code from the pending secret. It returns the recovery codes, which are
// shown exactly once.
func (s *Service) ConfirmEnrolment(ctx context.Context, req Request) (*RecoveryCodeSet, error) {
	if err := s.checkLimit(ctx, "confirm", req); err != nil {
		return nil, err
	}

	account, err := s.store.Account(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	enrolment, err := s.store.Get(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if enrolment == nil {
		return nil, ErrNoPendingEnrolment
	}
	if enrolment.Enabled {
		return nil, ErrAlreadyEnabled
	}
	if s.enrolmentExpired(enrolment) {
		return nil, ErrNoPendingEnrolment
	}

	secret, err := s.box.Open(enrolment.SecretCiphertext)
	if err != nil {
		return nil, err
	}

	step, ok := MatchStep(secret, req.Code, s.now(), enrolment.DigitCount(), enrolment.Period(), s.opts.Skew)
	if !ok {
		s.failVerification(ctx, account, req, "enrolment_confirm", "code_mismatch")
		return nil, ErrVerificationFailed
	}

	// The proving step is spent as part of confirmation, so the very code that
	// enabled the factor cannot immediately be replayed to satisfy a challenge.
	if err := s.store.Confirm(ctx, req.UserID, int64(step), s.now()); err != nil {
		return nil, err
	}

	codes, err := s.issueRecoveryCodes(ctx, account, req)
	if err != nil {
		return nil, err
	}

	// Mirror the state onto users.mfa_enabled, which the rest of the panel
	// already reads. A failure here is logged rather than fatal: the second
	// factor is on, and the source of truth is user_two_factor.
	if err := s.store.SetUserFlag(ctx, req.UserID, true); err != nil {
		s.logger.Error("failed to mirror mfa flag onto user record",
			zap.String("user_id", req.UserID.String()), zap.Error(err))
	}

	s.record(ctx, account, ActionEnabled, auditStatusSuccess, req, models.JSONMap{
		"recovery_codes": len(codes.Codes),
	})

	return codes, nil
}

// Required reports whether an account has a confirmed second factor, which is
// what the login path needs to decide whether to demand a code. It is a plain
// read: it neither audits nor consumes rate limit budget.
func (s *Service) Required(ctx context.Context, userID uuid.UUID) (bool, error) {
	enrolment, err := s.store.Get(ctx, userID)
	if err != nil {
		return false, err
	}
	return enrolment != nil && enrolment.Enabled, nil
}

// Verify checks one submitted factor - a TOTP code or a recovery code - for an
// account that already has two-factor enabled. Every failure is audited.
func (s *Service) Verify(ctx context.Context, req Request) (*VerifyResult, error) {
	if err := s.checkLimit(ctx, "verify", req); err != nil {
		return nil, err
	}

	account, err := s.store.Account(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	return s.verifyFactor(ctx, account, req, "verify")
}

// Disable turns two-factor off. It requires the current password AND a current
// code: either one alone is a credential an attacker may already hold, and
// turning off the second factor is exactly what an attacker wants first.
func (s *Service) Disable(ctx context.Context, req Request) error {
	if err := s.checkLimit(ctx, "disable", req); err != nil {
		return err
	}

	account, err := s.store.Account(ctx, req.UserID)
	if err != nil {
		return err
	}

	if !utils.CheckPassword(req.Password, account.PasswordHash) {
		s.failVerification(ctx, account, req, "disable", "password_mismatch")
		return ErrVerificationFailed
	}

	if _, err := s.verifyFactor(ctx, account, req, "disable"); err != nil {
		return err
	}

	if err := s.store.Delete(ctx, req.UserID); err != nil {
		return err
	}
	if err := s.store.SetUserFlag(ctx, req.UserID, false); err != nil {
		s.logger.Error("failed to clear mfa flag on user record",
			zap.String("user_id", req.UserID.String()), zap.Error(err))
	}

	s.record(ctx, account, ActionDisabled, auditStatusSuccess, req, nil)
	return nil
}

// RegenerateRecoveryCodes issues a fresh set and invalidates the old one. Like
// disabling, it needs the password and a current code, because a fresh set of
// codes is a fresh set of bypasses.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, req Request) (*RecoveryCodeSet, error) {
	if err := s.checkLimit(ctx, "recovery", req); err != nil {
		return nil, err
	}

	account, err := s.store.Account(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if !utils.CheckPassword(req.Password, account.PasswordHash) {
		s.failVerification(ctx, account, req, "regenerate_recovery_codes", "password_mismatch")
		return nil, ErrVerificationFailed
	}

	enrolment, err := s.store.Get(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if enrolment == nil || !enrolment.Enabled {
		return nil, ErrNotEnrolled
	}

	if _, err := s.verifyFactor(ctx, account, req, "regenerate_recovery_codes"); err != nil {
		return nil, err
	}

	return s.issueRecoveryCodes(ctx, account, req)
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// verifyFactor accepts either a TOTP code or a recovery code for an enabled
// account, applying the lockout and the replay guard.
func (s *Service) verifyFactor(ctx context.Context, account *Account, req Request, stage string) (*VerifyResult, error) {
	enrolment, err := s.store.Get(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if enrolment == nil || !enrolment.Enabled {
		return nil, ErrNotEnrolled
	}

	now := s.now()
	if enrolment.LockedUntil != nil && enrolment.LockedUntil.After(now) {
		s.record(ctx, account, ActionVerificationFailed, auditStatusFailure, req, models.JSONMap{
			"stage":        stage,
			"reason":       "locked_out",
			"locked_until": enrolment.LockedUntil.UTC().Format(time.RFC3339),
		})
		return nil, ErrLockedOut
	}

	submitted := strings.TrimSpace(req.Code)
	if submitted == "" {
		s.failVerification(ctx, account, req, stage, "empty_code")
		return nil, ErrVerificationFailed
	}

	// A submission of exactly the configured number of digits is a TOTP code;
	// anything else is treated as a recovery code. Recovery codes are drawn
	// from an alphabet that contains no digits-only strings of that length.
	if isDigits(submitted) && len(submitted) == enrolment.DigitCount() {
		return s.verifyTOTP(ctx, account, enrolment, req, stage)
	}

	return s.verifyRecoveryCode(ctx, account, req, stage)
}

func (s *Service) verifyTOTP(ctx context.Context, account *Account, enrolment *Enrolment, req Request, stage string) (*VerifyResult, error) {
	secret, err := s.box.Open(enrolment.SecretCiphertext)
	if err != nil {
		return nil, err
	}

	step, ok := MatchStep(secret, req.Code, s.now(), enrolment.DigitCount(), enrolment.Period(), s.opts.Skew)
	if !ok {
		s.failVerification(ctx, account, req, stage, "code_mismatch")
		return nil, ErrVerificationFailed
	}

	// Replay guard. The code was right, but if its step has already been spent
	// the code is being reused inside its own validity window - which is what a
	// shoulder-surfed or phished code looks like.
	spent, err := s.store.SpendStep(ctx, req.UserID, int64(step), s.now())
	if err != nil {
		return nil, err
	}
	if !spent {
		s.failVerification(ctx, account, req, stage, "code_replayed")
		return nil, ErrCodeReplayed
	}

	if err := s.store.ClearFailures(ctx, req.UserID); err != nil {
		s.logger.Warn("failed to clear two-factor failure counter", zap.Error(err))
	}

	unused, _, err := s.store.CountRecoveryCodes(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	s.record(ctx, account, ActionVerified, auditStatusSuccess, req, models.JSONMap{
		"stage":  stage,
		"method": MethodTOTP,
	})

	return &VerifyResult{
		Method:                 MethodTOTP,
		RecoveryCodesRemaining: unused,
		RecoveryCodesLow:       unused <= s.opts.LowRecoveryThreshold,
	}, nil
}

func (s *Service) verifyRecoveryCode(ctx context.Context, account *Account, req Request, stage string) (*VerifyResult, error) {
	codes, err := s.store.UnusedRecoveryCodes(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	for _, code := range codes {
		if !CheckRecoveryCode(req.Code, code.CodeHash) {
			continue
		}

		// Single use. The conditional update is what makes it single use even
		// when two requests carry the same code at the same instant.
		spent, err := s.store.SpendRecoveryCode(ctx, code.ID, s.now(), req.IPAddress)
		if err != nil {
			return nil, err
		}
		if !spent {
			s.failVerification(ctx, account, req, stage, "recovery_code_already_used")
			return nil, ErrVerificationFailed
		}

		if err := s.store.ClearFailures(ctx, req.UserID); err != nil {
			s.logger.Warn("failed to clear two-factor failure counter", zap.Error(err))
		}

		unused, total, err := s.store.CountRecoveryCodes(ctx, req.UserID)
		if err != nil {
			return nil, err
		}

		s.record(ctx, account, ActionRecoveryCodeUsed, auditStatusSuccess, req, models.JSONMap{
			"stage":     stage,
			"remaining": unused,
			"total":     total,
		})

		return &VerifyResult{
			Method:                 MethodRecoveryCode,
			RecoveryCodesRemaining: unused,
			RecoveryCodesLow:       unused <= s.opts.LowRecoveryThreshold,
		}, nil
	}

	s.failVerification(ctx, account, req, stage, "recovery_code_mismatch")
	return nil, ErrVerificationFailed
}

// issueRecoveryCodes generates, stores and returns a fresh set.
func (s *Service) issueRecoveryCodes(ctx context.Context, account *Account, req Request) (*RecoveryCodeSet, error) {
	codes, err := GenerateRecoveryCodes(s.opts.RecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := s.store.ReplaceRecoveryCodes(ctx, req.UserID, hashes, now); err != nil {
		return nil, err
	}

	s.record(ctx, account, ActionRecoveryCodesIssued, auditStatusSuccess, req, models.JSONMap{
		"count": len(codes),
	})

	return &RecoveryCodeSet{Codes: codes, Count: len(codes), GeneratedAt: now}, nil
}

// failVerification records a failure against the account, applies the lockout
// when the threshold is reached, and writes the audit entry. Requirement: every
// failed verification is audited, successful or not, so a brute force attempt
// is visible in the log rather than only in a counter.
func (s *Service) failVerification(ctx context.Context, account *Account, req Request, stage, reason string) {
	details := models.JSONMap{"stage": stage, "reason": reason}

	attempts, lockedUntil, err := s.store.RecordFailure(ctx, req.UserID, s.now(), s.opts.FailureThreshold, s.opts.LockoutDuration)
	if err != nil {
		s.logger.Error("failed to record two-factor failure", zap.Error(err))
	} else {
		details["failed_attempts"] = attempts
	}

	s.record(ctx, account, ActionVerificationFailed, auditStatusFailure, req, details)

	if lockedUntil != nil && lockedUntil.After(s.now()) {
		s.record(ctx, account, ActionLockedOut, auditStatusFailure, req, models.JSONMap{
			"stage":        stage,
			"locked_until": lockedUntil.UTC().Format(time.RFC3339),
		})
	}
}

// checkLimit consults the rate limiter. A limiter error is treated as a
// rejection: a limiter that cannot answer is not permission to guess.
func (s *Service) checkLimit(ctx context.Context, operation string, req Request) error {
	key := "two_factor:" + operation + ":" + req.UserID.String() + "|" + req.IPAddress
	allowed, err := s.limiter.Allow(ctx, key)
	if err != nil {
		s.logger.Error("two-factor rate limiter failed", zap.Error(err))
		return ErrRateLimited
	}
	if !allowed {
		return ErrRateLimited
	}
	return nil
}

// record writes one audit entry. Audit failures are logged, never returned:
// they must not turn a successful security operation into an error, but they
// must not pass unnoticed either.
func (s *Service) record(ctx context.Context, account *Account, action, status string, req Request, details models.JSONMap) {
	if s.audit == nil {
		return
	}
	if account == nil {
		return
	}

	userID := account.UserID
	if err := s.audit.Log(ctx, account.TenantID, &userID, action, AuditResource, &userID,
		details, req.IPAddress, req.UserAgent, status); err != nil {
		s.logger.Error("failed to write two-factor audit entry",
			zap.String("action", action), zap.Error(err))
	}
}

func (s *Service) now() time.Time { return s.opts.Now() }

func (s *Service) enrolmentExpired(enrolment *Enrolment) bool {
	if enrolment == nil || enrolment.Enabled {
		return false
	}
	return s.now().After(enrolment.CreatedAt.Add(s.opts.EnrolmentTTL))
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
