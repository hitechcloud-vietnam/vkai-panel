package twofactor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Enrolment is one account's second factor. It lives in its own table rather
// than in columns on `users`: the user repository selects `SELECT *` into
// models.User, so adding columns there would break every user query until that
// struct is updated in lockstep.
type Enrolment struct {
	UserID     uuid.UUID `db:"user_id"`
	TenantID   uuid.UUID `db:"tenant_id"`
	KeyVersion int       `db:"key_version"`

	// SecretCiphertext is the sealed TOTP secret. The plaintext secret never
	// leaves memory once enrolment is confirmed.
	SecretCiphertext string `db:"secret_ciphertext"`

	Algorithm     string `db:"algorithm"`
	Digits        int    `db:"digits"`
	PeriodSeconds int    `db:"period_seconds"`

	// Enabled is false for a started-but-unconfirmed enrolment. Nothing may
	// treat the account as protected until the user has proved one code.
	Enabled     bool       `db:"enabled"`
	ConfirmedAt *time.Time `db:"confirmed_at"`

	// LastStep is the highest time step already spent. It is what stops a code
	// captured off a shoulder or a phishing page from being replayed during the
	// rest of its validity window.
	LastStep   int64      `db:"last_step"`
	LastUsedAt *time.Time `db:"last_used_at"`

	FailedAttempts int        `db:"failed_attempts"`
	LockedUntil    *time.Time `db:"locked_until"`

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Period returns the configured time step as a duration.
func (e *Enrolment) Period() time.Duration {
	if e == nil || e.PeriodSeconds <= 0 {
		return DefaultPeriod
	}
	return time.Duration(e.PeriodSeconds) * time.Second
}

// DigitCount returns the configured digit count.
func (e *Enrolment) DigitCount() int {
	if e == nil || e.Digits <= 0 {
		return DefaultDigits
	}
	return e.Digits
}

// RecoveryCode is one stored single-use code. Only the hash is kept.
type RecoveryCode struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	CodeHash  string     `db:"code_hash"`
	UsedAt    *time.Time `db:"used_at"`
	UsedIP    *string    `db:"used_ip"`
	CreatedAt time.Time  `db:"created_at"`
}

// Account is the slice of the user record two-factor needs: the tenant for
// audit scoping, the username for the provisioning label, and the password
// hash for the re-authentication that enrolling and disabling require.
type Account struct {
	UserID       uuid.UUID `db:"id"`
	TenantID     uuid.UUID `db:"tenant_id"`
	Username     string    `db:"username"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
}

// ErrNoAccount means the user id does not resolve to a live account.
var ErrNoAccount = errors.New("account not found")

// Store is the persistence this package needs. It is an interface so the
// service can be tested without a database, and so the Postgres implementation
// stays the only place that knows SQL.
type Store interface {
	// Account loads the credentials slice of a user record.
	Account(ctx context.Context, userID uuid.UUID) (*Account, error)

	// Get returns the enrolment, or (nil, nil) when the account has none.
	Get(ctx context.Context, userID uuid.UUID) (*Enrolment, error)

	// Save writes a started enrolment, replacing any unconfirmed one.
	Save(ctx context.Context, enrolment *Enrolment) error

	// Confirm flips an enrolment to enabled and spends the proving step in one
	// statement, so a confirmation cannot be interleaved with a replay.
	Confirm(ctx context.Context, userID uuid.UUID, step int64, at time.Time) error

	// Delete removes the enrolment and every recovery code with it.
	Delete(ctx context.Context, userID uuid.UUID) error

	// SpendStep records step as used and reports whether it was still unspent.
	// This is the replay guard and must be atomic: two concurrent requests
	// carrying the same code must not both succeed.
	SpendStep(ctx context.Context, userID uuid.UUID, step int64, at time.Time) (bool, error)

	// RecordFailure increments the failure counter and applies the lockout once
	// the threshold is reached. It returns the resulting state.
	RecordFailure(ctx context.Context, userID uuid.UUID, at time.Time, threshold int, lockout time.Duration) (attempts int, lockedUntil *time.Time, err error)

	// ClearFailures resets the counter after a success.
	ClearFailures(ctx context.Context, userID uuid.UUID) error

	// ReplaceRecoveryCodes atomically discards the old set and stores a new one.
	ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, hashes []string, at time.Time) error

	// UnusedRecoveryCodes returns the codes still available.
	UnusedRecoveryCodes(ctx context.Context, userID uuid.UUID) ([]RecoveryCode, error)

	// SpendRecoveryCode marks one code used and reports whether it was still
	// unused, so a code cannot be spent twice by concurrent requests.
	SpendRecoveryCode(ctx context.Context, id uuid.UUID, at time.Time, ip string) (bool, error)

	// CountRecoveryCodes returns (unused, total).
	CountRecoveryCodes(ctx context.Context, userID uuid.UUID) (unused int, total int, err error)

	// SetUserFlag mirrors the enabled state onto users.mfa_enabled, which the
	// rest of the panel already reads.
	SetUserFlag(ctx context.Context, userID uuid.UUID, enabled bool) error
}

// ---------------------------------------------------------------------------
// In-memory store
// ---------------------------------------------------------------------------

// MemoryStore is a Store for tests and for a panel running without a database
// (the CLI recovery path). It is safe for concurrent use.
type MemoryStore struct {
	mu         sync.Mutex
	accounts   map[uuid.UUID]Account
	enrolments map[uuid.UUID]*Enrolment
	codes      map[uuid.UUID][]*RecoveryCode
	flags      map[uuid.UUID]bool
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		accounts:   make(map[uuid.UUID]Account),
		enrolments: make(map[uuid.UUID]*Enrolment),
		codes:      make(map[uuid.UUID][]*RecoveryCode),
		flags:      make(map[uuid.UUID]bool),
	}
}

// AddAccount registers an account so the service can re-authenticate it.
func (s *MemoryStore) AddAccount(account Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[account.UserID] = account
}

// UserFlag reports the mirrored users.mfa_enabled value.
func (s *MemoryStore) UserFlag(userID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flags[userID]
}

func (s *MemoryStore) Account(_ context.Context, userID uuid.UUID) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[userID]
	if !ok {
		return nil, ErrNoAccount
	}
	copied := account
	return &copied, nil
}

func (s *MemoryStore) Get(_ context.Context, userID uuid.UUID) (*Enrolment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrolment, ok := s.enrolments[userID]
	if !ok {
		return nil, nil
	}
	copied := *enrolment
	return &copied, nil
}

func (s *MemoryStore) Save(_ context.Context, enrolment *Enrolment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Same refusal as the Postgres store: a confirmed enrolment is never
	// overwritten by starting a new one.
	if existing, ok := s.enrolments[enrolment.UserID]; ok && existing.Enabled {
		return ErrAlreadyEnabled
	}
	copied := *enrolment
	if copied.CreatedAt.IsZero() {
		copied.CreatedAt = time.Now().UTC()
	}
	copied.UpdatedAt = time.Now().UTC()
	s.enrolments[enrolment.UserID] = &copied
	return nil
}

func (s *MemoryStore) Confirm(_ context.Context, userID uuid.UUID, step int64, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrolment, ok := s.enrolments[userID]
	if !ok {
		return ErrNoPendingEnrolment
	}
	enrolment.Enabled = true
	enrolment.ConfirmedAt = &at
	enrolment.LastStep = step
	enrolment.LastUsedAt = &at
	enrolment.FailedAttempts = 0
	enrolment.LockedUntil = nil
	enrolment.UpdatedAt = at
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.enrolments, userID)
	delete(s.codes, userID)
	return nil
}

func (s *MemoryStore) SpendStep(_ context.Context, userID uuid.UUID, step int64, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrolment, ok := s.enrolments[userID]
	if !ok {
		return false, ErrNotEnrolled
	}
	if enrolment.LastStep >= step {
		return false, nil
	}
	enrolment.LastStep = step
	enrolment.LastUsedAt = &at
	enrolment.UpdatedAt = at
	return true, nil
}

func (s *MemoryStore) RecordFailure(_ context.Context, userID uuid.UUID, at time.Time, threshold int, lockout time.Duration) (int, *time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrolment, ok := s.enrolments[userID]
	if !ok {
		return 0, nil, nil
	}
	enrolment.FailedAttempts++
	enrolment.UpdatedAt = at
	if threshold > 0 && enrolment.FailedAttempts >= threshold {
		until := at.Add(lockout)
		enrolment.LockedUntil = &until
	}
	return enrolment.FailedAttempts, enrolment.LockedUntil, nil
}

func (s *MemoryStore) ClearFailures(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enrolment, ok := s.enrolments[userID]
	if !ok {
		return nil
	}
	enrolment.FailedAttempts = 0
	enrolment.LockedUntil = nil
	return nil
}

func (s *MemoryStore) ReplaceRecoveryCodes(_ context.Context, userID uuid.UUID, hashes []string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := make([]*RecoveryCode, 0, len(hashes))
	for _, hash := range hashes {
		stored = append(stored, &RecoveryCode{
			ID:        uuid.New(),
			UserID:    userID,
			CodeHash:  hash,
			CreatedAt: at,
		})
	}
	s.codes[userID] = stored
	return nil
}

func (s *MemoryStore) UnusedRecoveryCodes(_ context.Context, userID uuid.UUID) ([]RecoveryCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var unused []RecoveryCode
	for _, code := range s.codes[userID] {
		if code.UsedAt == nil {
			unused = append(unused, *code)
		}
	}
	return unused, nil
}

func (s *MemoryStore) SpendRecoveryCode(_ context.Context, id uuid.UUID, at time.Time, ip string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, codes := range s.codes {
		for _, code := range codes {
			if code.ID != id {
				continue
			}
			if code.UsedAt != nil {
				return false, nil
			}
			code.UsedAt = &at
			address := ip
			code.UsedIP = &address
			return true, nil
		}
	}
	return false, nil
}

func (s *MemoryStore) CountRecoveryCodes(_ context.Context, userID uuid.UUID) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unused := 0
	codes := s.codes[userID]
	for _, code := range codes {
		if code.UsedAt == nil {
			unused++
		}
	}
	return unused, len(codes), nil
}

func (s *MemoryStore) SetUserFlag(_ context.Context, userID uuid.UUID, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flags[userID] = enabled
	return nil
}
