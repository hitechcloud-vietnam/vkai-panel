package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type AuditRepository struct {
	db *sqlx.DB
}

func NewAuditRepository(db *sqlx.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := `
		INSERT INTO audit_logs (tenant_id, user_id, action, resource, resource_id, details, ip_address, user_agent, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		log.TenantID, log.UserID, log.Action, log.Resource, log.ResourceID,
		log.Details, log.IPAddress, log.UserAgent, log.Status,
	).Scan(&log.ID, &log.CreatedAt)
}

func (r *AuditRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.AuditLog, error) {
	var log models.AuditLog
	err := r.db.GetContext(ctx, &log,
		"SELECT * FROM audit_logs WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditRepository) Search(ctx context.Context, tenantID uuid.UUID, req *models.AuditLogSearchRequest) ([]models.AuditLog, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if req.UserID != nil {
		where += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, *req.UserID)
		argIdx++
	}
	if req.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, req.Action)
		argIdx++
	}
	if req.Resource != "" {
		where += fmt.Sprintf(" AND resource = $%d", argIdx)
		args = append(args, req.Resource)
		argIdx++
	}
	if req.ResourceID != nil {
		where += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, *req.ResourceID)
		argIdx++
	}
	if req.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, req.Status)
		argIdx++
	}
	if req.Start != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *req.Start)
		argIdx++
	}
	if req.End != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *req.End)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM audit_logs " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get logs
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT * FROM audit_logs %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.Action, &l.Resource,
			&l.ResourceID, &l.Details, &l.IPAddress, &l.UserAgent, &l.Status, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}

	return logs, total, nil
}

func (r *AuditRepository) GetStats(ctx context.Context, tenantID uuid.UUID, days int) (*models.AuditLogStats, error) {
	stats := &models.AuditLogStats{
		ByAction:   make(map[string]int),
		ByResource: make(map[string]int),
		ByStatus:   make(map[string]int),
		ByUser:     make(map[string]int),
	}

	since := time.Now().AddDate(0, 0, -days)

	// Total logs
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2",
		tenantID, since,
	).Scan(&stats.TotalLogs)
	if err != nil {
		return nil, err
	}

	// By action
	rows, err := r.db.QueryContext(ctx,
		"SELECT action, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY action",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var count int
		if err := rows.Scan(&action, &count); err != nil {
			return nil, err
		}
		stats.ByAction[action] = count
	}

	// By resource
	rows, err = r.db.QueryContext(ctx,
		"SELECT resource, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY resource",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var resource string
		var count int
		if err := rows.Scan(&resource, &count); err != nil {
			return nil, err
		}
		stats.ByResource[resource] = count
	}

	// By status
	rows, err = r.db.QueryContext(ctx,
		"SELECT status, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY status",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.ByStatus[status] = count
	}

	// By user
	rows, err = r.db.QueryContext(ctx,
		"SELECT COALESCE(u.username, 'system'), COUNT(*) FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id WHERE a.tenant_id = $1 AND a.created_at >= $2 GROUP BY u.username",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		var count int
		if err := rows.Scan(&username, &count); err != nil {
			return nil, err
		}
		stats.ByUser[username] = count
	}

	// Recent logs
	rows, err = r.db.QueryContext(ctx,
		"SELECT * FROM audit_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 10",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.Action, &l.Resource,
			&l.ResourceID, &l.Details, &l.IPAddress, &l.UserAgent, &l.Status, &l.CreatedAt); err != nil {
			return nil, err
		}
		stats.RecentLogs = append(stats.RecentLogs, l)
	}

	return stats, nil
}

func (r *AuditRepository) DeleteOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM audit_logs WHERE tenant_id = $1 AND created_at < NOW() - INTERVAL '1 day' * $2",
		tenantID, days,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
