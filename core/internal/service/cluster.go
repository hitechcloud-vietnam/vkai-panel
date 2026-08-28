package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// Defaults that mirror the column defaults in migration 014, so a load
// balancer or HA pair created through the API looks the same as one created by
// a direct INSERT.
const (
	defaultSSLPort      = 443
	defaultFailoverMode = "automatic"
)

type ClusterService struct {
	repo   *repository.ClusterRepository
	logger *zap.Logger
}

func NewClusterService(repo *repository.ClusterRepository, logger *zap.Logger) *ClusterService {
	return &ClusterService{
		repo:   repo,
		logger: logger,
	}
}

// Clusters
func (s *ClusterService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateClusterRequest) (*models.Cluster, error) {
	cluster := &models.Cluster{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Status:      "active",
		Config:      req.Config,
	}
	if err := s.repo.Create(ctx, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (s *ClusterService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Cluster, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *ClusterService) List(ctx context.Context, tenantID uuid.UUID) ([]models.Cluster, error) {
	return s.repo.List(ctx, tenantID)
}

func (s *ClusterService) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateClusterRequest) (*models.Cluster, error) {
	return s.repo.Update(ctx, tenantID, id, req)
}

func (s *ClusterService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.Delete(ctx, tenantID, id)
}

// Cluster Nodes
func (s *ClusterService) AddNode(ctx context.Context, clusterID uuid.UUID, req *models.AddClusterNodeRequest) (*models.ClusterNode, error) {
	node := &models.ClusterNode{
		ClusterID: clusterID,
		ServerID:  req.ServerID,
		Role:      req.Role,
		Status:    "active",
		Weight:    req.Weight,
		Metadata:  req.Metadata,
	}
	if err := s.repo.AddNode(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (s *ClusterService) GetNodeByID(ctx context.Context, id uuid.UUID) (*models.ClusterNode, error) {
	return s.repo.GetNodeByID(ctx, id)
}

func (s *ClusterService) ListNodes(ctx context.Context, clusterID uuid.UUID) ([]models.ClusterNode, error) {
	return s.repo.ListNodes(ctx, clusterID)
}

func (s *ClusterService) UpdateNode(ctx context.Context, id uuid.UUID, req *models.UpdateClusterNodeRequest) (*models.ClusterNode, error) {
	return s.repo.UpdateNode(ctx, id, req)
}

func (s *ClusterService) UpdateNodeHeartbeat(ctx context.Context, id uuid.UUID) error {
	return s.repo.UpdateNodeHeartbeat(ctx, id)
}

func (s *ClusterService) RemoveNode(ctx context.Context, id uuid.UUID) error {
	return s.repo.RemoveNode(ctx, id)
}

// Load Balancers
func (s *ClusterService) CreateLoadBalancer(ctx context.Context, tenantID uuid.UUID, req *models.CreateLoadBalancerRequest) (*models.LoadBalancer, error) {
	clusterID := req.ClusterID
	lb := &models.LoadBalancer{
		TenantID:   tenantID,
		ClusterID:  &clusterID,
		Name:       req.Name,
		Type:       req.Type,
		Algorithm:  req.Algorithm,
		Status:     "active",
		ListenPort: req.ListenPort,
		SSLPort:    defaultSSLPort,
		Config:     req.Config,
	}
	if req.SSLPort != nil {
		lb.SSLPort = *req.SSLPort
	}
	if req.SSLEnabled != nil {
		lb.SSLEnabled = *req.SSLEnabled
	}
	if err := s.repo.CreateLoadBalancer(ctx, lb); err != nil {
		return nil, err
	}
	return lb, nil
}

func (s *ClusterService) GetLoadBalancerByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LoadBalancer, error) {
	return s.repo.GetLoadBalancerByID(ctx, tenantID, id)
}

func (s *ClusterService) ListLoadBalancers(ctx context.Context, tenantID uuid.UUID, clusterID *uuid.UUID) ([]models.LoadBalancer, error) {
	return s.repo.ListLoadBalancers(ctx, tenantID, clusterID)
}

func (s *ClusterService) UpdateLoadBalancer(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLoadBalancerRequest) (*models.LoadBalancer, error) {
	return s.repo.UpdateLoadBalancer(ctx, tenantID, id, req)
}

func (s *ClusterService) DeleteLoadBalancer(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteLoadBalancer(ctx, tenantID, id)
}

// HA Pairs
func (s *ClusterService) CreateHAPair(ctx context.Context, tenantID uuid.UUID, req *models.CreateHAPairRequest) (*models.HAPair, error) {
	ha := &models.HAPair{
		TenantID:          tenantID,
		Name:              req.Name,
		PrimaryServerID:   req.PrimaryServerID,
		SecondaryServerID: req.SecondaryServerID,
		VirtualIP:         req.VirtualIP,
		Status:            "active",
		FailoverMode:      defaultFailoverMode,
		Config:            req.Config,
	}
	if req.FailoverMode != nil {
		ha.FailoverMode = *req.FailoverMode
	}
	if err := s.repo.CreateHAPair(ctx, ha); err != nil {
		return nil, err
	}
	return ha, nil
}

func (s *ClusterService) GetHAPairByID(ctx context.Context, tenantID, id uuid.UUID) (*models.HAPair, error) {
	return s.repo.GetHAPairByID(ctx, tenantID, id)
}

func (s *ClusterService) ListHAPairs(ctx context.Context, tenantID uuid.UUID) ([]models.HAPair, error) {
	return s.repo.ListHAPairs(ctx, tenantID)
}

func (s *ClusterService) UpdateHAPair(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateHAPairRequest) (*models.HAPair, error) {
	return s.repo.UpdateHAPair(ctx, tenantID, id, req)
}

func (s *ClusterService) TriggerFailover(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.TriggerFailover(ctx, tenantID, id)
}

func (s *ClusterService) DeleteHAPair(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteHAPair(ctx, tenantID, id)
}
