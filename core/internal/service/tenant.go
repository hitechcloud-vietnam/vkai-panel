package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type TenantService struct {
	tenantRepo *repository.TenantRepository
	logger     *zap.Logger
}

func NewTenantService(tenantRepo *repository.TenantRepository, logger *zap.Logger) *TenantService {
	return &TenantService{
		tenantRepo: tenantRepo,
		logger:     logger,
	}
}

func (s *TenantService) Create(ctx context.Context, req models.CreateTenantRequest) (*models.Tenant, error) {
	// Check if slug exists
	existing, _ := s.tenantRepo.GetBySlug(ctx, req.Slug)
	if existing != nil {
		return nil, fmt.Errorf("tenant slug already exists")
	}

	tenant := &models.Tenant{
		ID:          uuid.New(),
		Name:        req.Name,
		Slug:        req.Slug,
		Domain:      req.Domain,
		Status:      "active",
		Plan:        req.Plan,
		MaxServers:  req.MaxServers,
		MaxWebsites: req.MaxWebsites,
	}

	if tenant.MaxServers == 0 {
		tenant.MaxServers = 10
	}
	if tenant.MaxWebsites == 0 {
		tenant.MaxWebsites = 50
	}

	if err := s.tenantRepo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return tenant, nil
}

func (s *TenantService) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	return s.tenantRepo.GetByID(ctx, id)
}

func (s *TenantService) List(ctx context.Context, page, perPage int) ([]models.Tenant, int64, error) {
	return s.tenantRepo.List(ctx, page, perPage)
}

func (s *TenantService) Update(ctx context.Context, tenant *models.Tenant) error {
	return s.tenantRepo.Update(ctx, tenant)
}

func (s *TenantService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.tenantRepo.Delete(ctx, id)
}
