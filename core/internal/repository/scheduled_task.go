package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type ScheduledTaskRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewScheduledTaskRepository(db *sqlx.DB, logger *zap.Logger) *ScheduledTaskRepository {
	return &ScheduledTaskRepository{db: db, logger: logger}
}

// ============================================================
// TASKS
// ============================================================

func (r *ScheduledTaskRepository) CreateTask(ctx context.Context, task *models.ScheduledTask) error {
	query := `INSERT INTO scheduled_tasks (id, tenant_id, name, description, task_type, command, script_content,
		http_endpoint, http_method, schedule, schedule_desc, timezone, is_enabled, priority, timeout, max_retries,
		retry_delay, tags, environment, notify_on_success, notify_on_failure, notify_emails, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,NOW(),NOW())
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		task.ID, task.TenantID, task.Name, task.Description, task.TaskType,
		task.Command, task.ScriptContent, task.HTTPEndpoint, task.HTTPMethod,
		task.Schedule, task.ScheduleDesc, task.Timezone, task.IsEnabled,
		task.Priority, task.Timeout, task.MaxRetries, task.RetryDelay,
		pq.Array(task.Tags), pq.Array(task.Environment),
		task.NotifyOnSuccess, task.NotifyOnFailure, pq.Array(task.NotifyEmails),
	).Scan(&task.CreatedAt, &task.UpdatedAt)
}

func (r *ScheduledTaskRepository) ListTasks(ctx context.Context, tenantID uuid.UUID, taskType string, enabled *bool) ([]models.ScheduledTask, error) {
	var tasks []models.ScheduledTask
	query := `SELECT * FROM scheduled_tasks WHERE tenant_id=$1`
	args := []interface{}{tenantID}

	if taskType != "" {
		query += ` AND task_type=$2`
		args = append(args, taskType)
	}
	if enabled != nil {
		query += fmt.Sprintf(` AND is_enabled=$%d`, len(args)+1)
		args = append(args, *enabled)
	}
	query += ` ORDER BY priority DESC, name ASC`

	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

func (r *ScheduledTaskRepository) GetTask(ctx context.Context, id uuid.UUID) (*models.ScheduledTask, error) {
	var task models.ScheduledTask
	if err := r.db.GetContext(ctx, &task, `SELECT * FROM scheduled_tasks WHERE id=$1`, id); err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	return &task, nil
}

func (r *ScheduledTaskRepository) UpdateTask(ctx context.Context, task *models.ScheduledTask) error {
	query := `UPDATE scheduled_tasks SET name=$1, description=$2, task_type=$3, command=$4, script_content=$5,
		http_endpoint=$6, http_method=$7, schedule=$8, schedule_desc=$9, timezone=$10, is_enabled=$11, priority=$12,
		timeout=$13, max_retries=$14, retry_delay=$15, tags=$16, environment=$17, notify_on_success=$18,
		notify_on_failure=$19, notify_emails=$20, updated_at=NOW() WHERE id=$21`
	_, err := r.db.ExecContext(ctx, query,
		task.Name, task.Description, task.TaskType, task.Command, task.ScriptContent,
		task.HTTPEndpoint, task.HTTPMethod, task.Schedule, task.ScheduleDesc, task.Timezone,
		task.IsEnabled, task.Priority, task.Timeout, task.MaxRetries, task.RetryDelay,
		pq.Array(task.Tags), pq.Array(task.Environment),
		task.NotifyOnSuccess, task.NotifyOnFailure, pq.Array(task.NotifyEmails), task.ID,
	)
	return err
}

func (r *ScheduledTaskRepository) DeleteTask(ctx context.Context, id uuid.UUID) error {
	_, _ = r.db.ExecContext(ctx, `DELETE FROM task_executions WHERE task_id=$1`, id)
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id=$1`, id)
	return err
}

func (r *ScheduledTaskRepository) ToggleTask(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE scheduled_tasks SET is_enabled = NOT is_enabled, updated_at=NOW() WHERE id=$1`, id)
	return err
}

func (r *ScheduledTaskRepository) UpdateTaskRunInfo(ctx context.Context, id uuid.UUID, status string, success bool) error {
	if success {
		_, err := r.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at=NOW(), last_status=$1, run_count=run_count+1, updated_at=NOW() WHERE id=$2`, status, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at=NOW(), last_status=$1, run_count=run_count+1, fail_count=fail_count+1, updated_at=NOW() WHERE id=$2`, status, id)
	return err
}

func (r *ScheduledTaskRepository) GetEnabledTasks(ctx context.Context) ([]models.ScheduledTask, error) {
	var tasks []models.ScheduledTask
	if err := r.db.SelectContext(ctx, &tasks, `SELECT * FROM scheduled_tasks WHERE is_enabled=true ORDER BY priority DESC`); err != nil {
		return nil, err
	}
	return tasks, nil
}

// ============================================================
// EXECUTIONS
// ============================================================

func (r *ScheduledTaskRepository) CreateExecution(ctx context.Context, exec *models.TaskExecution) error {
	query := `INSERT INTO task_executions (id, task_id, tenant_id, status, started_at, triggered_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())`
	_, err := r.db.ExecContext(ctx, query, exec.ID, exec.TaskID, exec.TenantID, exec.Status, exec.StartedAt, exec.TriggeredBy)
	return err
}

func (r *ScheduledTaskRepository) UpdateExecution(ctx context.Context, exec *models.TaskExecution) error {
	query := `UPDATE task_executions SET status=$1, finished_at=$2, duration=$3, exit_code=$4, output=$5, error_output=$6, retry_count=$7 WHERE id=$8`
	_, err := r.db.ExecContext(ctx, query, exec.Status, exec.FinishedAt, exec.Duration, exec.ExitCode, exec.Output, exec.ErrorOutput, exec.RetryCount, exec.ID)
	return err
}

func (r *ScheduledTaskRepository) ListExecutions(ctx context.Context, taskID uuid.UUID, limit int) ([]models.TaskExecution, error) {
	var execs []models.TaskExecution
	query := `SELECT * FROM task_executions WHERE task_id=$1 ORDER BY created_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &execs, query, taskID, limit); err != nil {
		return nil, err
	}
	return execs, nil
}

func (r *ScheduledTaskRepository) ListRecentExecutions(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TaskExecution, error) {
	var execs []models.TaskExecution
	query := `SELECT * FROM task_executions WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &execs, query, tenantID, limit); err != nil {
		return nil, err
	}
	return execs, nil
}

func (r *ScheduledTaskRepository) GetExecution(ctx context.Context, id uuid.UUID) (*models.TaskExecution, error) {
	var exec models.TaskExecution
	if err := r.db.GetContext(ctx, &exec, `SELECT * FROM task_executions WHERE id=$1`, id); err != nil {
		return nil, err
	}
	return &exec, nil
}

func (r *ScheduledTaskRepository) CleanupOldExecutions(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_executions WHERE tenant_id=$1 AND created_at < NOW() - INTERVAL '1 day' * $2`, tenantID, days)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ============================================================
// TEMPLATES
// ============================================================

func (r *ScheduledTaskRepository) CreateTemplate(ctx context.Context, tmpl *models.TaskTemplate) error {
	query := `INSERT INTO task_templates (id, tenant_id, name, description, task_type, command, script_content, schedule, tags, is_public, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW()) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		tmpl.ID, tmpl.TenantID, tmpl.Name, tmpl.Description, tmpl.TaskType,
		tmpl.Command, tmpl.ScriptContent, tmpl.Schedule, pq.Array(tmpl.Tags), tmpl.IsPublic,
	).Scan(&tmpl.CreatedAt)
}

func (r *ScheduledTaskRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.TaskTemplate, error) {
	var templates []models.TaskTemplate
	query := `SELECT * FROM task_templates WHERE tenant_id=$1 OR is_public=true ORDER BY name ASC`
	if err := r.db.SelectContext(ctx, &templates, query, tenantID); err != nil {
		return nil, err
	}
	return templates, nil
}

func (r *ScheduledTaskRepository) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_templates WHERE id=$1`, id)
	return err
}

// ============================================================
// GROUPS
// ============================================================

func (r *ScheduledTaskRepository) CreateGroup(ctx context.Context, group *models.TaskGroup) error {
	query := `INSERT INTO task_groups (id, tenant_id, name, description, color, tags, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW()) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		group.ID, group.TenantID, group.Name, group.Description, group.Color, pq.Array(group.Tags),
	).Scan(&group.CreatedAt)
}

func (r *ScheduledTaskRepository) ListGroups(ctx context.Context, tenantID uuid.UUID) ([]models.TaskGroup, error) {
	var groups []models.TaskGroup
	query := `SELECT * FROM task_groups WHERE tenant_id=$1 ORDER BY name ASC`
	if err := r.db.SelectContext(ctx, &groups, query, tenantID); err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *ScheduledTaskRepository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_groups WHERE id=$1`, id)
	return err
}

// ============================================================
// STATS
// ============================================================

func (r *ScheduledTaskRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.ScheduledTaskStats, error) {
	var stats models.ScheduledTaskStats
	err := r.db.GetContext(ctx, &stats, `
		SELECT
			(SELECT COUNT(*) FROM scheduled_tasks WHERE tenant_id=$1) AS total_tasks,
			(SELECT COUNT(*) FROM scheduled_tasks WHERE tenant_id=$1 AND is_enabled=true) AS enabled_tasks,
			(SELECT COUNT(*) FROM scheduled_tasks WHERE tenant_id=$1 AND is_enabled=false) AS disabled_tasks,
			(SELECT COUNT(*) FROM task_executions WHERE tenant_id=$1) AS total_executions,
			COALESCE((SELECT ROUND(COUNT(*) FILTER (WHERE status='success')::numeric / NULLIF(COUNT(*), 0) * 100, 1) FROM task_executions WHERE tenant_id=$1), 0) AS success_rate,
			(SELECT COUNT(*) FROM task_executions WHERE tenant_id=$1 AND status='failed' AND created_at > NOW() - INTERVAL '1 day') AS failed_today,
			(SELECT COUNT(*) FROM task_executions WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '1 day') AS run_today,
			COALESCE((SELECT AVG(duration) FROM task_executions WHERE tenant_id=$1 AND status='success'), 0) AS avg_duration
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get task stats: %w", err)
	}
	return &stats, nil
}
