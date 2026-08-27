package job

import (
	"time"

	"github.com/google/uuid"
)

// Task types
const (
	TaskTypeBackup        = "backup"
	TaskTypeRestore       = "restore"
	TaskTypeDeploy        = "deploy"
	TaskTypeSSL           = "ssl"
	TaskTypeCleanup       = "cleanup"
	TaskTypeHealthCheck   = "health_check"
	TaskTypeMetricCollect = "metric_collect"
	TaskTypeLogRotate     = "log_rotate"
	TaskTypeNotification  = "notification"
)

// Job statuses
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusArchived  = "archived"
)

// BackupPayload represents backup task payload
type BackupPayload struct {
	BackupID   uuid.UUID `json:"backup_id"`
	ServerID   uuid.UUID `json:"server_id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	BackupType string    `json:"backup_type"` // full, incremental, differential
	Target     string    `json:"target"`       // local, s3, gcs, azure
	Retention  int       `json:"retention_days"`
}

// RestorePayload represents restore task payload
type RestorePayload struct {
	BackupID   uuid.UUID `json:"backup_id"`
	ServerID   uuid.UUID `json:"server_id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	TargetPath string    `json:"target_path"`
}

// DeployPayload represents deploy task payload
type DeployPayload struct {
	DeploymentID uuid.UUID `json:"deployment_id"`
	Repository   string    `json:"repository"`
	Branch       string    `json:"branch"`
	CommitSHA    string    `json:"commit_sha"`
	ServerID     uuid.UUID `json:"server_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Environment  string    `json:"environment"` // staging, production
}

// SSLEventPayload represents SSL task payload
type SSLEventPayload struct {
	Domain   string    `json:"domain"`
	Action   string    `json:"action"` // issue, renew, revoke
	ServerID uuid.UUID `json:"server_id"`
	TenantID uuid.UUID `json:"tenant_id"`
}

// CleanupPayload represents cleanup task payload
type CleanupPayload struct {
	Type          string    `json:"type"` // logs, backups, temp_files
	ServerID      uuid.UUID `json:"server_id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	RetentionDays int       `json:"retention_days"`
}

// HealthCheckPayload represents health check task payload
type HealthCheckPayload struct {
	ServerID uuid.UUID `json:"server_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Services []string  `json:"services"` // nginx, mysql, redis, etc.
}

// MetricCollectPayload represents metric collection task payload
type MetricCollectPayload struct {
	ServerID uuid.UUID `json:"server_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Interval int       `json:"interval_seconds"`
}

// LogRotatePayload represents log rotation task payload
type LogRotatePayload struct {
	SourceID uuid.UUID `json:"source_id"`
	ServerID uuid.UUID `json:"server_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	MaxSize  int       `json:"max_size_mb"`
	MaxFiles int       `json:"max_files"`
}

// NotificationPayload represents notification task payload
type NotificationPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
	Channel        string    `json:"channel"` // email, slack, webhook, sms
	Recipient      string    `json:"recipient"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
}

// JobRecord represents a job record in the database
type JobRecord struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TaskID      string     `json:"task_id" db:"task_id"`
	TaskType    string     `json:"task_type" db:"task_type"`
	Status      string     `json:"status" db:"status"`
	Queue       string     `json:"queue" db:"queue"`
	Payload     []byte     `json:"payload" db:"payload"`
	Result      []byte     `json:"result" db:"result"`
	Error       string     `json:"error" db:"error"`
	RetryCount  int        `json:"retry_count" db:"retry_count"`
	MaxRetries  int        `json:"max_retries" db:"max_retries"`
	ScheduledAt *time.Time `json:"scheduled_at" db:"scheduled_at"`
	StartedAt   *time.Time `json:"started_at" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
	FailedAt    *time.Time `json:"failed_at" db:"failed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID    *uuid.UUID `json:"server_id" db:"server_id"`
	UserID      *uuid.UUID `json:"user_id" db:"user_id"`
}

// JobFilter represents filters for querying jobs
type JobFilter struct {
	TaskType string     `json:"task_type"`
	Status   string     `json:"status"`
	Queue    string     `json:"queue"`
	ServerID *uuid.UUID `json:"server_id"`
	UserID   *uuid.UUID `json:"user_id"`
	From     *time.Time `json:"from"`
	To       *time.Time `json:"to"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// JobStats represents job statistics
type JobStats struct {
	Total      int            `json:"total"`
	Completed  int            `json:"completed"`
	Failed     int            `json:"failed"`
	Pending    int            `json:"pending"`
	Active     int            `json:"active"`
	ByType     map[string]int `json:"by_type"`
	ByQueue    map[string]int `json:"by_queue"`
	AvgRuntime float64        `json:"avg_runtime_seconds"`
}
