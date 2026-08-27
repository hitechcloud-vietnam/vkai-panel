package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type ReverseProxyService struct {
	repo   *repository.ReverseProxyRepository
	logger *zap.Logger
}

func NewReverseProxyService(repo *repository.ReverseProxyRepository, logger *zap.Logger) *ReverseProxyService {
	return &ReverseProxyService{
		repo:   repo,
		logger: logger,
	}
}

// ReverseProxy operations
func (s *ReverseProxyService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateReverseProxyRequest) (*models.ReverseProxy, error) {
	serverID, err := uuid.Parse(req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("invalid server_id: %w", err)
	}

	proxy := &models.ReverseProxy{
		TenantID:       tenantID,
		ServerID:       serverID,
		Name:           req.Name,
		Domain:         req.Domain,
		ListenPort:     req.ListenPort,
		TargetURL:      req.TargetURL,
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		Protocol:       req.Protocol,
		SSLEnabled:     req.SSLEnabled,
		SSLRedirect:    req.SSLRedirect,
		SSLCertPath:    req.SSLCertPath,
		SSLKeyPath:     req.SSLKeyPath,
		Headers:        req.Headers,
		WebSocket:      req.WebSocket,
		LoadBalancer:   req.LoadBalancer,
		BackendServers: req.BackendServers,
		HealthCheck:    req.HealthCheck,
		HealthInterval: req.HealthInterval,
		Status:         "active",
		IsActive:       true,
	}

	if req.WebsiteID != "" {
		websiteID, err := uuid.Parse(req.WebsiteID)
		if err != nil {
			return nil, fmt.Errorf("invalid website_id: %w", err)
		}
		proxy.WebsiteID = &websiteID
	}

	if proxy.ListenPort == 0 {
		proxy.ListenPort = 80
	}
	if proxy.Protocol == "" {
		proxy.Protocol = "http"
	}

	if err := s.repo.Create(ctx, proxy); err != nil {
		return nil, err
	}

	s.logger.Info("Reverse proxy created",
		zap.String("proxy_id", proxy.ID.String()),
		zap.String("name", proxy.Name),
		zap.String("domain", proxy.Domain),
	)

	return proxy, nil
}

func (s *ReverseProxyService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.ReverseProxy, error) {
	proxy, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if proxy.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return proxy, nil
}

func (s *ReverseProxyService) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.ReverseProxy, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit, offset)
}

func (s *ReverseProxyService) ListByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.ReverseProxy, error) {
	proxies, err := s.repo.ListByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Filter by tenant
	var result []models.ReverseProxy
	for _, p := range proxies {
		if p.TenantID == tenantID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *ReverseProxyService) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateReverseProxyRequest) (*models.ReverseProxy, error) {
	proxy, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		proxy.Name = req.Name
	}
	if req.Domain != "" {
		proxy.Domain = req.Domain
	}
	if req.ListenPort > 0 {
		proxy.ListenPort = req.ListenPort
	}
	if req.TargetURL != "" {
		proxy.TargetURL = req.TargetURL
	}
	if req.TargetHost != "" {
		proxy.TargetHost = req.TargetHost
	}
	if req.TargetPort > 0 {
		proxy.TargetPort = req.TargetPort
	}
	if req.Protocol != "" {
		proxy.Protocol = req.Protocol
	}
	if req.SSLEnabled != nil {
		proxy.SSLEnabled = *req.SSLEnabled
	}
	if req.SSLRedirect != nil {
		proxy.SSLRedirect = *req.SSLRedirect
	}
	if req.SSLCertPath != "" {
		proxy.SSLCertPath = req.SSLCertPath
	}
	if req.SSLKeyPath != "" {
		proxy.SSLKeyPath = req.SSLKeyPath
	}
	if req.Headers != nil {
		proxy.Headers = req.Headers
	}
	if req.WebSocket != nil {
		proxy.WebSocket = *req.WebSocket
	}
	if req.LoadBalancer != nil {
		proxy.LoadBalancer = *req.LoadBalancer
	}
	if req.BackendServers != nil {
		proxy.BackendServers = req.BackendServers
	}
	if req.HealthCheck != "" {
		proxy.HealthCheck = req.HealthCheck
	}
	if req.HealthInterval > 0 {
		proxy.HealthInterval = req.HealthInterval
	}
	if req.IsActive != nil {
		proxy.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, proxy); err != nil {
		return nil, err
	}

	s.logger.Info("Reverse proxy updated",
		zap.String("proxy_id", proxy.ID.String()),
		zap.String("name", proxy.Name),
	)

	return proxy, nil
}

func (s *ReverseProxyService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.logger.Info("Reverse proxy deleted", zap.String("proxy_id", id.String()))
	return nil
}

// Access Log operations
func (s *ReverseProxyService) ListAccessLogs(ctx context.Context, tenantID, proxyID uuid.UUID, limit, offset int) ([]models.ReverseProxyAccessLog, int, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, proxyID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListAccessLogsByProxy(ctx, proxyID, limit, offset)
}

func (s *ReverseProxyService) ClearAccessLogs(ctx context.Context, tenantID, proxyID uuid.UUID) error {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, proxyID)
	if err != nil {
		return err
	}
	return s.repo.DeleteAccessLogsByProxy(ctx, proxyID)
}
