package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type WAFService struct {
	repo *repository.WAFRepository
}

func NewWAFService(repo *repository.WAFRepository) *WAFService {
	return &WAFService{repo: repo}
}

// Rules

func (s *WAFService) ListRules(ctx context.Context, tenantID uuid.UUID) ([]models.WAFRule, error) {
	return s.repo.ListRules(ctx, tenantID)
}

func (s *WAFService) GetRule(ctx context.Context, id uuid.UUID) (*models.WAFRule, error) {
	return s.repo.GetRule(ctx, id)
}

func (s *WAFService) CreateRule(ctx context.Context, rule *models.WAFRule) error {
	return s.repo.CreateRule(ctx, rule)
}

func (s *WAFService) UpdateRule(ctx context.Context, rule *models.WAFRule) error {
	return s.repo.UpdateRule(ctx, rule)
}

func (s *WAFService) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteRule(ctx, id)
}

func (s *WAFService) ToggleRule(ctx context.Context, id uuid.UUID, enabled bool) error {
	return s.repo.ToggleRule(ctx, id, enabled)
}

// Policies

func (s *WAFService) ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]models.WAFPolicy, error) {
	return s.repo.ListPolicies(ctx, tenantID)
}

func (s *WAFService) GetPolicy(ctx context.Context, id uuid.UUID) (*models.WAFPolicy, error) {
	return s.repo.GetPolicy(ctx, id)
}

func (s *WAFService) CreatePolicy(ctx context.Context, policy *models.WAFPolicy) error {
	return s.repo.CreatePolicy(ctx, policy)
}

func (s *WAFService) UpdatePolicy(ctx context.Context, policy *models.WAFPolicy) error {
	return s.repo.UpdatePolicy(ctx, policy)
}

func (s *WAFService) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeletePolicy(ctx, id)
}

// Events

func (s *WAFService) ListEvents(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.WAFEvent, error) {
	return s.repo.ListEvents(ctx, tenantID, limit, offset)
}

func (s *WAFService) CreateEvent(ctx context.Context, event *models.WAFEvent) error {
	return s.repo.CreateEvent(ctx, event)
}

func (s *WAFService) GetStats(ctx context.Context, tenantID uuid.UUID, days int) (*models.WAFStats, error) {
	since := time.Now().AddDate(0, 0, -days)
	return s.repo.GetStats(ctx, tenantID, since)
}
