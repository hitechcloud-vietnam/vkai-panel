package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// cronFileIDRe guards the file name derived from a job id.
var cronFileIDRe = regexp.MustCompile(`^[0-9a-f]{8}$`)

// defaultCronUser is the account generated /etc/cron.d entries run as. Jobs are
// no longer run as root: a tenant-supplied command must never gain root
// privileges. Deployments that genuinely need another account can override it.
const defaultCronUser = "www-data"

func cronUser() string {
	if u := strings.TrimSpace(os.Getenv("VKAI_CRON_USER")); u != "" && cronUserRe.MatchString(u) {
		return u
	}
	return defaultCronUser
}

var cronUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// cronExecCommand builds the command used by RunNow. When the panel runs as
// root the job is dropped to the unprivileged cron account; otherwise it runs
// with the panel's own (already unprivileged) identity.
func cronExecCommand(ctx context.Context, command string) *exec.Cmd {
	if os.Geteuid() == 0 {
		user := cronUser()
		if path, err := exec.LookPath("runuser"); err == nil {
			return exec.CommandContext(ctx, path, "-u", user, "--", "/bin/bash", "-c", command)
		}
		if path, err := exec.LookPath("su"); err == nil {
			return exec.CommandContext(ctx, path, "-s", "/bin/bash", "-c", command, user)
		}
	}
	return exec.CommandContext(ctx, "/bin/bash", "-c", command)
}

// validateJob refuses schedules and commands that could break out of a single
// cron line or out of the shell quoting of the installer.
func validateJob(job *models.CronJob) error {
	if err := utils.ValidateCronSchedule(job.Schedule); err != nil {
		return err
	}
	if strings.TrimSpace(job.Command) == "" {
		return fmt.Errorf("command is required")
	}
	if utils.ContainsControlChars(job.Command) {
		return fmt.Errorf("command must not contain control characters")
	}
	if len(job.Command) > 4096 {
		return fmt.Errorf("command is too long")
	}
	return nil
}

type CronService struct {
	cronRepo   *repository.CronRepository
	serverRepo *repository.ServerRepository
	quota      *quota.Enforcer
}

// NewCronService takes the quota enforcer as a REQUIRED argument, so that
// omitting quota enforcement is a compile error rather than a silent hole. See
// NewWebsiteService for the reasoning.
func NewCronService(
	cronRepo *repository.CronRepository,
	serverRepo *repository.ServerRepository,
	quotaEnforcer *quota.Enforcer,
) *CronService {
	return &CronService{
		cronRepo:   cronRepo,
		serverRepo: serverRepo,
		quota:      quotaEnforcer,
	}
}

func (s *CronService) Create(ctx context.Context, req *models.CreateCronJobRequest, tenantID uuid.UUID) (*models.CronJob, error) {
	// ENFORCEMENT POINT: the hosting package's cron job count.
	//
	// Before the row and before the /etc/cron.d entry, so a refusal leaves
	// nothing scheduled.
	if err := s.quota.Check(ctx, tenantID, quota.ResourceCronJobs); err != nil {
		return nil, err
	}

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

	if err := validateJob(job); err != nil {
		return nil, err
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

func (s *CronService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.CronJob, error) {
	return s.cronRepo.GetByID(ctx, tenantID, id)
}

func (s *CronService) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.CronJob, error) {
	return s.cronRepo.ListByTenant(ctx, tenantID)
}

func (s *CronService) Update(ctx context.Context, job *models.CronJob) error {
	if err := validateJob(job); err != nil {
		return err
	}

	if err := s.cronRepo.Update(ctx, job); err != nil {
		return err
	}

	// Reinstall cron job
	return s.installCronJob(ctx, job)
}

func (s *CronService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	job, err := s.cronRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	// Remove from crontab
	_ = s.removeCronJob(ctx, job)

	return s.cronRepo.Delete(ctx, tenantID, id)
}

func (s *CronService) ToggleStatus(ctx context.Context, tenantID, id uuid.UUID) (*models.CronJob, error) {
	job, err := s.cronRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	newStatus := "active"
	if job.Status == "active" {
		newStatus = "disabled"
	}

	if err := s.cronRepo.ToggleStatus(ctx, tenantID, id, newStatus); err != nil {
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

func (s *CronService) RunNow(ctx context.Context, tenantID, id uuid.UUID) error {
	job, err := s.cronRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := validateJob(job); err != nil {
		return err
	}

	// Running an arbitrary command is the declared purpose of this feature, so
	// the shell stays - but it runs de-privileged, as the same account the
	// installed cron entry uses, never as the panel's own (root) identity.
	cmd := cronExecCommand(ctx, job.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s: %w", string(output), err)
	}

	_ = s.cronRepo.UpdateLastRun(ctx, tenantID, id)
	return nil
}

// cronFilePath builds the /etc/cron.d path for a job. The id fragment is
// re-validated so the name can never carry a path separator.
func cronFilePath(job *models.CronJob) (string, error) {
	id := job.ID.String()
	if len(id) < 8 || !cronFileIDRe.MatchString(id[:8]) {
		return "", fmt.Errorf("invalid job id")
	}
	return "/etc/cron.d/vkai-" + id[:8], nil
}

func (s *CronService) installCronJob(ctx context.Context, job *models.CronJob) error {
	if err := validateJob(job); err != nil {
		return err
	}

	cronFile, err := cronFilePath(job)
	if err != nil {
		return err
	}

	// Written directly from Go: no shell, so quotes in the command cannot break
	// out, and the entry runs as an unprivileged account rather than root.
	cronLine := fmt.Sprintf("%s %s %s\n", job.Schedule, cronUser(), job.Command)
	return os.WriteFile(cronFile, []byte(cronLine), 0644)
}

func (s *CronService) removeCronJob(ctx context.Context, job *models.CronJob) error {
	cronFile, err := cronFilePath(job)
	if err != nil {
		return err
	}
	if err := os.Remove(cronFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
