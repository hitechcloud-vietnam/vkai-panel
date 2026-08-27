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
)

// Token types. They are carried in the "typ" claim AND enforced by using a
// different signing key per type, so a refresh token can never be replayed as
// an access token even if a future caller forgets to check the claim.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

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
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
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
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
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
