package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type NotificationService struct {
	repo   *repository.NotificationRepository
	logger *zap.Logger
}

func NewNotificationService(repo *repository.NotificationRepository, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		repo:   repo,
		logger: logger,
	}
}

// Notifications
func (s *NotificationService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationRequest) (*models.Notification, error) {
	notification := &models.Notification{
		TenantID: tenantID,
		UserID:   req.UserID,
		Type:     req.Type,
		Title:    req.Title,
		Message:  req.Message,
		Details:  req.Details,
		IsRead:   false,
	}
	if err := s.repo.Create(ctx, notification); err != nil {
		return nil, err
	}
	return notification, nil
}

func (s *NotificationService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Notification, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *NotificationService) List(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, isRead *bool, limit, offset int) ([]models.Notification, int, error) {
	return s.repo.List(ctx, tenantID, userID, isRead, limit, offset)
}

func (s *NotificationService) MarkAsRead(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.MarkAsRead(ctx, tenantID, id)
}

func (s *NotificationService) MarkAllAsRead(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.repo.MarkAllAsRead(ctx, tenantID, userID)
}

func (s *NotificationService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *NotificationService) CleanupOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	return s.repo.DeleteOld(ctx, tenantID, days)
}

// Templates
func (s *NotificationService) CreateTemplate(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationTemplateRequest) (*models.NotificationTemplate, error) {
	template := &models.NotificationTemplate{
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Subject:  req.Subject,
		Body:     req.Body,
		IsActive: true,
	}
	if err := s.repo.CreateTemplate(ctx, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *NotificationService) GetTemplateByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationTemplate, error) {
	return s.repo.GetTemplateByID(ctx, tenantID, id)
}

func (s *NotificationService) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID)
}

func (s *NotificationService) UpdateTemplate(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationTemplateRequest) (*models.NotificationTemplate, error) {
	return s.repo.UpdateTemplate(ctx, tenantID, id, req)
}

func (s *NotificationService) DeleteTemplate(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, tenantID, id)
}

// Channels
func (s *NotificationService) CreateChannel(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationChannelRequest) (*models.NotificationChannel, error) {
	channel := &models.NotificationChannel{
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Config:   req.Config,
		IsActive: true,
	}
	if err := s.repo.CreateChannel(ctx, channel); err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *NotificationService) GetChannelByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationChannel, error) {
	return s.repo.GetChannelByID(ctx, tenantID, id)
}

func (s *NotificationService) ListChannels(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationChannel, error) {
	return s.repo.ListChannels(ctx, tenantID)
}

func (s *NotificationService) UpdateChannel(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationChannelRequest) (*models.NotificationChannel, error) {
	return s.repo.UpdateChannel(ctx, tenantID, id, req)
}

func (s *NotificationService) DeleteChannel(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteChannel(ctx, tenantID, id)
}

// Preferences
func (s *NotificationService) GetPreferences(ctx context.Context, tenantID, userID uuid.UUID) ([]models.NotificationPreference, error) {
	return s.repo.GetPreferences(ctx, tenantID, userID)
}

func (s *NotificationService) SetPreference(ctx context.Context, tenantID, userID uuid.UUID, prefType, channel string, enabled bool) error {
	return s.repo.SetPreference(ctx, tenantID, userID, prefType, channel, enabled)
}
