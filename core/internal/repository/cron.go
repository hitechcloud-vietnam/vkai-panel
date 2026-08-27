package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type CronRepository struct {
	db *sqlx.DB
}

func NewCronRepository(db *sqlx.DB) *CronRepository {
	return &CronRepository{db: db}
}

func (r *CronRepository) Create(ctx context.Context, job *models.CronJob) error {
	query := `
		INSERT INTO cron_jobs (id, tenant_id, server_id, name, command, schedule, type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING created_at, updated_at`

	job.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		job.ID, job.TenantID, job.ServerID, job.Name,
		job.Command, job.Schedule, job.Type, job.Status,
	).Scan(&job.CreatedAt, &job.UpdatedAt)
}

func (r *CronRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.CronJob, error) {
	var job models.CronJob
	query := `SELECT * FROM cron_jobs WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &job, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("cron job not found: %w", err)
	}
	return &job, nil
}

func (r *CronRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.CronJob, error) {
	var jobs []models.CronJob
	query := `SELECT * FROM cron_jobs WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &jobs, query, tenantID); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *CronRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.CronJob, error) {
	var jobs []models.CronJob
	query := `SELECT * FROM cron_jobs WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &jobs, query, serverID); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *CronRepository) Update(ctx context.Context, job *models.CronJob) error {
	query := `UPDATE cron_jobs SET name = $2, command = $3, schedule = $4, type = $5, status = $6, updated_at = NOW() WHERE id = $1 AND tenant_id = $7`
	_, err := r.db.ExecContext(ctx, query,
		job.ID, job.Name, job.Command, job.Schedule, job.Type, job.Status, job.TenantID,
	)
	return err
}

func (r *CronRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM cron_jobs WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *CronRepository) UpdateLastRun(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `UPDATE cron_jobs SET last_run_at = NOW(), updated_at = NOW() WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *CronRepository) ToggleStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	query := `UPDATE cron_jobs SET status = $2, updated_at = NOW() WHERE id = $1 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, id, status, tenantID)
	return err
}
