package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type MailServerService struct {
	repo   *repository.MailServerRepository
	logger *zap.Logger
}

func NewMailServerService(repo *repository.MailServerRepository, logger *zap.Logger) *MailServerService {
	return &MailServerService{repo: repo, logger: logger}
}

func (s *MailServerService) CreateDomain(ctx context.Context, tenantID uuid.UUID, req models.CreateDomainRequest) (*models.MailDomain, error) {
	return s.repo.CreateDomain(ctx, tenantID, req)
}

func (s *MailServerService) ListDomains(ctx context.Context, tenantID uuid.UUID) ([]models.MailDomain, error) {
	return s.repo.ListDomains(ctx, tenantID)
}

func (s *MailServerService) GetDomain(ctx context.Context, id uuid.UUID) (*models.MailDomain, error) {
	return s.repo.GetDomain(ctx, id)
}

func (s *MailServerService) DeleteDomain(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDomain(ctx, id)
}

func (s *MailServerService) CreateAccount(ctx context.Context, tenantID uuid.UUID, req models.CreateAccountRequest) (*models.MailAccount, error) {
	// In production, hash the password here
	return s.repo.CreateAccount(ctx, tenantID, req, req.Password)
}

func (s *MailServerService) ListAccounts(ctx context.Context, tenantID uuid.UUID) ([]models.MailAccount, error) {
	return s.repo.ListAccounts(ctx, tenantID)
}

func (s *MailServerService) GetAccount(ctx context.Context, id uuid.UUID) (*models.MailAccount, error) {
	return s.repo.GetAccount(ctx, id)
}

func (s *MailServerService) UpdateAccount(ctx context.Context, id uuid.UUID, req models.UpdateAccountRequest) (*models.MailAccount, error) {
	return s.repo.UpdateAccount(ctx, id, req)
}

func (s *MailServerService) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAccount(ctx, id)
}

func (s *MailServerService) CreateAlias(ctx context.Context, tenantID uuid.UUID, req models.CreateAliasRequest) (*models.MailAlias, error) {
	return s.repo.CreateAlias(ctx, tenantID, req)
}

func (s *MailServerService) ListAliases(ctx context.Context, tenantID uuid.UUID) ([]models.MailAlias, error) {
	return s.repo.ListAliases(ctx, tenantID)
}

func (s *MailServerService) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAlias(ctx, id)
}

func (s *MailServerService) ListQueueItems(ctx context.Context, tenantID uuid.UUID) ([]models.MailQueueItem, error) {
	return s.repo.ListQueueItems(ctx, tenantID)
}

func (s *MailServerService) DeleteQueueItem(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteQueueItem(ctx, id)
}

func (s *MailServerService) FlushQueue(ctx context.Context, tenantID uuid.UUID) error {
	return s.repo.FlushQueue(ctx, tenantID)
}

func (s *MailServerService) GetSpamFilter(ctx context.Context, tenantID uuid.UUID) (*models.MailSpamFilter, error) {
	return s.repo.GetSpamFilter(ctx, tenantID)
}

func (s *MailServerService) UpdateSpamFilter(ctx context.Context, tenantID uuid.UUID, req models.UpdateSpamFilterRequest) (*models.MailSpamFilter, error) {
	return s.repo.UpdateSpamFilter(ctx, tenantID, req)
}

func (s *MailServerService) GetServerConfig(ctx context.Context, tenantID uuid.UUID) (*models.MailServerConfig, error) {
	return s.repo.GetServerConfig(ctx, tenantID)
}

func (s *MailServerService) UpdateServerConfig(ctx context.Context, tenantID uuid.UUID, req models.UpdateServerConfigRequest) (*models.MailServerConfig, error) {
	return s.repo.UpdateServerConfig(ctx, tenantID, req)
}

func (s *MailServerService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.MailStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}
