package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type ScheduledTaskService struct {
	repo   *repository.ScheduledTaskRepository
	logger *zap.Logger
}

func NewScheduledTaskService(repo *repository.ScheduledTaskRepository, logger *zap.Logger) *ScheduledTaskService {
	return &ScheduledTaskService{repo: repo, logger: logger}
}

// Tasks
func (s *ScheduledTaskService) CreateTask(ctx context.Context, tenantID uuid.UUID, req models.CreateScheduledTaskRequest) (*models.ScheduledTask, error) {
	task := &models.ScheduledTask{
		ID:              uuid.New(),
		TenantID:        tenantID,
		Name:            req.Name,
		Description:     req.Description,
		TaskType:        req.TaskType,
		Command:         req.Command,
		ScriptContent:   req.ScriptContent,
		HTTPEndpoint:    req.HTTPEndpoint,
		HTTPMethod:      req.HTTPMethod,
		Schedule:        req.Schedule,
		ScheduleDesc:    describeSchedule(req.Schedule),
		Timezone:        req.Timezone,
		IsEnabled:       true,
		Priority:        req.Priority,
		Timeout:         req.Timeout,
		MaxRetries:      req.MaxRetries,
		RetryDelay:      req.RetryDelay,
		Tags:            pq.StringArray(req.Tags),
		Environment:     pq.StringArray(req.Environment),
		NotifyOnSuccess: req.NotifyOnSuccess,
		NotifyOnFailure: req.NotifyOnFailure,
		NotifyEmails:    pq.StringArray(req.NotifyEmails),
	}
	if task.Priority == 0 {
		task.Priority = 2
	}
	if task.Timezone == "" {
		task.Timezone = "UTC"
	}
	if task.HTTPMethod == "" {
		task.HTTPMethod = "GET"
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ScheduledTaskService) ListTasks(ctx context.Context, tenantID uuid.UUID, taskType string, enabled *bool) ([]models.ScheduledTask, error) {
	return s.repo.ListTasks(ctx, tenantID, taskType, enabled)
}

func (s *ScheduledTaskService) GetTask(ctx context.Context, id uuid.UUID) (*models.ScheduledTask, error) {
	return s.repo.GetTask(ctx, id)
}

func (s *ScheduledTaskService) UpdateTask(ctx context.Context, id uuid.UUID, req models.UpdateScheduledTaskRequest) (*models.ScheduledTask, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.TaskType != nil {
		task.TaskType = *req.TaskType
	}
	if req.Command != nil {
		task.Command = *req.Command
	}
	if req.ScriptContent != nil {
		task.ScriptContent = *req.ScriptContent
	}
	if req.HTTPEndpoint != nil {
		task.HTTPEndpoint = *req.HTTPEndpoint
	}
	if req.HTTPMethod != nil {
		task.HTTPMethod = *req.HTTPMethod
	}
	if req.Schedule != nil {
		task.Schedule = *req.Schedule
		task.ScheduleDesc = describeSchedule(*req.Schedule)
	}
	if req.Timezone != nil {
		task.Timezone = *req.Timezone
	}
	if req.IsEnabled != nil {
		task.IsEnabled = *req.IsEnabled
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.Timeout != nil {
		task.Timeout = *req.Timeout
	}
	if req.MaxRetries != nil {
		task.MaxRetries = *req.MaxRetries
	}
	if req.RetryDelay != nil {
		task.RetryDelay = *req.RetryDelay
	}
	if req.Tags != nil {
		task.Tags = pq.StringArray(req.Tags)
	}
	if req.Environment != nil {
		task.Environment = pq.StringArray(req.Environment)
	}
	if req.NotifyOnSuccess != nil {
		task.NotifyOnSuccess = *req.NotifyOnSuccess
	}
	if req.NotifyOnFailure != nil {
		task.NotifyOnFailure = *req.NotifyOnFailure
	}
	if req.NotifyEmails != nil {
		task.NotifyEmails = pq.StringArray(req.NotifyEmails)
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ScheduledTaskService) DeleteTask(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTask(ctx, id)
}

func (s *ScheduledTaskService) ToggleTask(ctx context.Context, id uuid.UUID) error {
	return s.repo.ToggleTask(ctx, id)
}

func (s *ScheduledTaskService) RunTask(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.TaskExecution, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	exec := &models.TaskExecution{
		ID:          uuid.New(),
		TaskID:      task.ID,
		TenantID:    tenantID,
		Status:      "running",
		StartedAt:   timePtr(time.Now()),
		TriggeredBy: "manual",
	}

	if err := s.repo.CreateExecution(ctx, exec); err != nil {
		return nil, err
	}

	// Simulate execution (in real implementation, this would execute the command/script/HTTP request)
	go func() {
		time.Sleep(100 * time.Millisecond)
		now := time.Now()
		exec.FinishedAt = &now
		exec.Duration = 100
		exec.Status = "success"
		exec.Output = fmt.Sprintf("Task '%s' executed successfully", task.Name)
		exitCode := 0
		exec.ExitCode = &exitCode
		_ = s.repo.UpdateExecution(ctx, exec)
		_ = s.repo.UpdateTaskRunInfo(ctx, task.ID, "success", true)
	}()

	return exec, nil
}

// Executions
func (s *ScheduledTaskService) ListExecutions(ctx context.Context, taskID uuid.UUID, limit int) ([]models.TaskExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListExecutions(ctx, taskID, limit)
}

func (s *ScheduledTaskService) ListRecentExecutions(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TaskExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListRecentExecutions(ctx, tenantID, limit)
}

func (s *ScheduledTaskService) GetExecution(ctx context.Context, id uuid.UUID) (*models.TaskExecution, error) {
	return s.repo.GetExecution(ctx, id)
}

func (s *ScheduledTaskService) CleanupOldExecutions(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	if days <= 0 {
		days = 30
	}
	return s.repo.CleanupOldExecutions(ctx, tenantID, days)
}

// Templates
func (s *ScheduledTaskService) CreateTemplate(ctx context.Context, tenantID uuid.UUID, req models.CreateTaskTemplateRequest) (*models.TaskTemplate, error) {
	tmpl := &models.TaskTemplate{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Name:          req.Name,
		Description:   req.Description,
		TaskType:      req.TaskType,
		Command:       req.Command,
		ScriptContent: req.ScriptContent,
		Schedule:      req.Schedule,
		Tags:          pq.StringArray(req.Tags),
		IsPublic:      req.IsPublic,
	}
	if err := s.repo.CreateTemplate(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func (s *ScheduledTaskService) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.TaskTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID)
}

func (s *ScheduledTaskService) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, id)
}

// Groups
func (s *ScheduledTaskService) CreateGroup(ctx context.Context, tenantID uuid.UUID, req models.CreateTaskGroupRequest) (*models.TaskGroup, error) {
	group := &models.TaskGroup{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		Tags:        pq.StringArray(req.Tags),
	}
	if group.Color == "" {
		group.Color = "#3B82F6"
	}
	if err := s.repo.CreateGroup(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *ScheduledTaskService) ListGroups(ctx context.Context, tenantID uuid.UUID) ([]models.TaskGroup, error) {
	return s.repo.ListGroups(ctx, tenantID)
}

func (s *ScheduledTaskService) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteGroup(ctx, id)
}

// Stats
func (s *ScheduledTaskService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.ScheduledTaskStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}

func timePtr(t time.Time) *time.Time { return &t }

func describeSchedule(cron string) string {
	switch cron {
	case "* * * * *":
		return "Every minute"
	case "0 * * * *":
		return "Every hour"
	case "0 0 * * *":
		return "Every day at midnight"
	case "0 0 * * 0":
		return "Every Sunday at midnight"
	case "0 0 1 * *":
		return "1st of every month at midnight"
	default:
		return cron
	}
}
