package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type BackupService struct {
	backupRepo *repository.BackupRepository
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

	if job.Destination == "" {
		job.Destination = "/var/backups/vkai"
	}

	if err := s.backupRepo.CreateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create backup job: %w", err)
	}

	// Create destination directory
	os.MkdirAll(job.Destination, 0700)

	return job, nil
}

func (s *BackupService) GetJobByID(ctx context.Context, id uuid.UUID) (*models.BackupJob, error) {
	return s.backupRepo.GetJobByID(ctx, id)
}

func (s *BackupService) ListJobsByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.BackupJob, error) {
	return s.backupRepo.ListJobsByTenant(ctx, tenantID)
}

func (s *BackupService) UpdateJob(ctx context.Context, job *models.BackupJob) error {
	return s.backupRepo.UpdateJob(ctx, job)
}

func (s *BackupService) DeleteJob(ctx context.Context, id uuid.UUID) error {
	return s.backupRepo.DeleteJob(ctx, id)
}

func (s *BackupService) RunBackup(ctx context.Context, jobID uuid.UUID) (*models.BackupRecord, error) {
	job, err := s.backupRepo.GetJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

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

	cmd := exec.CommandContext(ctx, "tar", "-czf", backupPath, "-C", "/var/www", job.ResourceID.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar failed: %s: %w", string(output), err)
	}

	return backupPath, nil
}

func (s *BackupService) backupDatabase(ctx context.Context, job *models.BackupJob) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("database-%s-%s.sql.gz", job.ResourceID.String()[:8], timestamp)
	backupPath := filepath.Join(job.Destination, filename)

	// Use pg_dump or mysqldump based on database type
	// For now, default to pg_dump
	cmd := exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf("pg_dump -U postgres %s | gzip > %s", job.ResourceID.String(), backupPath))
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("database backup failed: %s: %w", string(output), err)
	}

	return backupPath, nil
}

func (s *BackupService) backupFiles(ctx context.Context, job *models.BackupJob) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("files-%s-%s.tar.gz", job.ResourceID.String()[:8], timestamp)
	backupPath := filepath.Join(job.Destination, filename)

	cmd := exec.CommandContext(ctx, "tar", "-czf", backupPath, job.ResourceID.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("file backup failed: %s: %w", string(output), err)
	}

	return backupPath, nil
}

func (s *BackupService) RestoreBackup(ctx context.Context, recordID uuid.UUID) error {
	record, err := s.backupRepo.GetJobByID(ctx, recordID)
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

func (s *BackupService) DeleteRecord(ctx context.Context, id uuid.UUID) error {
	return s.backupRepo.DeleteRecord(ctx, id)
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
				// Delete the backup file
				os.Remove(rec.Path)
				_ = s.backupRepo.DeleteRecord(ctx, rec.ID)
			}
		}
	}

	return nil
}
