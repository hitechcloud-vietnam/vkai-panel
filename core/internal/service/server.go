package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type ServerService struct {
	serverRepo *repository.ServerRepository
	logger     *zap.Logger
}

func NewServerService(serverRepo *repository.ServerRepository, logger *zap.Logger) *ServerService {
	return &ServerService{
		serverRepo: serverRepo,
		logger:     logger,
	}
}

func (s *ServerService) Create(ctx context.Context, tenantID uuid.UUID, req models.CreateServerRequest) (*models.Server, error) {
	server := &models.Server{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Hostname:    req.Hostname,
		IPAddress:   req.IPAddress,
		SSHPort:     req.SSHPort,
		AgentStatus: "offline",
		AgentToken:  uuid.New().String(),
		Location:    req.Location,
		Tags:        req.Tags,
		Role:        req.Role,
		Status:      "active",
	}

	if server.SSHPort == 0 {
		server.SSHPort = 22
	}

	if err := s.serverRepo.Create(ctx, server); err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return server, nil
}

func (s *ServerService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Server, error) {
	return s.serverRepo.GetByID(ctx, tenantID, id)
}

func (s *ServerService) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Server, int64, error) {
	return s.serverRepo.ListByTenant(ctx, tenantID, page, perPage)
}

func (s *ServerService) Update(ctx context.Context, server *models.Server) error {
	return s.serverRepo.Update(ctx, server)
}

func (s *ServerService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.serverRepo.Delete(ctx, tenantID, id)
}

func (s *ServerService) Heartbeat(ctx context.Context, serverID uuid.UUID, metrics *models.ServerMetric) error {
	return s.serverRepo.UpdateHeartbeat(ctx, serverID, metrics)
}

func (s *ServerService) GetMetrics(ctx context.Context, serverID uuid.UUID) (*models.ServerMetric, error) {
	return s.serverRepo.GetLatestMetrics(ctx, serverID)
}
