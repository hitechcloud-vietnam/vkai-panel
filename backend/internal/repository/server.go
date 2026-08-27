package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type ServerRepository struct {
	db *sqlx.DB
}

func NewServerRepository(db *sqlx.DB) *ServerRepository {
	return &ServerRepository{db: db}
}

func (r *ServerRepository) Create(ctx context.Context, server *models.Server) error {
	query := `
		INSERT INTO servers (id, tenant_id, hostname, ip_address, ipv6_address, ssh_port,
			agent_status, agent_token, os, kernel, cpu_cores, ram_total, disk_total,
			location, tags, role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		server.ID, server.TenantID, server.Hostname, server.IPAddress,
		server.IPv6Address, server.SSHPort, server.AgentStatus, server.AgentToken,
		server.OS, server.Kernel, server.CPUCores, server.RAMTotal, server.DiskTotal,
		server.Location, server.Tags, server.Role, server.Status,
	).Scan(&server.CreatedAt, &server.UpdatedAt)
}

func (r *ServerRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Server, error) {
	var server models.Server
	query := `SELECT * FROM servers WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &server, query, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return &server, nil
}

func (r *ServerRepository) GetByAgentToken(ctx context.Context, token string) (*models.Server, error) {
	var server models.Server
	query := `SELECT * FROM servers WHERE agent_token = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &server, query, token)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return &server, nil
}

func (r *ServerRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Server, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	var servers []models.Server
	offset := (page - 1) * perPage
	query := `SELECT * FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &servers, query, tenantID, perPage, offset); err != nil {
		return nil, 0, err
	}

	return servers, total, nil
}

func (r *ServerRepository) Update(ctx context.Context, server *models.Server) error {
	query := `
		UPDATE servers SET hostname=$1, ip_address=$2, ipv6_address=$3, ssh_port=$4,
		agent_status=$5, os=$6, kernel=$7, cpu_cores=$8, ram_total=$9, disk_total=$10,
		location=$11, tags=$12, role=$13, status=$14, updated_at=NOW()
		WHERE id=$15 AND tenant_id=$16 AND deleted_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query,
		server.Hostname, server.IPAddress, server.IPv6Address, server.SSHPort,
		server.AgentStatus, server.OS, server.Kernel, server.CPUCores,
		server.RAMTotal, server.DiskTotal, server.Location, server.Tags,
		server.Role, server.Status, server.ID, server.TenantID,
	)
	return err
}

func (r *ServerRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, metrics *models.ServerMetric) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update last seen
	_, err = tx.ExecContext(ctx, `UPDATE servers SET last_seen_at=NOW(), agent_status='online' WHERE id=$1`, id)
	if err != nil {
		return err
	}

	// Insert metrics
	_, err = tx.ExecContext(ctx, `
		INSERT INTO server_metrics (id, server_id, cpu_percent, ram_used, ram_total, disk_used, disk_total, net_in, net_out, load1, load5, load15, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
	`, uuid.New(), id, metrics.CPUPercent, metrics.RAMUsed, metrics.RAMTotal,
		metrics.DiskUsed, metrics.DiskTotal, metrics.NetIn, metrics.NetOut,
		metrics.Load1, metrics.Load5, metrics.Load15)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ServerRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `UPDATE servers SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *ServerRepository) GetLatestMetrics(ctx context.Context, serverID uuid.UUID) (*models.ServerMetric, error) {
	var metric models.ServerMetric
	query := `SELECT * FROM server_metrics WHERE server_id = $1 ORDER BY timestamp DESC LIMIT 1`
	err := r.db.GetContext(ctx, &metric, query, serverID)
	if err != nil {
		return nil, err
	}
	return &metric, nil
}
