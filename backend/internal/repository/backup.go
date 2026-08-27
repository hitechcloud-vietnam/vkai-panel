package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type BackupRepository struct {
	db *sqlx.DB
}

func NewBackupRepository(db *sqlx.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

// Backup Job operations
func (r *BackupRepository) CreateJob(ctx context.Context, job *models.BackupJob) error {
	query := `
		INSERT INTO backup_jobs (id, tenant_id, name, type, resource_id, destination, schedule, retention, encrypted, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING created_at, updated_at`

	job.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		job.ID, job.TenantID, job.Name, job.Type, job.ResourceID,
		job.Destination, job.Schedule, job.Retention, job.Encrypted, job.Status,
	).Scan(&job.CreatedAt, &job.UpdatedAt)
}

func (r *BackupRepository) GetJobByID(ctx context.Context, id uuid.UUID) (*models.BackupJob, error) {
	var job models.BackupJob
	query := `SELECT * FROM backup_jobs WHERE id = $1`
	if err := r.db.GetContext(ctx, &job, query, id); err != nil {
		return nil, fmt.Errorf("backup job not found: %w", err)
	}
	return &job, nil
}

func (r *BackupRepository) ListJobsByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.BackupJob, error) {
	var jobs []models.BackupJob
	query := `SELECT * FROM backup_jobs WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &jobs, query, tenantID); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *BackupRepository) UpdateJob(ctx context.Context, job *models.BackupJob) error {
	query := `UPDATE backup_jobs SET name = $2, destination = $3, schedule = $4, retention = $5, status = $6, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		job.ID, job.Name, job.Destination, job.Schedule, job.Retention, job.Status,
	)
	return err
}

func (r *BackupRepository) DeleteJob(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM backup_jobs WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// Backup Record operations
func (r *BackupRepository) CreateRecord(ctx context.Context, rec *models.BackupRecord) error {
	query := `
		INSERT INTO backup_records (id, job_id, tenant_id, size, path, status, started_at, error_msg)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
		RETURNING started_at`

	rec.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		rec.ID, rec.JobID, rec.TenantID, rec.Size, rec.Path, rec.Status, rec.ErrorMsg,
	).Scan(&rec.StartedAt)
}

func (r *BackupRepository) UpdateRecord(ctx context.Context, rec *models.BackupRecord) error {
	query := `UPDATE backup_records SET size = $2, status = $3, completed_at = $4, error_msg = $5 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, rec.ID, rec.Size, rec.Status, rec.CompletedAt, rec.ErrorMsg)
	return err
}

func (r *BackupRepository) ListRecordsByJob(ctx context.Context, jobID uuid.UUID) ([]models.BackupRecord, error) {
	var records []models.BackupRecord
	query := `SELECT * FROM backup_records WHERE job_id = $1 ORDER BY started_at DESC`
	if err := r.db.SelectContext(ctx, &records, query, jobID); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *BackupRepository) ListRecordsByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.BackupRecord, error) {
	var records []models.BackupRecord
	query := `SELECT * FROM backup_records WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &records, query, tenantID, limit); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *BackupRepository) DeleteRecord(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM backup_records WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
