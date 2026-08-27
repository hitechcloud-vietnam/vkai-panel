package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type LogRepository struct {
	db *sqlx.DB
}

func NewLogRepository(db *sqlx.DB) *LogRepository {
	return &LogRepository{db: db}
}

// Log entries
func (r *LogRepository) CreateEntry(ctx context.Context, entry *models.LogEntry) error {
	query := `
		INSERT INTO log_entries (server_id, tenant_id, source, level, message, details, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		entry.ServerID, entry.TenantID, entry.Source, entry.Level,
		entry.Message, entry.Details, entry.Timestamp,
	).Scan(&entry.ID, &entry.CreatedAt)
}

func (r *LogRepository) SearchEntries(ctx context.Context, tenantID uuid.UUID, req *models.LogSearchRequest) ([]models.LogEntry, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if req.ServerID != nil {
		where += fmt.Sprintf(" AND server_id = $%d", argIdx)
		args = append(args, *req.ServerID)
		argIdx++
	}
	if req.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", argIdx)
		args = append(args, req.Source)
		argIdx++
	}
	if req.Level != "" {
		where += fmt.Sprintf(" AND level = $%d", argIdx)
		args = append(args, req.Level)
		argIdx++
	}
	if req.Query != "" {
		where += fmt.Sprintf(" AND message ILIKE $%d", argIdx)
		args = append(args, "%"+req.Query+"%")
		argIdx++
	}
	if req.Start != nil {
		where += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, *req.Start)
		argIdx++
	}
	if req.End != nil {
		where += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, *req.End)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM log_entries " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get entries
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, server_id, tenant_id, source, level, message, details, timestamp, created_at
		FROM log_entries %s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []models.LogEntry
	for rows.Next() {
		var e models.LogEntry
		if err := rows.Scan(&e.ID, &e.ServerID, &e.TenantID, &e.Source, &e.Level,
			&e.Message, &e.Details, &e.Timestamp, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}

func (r *LogRepository) DeleteOldEntries(ctx context.Context, tenantID uuid.UUID, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM log_entries WHERE tenant_id = $1 AND timestamp < $2",
		tenantID, before,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Log sources
func (r *LogRepository) CreateSource(ctx context.Context, source *models.LogSource) error {
	query := `
		INSERT INTO log_sources (tenant_id, server_id, name, type, path, format, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		source.TenantID, source.ServerID, source.Name, source.Type,
		source.Path, source.Format, source.IsActive,
	).Scan(&source.ID, &source.CreatedAt, &source.UpdatedAt)
}

func (r *LogRepository) GetSourceByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LogSource, error) {
	var source models.LogSource
	err := r.db.GetContext(ctx, &source,
		"SELECT * FROM log_sources WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *LogRepository) ListSources(ctx context.Context, tenantID uuid.UUID, serverID *uuid.UUID) ([]models.LogSource, error) {
	var sources []models.LogSource
	var err error
	if serverID != nil {
		err = r.db.SelectContext(ctx, &sources,
			"SELECT * FROM log_sources WHERE tenant_id = $1 AND server_id = $2 ORDER BY name",
			tenantID, *serverID,
		)
	} else {
		err = r.db.SelectContext(ctx, &sources,
			"SELECT * FROM log_sources WHERE tenant_id = $1 ORDER BY name",
			tenantID,
		)
	}
	if err != nil {
		return nil, err
	}
	return sources, nil
}

func (r *LogRepository) UpdateSource(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLogSourceRequest) (*models.LogSource, error) {
	source, err := r.GetSourceByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		source.Name = *req.Name
	}
	if req.Type != nil {
		source.Type = *req.Type
	}
	if req.Path != nil {
		source.Path = *req.Path
	}
	if req.Format != nil {
		source.Format = *req.Format
	}
	if req.IsActive != nil {
		source.IsActive = *req.IsActive
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE log_sources SET name=$1, type=$2, path=$3, format=$4, is_active=$5, updated_at=NOW()
		 WHERE tenant_id=$6 AND id=$7`,
		source.Name, source.Type, source.Path, source.Format, source.IsActive,
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return source, nil
}

func (r *LogRepository) DeleteSource(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM log_sources WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// Log rotation
func (r *LogRepository) CreateRotation(ctx context.Context, rotation *models.LogRotation) error {
	query := `
		INSERT INTO log_rotations (tenant_id, server_id, source, max_size_mb, max_age_days, max_files, compress_old, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		rotation.TenantID, rotation.ServerID, rotation.Source,
		rotation.MaxSizeMB, rotation.MaxAgeDays, rotation.MaxFiles,
		rotation.CompressOld, rotation.IsActive,
	).Scan(&rotation.ID, &rotation.CreatedAt, &rotation.UpdatedAt)
}

func (r *LogRepository) GetRotationByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LogRotation, error) {
	var rotation models.LogRotation
	err := r.db.GetContext(ctx, &rotation,
		"SELECT * FROM log_rotations WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &rotation, nil
}

func (r *LogRepository) ListRotations(ctx context.Context, tenantID uuid.UUID, serverID *uuid.UUID) ([]models.LogRotation, error) {
	var rotations []models.LogRotation
	var err error
	if serverID != nil {
		err = r.db.SelectContext(ctx, &rotations,
			"SELECT * FROM log_rotations WHERE tenant_id = $1 AND server_id = $2 ORDER BY source",
			tenantID, *serverID,
		)
	} else {
		err = r.db.SelectContext(ctx, &rotations,
			"SELECT * FROM log_rotations WHERE tenant_id = $1 ORDER BY source",
			tenantID,
		)
	}
	if err != nil {
		return nil, err
	}
	return rotations, nil
}

func (r *LogRepository) UpdateRotation(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLogRotationRequest) (*models.LogRotation, error) {
	rotation, err := r.GetRotationByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.MaxSizeMB != nil {
		rotation.MaxSizeMB = *req.MaxSizeMB
	}
	if req.MaxAgeDays != nil {
		rotation.MaxAgeDays = *req.MaxAgeDays
	}
	if req.MaxFiles != nil {
		rotation.MaxFiles = *req.MaxFiles
	}
	if req.CompressOld != nil {
		rotation.CompressOld = *req.CompressOld
	}
	if req.IsActive != nil {
		rotation.IsActive = *req.IsActive
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE log_rotations SET max_size_mb=$1, max_age_days=$2, max_files=$3, compress_old=$4, is_active=$5, updated_at=NOW()
		 WHERE tenant_id=$6 AND id=$7`,
		rotation.MaxSizeMB, rotation.MaxAgeDays, rotation.MaxFiles,
		rotation.CompressOld, rotation.IsActive, tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return rotation, nil
}

func (r *LogRepository) DeleteRotation(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM log_rotations WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}
