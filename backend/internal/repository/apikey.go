package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// APIKeyRepository handles API key database operations
type APIKeyRepository struct {
	db *sqlx.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(db *sqlx.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// Create inserts a new API key into the database
func (r *APIKeyRepository) Create(ctx context.Context, apiKey *models.APIKey) error {
	query := `
		INSERT INTO api_keys (id, tenant_id, user_id, name, key_hash, key_prefix, scopes, expires_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
	).Scan(&apiKey.CreatedAt)
}

// GetByID retrieves an API key by ID and tenant ID
func (r *APIKeyRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	var apiKey models.APIKey
	query := `
		SELECT id, tenant_id, user_id, name, key_hash, key_prefix, scopes, last_used, expires_at, status, created_at
		FROM api_keys
		WHERE tenant_id = $1 AND id = $2
	`
	err := r.db.GetContext(ctx, &apiKey, query, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetByKeyPrefix retrieves an API key by its key prefix (for authentication lookup)
func (r *APIKeyRepository) GetByKeyPrefix(ctx context.Context, keyPrefix string) (*models.APIKey, error) {
	var apiKey models.APIKey
	query := `
		SELECT id, tenant_id, user_id, name, key_hash, key_prefix, scopes, last_used, expires_at, status, created_at
		FROM api_keys
		WHERE key_prefix = $1 AND status = 'active'
	`
	err := r.db.GetContext(ctx, &apiKey, query, keyPrefix)
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// ListByTenant retrieves all API keys for a tenant with pagination
func (r *APIKeyRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.APIKey, int, error) {
	// Count total
	var total int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM api_keys WHERE tenant_id = $1",
		tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get keys
	query := `
		SELECT id, tenant_id, user_id, name, key_hash, key_prefix, scopes, last_used, expires_at, status, created_at
		FROM api_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(
			&k.ID, &k.TenantID, &k.UserID, &k.Name, &k.KeyHash,
			&k.KeyPrefix, &k.Scopes, &k.LastUsed, &k.ExpiresAt,
			&k.Status, &k.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		keys = append(keys, k)
	}

	return keys, total, nil
}

// Update updates an existing API key
func (r *APIKeyRepository) Update(ctx context.Context, apiKey *models.APIKey) error {
	query := `
		UPDATE api_keys
		SET name = $1, scopes = $2, status = $3, expires_at = $4
		WHERE id = $5 AND tenant_id = $6
	`
	_, err := r.db.ExecContext(ctx, query,
		apiKey.Name,
		apiKey.Scopes,
		apiKey.Status,
		apiKey.ExpiresAt,
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

// UpdateLastUsed updates the last_used timestamp for an API key
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE api_keys SET last_used = $1 WHERE id = $2",
		time.Now(), id,
	)
	return err
}
