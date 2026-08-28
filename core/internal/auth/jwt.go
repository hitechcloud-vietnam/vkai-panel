package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrExpiredToken   = errors.New("token has expired")
	ErrInvalidClaims  = errors.New("invalid token claims")
	ErrWrongTokenType = errors.New("token is not valid for this operation")
	ErrRevokedToken   = errors.New("token has been revoked")

	// ErrChallengeSpent means a two-factor challenge was presented for a
	// second time. A challenge buys exactly one exchange attempt.
	ErrChallengeSpent = errors.New("two-factor challenge has already been used")

	// ErrSingleUseUnavailable means the manager has no revocation store, so it
	// cannot prove a challenge is being used for the first time. The two-factor
	// exchange refuses rather than accepting a challenge it cannot spend.
	ErrSingleUseUnavailable = errors.New("single-use tokens require a revocation store")
)

// Token types. They are carried in the "typ" claim AND enforced by using a
// different signing key per type, so a refresh token can never be replayed as
// an access token even if a future caller forgets to check the claim.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"

	// TokenTypeTwoFactorChallenge is the half-authenticated token minted when
	// the password was right and a second factor is still owed. It is a
	// distinct type - own "typ" claim, own signing key - rather than an access
	// token carrying a flag, precisely so that no code path can mistake it for
	// a credential: every request-path validator asks for TokenTypeAccess, and
	// a challenge fails that check at the signature, before any claim is read.
	//
	// It carries no roles and no permissions. Even if some future middleware
	// did accept it, there is nothing in it to authorise.
	TokenTypeTwoFactorChallenge = "2fa_challenge"
)

// DefaultTwoFactorChallengeTTL is how long the password step buys. Minutes, not
// hours: it is the window in which a stolen challenge is worth stealing, and a
// person reading six digits off a phone needs one.
const DefaultTwoFactorChallengeTTL = 5 * time.Minute

type TokenClaims struct {
	UserID      uuid.UUID `json:"user_id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	RoleIDs     []string  `json:"role_ids"`
	Permissions []string  `json:"perms,omitempty"`
	TokenType   string    `json:"typ"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// RevocationStore tracks the jti of tokens that must no longer be accepted.
type RevocationStore interface {
	Revoke(jti string, expiresAt time.Time)
	IsRevoked(jti string) bool
}

// SingleUseStore is the optional extension a revocation store implements when
// it can revoke a jti and report whether it was still live in one atomic step.
// That is what makes a two-factor challenge single use even when two requests
// carry the same challenge at the same instant. A store that does not implement
// it falls back to check-then-revoke, which is racy under exactly concurrent
// replay; the shared (Redis backed) store must implement this.
type SingleUseStore interface {
	RevocationStore

	// Consume revokes jti and reports true when this call was the one that
	// revoked it. A second call for the same jti returns false.
	Consume(jti string, expiresAt time.Time) bool
}

// memoryRevocationStore is the default store. It keeps revoked ids until they
// would have expired anyway. It is per-process: a multi-instance deployment
// should supply a shared (Redis backed) store via SetRevocationStore.
type memoryRevocationStore struct {
	mu      sync.RWMutex
	revoked map[string]time.Time
}

func newMemoryRevocationStore() *memoryRevocationStore {
	return &memoryRevocationStore{revoked: make(map[string]time.Time)}
}

func (s *memoryRevocationStore) Revoke(jti string, expiresAt time.Time) {
	if jti == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[jti] = expiresAt
	// Opportunistic cleanup so the map cannot grow without bound.
	now := time.Now()
	for id, exp := range s.revoked {
		if now.After(exp) {
			delete(s.revoked, id)
		}
	}
}

// Consume implements SingleUseStore.
func (s *memoryRevocationStore) Consume(jti string, expiresAt time.Time) bool {
	if jti == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if exp, ok := s.revoked[jti]; ok && time.Now().Before(exp) {
		return false
	}
	s.revoked[jti] = expiresAt
	return true
}

func (s *memoryRevocationStore) IsRevoked(jti string) bool {
	if jti == "" {
		return false
	}
	s.mu.RLock()
	exp, ok := s.revoked[jti]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return time.Now().Before(exp)
}

type JWTManager struct {
	accessSecret    []byte
	refreshSecret   []byte
	challengeSecret []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	challengeTTL    time.Duration
	issuer          string
	revocations     RevocationStore
}

// deriveKey produces a per-purpose signing key from the configured secret so
// the two token classes are cryptographically distinct.
func deriveKey(secret, purpose string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("vkai-panel/" + purpose))
	return mac.Sum(nil)
}

func NewJWTManager(secret string, accessTTL, refreshTTL time.Duration, issuer string) *JWTManager {
	return &JWTManager{
		accessSecret:    deriveKey(secret, TokenTypeAccess),
		refreshSecret:   deriveKey(secret, TokenTypeRefresh),
		challengeSecret: deriveKey(secret, TokenTypeTwoFactorChallenge),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		challengeTTL:    DefaultTwoFactorChallengeTTL,
		issuer:          issuer,
		revocations:     newMemoryRevocationStore(),
	}
}

// SetRevocationStore replaces the default in-process revocation store.
func (m *JWTManager) SetRevocationStore(store RevocationStore) {
	if store != nil {
		m.revocations = store
	}
}

func (m *JWTManager) secretFor(tokenType string) ([]byte, error) {
	switch tokenType {
	case TokenTypeAccess:
		return m.accessSecret, nil
	case TokenTypeRefresh:
		return m.refreshSecret, nil
	case TokenTypeTwoFactorChallenge:
		return m.challengeSecret, nil
	default:
		return nil, ErrWrongTokenType
	}
}

func (m *JWTManager) GenerateTokenPair(userID, tenantID uuid.UUID, username, email string, roleIDs []string) (*TokenPair, error) {
	return m.GenerateTokenPairWithPermissions(userID, tenantID, username, email, roleIDs, nil)
}

// GenerateTokenPairWithPermissions issues an access/refresh pair carrying the
// caller's roles and effective permissions.
func (m *JWTManager) GenerateTokenPairWithPermissions(userID, tenantID uuid.UUID, username, email string, roleIDs, permissions []string) (*TokenPair, error) {
	now := time.Now()

	accessClaims := &TokenClaims{
		UserID:      userID,
		TenantID:    tenantID,
		Username:    username,
		Email:       email,
		RoleIDs:     roleIDs,
		Permissions: permissions,
		TokenType:   TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTokenTTL)),
			ID:        uuid.New().String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenStr, err := accessToken.SignedString(m.accessSecret)
	if err != nil {
		return nil, err
	}

	refreshClaims := &TokenClaims{
		UserID:    userID,
		TenantID:  tenantID,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTokenTTL)),
			ID:        uuid.New().String(),
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenStr, err := refreshToken.SignedString(m.refreshSecret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
		ExpiresIn:    int(m.accessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// ValidateToken validates an access token. It is the default because every
// request-path caller wants an access token; refresh tokens must go through
// ValidateRefreshToken.
func (m *JWTManager) ValidateToken(tokenStr string) (*TokenClaims, error) {
	return m.validate(tokenStr, TokenTypeAccess)
}

// ValidateAccessToken validates a token that must be an access token.
func (m *JWTManager) ValidateAccessToken(tokenStr string) (*TokenClaims, error) {
	return m.validate(tokenStr, TokenTypeAccess)
}

// ValidateRefreshToken validates a token that must be a refresh token.
func (m *JWTManager) ValidateRefreshToken(tokenStr string) (*TokenClaims, error) {
	return m.validate(tokenStr, TokenTypeRefresh)
}

func (m *JWTManager) validate(tokenStr, expectedType string) (*TokenClaims, error) {
	secret, err := m.secretFor(expectedType)
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(tokenStr, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	if claims.TokenType != expectedType {
		return nil, ErrWrongTokenType
	}

	if m.revocations != nil && m.revocations.IsRevoked(claims.ID) {
		return nil, ErrRevokedToken
	}

	return claims, nil
}

// Revoke marks a token's jti as unusable for the remainder of its lifetime.
func (m *JWTManager) Revoke(claims *TokenClaims) {
	if claims == nil || m.revocations == nil {
		return
	}
	expiresAt := time.Now().Add(m.refreshTokenTTL)
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	m.revocations.Revoke(claims.ID, expiresAt)
}

// RevokeToken parses a token of the given type and revokes it. Used by logout
// so the refresh token handed back by the client is retired too.
func (m *JWTManager) RevokeToken(tokenStr, tokenType string) error {
	claims, err := m.validate(tokenStr, tokenType)
	if err != nil {
		return err
	}
	m.Revoke(claims)
	return nil
}

// ---------------------------------------------------------------------------
// Two-factor challenge
// ---------------------------------------------------------------------------

// TwoFactorChallenge is what the password step returns when the account owes a
// second factor. It is not a credential: it authorises exactly one call to the
// two-factor exchange and nothing else.
type TwoFactorChallenge struct {
	Token string `json:"challenge_token"`
	// ExpiresIn is the remaining life in seconds, for a client that wants to
	// show a countdown.
	ExpiresIn int `json:"expires_in"`
	// ExpiresAt is the hard deadline. A continuation challenge minted after a
	// mistyped code inherits this instant, so retrying can never extend the
	// window the password step opened.
	ExpiresAt time.Time `json:"-"`
}

// SetTwoFactorChallengeTTL overrides how long a challenge lives. A
// non-positive duration restores the default.
func (m *JWTManager) SetTwoFactorChallengeTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultTwoFactorChallengeTTL
	}
	m.challengeTTL = ttl
}

// TwoFactorChallengeTTL reports the configured challenge lifetime.
func (m *JWTManager) TwoFactorChallengeTTL() time.Duration {
	if m.challengeTTL <= 0 {
		return DefaultTwoFactorChallengeTTL
	}
	return m.challengeTTL
}

// GenerateTwoFactorChallenge mints a fresh challenge for a user whose password
// has just been accepted.
func (m *JWTManager) GenerateTwoFactorChallenge(userID, tenantID uuid.UUID) (*TwoFactorChallenge, error) {
	return m.newChallenge(userID, tenantID, time.Now().Add(m.TwoFactorChallengeTTL()))
}

// GenerateTwoFactorChallengeUntil mints a continuation challenge that expires
// no later than deadline. It is what a failed exchange hands back so the user
// retypes the code rather than the password, without the retry buying a single
// second of extra life. The deadline is also capped at the configured TTL, so a
// caller cannot mint a long-lived challenge by passing a distant instant.
func (m *JWTManager) GenerateTwoFactorChallengeUntil(userID, tenantID uuid.UUID, deadline time.Time) (*TwoFactorChallenge, error) {
	max := time.Now().Add(m.TwoFactorChallengeTTL())
	if deadline.After(max) {
		deadline = max
	}
	return m.newChallenge(userID, tenantID, deadline)
}

func (m *JWTManager) newChallenge(userID, tenantID uuid.UUID, expiresAt time.Time) (*TwoFactorChallenge, error) {
	now := time.Now()
	if !expiresAt.After(now) {
		return nil, ErrExpiredToken
	}

	// No username, no email, no roles, no permissions. The challenge names the
	// account it belongs to and says nothing else: it is handed to a caller who
	// has proved a password and nothing more, and it must not become a way to
	// read the account's shape.
	claims := &TokenClaims{
		UserID:    userID,
		TenantID:  tenantID,
		TokenType: TokenTypeTwoFactorChallenge,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.New().String(),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.challengeSecret)
	if err != nil {
		return nil, err
	}

	return &TwoFactorChallenge{
		Token:     signed,
		ExpiresIn: int(time.Until(expiresAt).Seconds()),
		ExpiresAt: expiresAt,
	}, nil
}

// SpendTwoFactorChallenge validates a challenge AND spends it, in that order,
// returning its claims. Every exchange attempt spends the challenge it was
// given, whether the code that came with it was right or wrong, which is what
// makes a challenge single use: a captured challenge is worth exactly one guess
// out of a million, and a replay of a challenge that already succeeded is
// refused outright.
func (m *JWTManager) SpendTwoFactorChallenge(tokenStr string) (*TokenClaims, error) {
	claims, err := m.validate(tokenStr, TokenTypeTwoFactorChallenge)
	if err != nil {
		// A spent challenge reads as revoked; name it for what it is.
		if errors.Is(err, ErrRevokedToken) {
			return nil, ErrChallengeSpent
		}
		return nil, err
	}

	expiresAt := time.Now().Add(m.TwoFactorChallengeTTL())
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	if m.revocations == nil {
		// Refuse rather than accept a challenge that cannot be spent: an
		// unspendable challenge is a replayable one.
		return nil, ErrSingleUseUnavailable
	}

	if store, ok := m.revocations.(SingleUseStore); ok {
		if !store.Consume(claims.ID, expiresAt) {
			return nil, ErrChallengeSpent
		}
		return claims, nil
	}

	// Fallback for a store that only offers revoke/check. validate() has
	// already rejected a revoked jti; revoking here closes the window for every
	// non-concurrent replay.
	m.revocations.Revoke(claims.ID, expiresAt)
	return claims, nil
}
