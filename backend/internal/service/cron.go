package service

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type CronService struct {
	cronRepo   *repository.CronRepository
	serverRepo *repository.ServerRepository
}

func NewCronService(cronRepo *repository.CronRepository, serverRepo *repository.ServerRepository) *CronService {
	return &CronService{
		cronRepo:   cronRepo,
		serverRepo: serverRepo,
	}
}

func (s *CronService) Create(ctx context.Context, req *models.CreateCronJobRequest, tenantID uuid.UUID) (*models.CronJob, error) {
	job := &models.CronJob{
		TenantID: tenantID,
		ServerID: req.ServerID,
		Name:     req.Name,
		Command:  req.Command,
		Schedule: req.Schedule,
		Type:     req.Type,
		Status:   "active",
	}

	if job.Type == "" {
		job.Type = "standard"
	}

	if err := s.cronRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create cron job: %w", err)
	}

	// Install cron job on server
	if err := s.installCronJob(ctx, job); err != nil {
		fmt.Printf("Warning: failed to install cron job on server: %v\n", err)
	}

	return job, nil
}

func (s *CronService) GetByID(ctx context.Context, id uuid.UUID) (*models.CronJob, error) {
	return s.cronRepo.GetByID(ctx, id)
}

func (s *CronService) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.CronJob, error) {
	return s.cronRepo.ListByTenant(ctx, tenantID)
}

func (s *CronService) Update(ctx context.Context, job *models.CronJob) error {
	if err := s.cronRepo.Update(ctx, job); err != nil {
		return err
	}

	// Reinstall cron job
	return s.installCronJob(ctx, job)
}

func (s *CronService) Delete(ctx context.Context, id uuid.UUID) error {
	job, err := s.cronRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Remove from crontab
	_ = s.removeCronJob(ctx, job)

	return s.cronRepo.Delete(ctx, id)
}

func (s *CronService) ToggleStatus(ctx context.Context, id uuid.UUID) (*models.CronJob, error) {
	job, err := s.cronRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	newStatus := "active"
	if job.Status == "active" {
		newStatus = "disabled"
	}

	if err := s.cronRepo.ToggleStatus(ctx, id, newStatus); err != nil {
		return nil, err
	}

	if newStatus == "active" {
		_ = s.installCronJob(ctx, job)
	} else {
		_ = s.removeCronJob(ctx, job)
	}

	job.Status = newStatus
	return job, nil
}

func (s *CronService) RunNow(ctx context.Context, id uuid.UUID) error {
	job, err := s.cronRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Execute the command
	cmd := exec.CommandContext(ctx, "bash", "-c", job.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s: %w", string(output), err)
	}

	_ = s.cronRepo.UpdateLastRun(ctx, id)
	return nil
}

func (s *CronService) installCronJob(ctx context.Context, job *models.CronJob) error {
	// Write cron entry to /etc/cron.d/
	cronLine := fmt.Sprintf("%s root %s\n", job.Schedule, job.Command)
	cronFile := fmt.Sprintf("/etc/cron.d/vkai-%s", job.ID.String()[:8])

	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("echo '%s' > %s", cronLine, cronFile))
	return cmd.Run()
}

func (s *CronService) removeCronJob(ctx context.Context, job *models.CronJob) error {
	cronFile := fmt.Sprintf("/etc/cron.d/vkai-%s", job.ID.String()[:8])
	cmd := exec.CommandContext(ctx, "rm", "-f", cronFile)
	return cmd.Run()
}
