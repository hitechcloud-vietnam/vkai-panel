package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type FileProtectionService struct {
	repo   *repository.FileProtectionRepository
	logger *zap.Logger
}

func NewFileProtectionService(repo *repository.FileProtectionRepository, logger *zap.Logger) *FileProtectionService {
	return &FileProtectionService{repo: repo, logger: logger}
}

func (s *FileProtectionService) CreateRule(ctx context.Context, tenantID uuid.UUID, req models.CreateProtectionRuleRequest) (*models.FileProtectionRule, error) {
	return s.repo.CreateRule(ctx, tenantID, req)
}

func (s *FileProtectionService) ListRules(ctx context.Context, tenantID uuid.UUID) ([]models.FileProtectionRule, error) {
	return s.repo.ListRules(ctx, tenantID)
}

func (s *FileProtectionService) GetRule(ctx context.Context, id uuid.UUID) (*models.FileProtectionRule, error) {
	return s.repo.GetRule(ctx, id)
}

func (s *FileProtectionService) UpdateRule(ctx context.Context, id uuid.UUID, req models.UpdateProtectionRuleRequest) (*models.FileProtectionRule, error) {
	return s.repo.UpdateRule(ctx, id, req)
}

func (s *FileProtectionService) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteRule(ctx, id)
}

func (s *FileProtectionService) ToggleRule(ctx context.Context, id uuid.UUID) (*models.FileProtectionRule, error) {
	return s.repo.ToggleRule(ctx, id)
}

func (s *FileProtectionService) ListChangeEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.FileChangeEvent, error) {
	return s.repo.ListChangeEvents(ctx, tenantID, limit)
}

func (s *FileProtectionService) MarkEventRead(ctx context.Context, id uuid.UUID) error {
	return s.repo.MarkEventRead(ctx, id)
}

func (s *FileProtectionService) MarkAllEventsRead(ctx context.Context, tenantID uuid.UUID) error {
	return s.repo.MarkAllEventsRead(ctx, tenantID)
}

func (s *FileProtectionService) ListQuarantineItems(ctx context.Context, tenantID uuid.UUID) ([]models.QuarantineItem, error) {
	return s.repo.ListQuarantineItems(ctx, tenantID)
}

func (s *FileProtectionService) RestoreQuarantineItem(ctx context.Context, id uuid.UUID) error {
	return s.repo.RestoreQuarantineItem(ctx, id)
}

func (s *FileProtectionService) DeleteQuarantineItem(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteQuarantineItem(ctx, id)
}

func (s *FileProtectionService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.FileProtectionStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}
