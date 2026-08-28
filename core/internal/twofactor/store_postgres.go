package twofactor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// PostgresStore is the production Store. Every statement is scoped by user id,
// and the two that must not race - spending a time step and spending a recovery
// code - are single conditional UPDATEs whose row count is the answer, not a
// read followed by a write.
type PostgresStore struct {
	db *sqlx.DB
}

// NewPostgresStore returns a Store backed by the panel database.
func NewPostgresStore(db *sqlx.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

const enrolmentColumns = `user_id, tenant_id, key_version, secret_ciphertext, algorithm, digits,
	period_seconds, enabled, confirmed_at, last_step, last_used_at, failed_attempts,
	locked_until, created_at, updated_at`

func (s *PostgresStore) Account(ctx context.Context, userID uuid.UUID) (*Account, error) {
	var account Account
	query := `SELECT id, tenant_id, username, email, password_hash
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	if err := s.db.GetContext(ctx, &account, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAccount
		}
		return nil, fmt.Errorf("load account for two-factor: %w", err)
	}
	return &account, nil
}

func (s *PostgresStore) Get(ctx context.Context, userID uuid.UUID) (*Enrolment, error) {
	var enrolment Enrolment
	query := `SELECT ` + enrolmentColumns + ` FROM user_two_factor WHERE user_id = $1`
	if err := s.db.GetContext(ctx, &enrolment, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No enrolment is a normal state, not an error.
			return nil, nil
		}
		return nil, fmt.Errorf("load two-factor enrolment: %w", err)
	}
	return &enrolment, nil
}

func (s *PostgresStore) Save(ctx context.Context, enrolment *Enrolment) error {
	// The conflict clause refuses to overwrite a confirmed enrolment. Replacing
	// a live second factor must go through Disable, which demands the password
	// and a current code; without this guard a stolen session could start a new
	// enrolment and quietly take over the factor.
	query := `
		INSERT INTO user_two_factor (
			user_id, tenant_id, key_version, secret_ciphertext, algorithm, digits,
			period_seconds, enabled, confirmed_at, last_step, last_used_at,
			failed_attempts, locked_until, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, NULL, 0, NULL, 0, NULL, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			key_version = EXCLUDED.key_version,
			secret_ciphertext = EXCLUDED.secret_ciphertext,
			algorithm = EXCLUDED.algorithm,
			digits = EXCLUDED.digits,
			period_seconds = EXCLUDED.period_seconds,
			enabled = FALSE,
			confirmed_at = NULL,
			last_step = 0,
			last_used_at = NULL,
			failed_attempts = 0,
			locked_until = NULL,
			created_at = NOW(),
			updated_at = NOW()
		WHERE user_two_factor.enabled = FALSE`

	result, err := s.db.ExecContext(ctx, query,
		enrolment.UserID, enrolment.TenantID, enrolment.KeyVersion,
		enrolment.SecretCiphertext, enrolment.Algorithm, enrolment.Digits,
		enrolment.PeriodSeconds,
	)
	if err != nil {
		return fmt.Errorf("save two-factor enrolment: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrAlreadyEnabled
	}
	return nil
}

func (s *PostgresStore) Confirm(ctx context.Context, userID uuid.UUID, step int64, at time.Time) error {
	query := `
		UPDATE user_two_factor
		SET enabled = TRUE, confirmed_at = $2, last_step = $3, last_used_at = $2,
			failed_attempts = 0, locked_until = NULL, updated_at = $2
		WHERE user_id = $1 AND enabled = FALSE`

	result, err := s.db.ExecContext(ctx, query, userID, at, step)
	if err != nil {
		return fmt.Errorf("confirm two-factor enrolment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm two-factor enrolment: %w", err)
	}
	if affected == 0 {
		return ErrNoPendingEnrolment
	}
	return nil
}

func (s *PostgresStore) Delete(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete two-factor enrolment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_two_factor_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_two_factor WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete two-factor enrolment: %w", err)
	}
	return tx.Commit()
}

func (s *PostgresStore) SpendStep(ctx context.Context, userID uuid.UUID, step int64, at time.Time) (bool, error) {
	// "last_step < $2" is the replay guard. Two requests carrying the same code
	// race here, and exactly one updates a row.
	query := `
		UPDATE user_two_factor
		SET last_step = $2, last_used_at = $3, updated_at = $3
		WHERE user_id = $1 AND enabled = TRUE AND last_step < $2`

	result, err := s.db.ExecContext(ctx, query, userID, step, at)
	if err != nil {
		return false, fmt.Errorf("record two-factor step: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("record two-factor step: %w", err)
	}
	return affected > 0, nil
}

func (s *PostgresStore) RecordFailure(ctx context.Context, userID uuid.UUID, at time.Time, threshold int, lockout time.Duration) (int, *time.Time, error) {
	query := `
		UPDATE user_two_factor
		SET failed_attempts = failed_attempts + 1,
			locked_until = CASE
				WHEN $2 > 0 AND failed_attempts + 1 >= $2 THEN $3
				ELSE locked_until
			END,
			updated_at = $4
		WHERE user_id = $1
		RETURNING failed_attempts, locked_until`

	var attempts int
	var lockedUntil *time.Time
	err := s.db.QueryRowContext(ctx, query, userID, threshold, at.Add(lockout), at).
		Scan(&attempts, &lockedUntil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("record two-factor failure: %w", err)
	}
	return attempts, lockedUntil, nil
}

func (s *PostgresStore) ClearFailures(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE user_two_factor SET failed_attempts = 0, locked_until = NULL WHERE user_id = $1`
	if _, err := s.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("clear two-factor failures: %w", err)
	}
	return nil
}

func (s *PostgresStore) ReplaceRecoveryCodes(ctx context.Context, userID uuid.UUID, hashes []string, at time.Time) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace recovery codes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The old set dies with the new one being written, in one transaction: a
	// user must never be left with zero usable codes because an insert failed.
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_two_factor_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("replace recovery codes: %w", err)
	}

	for _, hash := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_two_factor_recovery_codes (id, user_id, code_hash, created_at)
			 VALUES ($1, $2, $3, $4)`,
			uuid.New(), userID, hash, at,
		); err != nil {
			return fmt.Errorf("replace recovery codes: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) UnusedRecoveryCodes(ctx context.Context, userID uuid.UUID) ([]RecoveryCode, error) {
	var codes []RecoveryCode
	query := `SELECT id, user_id, code_hash, used_at, used_ip, created_at
		FROM user_two_factor_recovery_codes
		WHERE user_id = $1 AND used_at IS NULL
		ORDER BY created_at`
	if err := s.db.SelectContext(ctx, &codes, query, userID); err != nil {
		return nil, fmt.Errorf("load recovery codes: %w", err)
	}
	return codes, nil
}

func (s *PostgresStore) SpendRecoveryCode(ctx context.Context, id uuid.UUID, at time.Time, ip string) (bool, error) {
	query := `
		UPDATE user_two_factor_recovery_codes
		SET used_at = $2, used_ip = $3
		WHERE id = $1 AND used_at IS NULL`

	result, err := s.db.ExecContext(ctx, query, id, at, ip)
	if err != nil {
		return false, fmt.Errorf("spend recovery code: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("spend recovery code: %w", err)
	}
	return affected > 0, nil
}

func (s *PostgresStore) CountRecoveryCodes(ctx context.Context, userID uuid.UUID) (int, int, error) {
	var row struct {
		Unused int `db:"unused"`
		Total  int `db:"total"`
	}
	query := `SELECT COUNT(*) FILTER (WHERE used_at IS NULL) AS unused, COUNT(*) AS total
		FROM user_two_factor_recovery_codes WHERE user_id = $1`
	if err := s.db.GetContext(ctx, &row, query, userID); err != nil {
		return 0, 0, fmt.Errorf("count recovery codes: %w", err)
	}
	return row.Unused, row.Total, nil
}

func (s *PostgresStore) SetUserFlag(ctx context.Context, userID uuid.UUID, enabled bool) error {
	// users.mfa_enabled already exists and is what the rest of the panel reads.
	// users.mfa_secret is deliberately left NULL: the secret lives encrypted in
	// user_two_factor and must never be written to a plaintext column.
	query := `UPDATE users SET mfa_enabled = $2, mfa_secret = NULL, updated_at = NOW() WHERE id = $1`
	if _, err := s.db.ExecContext(ctx, query, userID, enabled); err != nil {
		return fmt.Errorf("update user mfa flag: %w", err)
	}
	return nil
}

// compile-time assertion that the Postgres store satisfies the interface.
var _ Store = (*PostgresStore)(nil)
