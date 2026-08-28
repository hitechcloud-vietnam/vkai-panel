package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func newTestManager(t *testing.T) *JWTManager {
	t.Helper()
	return NewJWTManager("test-secret", 15*time.Minute, 24*time.Hour, "vkai-panel-test")
}

// A challenge must be useless everywhere a credential is expected. It is a
// distinct token type with its own signing key, so it fails at the signature
// before any claim is read.
func TestChallengeIsNotACredential(t *testing.T) {
	m := newTestManager(t)

	challenge, err := m.GenerateTwoFactorChallenge(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GenerateTwoFactorChallenge: %v", err)
	}

	if _, err := m.ValidateToken(challenge.Token); err == nil {
		t.Fatal("a two-factor challenge was accepted as an access token")
	}
	if _, err := m.ValidateAccessToken(challenge.Token); err == nil {
		t.Fatal("a two-factor challenge was accepted by ValidateAccessToken")
	}
	if _, err := m.ValidateRefreshToken(challenge.Token); err == nil {
		t.Fatal("a two-factor challenge was accepted as a refresh token")
	}
}

// Neither may a real token be spent as a challenge.
func TestAccessAndRefreshTokensAreNotChallenges(t *testing.T) {
	m := newTestManager(t)

	pair, err := m.GenerateTokenPair(uuid.New(), uuid.New(), "admin", "admin@example.test", nil)
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if _, err := m.SpendTwoFactorChallenge(pair.AccessToken); err == nil {
		t.Fatal("an access token was accepted as a two-factor challenge")
	}
	if _, err := m.SpendTwoFactorChallenge(pair.RefreshToken); err == nil {
		t.Fatal("a refresh token was accepted as a two-factor challenge")
	}
}

// The challenge carries no authorisation to ignore.
func TestChallengeCarriesNoPermissions(t *testing.T) {
	m := newTestManager(t)
	userID, tenantID := uuid.New(), uuid.New()

	challenge, err := m.GenerateTwoFactorChallenge(userID, tenantID)
	if err != nil {
		t.Fatalf("GenerateTwoFactorChallenge: %v", err)
	}

	claims, err := m.SpendTwoFactorChallenge(challenge.Token)
	if err != nil {
		t.Fatalf("SpendTwoFactorChallenge: %v", err)
	}
	if claims.UserID != userID || claims.TenantID != tenantID {
		t.Fatal("challenge is not bound to the account it was minted for")
	}
	if len(claims.RoleIDs) != 0 || len(claims.Permissions) != 0 {
		t.Fatalf("challenge carries authorisation: roles=%v perms=%v", claims.RoleIDs, claims.Permissions)
	}
	if claims.Username != "" || claims.Email != "" {
		t.Fatal("challenge discloses account details")
	}
	if claims.TokenType != TokenTypeTwoFactorChallenge {
		t.Fatalf("token type = %q, want %q", claims.TokenType, TokenTypeTwoFactorChallenge)
	}
}

func TestChallengeIsSingleUse(t *testing.T) {
	m := newTestManager(t)

	challenge, err := m.GenerateTwoFactorChallenge(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GenerateTwoFactorChallenge: %v", err)
	}

	if _, err := m.SpendTwoFactorChallenge(challenge.Token); err != nil {
		t.Fatalf("first spend: %v", err)
	}
	if _, err := m.SpendTwoFactorChallenge(challenge.Token); !errors.Is(err, ErrChallengeSpent) {
		t.Fatalf("second spend error = %v, want ErrChallengeSpent", err)
	}
}

func TestExpiredChallengeIsRefused(t *testing.T) {
	m := newTestManager(t)
	userID := uuid.New()

	// Signed with the manager's own challenge key, but already expired: this is
	// what a challenge left in a tab over lunch looks like.
	claims := &TokenClaims{
		UserID:    userID,
		TenantID:  uuid.New(),
		TokenType: TokenTypeTwoFactorChallenge,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "vkai-panel-test",
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-10 * time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-10 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			ID:        uuid.New().String(),
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.challengeSecret)
	if err != nil {
		t.Fatalf("sign expired challenge: %v", err)
	}

	if _, err := m.SpendTwoFactorChallenge(expired); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired challenge error = %v, want ErrExpiredToken", err)
	}
}

// A continuation challenge may never outlive the deadline the password step
// set, however far in the future the caller asks for.
func TestContinuationChallengeCannotExtendTheWindow(t *testing.T) {
	m := newTestManager(t)
	m.SetTwoFactorChallengeTTL(2 * time.Minute)

	deadline := time.Now().Add(30 * time.Second)
	next, err := m.GenerateTwoFactorChallengeUntil(uuid.New(), uuid.New(), deadline)
	if err != nil {
		t.Fatalf("GenerateTwoFactorChallengeUntil: %v", err)
	}
	if next.ExpiresAt.After(deadline.Add(time.Second)) {
		t.Fatalf("continuation expires at %v, past the original deadline %v", next.ExpiresAt, deadline)
	}

	// A deadline past the configured TTL is capped, not honoured.
	far, err := m.GenerateTwoFactorChallengeUntil(uuid.New(), uuid.New(), time.Now().Add(72*time.Hour))
	if err != nil {
		t.Fatalf("GenerateTwoFactorChallengeUntil (far): %v", err)
	}
	if far.ExpiresAt.After(time.Now().Add(2*time.Minute + time.Second)) {
		t.Fatalf("challenge lives until %v, past the configured TTL", far.ExpiresAt)
	}

	// A deadline in the past mints nothing at all.
	if _, err := m.GenerateTwoFactorChallengeUntil(uuid.New(), uuid.New(), time.Now().Add(-time.Second)); err == nil {
		t.Fatal("a challenge was minted with a deadline in the past")
	}
}

// The challenge TTL is minutes, not hours.
func TestDefaultChallengeTTLIsShort(t *testing.T) {
	if DefaultTwoFactorChallengeTTL > 10*time.Minute {
		t.Fatalf("default challenge TTL is %v; it must be minutes, not hours", DefaultTwoFactorChallengeTTL)
	}
	m := newTestManager(t)
	if m.TwoFactorChallengeTTL() != DefaultTwoFactorChallengeTTL {
		t.Fatalf("manager challenge TTL = %v, want %v", m.TwoFactorChallengeTTL(), DefaultTwoFactorChallengeTTL)
	}
}
