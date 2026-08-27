package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type AuthService struct {
	userRepo   *repository.UserRepository
	tenantRepo *repository.TenantRepository
	jwtManager *auth.JWTManager
	logger     *zap.Logger

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

// errInvalidCredentials is returned for every failed login reason so the API
// never reveals whether a username exists or an account is disabled.
var errInvalidCredentials = fmt.Errorf("invalid credentials")

func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, ip string) (*models.LoginResponse, error) {
	if s.failures.locked(req.Username) {
		s.logger.Warn("login rejected: account temporarily locked",
			zap.String("username", req.Username), zap.String("ip", ip))
		return nil, fmt.Errorf("too many failed attempts, try again later")
	}

	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: unknown user",
			zap.String("username", req.Username), zap.String("ip", ip))
		return nil, errInvalidCredentials
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: bad password",
			zap.String("username", req.Username), zap.String("ip", ip))
		return nil, errInvalidCredentials
	}

	// Checked after the password so a disabled account is indistinguishable
	// from a wrong password to an unauthenticated caller.
	if user.Status != "active" {
		s.failures.fail(req.Username)
		s.logger.Warn("failed login: inactive account",
			zap.String("username", req.Username), zap.String("ip", ip))
		return nil, errInvalidCredentials
	}

	s.failures.reset(req.Username)

	roleIDs, permissions := s.loadAuthorization(ctx, user.ID)

	tokenPair, err := s.jwtManager.GenerateTokenPairWithPermissions(
		user.ID, user.TenantID, user.Username, user.Email, roleIDs, permissions,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID, ip)

	return &models.LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    tokenPair.TokenType,
		User:         *user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	// Only a token minted as a refresh token is accepted here, and an access
	// token is rejected by both the "typ" claim and the signing key.
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
