package service

// The parts of the API key service that the HTTP tests cannot reach: what a
// key's stored digest may be, and what a key's lifetime may be.

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type memoryKeyStore struct {
	mu   sync.Mutex
	keys map[uuid.UUID]models.APIKey
}

func newMemoryKeyStore() *memoryKeyStore {
	return &memoryKeyStore{keys: make(map[uuid.UUID]models.APIKey)}
}

func (s *memoryKeyStore) put(key models.APIKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[key.ID] = key
}

func (s *memoryKeyStore) get(id uuid.UUID) models.APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys[id]
}

func (s *memoryKeyStore) Create(_ context.Context, key *models.APIKey) error {
	key.CreatedAt = time.Now()
	s.put(*key)
	return nil
}

func (s *memoryKeyStore) GetByID(_ context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID {
		return nil, sql.ErrNoRows
	}
	return &key, nil
}

func (s *memoryKeyStore) ListByPrefixes(_ context.Context, prefixes []string) ([]models.APIKey, error) {
	wanted := map[string]bool{}
	for _, p := range prefixes {
		wanted[p] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.APIKey
	for _, key := range s.keys {
		if wanted[key.KeyPrefix] {
			out = append(out, key)
		}
	}
	return out, nil
}

func (s *memoryKeyStore) ListByTenant(_ context.Context, tenantID uuid.UUID, _, _ int) ([]models.APIKey, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.APIKey
	for _, key := range s.keys {
		if key.TenantID == tenantID {
			out = append(out, key)
		}
	}
	return out, len(out), nil
}

func (s *memoryKeyStore) Update(_ context.Context, key *models.APIKey) error {
	s.put(*key)
	return nil
}

func (s *memoryKeyStore) Delete(_ context.Context, _, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, id)
	return nil
}

func (s *memoryKeyStore) MarkUsed(_ context.Context, id uuid.UUID, at time.Time, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.keys[id]
	key.LastUsed = &at
	address := ip
	key.LastUsedIP = &address
	s.keys[id] = key
	return nil
}

func (s *memoryKeyStore) UpgradeHash(_ context.Context, id uuid.UUID, oldHash, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.keys[id]
	if key.KeyHash != oldHash {
		return nil
	}
	key.KeyHash = newHash
	s.keys[id] = key
	return nil
}

func (s *memoryKeyStore) Revoke(_ context.Context, tenantID, id uuid.UUID, reason string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID || key.RevokedAt != nil {
		return 0, nil
	}
	when, why := at, reason
	key.RevokedAt, key.RevokedReason, key.Status = &when, &why, "revoked"
	s.keys[id] = key
	return 1, nil
}

func (s *memoryKeyStore) MarkSuperseded(_ context.Context, tenantID, id uuid.UUID, deadline time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID {
		return 0, nil
	}
	when := deadline
	key.RotationDeadline, key.Status = &when, "superseded"
	s.keys[id] = key
	return 1, nil
}

type noAuthority struct{}

func (noAuthority) GetRoleNames(context.Context, uuid.UUID) ([]string, error)       { return nil, nil }
func (noAuthority) GetPermissionNames(context.Context, uuid.UUID) ([]string, error) { return nil, nil }

func newTestKeyService(t *testing.T) (*APIKeyService, *memoryKeyStore) {
	t.Helper()
	t.Setenv("VKAI_SECRET_KEY", strings.Repeat("3f", 32))
	store := newMemoryKeyStore()
	svc := NewAPIKeyServiceWithStore(store, noAuthority{}, nil, zap.NewNop())
	if !svc.Available() {
		t.Fatal("the service reported itself unavailable with a master key present")
	}
	return svc, store
}

func TestCreateAppliesADefaultLifetime(t *testing.T) {
	svc, _ := newTestKeyService(t)
	ctx := context.Background()
	tenantID, userID := uuid.New(), uuid.New()

	created, err := svc.CreateAPIKey(ctx, tenantID, userID, &models.CreateAPIKeyRequest{
		Name:   "no expiry given",
		Scopes: []string{"website:read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if created.ExpiresAt == nil {
		t.Fatal("a key was created with no expiry; it would outlive everyone who knew what it was for")
	}
	life := time.Until(*created.ExpiresAt)
	if life < DefaultAPIKeyLifetime-time.Minute || life > DefaultAPIKeyLifetime+time.Minute {
		t.Fatalf("the default lifetime is %v, want about %v", life, DefaultAPIKeyLifetime)
	}
	if !strings.HasPrefix(created.Key, auth.APIKeyPrefixMarker) {
		t.Fatalf("the key does not look like one of ours: %q", created.Key)
	}
	if created.KeyHash == created.Key {
		t.Fatal("the stored digest is the key")
	}
}

func TestCreateRefusesAnImpossibleLifetime(t *testing.T) {
	svc, _ := newTestKeyService(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	if _, err := svc.CreateAPIKey(ctx, uuid.New(), uuid.New(), &models.CreateAPIKeyRequest{
		Name: "already expired", Scopes: []string{"website:read"}, ExpiresAt: &past,
	}); err == nil {
		t.Fatal("a key expiring in the past was created")
	}

	forever := time.Now().Add(MaxAPIKeyLifetime + 24*time.Hour)
	if _, err := svc.CreateAPIKey(ctx, uuid.New(), uuid.New(), &models.CreateAPIKeyRequest{
		Name: "forever", Scopes: []string{"website:read"}, ExpiresAt: &forever,
	}); err == nil {
		t.Fatal("a key living longer than the maximum was created")
	}
}

func TestAuthenticateRefusesAnExpiredKey(t *testing.T) {
	svc, store := newTestKeyService(t)
	ctx := context.Background()
	tenantID, userID := uuid.New(), uuid.New()

	created, err := svc.CreateAPIKey(ctx, tenantID, userID, &models.CreateAPIKeyRequest{
		Name: "short lived", Scopes: []string{"website:read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, err := svc.Authenticate(ctx, created.Key, "203.0.113.1"); err != nil {
		t.Fatalf("a fresh key did not authenticate: %v", err)
	}

	svc.SetClock(func() time.Time { return time.Now().Add(DefaultAPIKeyLifetime + time.Hour) })
	if _, err := svc.Authenticate(ctx, created.Key, "203.0.113.1"); err == nil {
		t.Fatal("an expired key authenticated")
	}
	_ = store
}

// TestAuthenticateUpgradesALegacyDigestOnUse is how the last unpeppered row
// disappears without a migration - a migration could not do it, because it
// would need the keys themselves.
func TestAuthenticateUpgradesALegacyDigestOnUse(t *testing.T) {
	svc, store := newTestKeyService(t)
	ctx := context.Background()
	tenantID, userID := uuid.New(), uuid.New()

	rawKey, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	id := uuid.New()
	store.put(models.APIKey{
		ID:        id,
		TenantID:  tenantID,
		UserID:    userID,
		Name:      "written by an older build",
		KeyHash:   auth.LegacyDigest(rawKey),
		KeyPrefix: auth.APIKeyPrefix(rawKey),
		Scopes:    []string{"website:read"},
		ExpiresAt: &expires,
		Status:    "active",
	})

	principal, err := svc.Authenticate(ctx, rawKey, "203.0.113.1")
	if err != nil {
		t.Fatalf("a key stored under the previous scheme stopped working: %v", err)
	}
	if principal.KeyID != id {
		t.Fatalf("the wrong key matched: %v", principal.KeyID)
	}

	stored := store.get(id).KeyHash
	if !strings.HasPrefix(stored, "hmac-sha256$") {
		t.Fatalf("the digest was not rewritten in the current format: %q", stored)
	}
	if _, err := svc.Authenticate(ctx, rawKey, "203.0.113.1"); err != nil {
		t.Fatalf("the key stopped working after its digest was upgraded: %v", err)
	}
}

// TestAuthenticateRefusesAPlaintextDigest is the finding, enforced.
//
// service/multi_user.go stored the key itself in key_hash. Such a row is a
// live credential in a table that goes into every backup, and it is refused
// rather than honoured so that the key has to be re-minted.
func TestAuthenticateRefusesAPlaintextDigest(t *testing.T) {
	svc, store := newTestKeyService(t)
	ctx := context.Background()

	rawKey := "vkai_" + uuid.New().String() + uuid.New().String()
	expires := time.Now().Add(time.Hour)
	store.put(models.APIKey{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		UserID:    uuid.New(),
		Name:      "stored in the clear",
		KeyHash:   rawKey,
		KeyPrefix: rawKey[:8],
		Scopes:    []string{"website:read"},
		ExpiresAt: &expires,
		Status:    "active",
	})

	if _, err := svc.Authenticate(ctx, rawKey, "203.0.113.1"); err == nil {
		t.Fatal("a row whose key_hash is the key itself authenticated")
	}
}

func TestAuthenticatePicksTheRightKeyAmongCollidingPrefixes(t *testing.T) {
	// The legacy prefix convention is eight characters, which every key minted
	// by that path shares. A lookup that took the first row would authenticate
	// against the wrong digest and refuse a valid key.
	svc, store := newTestKeyService(t)
	ctx := context.Background()
	tenantID, userID := uuid.New(), uuid.New()

	expires := time.Now().Add(time.Hour)
	var wanted uuid.UUID
	var wantedKey string

	for i := 0; i < 5; i++ {
		rawKey, _ := auth.GenerateAPIKey()
		id := uuid.New()
		store.put(models.APIKey{
			ID:        id,
			TenantID:  tenantID,
			UserID:    userID,
			Name:      "colliding prefix",
			KeyHash:   auth.LegacyDigest(rawKey),
			KeyPrefix: rawKey[:8], // "vk_live_" for every one of them
			Scopes:    []string{"website:read"},
			ExpiresAt: &expires,
			Status:    "active",
		})
		if i == 3 {
			wanted, wantedKey = id, rawKey
		}
	}

	principal, err := svc.Authenticate(ctx, wantedKey, "203.0.113.1")
	if err != nil {
		t.Fatalf("a valid key was refused among keys sharing its prefix: %v", err)
	}
	if principal.KeyID != wanted {
		t.Fatalf("matched key %v, want %v", principal.KeyID, wanted)
	}
}

func TestRotateRefusesARevokedKeyAndCapsTheOverlap(t *testing.T) {
	svc, _ := newTestKeyService(t)
	ctx := context.Background()
	tenantID, userID := uuid.New(), uuid.New()

	created, err := svc.CreateAPIKey(ctx, tenantID, userID, &models.CreateAPIKeyRequest{
		Name: "rotate me", Scopes: []string{"website:read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	if _, err := svc.Rotate(ctx, tenantID, userID, created.ID, &models.RotateAPIKeyRequest{
		OverlapHours: int(MaxRotationOverlap.Hours()) + 1,
	}); err == nil {
		t.Fatal("an overlap longer than the cap was accepted; that is a second live key, not a rotation")
	}

	if err := svc.Revoke(ctx, tenantID, userID, created.ID, "leaked"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.Rotate(ctx, tenantID, userID, created.ID, nil); err == nil {
		t.Fatal("a revoked key was rotated; a revoked secret must not be the parent of a live one")
	}
}

func TestServiceWithoutAMasterKeyRefusesEverything(t *testing.T) {
	t.Setenv("VKAI_SECRET_KEY", "")
	svc := NewAPIKeyServiceWithStore(newMemoryKeyStore(), noAuthority{}, nil, zap.NewNop())

	if svc.Available() {
		t.Fatal("the service reported itself available with no master key")
	}
	if _, err := svc.CreateAPIKey(context.Background(), uuid.New(), uuid.New(), &models.CreateAPIKeyRequest{
		Name: "x", Scopes: []string{"website:read"},
	}); err != ErrAPIKeyUnavailable {
		t.Fatalf("CreateAPIKey = %v, want ErrAPIKeyUnavailable rather than a silent fallback", err)
	}
	if _, err := svc.Authenticate(context.Background(), "vk_live_whatever", ""); err != ErrAPIKeyUnavailable {
		t.Fatalf("Authenticate = %v, want ErrAPIKeyUnavailable", err)
	}
}
