package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ScheduledTask represents an advanced scheduled task
type ScheduledTask struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Name          string         `json:"name" db:"name"`
	Description   string         `json:"description" db:"description"`
	TaskType      string         `json:"task_type" db:"task_type"` // command, script, http, backup, cleanup, custom
	Command       string         `json:"command" db:"command"`
	ScriptContent string         `json:"script_content" db:"script_content"`
	HTTPEndpoint  string         `json:"http_endpoint" db:"http_endpoint"`
	HTTPMethod    string         `json:"http_method" db:"http_method"`
	Schedule      string         `json:"schedule" db:"schedule"` // cron expression
	ScheduleDesc  string         `json:"schedule_desc" db:"schedule_desc"`
	Timezone      string         `json:"timezone" db:"timezone"`
	IsEnabled     bool           `json:"is_enabled" db:"is_enabled"`
	Priority      int            `json:"priority" db:"priority"` // 1=low, 2=normal, 3=high, 4=critical
	Timeout       int            `json:"timeout" db:"timeout"`   // seconds, 0 = no timeout
	MaxRetries    int            `json:"max_retries" db:"max_retries"`
	RetryDelay    int            `json:"retry_delay" db:"retry_delay"` // seconds
	Tags          pq.StringArray `json:"tags" db:"tags"`
	Environment   pq.StringArray `json:"environment" db:"environment"` // KEY=VALUE pairs
	NotifyOnSuccess bool         `json:"notify_on_success" db:"notify_on_success"`
	NotifyOnFailure bool         `json:"notify_on_failure" db:"notify_on_failure"`
	NotifyEmails  pq.StringArray `json:"notify_emails" db:"notify_emails"`
	LastRunAt     *time.Time     `json:"last_run_at" db:"last_run_at"`
	LastStatus    string         `json:"last_status" db:"last_status"`
	NextRunAt     *time.Time     `json:"next_run_at" db:"next_run_at"`
	RunCount      int            `json:"run_count" db:"run_count"`
	FailCount     int            `json:"fail_count" db:"fail_count"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
}

// TaskExecution represents a single execution of a task
type TaskExecution struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TaskID      uuid.UUID  `json:"task_id" db:"task_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Status      string     `json:"status" db:"status"` // pending, running, success, failed, timeout, cancelled
	StartedAt   *time.Time `json:"started_at" db:"started_at"`
	FinishedAt  *time.Time `json:"finished_at" db:"finished_at"`
	Duration    int        `json:"duration" db:"duration"` // milliseconds
	ExitCode    *int       `json:"exit_code" db:"exit_code"`
	Output      string     `json:"output" db:"output"`
	ErrorOutput string     `json:"error_output" db:"error_output"`
	RetryCount  int        `json:"retry_count" db:"retry_count"`
	TriggeredBy string     `json:"triggered_by" db:"triggered_by"` // schedule, manual, retry
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// TaskTemplate represents a reusable task template
type TaskTemplate struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	TenantID    uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	TaskType    string         `json:"task_type" db:"task_type"`
	Command     string         `json:"command" db:"command"`
	ScriptContent string       `json:"script_content" db:"script_content"`
	Schedule    string         `json:"schedule" db:"schedule"`
	Tags        pq.StringArray `json:"tags" db:"tags"`
	IsPublic    bool           `json:"is_public" db:"is_public"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
}

// TaskGroup represents a group of related tasks
type TaskGroup struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	TenantID    uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	Color       string         `json:"color" db:"color"`
	TaskCount   int            `json:"task_count" db:"task_count"`
	Tags        pq.StringArray `json:"tags" db:"tags"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
}

// Request types
type CreateScheduledTaskRequest struct {
	Name            string   `json:"name" binding:"required"`
	Description     string   `json:"description"`
	TaskType        string   `json:"task_type" binding:"required"`
	Command         string   `json:"command"`
	ScriptContent   string   `json:"script_content"`
	HTTPEndpoint    string   `json:"http_endpoint"`
	HTTPMethod      string   `json:"http_method"`
	Schedule        string   `json:"schedule" binding:"required"`
	Timezone        string   `json:"timezone"`
	Priority        int      `json:"priority"`
	Timeout         int      `json:"timeout"`
	MaxRetries      int      `json:"max_retries"`
	RetryDelay      int      `json:"retry_delay"`
	Tags            []string `json:"tags"`
	Environment     []string `json:"environment"`
	NotifyOnSuccess bool     `json:"notify_on_success"`
	NotifyOnFailure bool     `json:"notify_on_failure"`
	NotifyEmails    []string `json:"notify_emails"`
}

type UpdateScheduledTaskRequest struct {
	Name            *string  `json:"name"`
	Description     *string  `json:"description"`
	TaskType        *string  `json:"task_type"`
	Command         *string  `json:"command"`
	ScriptContent   *string  `json:"script_content"`
	HTTPEndpoint    *string  `json:"http_endpoint"`
	HTTPMethod      *string  `json:"http_method"`
	Schedule        *string  `json:"schedule"`
	Timezone        *string  `json:"timezone"`
	IsEnabled       *bool    `json:"is_enabled"`
	Priority        *int     `json:"priority"`
	Timeout         *int     `json:"timeout"`
	MaxRetries      *int     `json:"max_retries"`
	RetryDelay      *int     `json:"retry_delay"`
	Tags            []string `json:"tags"`
	Environment     []string `json:"environment"`
	NotifyOnSuccess *bool    `json:"notify_on_success"`
	NotifyOnFailure *bool    `json:"notify_on_failure"`
	NotifyEmails    []string `json:"notify_emails"`
}

type CreateTaskTemplateRequest struct {
	Name          string   `json:"name" binding:"required"`
	Description   string   `json:"description"`
	TaskType      string   `json:"task_type" binding:"required"`
	Command       string   `json:"command"`
	ScriptContent string   `json:"script_content"`
	Schedule      string   `json:"schedule"`
	Tags          []string `json:"tags"`
	IsPublic      bool     `json:"is_public"`
}

type CreateTaskGroupRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	Tags        []string `json:"tags"`
}

// Stats
type ScheduledTaskStats struct {
	TotalTasks      int     `json:"total_tasks" db:"total_tasks"`
	EnabledTasks    int     `json:"enabled_tasks" db:"enabled_tasks"`
	DisabledTasks   int     `json:"disabled_tasks" db:"disabled_tasks"`
	TotalExecutions int     `json:"total_executions" db:"total_executions"`
	SuccessRate     float64 `json:"success_rate" db:"success_rate"`
	FailedToday     int     `json:"failed_today" db:"failed_today"`
	RunToday        int     `json:"run_today" db:"run_today"`
	AvgDuration     float64 `json:"avg_duration" db:"avg_duration"` // milliseconds
}
