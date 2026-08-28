package agentpki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Errors the store returns. Handlers map them onto status codes, so they are
// values rather than formatted strings.
var (
	ErrNotFound         = errors.New("agentpki: record not found")
	ErrTokenUsed        = errors.New("agentpki: enrolment token has already been used")
	ErrTokenExpired     = errors.New("agentpki: enrolment token has expired")
	ErrRevoked          = errors.New("agentpki: certificate is revoked")
	ErrUnknownAgent     = errors.New("agentpki: certificate was not issued to a known agent")
	ErrWrongCertificate = errors.New("agentpki: certificate is not the one issued to this agent")
	ErrBadRole          = errors.New("agentpki: certificate does not carry the expected role")
)

// EnrolmentToken is one single-use invitation to join. The secret itself is
// never stored: only its SHA-256 digest, so a copy of the state file does not
// let its reader enrol anything.
type EnrolmentToken struct {
	ID         string     `json:"id"`
	SecretHash []byte     `json:"secret_hash"`
	ServerID   string     `json:"server_id,omitempty"`
	Hostname   string     `json:"hostname,omitempty"`
	CreatedBy  string     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	UsedAt     *time.Time `json:"used_at,omitempty"`
	UsedBy     string     `json:"used_by,omitempty"`
}

// CertRecord is what the panel remembers about one issued certificate. The
// public key is kept so a signed request from an agent can be verified without
// the agent having to send its certificate back.
type CertRecord struct {
	Serial       string     `json:"serial"`
	Fingerprint  string     `json:"fingerprint"`
	PublicKeyDER []byte     `json:"public_key_der"`
	NotBefore    time.Time  `json:"not_before"`
	NotAfter     time.Time  `json:"not_after"`
	IssuedAt     time.Time  `json:"issued_at"`
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
}

// AgentRecord is one enrolled agent: its identity, the certificate it is using
// now, and the one it used before the last rotation.
type AgentRecord struct {
	AgentID    string      `json:"agent_id"`
	Hostname   string      `json:"hostname,omitempty"`
	ServerID   string      `json:"server_id,omitempty"`
	Role       string      `json:"role"`
	Current    CertRecord  `json:"current"`
	Previous   *CertRecord `json:"previous,omitempty"`
	EnrolledAt time.Time   `json:"enrolled_at"`
	RenewedAt  time.Time   `json:"renewed_at"`
	LastSeenAt *time.Time  `json:"last_seen_at,omitempty"`
	Revoked    bool        `json:"revoked"`
	RevokedAt  *time.Time  `json:"revoked_at,omitempty"`
}

// RevokedCert is one entry on the deny list.
type RevokedCert struct {
	Serial    string    `json:"serial"`
	AgentID   string    `json:"agent_id,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	RevokedAt time.Time `json:"revoked_at"`
}

// Store holds everything the authority must remember across restarts. It is an
// interface for two reasons: the tests want an in-memory implementation, and a
// panel that grows to more than one API process will want a database-backed one
// without any other part of this package changing.
type Store interface {
	// PutEnrolment records a freshly minted token.
	PutEnrolment(ctx context.Context, tok *EnrolmentToken) error

	// ConsumeEnrolment marks the token used and returns it, atomically. It must
	// return ErrTokenUsed if the token was already spent and ErrTokenExpired if
	// now is past its expiry - and it must not mark an expired or spent token
	// as used a second time. This atomicity is the whole point of the method:
	// two installers racing with the same token must not both get a
	// certificate.
	ConsumeEnrolment(ctx context.Context, id string, now time.Time) (*EnrolmentToken, error)

	// GetEnrolment reads a token without consuming it.
	GetEnrolment(ctx context.Context, id string) (*EnrolmentToken, error)

	// PutAgent inserts or replaces an agent record.
	PutAgent(ctx context.Context, rec *AgentRecord) error

	// GetAgent returns ErrNotFound when the agent is unknown.
	GetAgent(ctx context.Context, agentID string) (*AgentRecord, error)

	// ListAgents returns every record, ordered by agent id.
	ListAgents(ctx context.Context) ([]*AgentRecord, error)

	// DeleteAgent forgets an agent. Its serials stay on the deny list.
	DeleteAgent(ctx context.Context, agentID string) error

	// Revoke adds a serial to the deny list. Revoking twice is not an error.
	Revoke(ctx context.Context, entry RevokedCert) error

	// IsRevoked is called on every handshake, so it must be cheap.
	IsRevoked(ctx context.Context, serial string) (bool, error)

	// DenyList returns every revoked serial, newest first.
	DenyList(ctx context.Context) ([]RevokedCert, error)
}

// ============================================================
// MEMORY STORE
// ============================================================

// MemoryStore keeps everything in process memory. It is what the tests use and
// what a panel started without a writable state directory falls back to; an
// agent enrolled against it has to enrol again after a restart.
type MemoryStore struct {
	mu         sync.RWMutex
	enrolments map[string]*EnrolmentToken
	agents     map[string]*AgentRecord
	revoked    map[string]*RevokedCert
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		enrolments: make(map[string]*EnrolmentToken),
		agents:     make(map[string]*AgentRecord),
		revoked:    make(map[string]*RevokedCert),
	}
}

func (s *MemoryStore) PutEnrolment(_ context.Context, tok *EnrolmentToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrolments[tok.ID] = cloneEnrolment(tok)
	return nil
}

func (s *MemoryStore) ConsumeEnrolment(_ context.Context, id string, now time.Time) (*EnrolmentToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, ok := s.enrolments[id]
	if !ok {
		return nil, ErrNotFound
	}
	if tok.UsedAt != nil {
		return nil, ErrTokenUsed
	}
	if now.After(tok.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	used := now
	tok.UsedAt = &used
	return cloneEnrolment(tok), nil
}

func (s *MemoryStore) GetEnrolment(_ context.Context, id string) (*EnrolmentToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.enrolments[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneEnrolment(tok), nil
}

func (s *MemoryStore) PutAgent(_ context.Context, rec *AgentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[rec.AgentID] = cloneAgent(rec)
	return nil
}

func (s *MemoryStore) GetAgent(_ context.Context, agentID string) (*AgentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.agents[agentID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneAgent(rec), nil
}

func (s *MemoryStore) ListAgents(_ context.Context) ([]*AgentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*AgentRecord, 0, len(s.agents))
	for _, rec := range s.agents {
		out = append(out, cloneAgent(rec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out, nil
}

func (s *MemoryStore) DeleteAgent(_ context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return ErrNotFound
	}
	delete(s.agents, agentID)
	return nil
}

func (s *MemoryStore) Revoke(_ context.Context, entry RevokedCert) error {
	if entry.Serial == "" {
		return errors.New("agentpki: cannot revoke an empty serial")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.revoked[entry.Serial]; ok {
		return nil
	}
	clone := entry
	s.revoked[entry.Serial] = &clone
	return nil
}

func (s *MemoryStore) IsRevoked(_ context.Context, serial string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.revoked[serial]
	return ok, nil
}

func (s *MemoryStore) DenyList(_ context.Context) ([]RevokedCert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RevokedCert, 0, len(s.revoked))
	for _, entry := range s.revoked {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RevokedAt.After(out[j].RevokedAt) })
	return out, nil
}

// ============================================================
// FILE STORE
// ============================================================

// FileStore is MemoryStore with the state mirrored to one JSON file next to the
// CA key, written 0600 through a temporary file and a rename so a crash during
// a write cannot leave a half-written state behind.
//
// It is deliberately the default: this state belongs to the CA, the CA key is
// already on this host and nowhere else, and putting the two in the same
// directory means an operator backs up or moves one thing, not two. A panel
// that runs more than one API process needs a Store backed by the database
// instead - the interface is the seam for that.
type FileStore struct {
	mem  *MemoryStore
	path string
	mu   sync.Mutex
}

type fileState struct {
	Enrolments map[string]*EnrolmentToken `json:"enrolments"`
	Agents     map[string]*AgentRecord    `json:"agents"`
	Revoked    map[string]*RevokedCert    `json:"revoked"`
}

// NewFileStore loads the state at path, creating an empty one if it is absent.
func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{mem: NewMemoryStore(), path: path}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var state fileState
		if jsonErr := json.Unmarshal(data, &state); jsonErr != nil {
			return nil, fmt.Errorf("agentpki: %s is not readable state: %w", path, jsonErr)
		}
		if state.Enrolments != nil {
			s.mem.enrolments = state.Enrolments
		}
		if state.Agents != nil {
			s.mem.agents = state.Agents
		}
		if state.Revoked != nil {
			s.mem.revoked = state.Revoked
		}
	case errors.Is(err, os.ErrNotExist):
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
			return nil, mkErr
		}
	default:
		return nil, err
	}
	return s, nil
}

func (s *FileStore) flush() error {
	s.mem.mu.RLock()
	state := fileState{
		Enrolments: s.mem.enrolments,
		Agents:     s.mem.agents,
		Revoked:    s.mem.revoked,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	s.mem.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *FileStore) write(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(); err != nil {
		return err
	}
	return s.flush()
}

func (s *FileStore) PutEnrolment(ctx context.Context, tok *EnrolmentToken) error {
	return s.write(func() error { return s.mem.PutEnrolment(ctx, tok) })
}

func (s *FileStore) ConsumeEnrolment(ctx context.Context, id string, now time.Time) (*EnrolmentToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.mem.ConsumeEnrolment(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if err := s.flush(); err != nil {
		return nil, err
	}
	return tok, nil
}

func (s *FileStore) GetEnrolment(ctx context.Context, id string) (*EnrolmentToken, error) {
	return s.mem.GetEnrolment(ctx, id)
}

func (s *FileStore) PutAgent(ctx context.Context, rec *AgentRecord) error {
	return s.write(func() error { return s.mem.PutAgent(ctx, rec) })
}

func (s *FileStore) GetAgent(ctx context.Context, agentID string) (*AgentRecord, error) {
	return s.mem.GetAgent(ctx, agentID)
}

func (s *FileStore) ListAgents(ctx context.Context) ([]*AgentRecord, error) {
	return s.mem.ListAgents(ctx)
}

func (s *FileStore) DeleteAgent(ctx context.Context, agentID string) error {
	return s.write(func() error { return s.mem.DeleteAgent(ctx, agentID) })
}

func (s *FileStore) Revoke(ctx context.Context, entry RevokedCert) error {
	return s.write(func() error { return s.mem.Revoke(ctx, entry) })
}

func (s *FileStore) IsRevoked(ctx context.Context, serial string) (bool, error) {
	return s.mem.IsRevoked(ctx, serial)
}

func (s *FileStore) DenyList(ctx context.Context) ([]RevokedCert, error) {
	return s.mem.DenyList(ctx)
}

// ============================================================
// HELPERS
// ============================================================

func cloneEnrolment(tok *EnrolmentToken) *EnrolmentToken {
	clone := *tok
	clone.SecretHash = append([]byte(nil), tok.SecretHash...)
	if tok.UsedAt != nil {
		used := *tok.UsedAt
		clone.UsedAt = &used
	}
	return &clone
}

func cloneAgent(rec *AgentRecord) *AgentRecord {
	clone := *rec
	clone.Current = cloneCert(rec.Current)
	if rec.Previous != nil {
		prev := cloneCert(*rec.Previous)
		clone.Previous = &prev
	}
	if rec.LastSeenAt != nil {
		seen := *rec.LastSeenAt
		clone.LastSeenAt = &seen
	}
	if rec.RevokedAt != nil {
		revoked := *rec.RevokedAt
		clone.RevokedAt = &revoked
	}
	return &clone
}

func cloneCert(rec CertRecord) CertRecord {
	clone := rec
	clone.PublicKeyDER = append([]byte(nil), rec.PublicKeyDER...)
	if rec.SupersededAt != nil {
		superseded := *rec.SupersededAt
		clone.SupersededAt = &superseded
	}
	return clone
}
