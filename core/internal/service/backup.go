package service

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// backupRoot is the only tree backups may be written to: /vkai-panel/www/backup
// unless VKAI_BACKUP_ROOT points at a dedicated volume. Everything a caller
// supplies as a destination must resolve inside it.
func backupRoot() string {
	return config.BackupRoot()
}

// validateDestination makes sure a backup destination is a plain absolute path
// inside the backup root. It is the single gate that keeps shell metacharacters
// and traversal out of every path we later hand to an external tool.
func validateDestination(dest string) (string, error) {
	if strings.TrimSpace(dest) == "" {
		return backupRoot(), nil
	}
	if err := utils.ValidateAbsolutePath(dest, "destination"); err != nil {
		return "", err
	}
	clean, err := utils.EnsureWithinRoot(backupRoot(), dest)
	if err != nil {
		return "", err
	}
	return clean, nil
}

type BackupService struct {
	backupRepo *repository.BackupRepository

	// The offsite half of this service - destinations, encryption, restore and
	// the restorability check - keeps its state here. It is behind a sync.Once
	// so that NewBackupService keeps its single-argument signature and
	// cmd/api/main.go needs no change to gain any of it; see backup_offsite.go.
	offsiteOnce  sync.Once
	offsiteState *offsiteState
}

func NewBackupService(backupRepo *repository.BackupRepository) *BackupService {
	return &BackupService{
		backupRepo: backupRepo,
	}
}

func (s *BackupService) CreateJob(ctx context.Context, req *models.CreateBackupJobRequest, tenantID uuid.UUID) (*models.BackupJob, error) {
	job := &models.BackupJob{
		TenantID:    tenantID,
		Name:        req.Name,
		Type:        req.Type,
		ResourceID:  req.ResourceID,
		Destination: req.Destination,
		Schedule:    req.Schedule,
		Retention:   req.Retention,
		Encrypted:   req.Encrypted,
		Status:      "active",
	}

	if job.Retention == 0 {
		job.Retention = 30 // Default 30 days
	}

	dest, err := validateDestination(job.Destination)
	if err != nil {
		return nil, err
	}
	job.Destination = dest

	if err := s.backupRepo.CreateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create backup job: %w", err)
	}

	// Create destination directory
	os.MkdirAll(job.Destination, 0700)

	return job, nil
}

func (s *BackupService) GetJobByID(ctx context.Context, tenantID, id uuid.UUID) (*models.BackupJob, error) {
	return s.backupRepo.GetJobByID(ctx, tenantID, id)
}

func (s *BackupService) ListJobsByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.BackupJob, error) {
	return s.backupRepo.ListJobsByTenant(ctx, tenantID)
}

func (s *BackupService) UpdateJob(ctx context.Context, job *models.BackupJob) error {
	dest, err := validateDestination(job.Destination)
	if err != nil {
		return err
	}
	job.Destination = dest
	return s.backupRepo.UpdateJob(ctx, job)
}

func (s *BackupService) DeleteJob(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.backupRepo.DeleteJob(ctx, tenantID, id)
}

func (s *BackupService) RunBackup(ctx context.Context, tenantID, jobID uuid.UUID) (*models.BackupRecord, error) {
	job, err := s.backupRepo.GetJobByID(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}

	// A job row may predate the destination validation, so re-check it before
	// any path from it reaches an external command.
	dest, err := validateDestination(job.Destination)
	if err != nil {
		return nil, err
	}
	job.Destination = dest

	record := &models.BackupRecord{
		JobID:    jobID,
		TenantID: job.TenantID,
		Status:   "running",
	}

	if err := s.backupRepo.CreateRecord(ctx, record); err != nil {
		return nil, err
	}

	// Execute backup based on type
	var backupPath string
	var backupErr error

	switch job.Type {
	case "website":
		backupPath, backupErr = s.backupWebsite(ctx, job)
	case "database":
		backupPath, backupErr = s.backupDatabase(ctx, job)
	case "files":
		backupPath, backupErr = s.backupFiles(ctx, job)
	default:
		backupErr = fmt.Errorf("unsupported backup type: %s", job.Type)
	}

	if backupErr != nil {
		record.Status = "failed"
		record.ErrorMsg = backupErr.Error()
		now := time.Now()
		record.CompletedAt = &now
		_ = s.backupRepo.UpdateRecord(ctx, record)
		return record, backupErr
	}

	// Get file size
	if info, err := os.Stat(backupPath); err == nil {
		record.Size = info.Size()
	}

	record.Path = backupPath
	record.Status = "completed"
	now := time.Now()
	record.CompletedAt = &now
	if err := s.backupRepo.UpdateRecord(ctx, record); err != nil {
		return nil, err
	}

	return record, nil
}

func (s *BackupService) backupWebsite(ctx context.Context, job *models.BackupJob) (string, error) {
	// Create tar.gz of website directory
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("website-%s-%s.tar.gz", job.ResourceID.String()[:8], timestamp)
	backupPath := filepath.Join(job.Destination, filename)

	if err := utils.ValidateCommandArg(backupPath, "backup path"); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "tar", "-czf", backupPath, "-C", config.WebRoot(), "--", job.ResourceID.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar failed: %s: %w", string(output), err)
	}

	return backupPath, nil
}

func (s *BackupService) backupDatabase(ctx context.Context, job *models.BackupJob) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("database-%s-%s.sql.gz", job.ResourceID.String()[:8], timestamp)
	backupPath := filepath.Join(job.Destination, filename)

	// The gzip pipeline is built in Go: no shell, so nothing in the destination
	// or the database name can ever be interpreted as a command.
	dbName := job.ResourceID.String()
	if err := utils.ValidateSQLIdentifierOrUUID(dbName, "database name"); err != nil {
		return "", err
	}

	f, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "pg_dump", "-U", "postgres", "--", dbName)
	cmd.Stdout = gz
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = gz.Close()
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("database backup failed: %s: %w", stderr.String(), err)
	}
	if err := gz.Close(); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("failed to finalise backup: %w", err)
	}

	return backupPath, nil
}

func (s *BackupService) backupFiles(ctx context.Context, job *models.BackupJob) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("files-%s-%s.tar.gz", job.ResourceID.String()[:8], timestamp)
	backupPath := filepath.Join(job.Destination, filename)

	if err := utils.ValidateCommandArg(backupPath, "backup path"); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "tar", "-czf", backupPath, "--", job.ResourceID.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("file backup failed: %s: %w", string(output), err)
	}

	return backupPath, nil
}

func (s *BackupService) RestoreBackup(ctx context.Context, tenantID, recordID uuid.UUID) error {
	record, err := s.backupRepo.GetJobByID(ctx, tenantID, recordID)
	if err != nil {
		return fmt.Errorf("backup record not found: %w", err)
	}
	_ = record
	// Restore logic based on backup type
	// This would extract the backup and restore to original location
	return fmt.Errorf("restore not yet implemented")
}

func (s *BackupService) ListRecordsByJob(ctx context.Context, jobID uuid.UUID) ([]models.BackupRecord, error) {
	return s.backupRepo.ListRecordsByJob(ctx, jobID)
}

func (s *BackupService) ListRecordsByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.BackupRecord, error) {
	return s.backupRepo.ListRecordsByTenant(ctx, tenantID, limit)
}

func (s *BackupService) DeleteRecord(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.backupRepo.DeleteRecord(ctx, tenantID, id)
}

func (s *BackupService) CleanupOldBackups(ctx context.Context, tenantID uuid.UUID) error {
	jobs, err := s.backupRepo.ListJobsByTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		records, err := s.backupRepo.ListRecordsByJob(ctx, job.ID)
		if err != nil {
			continue
		}

		cutoff := time.Now().AddDate(0, 0, -job.Retention)
		for _, rec := range records {
			if rec.StartedAt.Before(cutoff) && rec.Status == "completed" {
				// Only remove files that still sit inside the backup root.
				if _, err := utils.EnsureWithinRoot(backupRoot(), rec.Path); err == nil {
					os.Remove(rec.Path)
				}
				_ = s.backupRepo.DeleteRecord(ctx, tenantID, rec.ID)
			}
		}
	}

	return nil
}
