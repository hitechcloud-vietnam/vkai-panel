package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"go.uber.org/zap"
)

// JobService handles job business logic
type JobService struct {
	repo   *repository.JobRepository
	queue  *job.QueueManager
	logger *zap.Logger
}

// NewJobService creates a new job service
func NewJobService(repo *repository.JobRepository, queue *job.QueueManager, logger *zap.Logger) *JobService {
	return &JobService{
		repo:   repo,
		queue:  queue,
		logger: logger,
	}
}

// EnqueueBackup enqueues a backup job
func (s *JobService) EnqueueBackup(ctx context.Context, tenantID uuid.UUID, payload *job.BackupPayload) (*job.JobRecord, error) {
	// Create job record
	record := &job.JobRecord{
		ID:         uuid.New(),
		TaskType:   job.TaskTypeBackup,
		Status:     job.StatusPending,
		Queue:      "default",
		MaxRetries: 3,
		TenantID:   tenantID,
		ServerID:   &payload.ServerID,
	}

	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	record.Payload = payloadBytes

	// Save to database
	if err := s.repo.CreateJob(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	// Enqueue to Redis
	taskInfo, err := s.queue.EnqueueJob(ctx, job.TaskTypeBackup, payload,
		asynq.Queue("default"),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
	)
	if err != nil {
		// Update status to failed
		s.repo.UpdateJobStatus(ctx, record.ID, job.StatusFailed, nil, err.Error())
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	// Update task ID
	record.TaskID = taskInfo.ID
	if err := s.repo.UpdateJobStatus(ctx, record.ID, job.StatusPending, nil, ""); err != nil {
		s.logger.Warn("Failed to update task ID", zap.Error(err))
	}

	return record, nil
}

// EnqueueRestore enqueues a restore job
func (s *JobService) EnqueueRestore(ctx context.Context, tenantID uuid.UUID, payload *job.RestorePayload) (*job.JobRecord, error) {
	record := &job.JobRecord{
		ID:         uuid.New(),
		TaskType:   job.TaskTypeRestore,
		Status:     job.StatusPending,
		Queue:      "critical",
		MaxRetries: 1,
		TenantID:   tenantID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	record.Payload = payloadBytes

	if err := s.repo.CreateJob(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	taskInfo, err := s.queue.EnqueueJob(ctx, job.TaskTypeRestore, payload,
		asynq.Queue("critical"),
		asynq.MaxRetry(1),
		asynq.Timeout(60*time.Minute),
	)
	if err != nil {
		s.repo.UpdateJobStatus(ctx, record.ID, job.StatusFailed, nil, err.Error())
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	record.TaskID = taskInfo.ID
	return record, nil
}

// EnqueueDeploy enqueues a deployment job
func (s *JobService) EnqueueDeploy(ctx context.Context, tenantID uuid.UUID, payload *job.DeployPayload) (*job.JobRecord, error) {
	record := &job.JobRecord{
		ID:         uuid.New(),
		TaskType:   job.TaskTypeDeploy,
		Status:     job.StatusPending,
		Queue:      "default",
		MaxRetries: 2,
		TenantID:   tenantID,
		ServerID:   &payload.ServerID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	record.Payload = payloadBytes

	if err := s.repo.CreateJob(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	taskInfo, err := s.queue.EnqueueJob(ctx, job.TaskTypeDeploy, payload,
		asynq.Queue("default"),
		asynq.MaxRetry(2),
		asynq.Timeout(15*time.Minute),
	)
	if err != nil {
		s.repo.UpdateJobStatus(ctx, record.ID, job.StatusFailed, nil, err.Error())
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	record.TaskID = taskInfo.ID
	return record, nil
}

// EnqueueSSL enqueues an SSL job
func (s *JobService) EnqueueSSL(ctx context.Context, tenantID uuid.UUID, payload *job.SSLEventPayload) (*job.JobRecord, error) {
	record := &job.JobRecord{
		ID:         uuid.New(),
		TaskType:   job.TaskTypeSSL,
		Status:     job.StatusPending,
		Queue:      "default",
		MaxRetries: 3,
		TenantID:   tenantID,
		ServerID:   &payload.ServerID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	record.Payload = payloadBytes

	if err := s.repo.CreateJob(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	taskInfo, err := s.queue.EnqueueJob(ctx, job.TaskTypeSSL, payload,
		asynq.Queue("default"),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Minute),
	)
	if err != nil {
		s.repo.UpdateJobStatus(ctx, record.ID, job.StatusFailed, nil, err.Error())
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	record.TaskID = taskInfo.ID
	return record, nil
}

// EnqueueCleanup enqueues a cleanup job
func (s *JobService) EnqueueCleanup(ctx context.Context, tenantID uuid.UUID, payload *job.CleanupPayload) (*job.JobRecord, error) {
	record := &job.JobRecord{
		ID:         uuid.New(),
		TaskType:   job.TaskTypeCleanup,
		Status:     job.StatusPending,
		Queue:      "low",
		MaxRetries: 2,
		TenantID:   tenantID,
		ServerID:   &payload.ServerID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	record.Payload = payloadBytes

	if err := s.repo.CreateJob(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	taskInfo, err := s.queue.EnqueueJob(ctx, job.TaskTypeCleanup, payload,
		asynq.Queue("low"),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
	)
	if err != nil {
		s.repo.UpdateJobStatus(ctx, record.ID, job.StatusFailed, nil, err.Error())
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	record.TaskID = taskInfo.ID
	return record, nil
}

// GetJob retrieves a job by ID
func (s *JobService) GetJob(ctx context.Context, id uuid.UUID) (*job.JobRecord, error) {
	return s.repo.GetJob(ctx, id)
}

// ListJobs lists jobs with filters
func (s *JobService) ListJobs(ctx context.Context, tenantID uuid.UUID, filter *job.JobFilter) ([]*job.JobRecord, int, error) {
	return s.repo.ListJobs(ctx, tenantID, filter)
}

// GetJobStats returns job statistics
func (s *JobService) GetJobStats(ctx context.Context, tenantID uuid.UUID) (*job.JobStats, error) {
	return s.repo.GetJobStats(ctx, tenantID)
}

// CancelJob cancels a job
func (s *JobService) CancelJob(ctx context.Context, id uuid.UUID) error {
	record, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if record.Status != job.StatusPending && record.Status != job.StatusActive {
		return fmt.Errorf("cannot cancel job with status: %s", record.Status)
	}

	// Cancel in queue
	if record.TaskID != "" {
		if err := s.queue.CancelJob(record.TaskID); err != nil {
			s.logger.Warn("Failed to cancel job in queue", zap.Error(err))
		}
	}

	// Update status
	return s.repo.UpdateJobStatus(ctx, id, job.StatusCancelled, nil, "Cancelled by user")
}

// DeleteJob deletes a job
func (s *JobService) DeleteJob(ctx context.Context, id uuid.UUID) error {
	record, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if record.Status == job.StatusActive {
		return fmt.Errorf("cannot delete active job")
	}

	return s.repo.DeleteJob(ctx, id)
}

// RetryJob retries a failed job
func (s *JobService) RetryJob(ctx context.Context, id uuid.UUID) error {
	record, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if record.Status != job.StatusFailed {
		return fmt.Errorf("can only retry failed jobs")
	}

	// Re-enqueue
	if record.TaskID != "" {
		if err := s.queue.RetryJob(record.TaskID); err != nil {
			s.logger.Warn("Failed to retry job in queue", zap.Error(err))
		}
	}

	return s.repo.UpdateJobStatus(ctx, id, job.StatusPending, nil, "")
}

// CleanupOldJobs cleans up old completed/failed jobs
func (s *JobService) CleanupOldJobs(ctx context.Context, tenantID uuid.UUID, retentionDays int) (int64, error) {
	return s.repo.CleanupOldJobs(ctx, tenantID, retentionDays)
}

// GetQueueStats returns queue statistics
func (s *JobService) GetQueueStats() (map[string]interface{}, error) {
	return s.queue.GetQueueStats()
}
