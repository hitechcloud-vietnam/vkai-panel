package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
)

// JobRepository handles job database operations
type JobRepository struct {
	db *sqlx.DB
}

// NewJobRepository creates a new job repository
func NewJobRepository(db *sqlx.DB) *JobRepository {
	return &JobRepository{db: db}
}

// CreateJob creates a new job record
func (r *JobRepository) CreateJob(ctx context.Context, record *job.JobRecord) error {
	query := `
		INSERT INTO jobs (
			id, task_id, task_type, status, queue, payload, 
			max_retries, scheduled_at, tenant_id, server_id, user_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		record.ID,
		record.TaskID,
		record.TaskType,
		record.Status,
		record.Queue,
		record.Payload,
		record.MaxRetries,
		record.ScheduledAt,
		record.TenantID,
		record.ServerID,
		record.UserID,
	)

	return err
}

// GetJob retrieves a job by ID
func (r *JobRepository) GetJob(ctx context.Context, id uuid.UUID) (*job.JobRecord, error) {
	query := `
		SELECT id, task_id, task_type, status, queue, payload, result, error,
			   retry_count, max_retries, scheduled_at, started_at, completed_at,
			   failed_at, created_at, updated_at, tenant_id, server_id, user_id
		FROM jobs
		WHERE id = $1
	`

	var record job.JobRecord
	err := r.db.GetContext(ctx, &record, query, id)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

// GetJobByTaskID retrieves a job by task ID
func (r *JobRepository) GetJobByTaskID(ctx context.Context, taskID string) (*job.JobRecord, error) {
	query := `
		SELECT id, task_id, task_type, status, queue, payload, result, error,
			   retry_count, max_retries, scheduled_at, started_at, completed_at,
			   failed_at, created_at, updated_at, tenant_id, server_id, user_id
		FROM jobs
		WHERE task_id = $1
	`

	var record job.JobRecord
	err := r.db.GetContext(ctx, &record, query, taskID)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

// ListJobs lists jobs with filters
func (r *JobRepository) ListJobs(ctx context.Context, tenantID uuid.UUID, filter *job.JobFilter) ([]*job.JobRecord, int, error) {
	query := `
		SELECT id, task_id, task_type, status, queue, payload, result, error,
			   retry_count, max_retries, scheduled_at, started_at, completed_at,
			   failed_at, created_at, updated_at, tenant_id, server_id, user_id
		FROM jobs
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	argIndex := 2

	if filter.TaskType != "" {
		query += fmt.Sprintf(" AND task_type = $%d", argIndex)
		args = append(args, filter.TaskType)
		argIndex++
	}

	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, filter.Status)
		argIndex++
	}

	if filter.Queue != "" {
		query += fmt.Sprintf(" AND queue = $%d", argIndex)
		args = append(args, filter.Queue)
		argIndex++
	}

	if filter.ServerID != nil {
		query += fmt.Sprintf(" AND server_id = $%d", argIndex)
		args = append(args, *filter.ServerID)
		argIndex++
	}

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, *filter.UserID)
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

	var records []*job.JobRecord
	err = r.db.SelectContext(ctx, &records, query, args...)
	if err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// UpdateJobStatus updates a job's status
func (r *JobRepository) UpdateJobStatus(ctx context.Context, id uuid.UUID, status string, result []byte, errMsg string) error {
	query := `
		UPDATE jobs
		SET status = $1, result = $2, error = $3, updated_at = $4
		WHERE id = $5
	`

	_, err := r.db.ExecContext(ctx, query, status, result, errMsg, time.Now(), id)
	return err
}

// UpdateJobStarted updates a job when it starts processing
func (r *JobRepository) UpdateJobStarted(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE jobs
		SET status = 'active', started_at = $1, updated_at = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

// UpdateJobCompleted updates a job when it completes
func (r *JobRepository) UpdateJobCompleted(ctx context.Context, id uuid.UUID, result []byte) error {
	query := `
		UPDATE jobs
		SET status = 'completed', result = $1, completed_at = $2, updated_at = $2
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, result, time.Now(), id)
	return err
}

// UpdateJobFailed updates a job when it fails
func (r *JobRepository) UpdateJobFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	query := `
		UPDATE jobs
		SET status = 'failed', error = $1, failed_at = $2, updated_at = $2, retry_count = retry_count + 1
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, errMsg, time.Now(), id)
	return err
}

// DeleteJob deletes a job
func (r *JobRepository) DeleteJob(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM jobs WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// GetJobStats returns job statistics
func (r *JobRepository) GetJobStats(ctx context.Context, tenantID uuid.UUID) (*job.JobStats, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
			COUNT(CASE WHEN status = 'active' THEN 1 END) as active
		FROM jobs
		WHERE tenant_id = $1
	`

	var stats job.JobStats
	err := r.db.GetContext(ctx, &stats, query, tenantID)
	if err != nil {
		return nil, err
	}

	// Get counts by type
	typeQuery := `
		SELECT task_type, COUNT(*) as count
		FROM jobs
		WHERE tenant_id = $1
		GROUP BY task_type
	`

	typeRows, err := r.db.QueryContext(ctx, typeQuery, tenantID)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()

	stats.ByType = make(map[string]int)
	for typeRows.Next() {
		var taskType string
		var count int
		if err := typeRows.Scan(&taskType, &count); err != nil {
			return nil, err
		}
		stats.ByType[taskType] = count
	}

	// Get counts by queue
	queueQuery := `
		SELECT queue, COUNT(*) as count
		FROM jobs
		WHERE tenant_id = $1
		GROUP BY queue
	`

	queueRows, err := r.db.QueryContext(ctx, queueQuery, tenantID)
	if err != nil {
		return nil, err
	}
	defer queueRows.Close()

	stats.ByQueue = make(map[string]int)
	for queueRows.Next() {
		var queue string
		var count int
		if err := queueRows.Scan(&queue, &count); err != nil {
			return nil, err
		}
		stats.ByQueue[queue] = count
	}

	// Get average runtime
	runtimeQuery := `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - started_at))), 0)
		FROM jobs
		WHERE tenant_id = $1 AND status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL
	`

	err = r.db.GetContext(ctx, &stats.AvgRuntime, runtimeQuery, tenantID)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// CleanupOldJobs deletes jobs older than retention days
func (r *JobRepository) CleanupOldJobs(ctx context.Context, tenantID uuid.UUID, retentionDays int) (int64, error) {
	query := `
		DELETE FROM jobs
		WHERE tenant_id = $1 AND created_at < $2 AND status IN ('completed', 'failed', 'cancelled')
	`

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := r.db.ExecContext(ctx, query, tenantID, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
