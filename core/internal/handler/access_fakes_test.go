package handler

// In-memory stores for the access-control tests.
//
// They exist so the behaviour - scopes, rotation, revocation, session binding -
// can be driven through the REAL router, the real middleware chain and the real
// services over HTTP, without a database in the loop. The SQL those services
// use is proved separately, against a real PostgreSQL, in
// internal/repository/access_live_test.go.

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

type fakeKeyStore struct {
	mu   sync.Mutex
	keys map[uuid.UUID]models.APIKey
}

func newFakeKeyStore() *fakeKeyStore {
	return &fakeKeyStore{keys: make(map[uuid.UUID]models.APIKey)}
}

func (s *fakeKeyStore) Create(_ context.Context, key *models.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key.CreatedAt = time.Now()
	s.keys[key.ID] = *key
	return nil
}

func (s *fakeKeyStore) GetByID(_ context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID {
		return nil, sql.ErrNoRows
	}
	return &key, nil
}

func (s *fakeKeyStore) ListByPrefixes(_ context.Context, prefixes []string) ([]models.APIKey, error) {
	wanted := make(map[string]bool, len(prefixes))
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

func (s *fakeKeyStore) ListByTenant(_ context.Context, tenantID uuid.UUID, limit, offset int) ([]models.APIKey, int, error) {
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

func (s *fakeKeyStore) Update(_ context.Context, key *models.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[key.ID] = *key
	return nil
}

func (s *fakeKeyStore) Delete(_ context.Context, tenantID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, id)
	return nil
}

func (s *fakeKeyStore) MarkUsed(_ context.Context, id uuid.UUID, at time.Time, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok {
		return sql.ErrNoRows
	}
	key.LastUsed = &at
	address := ip
	key.LastUsedIP = &address
	s.keys[id] = key
	return nil
}

func (s *fakeKeyStore) UpgradeHash(_ context.Context, id uuid.UUID, oldHash, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.KeyHash != oldHash {
		return nil
	}
	key.KeyHash = newHash
	s.keys[id] = key
	return nil
}

func (s *fakeKeyStore) Revoke(_ context.Context, tenantID, id uuid.UUID, reason string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID || key.RevokedAt != nil {
		return 0, nil
	}
	when := at
	why := reason
	key.RevokedAt = &when
	key.RevokedReason = &why
	key.Status = "revoked"
	s.keys[id] = key
	return 1, nil
}

func (s *fakeKeyStore) MarkSuperseded(_ context.Context, tenantID, id uuid.UUID, deadline time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[id]
	if !ok || key.TenantID != tenantID || key.RevokedAt != nil {
		return 0, nil
	}
	when := deadline
	key.RotationDeadline = &when
	key.Status = "superseded"
	s.keys[id] = key
	return 1, nil
}

// ---------------------------------------------------------------------------
// The authority behind a key
// ---------------------------------------------------------------------------

type fakeAuthority struct {
	roles       []string
	permissions []string
}

func (a *fakeAuthority) GetRoleNames(context.Context, uuid.UUID) ([]string, error) {
	return a.roles, nil
}

func (a *fakeAuthority) GetPermissionNames(context.Context, uuid.UUID) ([]string, error) {
	return a.permissions, nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

type fakeSessionStore struct {
	mu      sync.Mutex
	byToken map[string]*models.PanelSession
	byID    map[uuid.UUID]*models.PanelSession
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		byToken: make(map[string]*models.PanelSession),
		byID:    make(map[uuid.UUID]*models.PanelSession),
	}
}

func (s *fakeSessionStore) Establish(_ context.Context, session *models.PanelSession) (*models.PanelSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byToken[session.TokenID]; ok {
		copied := *existing
		return &copied, nil
	}
	stored := *session
	stored.CreatedAt = time.Now()
	s.byToken[stored.TokenID] = &stored
	s.byID[stored.ID] = &stored
	copied := stored
	return &copied, nil
}

func (s *fakeSessionStore) GetByTokenID(_ context.Context, tokenID string) (*models.PanelSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byToken[tokenID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copied := *session
	return &copied, nil
}

func (s *fakeSessionStore) GetByID(_ context.Context, tenantID, id uuid.UUID) (*models.PanelSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok || session.TenantID != tenantID {
		return nil, sql.ErrNoRows
	}
	copied := *session
	return &copied, nil
}

func (s *fakeSessionStore) Touch(_ context.Context, id uuid.UUID, ip string, at time.Time, originMoved, reauthRequired bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok {
		return sql.ErrNoRows
	}
	address := ip
	session.LastSeenIP = &address
	session.LastSeenAt = at
	if originMoved {
		session.OriginChanges++
	}
	if reauthRequired {
		session.ReauthRequired = true
	}
	return nil
}

func (s *fakeSessionStore) Rebind(_ context.Context, id uuid.UUID, ip, network string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok || session.RevokedAt != nil {
		return sql.ErrNoRows
	}
	session.OriginIP = ip
	session.OriginNetwork = network
	session.LastSeenAt = at
	session.ReauthRequired = false
	return nil
}

func (s *fakeSessionStore) ListForUser(_ context.Context, tenantID, userID uuid.UUID) ([]models.PanelSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.PanelSession
	for _, session := range s.byID {
		if session.TenantID == tenantID && session.UserID == userID &&
			session.RevokedAt == nil && session.ExpiresAt.After(time.Now()) {
			out = append(out, *session)
		}
	}
	return out, nil
}

func (s *fakeSessionStore) Revoke(_ context.Context, tenantID, userID, id uuid.UUID, reason string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok || session.TenantID != tenantID || session.UserID != userID || session.RevokedAt != nil {
		return 0, nil
	}
	when := at
	why := reason
	session.RevokedAt = &when
	session.RevokedReason = &why
	return 1, nil
}

func (s *fakeSessionStore) RevokeByTokenID(_ context.Context, tokenID, reason string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byToken[tokenID]
	if !ok || session.RevokedAt != nil {
		return 0, nil
	}
	when := at
	why := reason
	session.RevokedAt = &when
	session.RevokedReason = &why
	return 1, nil
}

func (s *fakeSessionStore) RevokeAllForUser(_ context.Context, tenantID, userID uuid.UUID, reason string, at time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for _, session := range s.byID {
		if session.TenantID == tenantID && session.UserID == userID && session.RevokedAt == nil {
			when := at
			why := reason
			session.RevokedAt = &when
			session.RevokedReason = &why
			count++
		}
	}
	return count, nil
}

func (s *fakeSessionStore) PurgeExpired(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for id, session := range s.byID {
		if session.ExpiresAt.Before(before) {
			delete(s.byID, id)
			delete(s.byToken, session.TokenID)
			count++
		}
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// The account behind a session, for the password check that re-binds one
// ---------------------------------------------------------------------------

type fakeUsers struct {
	user *models.User
}

func (u *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	if u.user == nil || u.user.ID != id {
		return nil, sql.ErrNoRows
	}
	copied := *u.user
	return &copied, nil
}
