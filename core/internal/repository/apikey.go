package repository

// API key storage.
//
// Two things in here are deliberate and worth stating, because both were wrong
// before.
//
//  1. Every statement names its columns. `SELECT *` into a struct is how a
//     migration that adds a column turns into a runtime failure on every
//     install, and this table already has one such reader elsewhere
//     (multi_user.go) that this file does not copy.
//
//  2. The authentication lookup returns EVERY key that could match the
//     presented prefix, not the first one. Key prefixes are not unique - there
//     are two prefix conventions in this codebase, 12 characters here and 8 in
//     service/multi_user.go, and the 8 character form collides for every key -
//     so a lookup that took one row would authenticate against the wrong key's
//     digest and refuse a valid key. The digest comparison in the service is
//     what decides, and it runs over all the candidates.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// apiKeyColumns is the full column list of api_keys, in the order the struct
// declares it. Anything added to migrations/pending/apikey_scopes.sql belongs
// here and on models.APIKey.
const apiKeyColumns = `id, tenant_id, user_id, name, key_hash, key_prefix, scopes,
	last_used, expires_at, status, created_at, revoked_at, revoked_reason,
	rotated_from, rotation_deadline, last_used_ip, allowed_cidrs`

// APIKeyRepository handles API key database operations
type APIKeyRepository struct {
	db *sqlx.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(db *sqlx.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// Create inserts a new API key.
func (r *APIKeyRepository) Create(ctx context.Context, apiKey *models.APIKey) error {
	query := `
		INSERT INTO api_keys (
			id, tenant_id, user_id, name, key_hash, key_prefix, scopes,
			expires_at, status, rotated_from, rotation_deadline, allowed_cidrs
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at
	`
	return r.db.QueryRowContext(ctx, query,
		apiKey.ID,
		apiKey.TenantID,
		apiKey.UserID,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.KeyPrefix,
		apiKey.Scopes,
		apiKey.ExpiresAt,
		apiKey.Status,
		apiKey.RotatedFrom,
		apiKey.RotationDeadline,
		apiKey.AllowedCIDRs,
	).Scan(&apiKey.CreatedAt)
}

// GetByID retrieves an API key by ID and tenant ID
func (r *APIKeyRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	var apiKey models.APIKey
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE tenant_id = $1 AND id = $2`
	if err := r.db.GetContext(ctx, &apiKey, query, tenantID, id); err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// ListByPrefixes returns every key whose stored prefix is one of the given
// candidates, whatever its status. Expiry, revocation and rotation deadlines
// are decided by the service, which needs to be able to tell "no such key"
// from "the key you have was retired" in the audit trail while telling the
// caller neither.
func (r *APIKeyRepository) ListByPrefixes(ctx context.Context, prefixes []string) ([]models.APIKey, error) {
	if len(prefixes) == 0 {
		return nil, nil
	}
	var keys []models.APIKey
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE key_prefix = ANY($1)`
	if err := r.db.SelectContext(ctx, &keys, query, pq.Array(prefixes)); err != nil {
		return nil, err
	}
	return keys, nil
}

// ListByTenant retrieves all API keys for a tenant with pagination
func (r *APIKeyRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.APIKey, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM api_keys WHERE tenant_id = $1",
		tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	var keys []models.APIKey
	query := `SELECT ` + apiKeyColumns + `
		FROM api_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &keys, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}
	return keys, total, nil
}

// ListByUser returns a user's own keys, newest first.
func (r *APIKeyRepository) ListByUser(ctx context.Context, tenantID, userID uuid.UUID) ([]models.APIKey, error) {
	var keys []models.APIKey
	query := `SELECT ` + apiKeyColumns + `
		FROM api_keys
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &keys, query, tenantID, userID); err != nil {
		return nil, err
	}
	return keys, nil
}

// Update writes back the fields an operator may change.
func (r *APIKeyRepository) Update(ctx context.Context, apiKey *models.APIKey) error {
	query := `
		UPDATE api_keys
		SET name = $1, scopes = $2, status = $3, expires_at = $4,
		    allowed_cidrs = $5, revoked_at = $6, revoked_reason = $7,
		    rotation_deadline = $8
		WHERE id = $9 AND tenant_id = $10
	`
	_, err := r.db.ExecContext(ctx, query,
		apiKey.Name,
		apiKey.Scopes,
		apiKey.Status,
		apiKey.ExpiresAt,
		apiKey.AllowedCIDRs,
		apiKey.RevokedAt,
		apiKey.RevokedReason,
		apiKey.RotationDeadline,
		apiKey.ID,
		apiKey.TenantID,
	)
	return err
}

// Delete removes an API key by ID and tenant ID
func (r *APIKeyRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM api_keys WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// MarkUsed records when and where a key was last presented.
func (r *APIKeyRepository) MarkUsed(ctx context.Context, id uuid.UUID, at time.Time, ip string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE api_keys SET last_used = $1, last_used_ip = NULLIF($2, '') WHERE id = $3",
		at, ip, id,
	)
	return err
}

// UpdateLastUsed is kept for callers that only have the id. It records the
// time and leaves the address alone.
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE api_keys SET last_used = $1 WHERE id = $2",
		time.Now(), id,
	)
	return err
}

// UpgradeHash rewrites a key's stored digest, and only if it still holds the
// digest the caller read. That condition is what makes it safe to run from the
// request path: if the key was rotated or revoked between the read and this
// write, nothing happens.
func (r *APIKeyRepository) UpgradeHash(ctx context.Context, id uuid.UUID, oldHash, newHash string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE api_keys SET key_hash = $1 WHERE id = $2 AND key_hash = $3",
		newHash, id, oldHash,
	)
	return err
}

// Revoke retires a key. It reports how many rows changed, so the caller can
// tell "revoked" from "was already revoked, or is not yours".
func (r *APIKeyRepository) Revoke(ctx context.Context, tenantID, id uuid.UUID, reason string, at time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE api_keys
		SET status = 'revoked', revoked_at = $1, revoked_reason = NULLIF($2, '')
		WHERE tenant_id = $3 AND id = $4 AND revoked_at IS NULL`,
		at, reason, tenantID, id,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkSuperseded puts a key into its rotation overlap: it keeps working until
// deadline and is refused afterwards.
func (r *APIKeyRepository) MarkSuperseded(ctx context.Context, tenantID, id uuid.UUID, deadline time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE api_keys
		SET status = 'superseded', rotation_deadline = $1
		WHERE tenant_id = $2 AND id = $3 AND revoked_at IS NULL`,
		deadline, tenantID, id,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListExpiring returns live keys that expire, or whose rotation overlap ends,
// within the given window. It is what a "your integration stops working on
// Friday" notice is built from.
func (r *APIKeyRepository) ListExpiring(ctx context.Context, tenantID uuid.UUID, within time.Duration) ([]models.APIKey, error) {
	var keys []models.APIKey
	query := `SELECT ` + apiKeyColumns + `
		FROM api_keys
		WHERE tenant_id = $1
		  AND revoked_at IS NULL
		  AND (
		        (expires_at IS NOT NULL AND expires_at BETWEEN NOW() AND NOW() + $2::interval)
		     OR (rotation_deadline IS NOT NULL AND rotation_deadline BETWEEN NOW() AND NOW() + $2::interval)
		      )
		ORDER BY COALESCE(rotation_deadline, expires_at) ASC`
	interval := within.String()
	if err := r.db.SelectContext(ctx, &keys, query, tenantID, interval); err != nil {
		return nil, err
	}
	return keys, nil
}
