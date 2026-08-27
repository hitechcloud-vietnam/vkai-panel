package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type LogService struct {
	repo   *repository.LogRepository
	logger *zap.Logger
}

func NewLogService(repo *repository.LogRepository, logger *zap.Logger) *LogService {
	return &LogService{
		repo:   repo,
		logger: logger,
	}
}

// Log entries
func (s *LogService) RecordEntry(ctx context.Context, tenantID, serverID uuid.UUID, source, level, message string, details models.JSONMap) error {
	entry := &models.LogEntry{
		ServerID:  serverID,
		TenantID:  tenantID,
		Source:    source,
		Level:     level,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}
	return s.repo.CreateEntry(ctx, entry)
}

func (s *LogService) SearchEntries(ctx context.Context, tenantID uuid.UUID, req *models.LogSearchRequest) ([]models.LogEntry, int, error) {
	return s.repo.SearchEntries(ctx, tenantID, req)
}

func (s *LogService) CleanupOldEntries(ctx context.Context, tenantID uuid.UUID, retentionDays int) (int64, error) {
	before := time.Now().AddDate(0, 0, -retentionDays)
	return s.repo.DeleteOldEntries(ctx, tenantID, before)
}

// Log sources
func (s *LogService) CreateSource(ctx context.Context, tenantID uuid.UUID, req *models.CreateLogSourceRequest) (*models.LogSource, error) {
	source := &models.LogSource{
		TenantID: tenantID,
		ServerID: req.ServerID,
		Name:     req.Name,
		Type:     req.Type,
		Path:     req.Path,
		Format:   req.Format,
		IsActive: true,
	}
	if err := s.repo.CreateSource(ctx, source); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *LogService) GetSourceByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LogSource, error) {
	return s.repo.GetSourceByID(ctx, tenantID, id)
}

func (s *LogService) ListSources(ctx context.Context, tenantID uuid.UUID, serverID *uuid.UUID) ([]models.LogSource, error) {
	return s.repo.ListSources(ctx, tenantID, serverID)
}

func (s *LogService) UpdateSource(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLogSourceRequest) (*models.LogSource, error) {
	return s.repo.UpdateSource(ctx, tenantID, id, req)
}

func (s *LogService) DeleteSource(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteSource(ctx, tenantID, id)
}

// Log rotation
func (s *LogService) CreateRotation(ctx context.Context, tenantID uuid.UUID, req *models.CreateLogRotationRequest) (*models.LogRotation, error) {
	rotation := &models.LogRotation{
		TenantID:    tenantID,
		ServerID:    req.ServerID,
		Source:      req.Source,
		MaxSizeMB:   req.MaxSizeMB,
		MaxAgeDays:  req.MaxAgeDays,
		MaxFiles:    req.MaxFiles,
		CompressOld: req.CompressOld,
		IsActive:    true,
	}
	if err := s.repo.CreateRotation(ctx, rotation); err != nil {
		return nil, err
	}
	return rotation, nil
}

func (s *LogService) GetRotationByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LogRotation, error) {
	return s.repo.GetRotationByID(ctx, tenantID, id)
}

func (s *LogService) ListRotations(ctx context.Context, tenantID uuid.UUID, serverID *uuid.UUID) ([]models.LogRotation, error) {
	return s.repo.ListRotations(ctx, tenantID, serverID)
}

func (s *LogService) UpdateRotation(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLogRotationRequest) (*models.LogRotation, error) {
	return s.repo.UpdateRotation(ctx, tenantID, id, req)
}

func (s *LogService) DeleteRotation(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteRotation(ctx, tenantID, id)
}
