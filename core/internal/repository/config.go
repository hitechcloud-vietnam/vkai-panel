package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// ConfigRepository handles config database operations
type ConfigRepository struct {
	db *sqlx.DB
}

// NewConfigRepository creates a new config repository
func NewConfigRepository(db *sqlx.DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

// CreateSnapshot creates a new config snapshot
func (r *ConfigRepository) CreateSnapshot(ctx context.Context, snapshot *config.ConfigSnapshot) error {
	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.Content)))
	snapshot.Checksum = checksum

	// Get next version
	var maxVersion int
	err := r.db.GetContext(ctx, &maxVersion,
		"SELECT COALESCE(MAX(version), 0) FROM config_snapshots WHERE config_type = $1 AND name = $2 AND server_id = $3",
		snapshot.ConfigType, snapshot.Name, snapshot.ServerID,
	)
	if err != nil {
		return err
	}
	snapshot.Version = maxVersion + 1

	query := `
		INSERT INTO config_snapshots (
			id, config_type, name, path, content, checksum, version,
			is_active, is_automatic, description, tenant_id, server_id, user_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`

	_, err = r.db.ExecContext(ctx, query,
		snapshot.ID,
		snapshot.ConfigType,
		snapshot.Name,
		snapshot.Path,
		snapshot.Content,
		snapshot.Checksum,
		snapshot.Version,
		snapshot.IsActive,
		snapshot.IsAutomatic,
		snapshot.Description,
		snapshot.TenantID,
		snapshot.ServerID,
		snapshot.UserID,
	)

	return err
}

// GetSnapshot retrieves a snapshot by ID
func (r *ConfigRepository) GetSnapshot(ctx context.Context, id uuid.UUID) (*config.ConfigSnapshot, error) {
	query := `
		SELECT id, config_type, name, path, content, checksum, version,
			   is_active, is_automatic, description, created_at, tenant_id, server_id, user_id
		FROM config_snapshots
		WHERE id = $1
	`

	var snapshot config.ConfigSnapshot
	err := r.db.GetContext(ctx, &snapshot, query, id)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// GetActiveSnapshot retrieves the active snapshot for a config
func (r *ConfigRepository) GetActiveSnapshot(ctx context.Context, configType config.ConfigType, name string, serverID uuid.UUID) (*config.ConfigSnapshot, error) {
	query := `
		SELECT id, config_type, name, path, content, checksum, version,
			   is_active, is_automatic, description, created_at, tenant_id, server_id, user_id
		FROM config_snapshots
		WHERE config_type = $1 AND name = $2 AND server_id = $3 AND is_active = true
	`

	var snapshot config.ConfigSnapshot
	err := r.db.GetContext(ctx, &snapshot, query, configType, name, serverID)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// ListSnapshots lists snapshots with filters
func (r *ConfigRepository) ListSnapshots(ctx context.Context, tenantID uuid.UUID, filter *config.ConfigFilter) ([]*config.ConfigSnapshot, int, error) {
	query := `
		SELECT id, config_type, name, path, content, checksum, version,
			   is_active, is_automatic, description, created_at, tenant_id, server_id, user_id
		FROM config_snapshots
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	argIndex := 2

	if filter.ConfigType != "" {
		query += fmt.Sprintf(" AND config_type = $%d", argIndex)
		args = append(args, filter.ConfigType)
		argIndex++
	}

	if filter.Name != "" {
		query += fmt.Sprintf(" AND name ILIKE $%d", argIndex)
		args = append(args, "%"+filter.Name+"%")
		argIndex++
	}

	if filter.ServerID != nil {
		query += fmt.Sprintf(" AND server_id = $%d", argIndex)
		args = append(args, *filter.ServerID)
		argIndex++
	}

	if filter.IsActive != nil {
		query += fmt.Sprintf(" AND is_active = $%d", argIndex)
		args = append(args, *filter.IsActive)
		argIndex++
	}

	if filter.From != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIndex)
		args = append(args, *filter.From)
		argIndex++
	}

	if filter.To != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIndex)
		args = append(args, *filter.To)
		argIndex++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS count"
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	query += " ORDER BY created_at DESC"
	if filter.PageSize > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, filter.PageSize)
		argIndex++
	}

	if filter.Page > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, offset)
		argIndex++
	}

	var snapshots []*config.ConfigSnapshot
	err = r.db.SelectContext(ctx, &snapshots, query, args...)
	if err != nil {
		return nil, 0, err
	}

	return snapshots, total, nil
}

// SetActiveSnapshot sets a snapshot as active and deactivates others
func (r *ConfigRepository) SetActiveSnapshot(ctx context.Context, id uuid.UUID) error {
	// Get the snapshot
	snapshot, err := r.GetSnapshot(ctx, id)
	if err != nil {
		return err
	}

	// Deactivate all snapshots of same type/name/server
	_, err = r.db.ExecContext(ctx,
		"UPDATE config_snapshots SET is_active = false WHERE config_type = $1 AND name = $2 AND server_id = $3",
		snapshot.ConfigType, snapshot.Name, snapshot.ServerID,
	)
	if err != nil {
		return err
	}

	// Activate the target snapshot
	_, err = r.db.ExecContext(ctx,
		"UPDATE config_snapshots SET is_active = true WHERE id = $1",
		id,
	)

	return err
}

// DeleteSnapshot deletes a snapshot
func (r *ConfigRepository) DeleteSnapshot(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM config_snapshots WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// GetSnapshotHistory returns version history for a config
func (r *ConfigRepository) GetSnapshotHistory(ctx context.Context, configType config.ConfigType, name string, serverID uuid.UUID, limit int) ([]*config.ConfigSnapshot, error) {
	query := `
		SELECT id, config_type, name, path, content, checksum, version,
			   is_active, is_automatic, description, created_at, tenant_id, server_id, user_id
		FROM config_snapshots
		WHERE config_type = $1 AND name = $2 AND server_id = $3
		ORDER BY version DESC
		LIMIT $4
	`

	var snapshots []*config.ConfigSnapshot
	err := r.db.SelectContext(ctx, &snapshots, query, configType, name, serverID, limit)
	if err != nil {
		return nil, err
	}

	return snapshots, nil
}

// GetConfigStats returns config statistics
func (r *ConfigRepository) GetConfigStats(ctx context.Context, tenantID uuid.UUID) (*config.ConfigStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_snapshots,
			COUNT(CASE WHEN is_active = true THEN 1 END) as active_configs
		FROM config_snapshots
		WHERE tenant_id = $1
	`

	var stats config.ConfigStats
	err := r.db.GetContext(ctx, &stats, query, tenantID)
	if err != nil {
		return nil, err
	}

	// Get counts by type
	typeQuery := `
		SELECT config_type, COUNT(*) as count
		FROM config_snapshots
		WHERE tenant_id = $1
		GROUP BY config_type
	`

	typeRows, err := r.db.QueryContext(ctx, typeQuery, tenantID)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()

	stats.ByType = make(map[string]int)
	for typeRows.Next() {
		var configType string
		var count int
		if err := typeRows.Scan(&configType, &count); err != nil {
			return nil, err
		}
		stats.ByType[configType] = count
	}

	// Get counts by server. The servers table identifies a server by hostname;
	// it has never had a name column, so this query used to fail outright and
	// take the whole config stats endpoint down with it.
	serverQuery := `
		SELECT s.hostname, COUNT(*) as count
		FROM config_snapshots cs
		JOIN servers s ON cs.server_id = s.id
		WHERE cs.tenant_id = $1
		GROUP BY s.hostname
	`

	serverRows, err := r.db.QueryContext(ctx, serverQuery, tenantID)
	if err != nil {
		return nil, err
	}
	defer serverRows.Close()

	stats.ByServer = make(map[string]int)
	for serverRows.Next() {
		var serverName string
		var count int
		if err := serverRows.Scan(&serverName, &count); err != nil {
			return nil, err
		}
		stats.ByServer[serverName] = count
	}

	// Get last snapshot time
	lastQuery := `
		SELECT MAX(created_at)
		FROM config_snapshots
		WHERE tenant_id = $1
	`

	err = r.db.GetContext(ctx, &stats.LastSnapshot, lastQuery, tenantID)
	if err != nil {
		stats.LastSnapshot = nil
	}

	// Get storage used
	storageQuery := `
		SELECT COALESCE(SUM(LENGTH(content)), 0)
		FROM config_snapshots
		WHERE tenant_id = $1
	`

	err = r.db.GetContext(ctx, &stats.StorageUsed, storageQuery, tenantID)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// CleanupOldSnapshots deletes old snapshots keeping only recent versions
func (r *ConfigRepository) CleanupOldSnapshots(ctx context.Context, tenantID uuid.UUID, keepVersions int) (int64, error) {
	query := `
		DELETE FROM config_snapshots
		WHERE tenant_id = $1 AND id NOT IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY config_type, name, server_id 
					ORDER BY version DESC
				) as rn
				FROM config_snapshots
				WHERE tenant_id = $2
			) ranked
			WHERE rn <= $3
		)
	`

	result, err := r.db.ExecContext(ctx, query, tenantID, tenantID, keepVersions)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// CreateTemplate creates a new config template
func (r *ConfigRepository) CreateTemplate(ctx context.Context, template *config.ConfigTemplate) error {
	query := `
		INSERT INTO config_templates (
			id, name, config_type, content, description, variables, is_default, tenant_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		template.ID,
		template.Name,
		template.ConfigType,
		template.Content,
		template.Description,
		template.Variables,
		template.IsDefault,
		template.TenantID,
	)

	return err
}

// GetTemplate retrieves a template by ID
func (r *ConfigRepository) GetTemplate(ctx context.Context, id uuid.UUID) (*config.ConfigTemplate, error) {
	query := `
		SELECT id, name, config_type, content, description, variables, is_default, created_at, updated_at, tenant_id
		FROM config_templates
		WHERE id = $1
	`

	var template config.ConfigTemplate
	err := r.db.GetContext(ctx, &template, query, id)
	if err != nil {
		return nil, err
	}

	return &template, nil
}

// ListTemplates lists templates
func (r *ConfigRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID, configType config.ConfigType) ([]*config.ConfigTemplate, error) {
	query := `
		SELECT id, name, config_type, content, description, variables, is_default, created_at, updated_at, tenant_id
		FROM config_templates
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}

	if configType != "" {
		query += " AND config_type = $2"
		args = append(args, configType)
	}

	query += " ORDER BY name"

	var templates []*config.ConfigTemplate
	err := r.db.SelectContext(ctx, &templates, query, args...)
	if err != nil {
		return nil, err
	}

	return templates, nil
}

// UpdateTemplate updates a template
func (r *ConfigRepository) UpdateTemplate(ctx context.Context, template *config.ConfigTemplate) error {
	query := `
		UPDATE config_templates
		SET name = $1, content = $2, description = $3, variables = $4, is_default = $5, updated_at = $6
		WHERE id = $7
	`

	_, err := r.db.ExecContext(ctx, query,
		template.Name,
		template.Content,
		template.Description,
		template.Variables,
		template.IsDefault,
		time.Now(),
		template.ID,
	)

	return err
}

// DeleteTemplate deletes a template
func (r *ConfigRepository) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM config_templates WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
