package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/twofactor"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// userStore is the slice of the user repository the auth service needs. It is
// an interface so the login gate can be tested without a database; the concrete
// *repository.UserRepository satisfies it.
type userStore interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	UpdateLastLogin(ctx context.Context, id uuid.UUID, ip string) error
	GetRoleNames(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetPermissionNames(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// TwoFactorVerifier is the slice of internal/twofactor.Service the login gate
// consults. *twofactor.Service satisfies it as it stands, so wiring is one
// call to SetTwoFactor; the interface exists so this package does not have to
// build a two-factor service to be tested.
type TwoFactorVerifier interface {
	// Required reports whether the account has a confirmed second factor.
	Required(ctx context.Context, userID uuid.UUID) (bool, error)

	// Verify checks one submitted TOTP or recovery code. It applies its own
	// rate limit, per-account lockout, replay guard and audit trail.
	Verify(ctx context.Context, req twofactor.Request) (*twofactor.VerifyResult, error)
}

type AuthService struct {
	userRepo   userStore
	tenantRepo *repository.TenantRepository
	jwtManager *auth.JWTManager
	logger     *zap.Logger

	// twoFactor is nil when the panel has no two-factor service wired. That is
	// not "two-factor off": an account that has enrolled is refused a session
	// rather than handed one, because a login that ignores an enabled second
	// factor is worse than a login that fails. See twoFactorRequired.
	twoFactor TwoFactorVerifier

	// audit is the tamper-evident trail. nil means sign-in events are not
	// recorded, which is a deployment fault rather than a mode: SetAudit says
	// so at start-up. A failed audit write never fails a sign-in - refusing
	// authentication because the log is unhappy hands an attacker a denial of
	// service - so every call here goes through AuditService.Record.
	audit *AuditService

	failures *loginFailureTracker
}

// loginAttemptLimit / loginLockout implement account lockout, which - unlike
// per-IP rate limiting - also stops a brute force spread across many source
// addresses.
const (
	loginAttemptLimit = 5
	loginLockout      = 15 * time.Minute
	loginFailureTTL   = 30 * time.Minute
)

type loginFailure struct {
	count       int
	lockedUntil time.Time
	lastSeen    time.Time
}

type loginFailureTracker struct {
	mu       sync.Mutex
	failures map[string]*loginFailure
}

func newLoginFailureTracker() *loginFailureTracker {
	return &loginFailureTracker{failures: make(map[string]*loginFailure)}
}

// locked reports whether the account is currently locked out.
func (t *loginFailureTracker) locked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, ok := t.failures[key]
	if !ok {
		return false
	}
	return time.Now().Before(f.lockedUntil)
}

func (t *loginFailureTracker) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	f, ok := t.failures[key]
	if !ok {
		f = &loginFailure{}
		t.failures[key] = f
	}
	f.count++
	f.lastSeen = time.Now()
	if f.count >= loginAttemptLimit {
		// Back off exponentially, capped at one hour.
		lock := loginLockout << uint(min(f.count-loginAttemptLimit, 2))
		if lock > time.Hour {
			lock = time.Hour
		}
		f.lockedUntil = time.Now().Add(lock)
	}
}

func (t *loginFailureTracker) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

func (t *loginFailureTracker) sweepLocked() {
	cutoff := time.Now().Add(-loginFailureTTL)
	for k, f := range t.failures {
		if f.lastSeen.Before(cutoff) && time.Now().After(f.lockedUntil) {
			delete(t.failures, k)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewAuthService(
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		jwtManager: jwtManager,
		logger:     logger,
		failures:   newLoginFailureTracker(),
	}
}

// Sentinel errors of the sign-in flow. They are deliberately coarse: an
// unauthenticated caller learns whether the whole attempt failed and nothing
// about which part of it did.
var (
	// errInvalidCredentials is returned for every failed password-step reason
	// so the API never reveals whether a username exists or an account is
	// disabled.
	errInvalidCredentials = errors.New("invalid credentials")

	// ErrChallengeInvalid means the presented two-factor challenge was
	// missing, malformed, expired or already spent. The caller must start
	// again from the password.
	ErrChallengeInvalid = errors.New("your sign-in attempt is no longer valid; sign in again")

	// ErrVerificationFailed means the challenge was good and the code was not.
	// It says nothing else: the challenge already proved the username and the
	// password, so there is nothing further to disclose, and a caller who
	// stole a challenge learns only that this one guess was wrong.
	ErrVerificationFailed = errors.New("verification failed")

	// ErrTooManyAttempts means the exchange is rate limited or the account is
	// in a two-factor lockout.
	ErrTooManyAttempts = errors.New("too many failed attempts, try again later")

	// ErrTwoFactorUnavailable means the account has a second factor but this
	// process cannot check it. It is a refusal, never a bypass.
	ErrTwoFactorUnavailable = errors.New("sign-in is temporarily unavailable for this account")
)

// TwoFactorRetryError is returned when the code was wrong but the challenge
// window has time left. It carries a fresh challenge - the presented one was
// spent by the attempt - so the client asks for the code again instead of the
// password. The replacement inherits the original deadline, so retrying never
// extends the window the password step opened.
type TwoFactorRetryError struct {
	ChallengeToken     string
	ChallengeExpiresIn int
}

func (e *TwoFactorRetryError) Error() string { return ErrVerificationFailed.Error() }

// Unwrap lets callers test the failure with errors.Is(err, ErrVerificationFailed)
// whether or not a retry challenge came with it.
func (e *TwoFactorRetryError) Unwrap() error { return ErrVerificationFailed }

// SetTwoFactor installs the two-factor service the login gate consults. Call it
// once at start-up, before the router serves anything:
//
//	authService.SetTwoFactor(handler.TwoFactorServiceOf(twoFactorHandler))
//
// Passing nil, or a nil *twofactor.Service, leaves the gate unwired - which
// makes every enrolled account unable to sign in rather than able to sign in
// with a password alone - and says so in the log.
func (s *AuthService) SetTwoFactor(v TwoFactorVerifier) {
	if isNilVerifier(v) {
		s.twoFactor = nil
		s.logger.Warn("two-factor verifier not wired into the auth service; " +
			"accounts with two-factor enabled will be refused sign-in")
		return
	}
	s.twoFactor = v
}

// SetAudit installs the audit trail sign-in events are written to. Call it once
// at start-up, alongside SetTwoFactor:
//
//	authService.SetAudit(auditService)
//
// It is a setter rather than a constructor argument so that adding it did not
// change NewAuthService's signature, which several call sites and tests pass
// positionally.
func (s *AuthService) SetAudit(a *AuditService) {
	if a == nil || a.repo == nil {
		s.audit = nil
		s.logger.Warn("audit service not wired into the auth service; " +
			"sign-in successes and failures will NOT be recorded in the audit trail")
		return
	}
	s.audit = a
}

// recordSignInFailure writes one refused sign-in to the trail.
//
// reason is the internal cause - unknown_user, bad_password, inactive_account,
// locked_out. It goes in the trail because whoever reads the trail is entitled
// to know which; it never goes back to the caller, who is told only that the
// attempt failed.
//
// user may be nil, which is the case that matters most: a spray against
// invented usernames has no tenant and no user id, and dropping those entries
// would leave the single most recognisable attack pattern invisible. They are
// recorded against the default tenant with the attempted username in the
// details. See audit.DefaultTenantID.
func (s *AuthService) recordSignInFailure(ctx context.Context, user *models.User, username, reason, ip string) {
	if s.audit == nil {
		return
	}

	tenantID := audit.DefaultTenantID
	var userID *uuid.UUID
	attributed := false
	if user != nil {
		tenantID = user.TenantID
		id := user.ID
		userID = &id
		attributed = true
	}

	s.audit.Record(ctx, tenantID, userID,
		audit.ActionSignInFailed, audit.ResourceSession, userID,
		models.JSONMap{
			"username":   username,
			"reason":     reason,
			"attributed": attributed,
		},
		ip, "", audit.StatusFailure)
}

// isNilVerifier catches the classic interface trap: a nil *twofactor.Service
// stored in an interface is not == nil, and calling through it panics on the
// first login. A wiring mistake must not take the panel down.
func isNilVerifier(v TwoFactorVerifier) bool {
	if v == nil {
		return true
	}
	value := reflect.ValueOf(v)
	switch value.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		return value.IsNil()
	default:
		return false
	}
}

// Login is the password step.
//
// It returns one of two things and never both: a session, or a two-factor
// challenge. The challenge is only ever minted after the password has been
// checked, which is what keeps the existence of a second factor secret from
// anyone who does not already know the password - every wrong password gets
// the same errInvalidCredentials regardless of enrolment. Anything that moves
// the enrolment lookup above the password check breaks that property.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, ip string) (*models.LoginResponse, error) {
	if s.failures.locked(req.Username) {
		s.logger.Warn("login rejected: account temporarily locked",
			zap.String("username", req.Username), zap.String("ip", ip))
		s.recordSignInFailure(ctx, nil, req.Username, "locked_out", ip)
		return nil, ErrTooManyAttempts
	}

	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: unknown user",
			zap.String("username", req.Username), zap.String("ip", ip))
		s.recordSignInFailure(ctx, nil, req.Username, "unknown_user", ip)
		return nil, errInvalidCredentials
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: bad password",
			zap.String("username", req.Username), zap.String("ip", ip))
		s.recordSignInFailure(ctx, user, req.Username, "bad_password", ip)
		return nil, errInvalidCredentials
	}

	// Checked after the password so a disabled account is indistinguishable
	// from a wrong password to an unauthenticated caller.
	if user.Status != "active" {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: inactive account",
			zap.String("username", req.Username), zap.String("ip", ip))
		s.recordSignInFailure(ctx, user, req.Username, "inactive_account", ip)
		return nil, errInvalidCredentials
	}

	s.failures.reset(req.Username)

	required, err := s.twoFactorRequired(ctx, user)
	if err != nil {
		return nil, err
	}

	if required {
		challenge, err := s.jwtManager.GenerateTwoFactorChallenge(user.ID, user.TenantID)
		if err != nil {
			s.logger.Error("failed to mint two-factor challenge",
				zap.String("user_id", user.ID.String()), zap.Error(err))
			return nil, ErrTwoFactorUnavailable
		}

		s.logger.Info("password accepted; second factor owed",
			zap.String("user_id", user.ID.String()), zap.String("ip", ip))

		// No access token, no refresh token, no user record. The password step
		// mints nothing usable for an account with a second factor.
		return &models.LoginResponse{
			TwoFactorRequired:  true,
			ChallengeToken:     challenge.Token,
			ChallengeExpiresIn: challenge.ExpiresIn,
		}, nil
	}

	return s.issueSession(ctx, user, ip)
}

// TwoFactorExchange is the input to the second step.
type TwoFactorExchange struct {
	ChallengeToken string
	Code           string
	IP             string
	UserAgent      string
}

// CompleteTwoFactorLogin exchanges a challenge plus one current code for the
// real token pair. The code may be a TOTP code or a recovery code; twofactor's
// Verify tells them apart, spends whichever was used, and audits the attempt
// either way.
//
// The challenge is spent before the code is checked, so one challenge buys one
// attempt whatever its outcome.
func (s *AuthService) CompleteTwoFactorLogin(ctx context.Context, req TwoFactorExchange) (*models.LoginResponse, error) {
	claims, err := s.jwtManager.SpendTwoFactorChallenge(req.ChallengeToken)
	if err != nil {
		s.logger.Warn("two-factor exchange rejected: challenge not usable",
			zap.String("ip", req.IP), zap.Error(err))
		return nil, ErrChallengeInvalid
	}

	// Account lockout for the exchange, alongside the per-address rate limit in
	// front of the endpoint and twofactor's own limiter and lockout. Keyed by
	// user id, so spreading the attempt over many addresses does not evade it.
	key := twoFactorFailureKey(claims.UserID)
	if s.failures.locked(key) {
		s.logger.Warn("two-factor exchange rejected: account temporarily locked",
			zap.String("user_id", claims.UserID.String()), zap.String("ip", req.IP))
		return nil, ErrTooManyAttempts
	}

	if s.twoFactor == nil {
		s.logger.Error("two-factor exchange attempted with no two-factor service wired",
			zap.String("user_id", claims.UserID.String()))
		return nil, ErrTwoFactorUnavailable
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		s.failures.fail(key)
		s.logger.Warn("two-factor exchange rejected: user no longer resolvable",
			zap.String("user_id", claims.UserID.String()), zap.Error(err))
		return nil, ErrChallengeInvalid
	}

	// Re-checked here: an account disabled between the two steps must not
	// complete a sign-in that was already half done.
	if user.Status != "active" {
		s.failures.fail(key)
		s.logger.Warn("two-factor exchange rejected: inactive account",
			zap.String("user_id", user.ID.String()), zap.String("ip", req.IP))
		return nil, ErrChallengeInvalid
	}

	if _, err := s.twoFactor.Verify(ctx, twofactor.Request{
		UserID:    user.ID,
		IPAddress: req.IP,
		UserAgent: req.UserAgent,
		Code:      req.Code,
	}); err != nil {
		s.failures.fail(key)
		s.logger.Warn("two-factor exchange failed",
			zap.String("user_id", user.ID.String()),
			zap.String("ip", req.IP),
			zap.Error(err))

		if errors.Is(err, twofactor.ErrLockedOut) || errors.Is(err, twofactor.ErrRateLimited) {
			return nil, ErrTooManyAttempts
		}
		if errors.Is(err, twofactor.ErrNotEnrolled) {
			// The enrolment vanished between the two steps.
			return nil, ErrChallengeInvalid
		}
		return s.retryOrFail(user, claims)
	}

	s.failures.reset(key)

	return s.issueSession(ctx, user, req.IP)
}

// retryOrFail turns a wrong code into a retry - a replacement challenge with
// the original deadline - or, when the window has run out, into a plain
// failure that sends the client back to the password step.
func (s *AuthService) retryOrFail(user *models.User, spent *auth.TokenClaims) (*models.LoginResponse, error) {
	if spent.ExpiresAt == nil || !spent.ExpiresAt.Time.After(time.Now()) {
		return nil, ErrVerificationFailed
	}

	next, err := s.jwtManager.GenerateTwoFactorChallengeUntil(user.ID, user.TenantID, spent.ExpiresAt.Time)
	if err != nil {
		return nil, ErrVerificationFailed
	}

	return nil, &TwoFactorRetryError{
		ChallengeToken:     next.Token,
		ChallengeExpiresIn: next.ExpiresIn,
	}
}

// twoFactorRequired decides whether this account owes a second factor.
//
// It fails closed in both directions. If the enrolment cannot be read, sign-in
// is refused rather than allowed. If the account is flagged as enrolled but no
// two-factor service is wired into this process, sign-in is refused rather
// than completed with a password alone - that state is a deployment bug, and
// handing out tokens would be exactly the silent bypass this gate exists to
// prevent.
func (s *AuthService) twoFactorRequired(ctx context.Context, user *models.User) (bool, error) {
	if s.twoFactor == nil {
		if user.MFAEnabled {
			s.logger.Error("refusing sign-in: account has two-factor enabled but no two-factor service is wired",
				zap.String("user_id", user.ID.String()))
			return false, ErrTwoFactorUnavailable
		}
		return false, nil
	}

	enrolled, err := s.twoFactor.Required(ctx, user.ID)
	if err != nil {
		s.logger.Error("refusing sign-in: two-factor enrolment could not be read",
			zap.String("user_id", user.ID.String()), zap.Error(err))
		return false, ErrTwoFactorUnavailable
	}

	// Either source saying yes is enough. user_two_factor is the source of
	// truth; users.mfa_enabled is its mirror, and a mirror that failed to
	// update must not become a bypass in either direction.
	return enrolled || user.MFAEnabled, nil
}

// issueSession mints the real token pair. It is the ONLY place in this service
// that does so for a login, so there is exactly one point past which a caller
// is authenticated, and both steps go through it.
func (s *AuthService) issueSession(ctx context.Context, user *models.User, ip string) (*models.LoginResponse, error) {
	roleIDs, permissions := s.loadAuthorization(ctx, user.ID)

	tokenPair, err := s.jwtManager.GenerateTokenPairWithPermissions(
		user.ID, user.TenantID, user.Username, user.Email, roleIDs, permissions,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID, ip)

	// Every session this panel ever issues passes through here, whether the
	// account owed a second factor or not, so this is the one place a granted
	// sign-in can be recorded without being recorded twice.
	if s.audit != nil {
		userID := user.ID
		s.audit.Record(ctx, user.TenantID, &userID,
			audit.ActionSignIn, audit.ResourceSession, &userID,
			models.JSONMap{
				"username":    user.Username,
				"two_factor":  user.MFAEnabled,
				"roles":       roleIDs,
				"permissions": len(permissions),
			},
			ip, "", audit.StatusSuccess)
	}

	return &models.LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    tokenPair.TokenType,
		User:         user,
	}, nil
}

// twoFactorFailureKey namespaces the exchange lockout so it cannot collide with
// the username-keyed password lockout.
func twoFactorFailureKey(userID uuid.UUID) string {
	return "2fa:" + userID.String()
}

// RefreshToken exchanges a refresh token for a new pair.
//
// This is not a way around the second factor and cannot become one: a refresh
// token is minted only by issueSession, which is reached only after the gate.
// The check below is for the deployment that loses its two-factor wiring while
// sessions are live - the same refusal the password step makes.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	// Only a token minted as a refresh token is accepted here, and an access
	// token - or a two-factor challenge - is rejected by both the "typ" claim
	// and the signing key.
	claims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	if user.Status != "active" {
		return nil, fmt.Errorf("invalid refresh token")
	}

	if user.MFAEnabled && s.twoFactor == nil {
		s.logger.Error("refusing refresh: account has two-factor enabled but no two-factor service is wired",
			zap.String("user_id", user.ID.String()))
		return nil, ErrTwoFactorUnavailable
	}

	roleIDs, permissions := s.loadAuthorization(ctx, user.ID)

	pair, err := s.jwtManager.GenerateTokenPairWithPermissions(
		user.ID, user.TenantID, user.Username, user.Email, roleIDs, permissions,
	)
	if err != nil {
		return nil, err
	}

	// Rotation: the presented refresh token is retired so a stolen copy cannot
	// be replayed after the legitimate client has refreshed.
	s.jwtManager.Revoke(claims)

	return pair, nil
}

// Logout revokes the caller's access token and, when supplied, their refresh
// token, so neither works for the remainder of its lifetime.
func (s *AuthService) Logout(accessClaims *auth.TokenClaims, refreshToken string) {
	s.jwtManager.Revoke(accessClaims)
	if refreshToken != "" {
		_ = s.jwtManager.RevokeToken(refreshToken, auth.TokenTypeRefresh)
	}

	// Logout has no context of its own - the signature is fixed by its callers -
	// and the write must not outlive the request by much, so it gets its own
	// short one rather than context.Background().
	if s.audit != nil && accessClaims != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		userID := accessClaims.UserID
		s.audit.Record(ctx, accessClaims.TenantID, &userID,
			audit.ActionSignOut, audit.ResourceSession, &userID,
			models.JSONMap{"username": accessClaims.Username},
			"", "", audit.StatusSuccess)
	}
}

// loadAuthorization fetches the roles and effective permissions for a user.
// A lookup failure yields an empty set: it must never silently grant access.
func (s *AuthService) loadAuthorization(ctx context.Context, userID uuid.UUID) ([]string, []string) {
	roleIDs, err := s.userRepo.GetRoleNames(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load user roles", zap.Error(err))
		roleIDs = []string{}
	}

	permissions, err := s.userRepo.GetPermissionNames(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load user permissions", zap.Error(err))
		permissions = []string{}
	}

	return roleIDs, permissions
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}
