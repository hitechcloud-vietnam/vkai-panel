package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type AuditService struct {
	repo   *repository.AuditRepository
	logger *zap.Logger
}

func NewAuditService(repo *repository.AuditRepository, logger *zap.Logger) *AuditService {
	return &AuditService{
		repo:   repo,
		logger: logger,
	}
}

func (s *AuditService) Log(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, action, resource string, resourceID *uuid.UUID, details models.JSONMap, ipAddress, userAgent, status string) error {
	log := &models.AuditLog{
		TenantID:   tenantID,
		UserID:     userID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Status:     status,
	}
	return s.repo.Create(ctx, log)
}

func (s *AuditService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.AuditLog, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *AuditService) Search(ctx context.Context, tenantID uuid.UUID, req *models.AuditLogSearchRequest) ([]models.AuditLog, int, error) {
	return s.repo.Search(ctx, tenantID, req)
}

func (s *AuditService) GetStats(ctx context.Context, tenantID uuid.UUID, days int) (*models.AuditLogStats, error) {
	return s.repo.GetStats(ctx, tenantID, days)
}

func (s *AuditService) CleanupOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	return s.repo.DeleteOld(ctx, tenantID, days)
}
