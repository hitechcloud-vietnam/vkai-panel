package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"go.uber.org/zap"
)

type EmailMarketingService struct {
	repo   *repository.EmailMarketingRepository
	logger *zap.Logger
}

func NewEmailMarketingService(repo *repository.EmailMarketingRepository, logger *zap.Logger) *EmailMarketingService {
	return &EmailMarketingService{repo: repo, logger: logger}
}

// Campaigns
func (s *EmailMarketingService) CreateCampaign(ctx context.Context, c *models.EmailCampaign) error {
	if c.Status == "" {
		c.Status = "draft"
	}
	return s.repo.CreateCampaign(ctx, c)
}

func (s *EmailMarketingService) ListCampaigns(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.EmailCampaign, int, error) {
	return s.repo.ListCampaigns(ctx, tenantID, limit, offset)
}

func (s *EmailMarketingService) GetCampaign(ctx context.Context, tenantID, id uuid.UUID) (*models.EmailCampaign, error) {
	return s.repo.GetCampaign(ctx, tenantID, id)
}

func (s *EmailMarketingService) UpdateCampaign(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateCampaignRequest) error {
	return s.repo.UpdateCampaign(ctx, tenantID, id, req)
}

func (s *EmailMarketingService) DeleteCampaign(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteCampaign(ctx, tenantID, id)
}

func (s *EmailMarketingService) SendCampaign(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.UpdateCampaignStatus(ctx, tenantID, id, "sending")
}

func (s *EmailMarketingService) PauseCampaign(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.UpdateCampaignStatus(ctx, tenantID, id, "paused")
}

// Contacts
func (s *EmailMarketingService) CreateContact(ctx context.Context, c *models.EmailContact) error {
	return s.repo.CreateContact(ctx, c)
}

func (s *EmailMarketingService) ListContacts(ctx context.Context, tenantID uuid.UUID, limit, offset int, search string) ([]models.EmailContact, int, error) {
	return s.repo.ListContacts(ctx, tenantID, limit, offset, search)
}

func (s *EmailMarketingService) DeleteContact(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteContact(ctx, tenantID, id)
}

// Lists
func (s *EmailMarketingService) CreateList(ctx context.Context, l *models.EmailList) error {
	return s.repo.CreateList(ctx, l)
}

func (s *EmailMarketingService) ListLists(ctx context.Context, tenantID uuid.UUID) ([]models.EmailList, error) {
	return s.repo.ListLists(ctx, tenantID)
}

func (s *EmailMarketingService) DeleteList(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteList(ctx, tenantID, id)
}

// Templates
func (s *EmailMarketingService) CreateTemplate(ctx context.Context, t *models.EmailTemplate) error {
	return s.repo.CreateTemplate(ctx, t)
}

func (s *EmailMarketingService) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.EmailTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID)
}

func (s *EmailMarketingService) DeleteTemplate(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, tenantID, id)
}

// Stats
func (s *EmailMarketingService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.EmailStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}
