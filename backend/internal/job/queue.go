package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// QueueManager manages the job queue
type QueueManager struct {
	client  *asynq.Client
	server  *asynq.Server
	inspector *asynq.Inspector
	logger  *zap.Logger
}

// NewQueueManager creates a new queue manager
func NewQueueManager(redisAddr, redisPassword string, redisDB int, logger *zap.Logger) *QueueManager {
	redisOpt := asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	}

	client := asynq.NewClient(redisOpt)
	inspector := asynq.NewInspector(redisOpt)

	return &QueueManager{
		client:    client,
		inspector: inspector,
		logger:    logger,
	}
}

// StartWorker starts the job worker
func (qm *QueueManager) StartWorker(concurrency int) {
	redisOpt := asynq.RedisClientOpt{
		Addr: "localhost:6379",
	}

	qm.server = asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeBackup, qm.handleBackupTask)
	mux.HandleFunc(TaskTypeRestore, qm.handleRestoreTask)
	mux.HandleFunc(TaskTypeDeploy, qm.handleDeployTask)
	mux.HandleFunc(TaskTypeSSL, qm.handleSSLTask)
	mux.HandleFunc(TaskTypeCleanup, qm.handleCleanupTask)
	mux.HandleFunc(TaskTypeHealthCheck, qm.handleHealthCheckTask)
	mux.HandleFunc(TaskTypeMetricCollect, qm.handleMetricCollectTask)
	mux.HandleFunc(TaskTypeLogRotate, qm.handleLogRotateTask)
	mux.HandleFunc(TaskTypeNotification, qm.handleNotificationTask)

	if err := qm.server.Run(mux); err != nil {
		qm.logger.Fatal("Failed to start job worker", zap.Error(err))
	}
}

// StopWorker stops the job worker
func (qm *QueueManager) StopWorker() {
	if qm.server != nil {
		qm.server.Stop()
	}
}

// EnqueueJob enqueues a new job
func (qm *QueueManager) EnqueueJob(ctx context.Context, taskType string, payload interface{}, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(taskType, data, opts...)
	info, err := qm.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	qm.logger.Info("Job enqueued",
		zap.String("task_id", info.ID),
		zap.String("task_type", taskType),
		zap.String("queue", info.Queue),
	)

	return info, nil
}

// GetJobStatus returns the status of a job
func (qm *QueueManager) GetJobStatus(taskID string) (*JobStatus, error) {
	info, err := qm.inspector.GetTaskInfo("default", taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}

	return &JobStatus{
		ID:        info.ID,
		Type:      info.Type,
		Status:    string(info.State),
		Queue:     info.Queue,
		Payload:   info.Payload,
		CreatedAt: info.NextProcessAt,
		UpdatedAt: info.LastFailedAt,
	}, nil
}

// GetQueueStats returns queue statistics
func (qm *QueueManager) GetQueueStats() (map[string]interface{}, error) {
	queues, err := qm.inspector.Queues()
	if err != nil {
		return nil, fmt.Errorf("failed to get queues: %w", err)
	}

	stats := make(map[string]interface{})
	for _, queue := range queues {
		info, err := qm.inspector.GetQueueInfo(queue)
		if err != nil {
			continue
		}
		stats[queue] = map[string]interface{}{
			"active":    info.Active,
			"pending":   info.Pending,
			"scheduled": info.Scheduled,
			"retry":     info.Retry,
			"archived":  info.Archived,
			"completed": info.Completed,
		}
	}

	return stats, nil
}

// CancelJob cancels a job
func (qm *QueueManager) CancelJob(taskID string) error {
	return qm.inspector.CancelProcessing(taskID)
}

// DeleteJob deletes a job
func (qm *QueueManager) DeleteJob(taskID string) error {
	return qm.inspector.DeleteTask("default", taskID)
}

// RetryJob retries a failed job
func (qm *QueueManager) RetryJob(taskID string) error {
	return qm.inspector.RunTask("default", taskID)
}

// ArchiveJob archives a job
func (qm *QueueManager) ArchiveJob(taskID string) error {
	return qm.inspector.ArchiveTask("default", taskID)
}

// Close closes the queue manager
func (qm *QueueManager) Close() error {
	return qm.client.Close()
}

// Task handlers
func (qm *QueueManager) handleBackupTask(ctx context.Context, task *asynq.Task) error {
	var payload BackupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal backup payload: %w", err)
	}

	qm.logger.Info("Processing backup task",
		zap.String("backup_id", payload.BackupID.String()),
		zap.String("server_id", payload.ServerID.String()),
	)

	// TODO: Implement actual backup logic
	time.Sleep(2 * time.Second)

	return nil
}

func (qm *QueueManager) handleRestoreTask(ctx context.Context, task *asynq.Task) error {
	var payload RestorePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal restore payload: %w", err)
	}

	qm.logger.Info("Processing restore task",
		zap.String("backup_id", payload.BackupID.String()),
	)

	// TODO: Implement actual restore logic
	time.Sleep(3 * time.Second)

	return nil
}

func (qm *QueueManager) handleDeployTask(ctx context.Context, task *asynq.Task) error {
	var payload DeployPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal deploy payload: %w", err)
	}

	qm.logger.Info("Processing deploy task",
		zap.String("deployment_id", payload.DeploymentID.String()),
		zap.String("repository", payload.Repository),
	)

	// TODO: Implement actual deployment logic
	time.Sleep(5 * time.Second)

	return nil
}

func (qm *QueueManager) handleSSLTask(ctx context.Context, task *asynq.Task) error {
	var payload SSLEventPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal SSL payload: %w", err)
	}

	qm.logger.Info("Processing SSL task",
		zap.String("domain", payload.Domain),
		zap.String("action", payload.Action),
	)

	// TODO: Implement actual SSL logic
	time.Sleep(2 * time.Second)

	return nil
}

func (qm *QueueManager) handleCleanupTask(ctx context.Context, task *asynq.Task) error {
	var payload CleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal cleanup payload: %w", err)
	}

	qm.logger.Info("Processing cleanup task",
		zap.String("type", payload.Type),
		zap.Int("retention_days", payload.RetentionDays),
	)

	// TODO: Implement actual cleanup logic
	time.Sleep(1 * time.Second)

	return nil
}

func (qm *QueueManager) handleHealthCheckTask(ctx context.Context, task *asynq.Task) error {
	var payload HealthCheckPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal health check payload: %w", err)
	}

	qm.logger.Info("Processing health check task",
		zap.String("server_id", payload.ServerID.String()),
	)

	// TODO: Implement actual health check logic
	time.Sleep(500 * time.Millisecond)

	return nil
}

func (qm *QueueManager) handleMetricCollectTask(ctx context.Context, task *asynq.Task) error {
	var payload MetricCollectPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal metric collect payload: %w", err)
	}

	qm.logger.Info("Processing metric collect task",
		zap.String("server_id", payload.ServerID.String()),
	)

	// TODO: Implement actual metric collection logic
	time.Sleep(1 * time.Second)

	return nil
}

func (qm *QueueManager) handleLogRotateTask(ctx context.Context, task *asynq.Task) error {
	var payload LogRotatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal log rotate payload: %w", err)
	}

	qm.logger.Info("Processing log rotate task",
		zap.String("source_id", payload.SourceID.String()),
	)

	// TODO: Implement actual log rotation logic
	time.Sleep(1 * time.Second)

	return nil
}

func (qm *QueueManager) handleNotificationTask(ctx context.Context, task *asynq.Task) error {
	var payload NotificationPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal notification payload: %w", err)
	}

	qm.logger.Info("Processing notification task",
		zap.String("notification_id", payload.NotificationID.String()),
		zap.String("channel", payload.Channel),
	)

	// TODO: Implement actual notification sending logic
	time.Sleep(500 * time.Millisecond)

	return nil
}

// JobStatus represents the status of a job
type JobStatus struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Queue     string    `json:"queue"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
