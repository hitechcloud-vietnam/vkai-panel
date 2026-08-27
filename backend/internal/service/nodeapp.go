package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type NodeAppService struct {
	nodeAppRepo *repository.NodeAppRepository
	logger      *zap.Logger
}

func NewNodeAppService(nodeAppRepo *repository.NodeAppRepository, logger *zap.Logger) *NodeAppService {
	return &NodeAppService{
		nodeAppRepo: nodeAppRepo,
		logger:      logger,
	}
}

// Create creates a new Node.js application
func (s *NodeAppService) Create(ctx context.Context, req *models.CreateNodeAppRequest, tenantID string) (*models.NodeApp, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	serverUUID, err := uuid.Parse(req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("invalid server ID: %w", err)
	}

	var websiteUUID *uuid.UUID
	if req.WebsiteID != "" {
		wID, err := uuid.Parse(req.WebsiteID)
		if err != nil {
			return nil, fmt.Errorf("invalid website ID: %w", err)
		}
		websiteUUID = &wID
	}

	// Set defaults
	nodeVersion := req.NodeVersion
	if nodeVersion == "" {
		nodeVersion = "18"
	}

	startScript := req.StartScript
	if startScript == "" {
		startScript = "npm start"
	}

	stopScript := req.StopScript
	if stopScript == "" {
		stopScript = "kill $PID"
	}

	maxRestarts := req.MaxRestarts
	if maxRestarts == 0 {
		maxRestarts = 5
	}

	app := &models.NodeApp{
		ID:            uuid.New(),
		TenantID:      tenantUUID,
		ServerID:      serverUUID,
		WebsiteID:     websiteUUID,
		Name:          req.Name,
		Description:   req.Description,
		Path:          req.Path,
		Port:          req.Port,
		NodeVersion:   nodeVersion,
		StartScript:   startScript,
		StopScript:    stopScript,
		RestartScript: req.RestartScript,
		Status:        "stopped",
		IsActive:      true,
		AutoRestart:   req.AutoRestart,
		MaxRestarts:   maxRestarts,
	}

	if err := s.nodeAppRepo.Create(ctx, app); err != nil {
		return nil, fmt.Errorf("failed to create node app: %w", err)
	}

	s.logger.Info("Node.js app created",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
		zap.Int("port", app.Port),
	)

	return app, nil
}

// Get gets a Node.js application by ID
func (s *NodeAppService) Get(ctx context.Context, id, tenantID string) (*models.NodeApp, error) {
	appUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	app, err := s.nodeAppRepo.GetByID(ctx, appUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node app: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if app.TenantID != tenantUUID {
		return nil, fmt.Errorf("app not found")
	}

	return app, nil
}

// List lists all Node.js applications for a tenant
func (s *NodeAppService) List(ctx context.Context, tenantID string, page, perPage int) ([]models.NodeApp, int, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid tenant ID: %w", err)
	}

	offset := (page - 1) * perPage
	return s.nodeAppRepo.ListByTenant(ctx, tenantUUID, perPage, offset)
}

// Update updates a Node.js application
func (s *NodeAppService) Update(ctx context.Context, id, tenantID string, req *models.UpdateNodeAppRequest) (*models.NodeApp, error) {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		app.Name = req.Name
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.Port > 0 {
		app.Port = req.Port
	}
	if req.NodeVersion != "" {
		app.NodeVersion = req.NodeVersion
	}
	if req.StartScript != "" {
		app.StartScript = req.StartScript
	}
	if req.StopScript != "" {
		app.StopScript = req.StopScript
	}
	if req.RestartScript != "" {
		app.RestartScript = req.RestartScript
	}
	if req.AutoRestart != nil {
		app.AutoRestart = *req.AutoRestart
	}
	if req.MaxRestarts > 0 {
		app.MaxRestarts = req.MaxRestarts
	}
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}

	if err := s.nodeAppRepo.Update(ctx, app); err != nil {
		return nil, fmt.Errorf("failed to update node app: %w", err)
	}

	s.logger.Info("Node.js app updated",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
	)

	return app, nil
}

// Delete deletes a Node.js application
func (s *NodeAppService) Delete(ctx context.Context, id, tenantID string) error {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.nodeAppRepo.Delete(ctx, app.ID); err != nil {
		return fmt.Errorf("failed to delete node app: %w", err)
	}

	s.logger.Info("Node.js app deleted",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
	)

	return nil
}

// Start starts a Node.js application
func (s *NodeAppService) Start(ctx context.Context, id, tenantID string) error {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return err
	}

	// TODO: Implement actual start logic using systemd
	// This would create a systemd service file and start it
	app.Status = "running"
	if err := s.nodeAppRepo.Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}

	s.logger.Info("Node.js app started",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
	)

	return nil
}

// Stop stops a Node.js application
func (s *NodeAppService) Stop(ctx context.Context, id, tenantID string) error {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return err
	}

	// TODO: Implement actual stop logic using systemd
	app.Status = "stopped"
	if err := s.nodeAppRepo.Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}

	s.logger.Info("Node.js app stopped",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
	)

	return nil
}

// Restart restarts a Node.js application
func (s *NodeAppService) Restart(ctx context.Context, id, tenantID string) error {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return err
	}

	// TODO: Implement actual restart logic using systemd
	app.Status = "running"
	if err := s.nodeAppRepo.Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update app status: %w", err)
	}

	s.logger.Info("Node.js app restarted",
		zap.String("id", app.ID.String()),
		zap.String("name", app.Name),
	)

	return nil
}

// GetStatus gets the status of a Node.js application
func (s *NodeAppService) GetStatus(ctx context.Context, id, tenantID string) (string, error) {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return "", err
	}

	// TODO: Implement actual status check using systemd
	return app.Status, nil
}

// GetLogs gets the logs of a Node.js application
func (s *NodeAppService) GetLogs(ctx context.Context, id, tenantID string, lines int) ([]string, error) {
	app, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual log reading
	_ = app
	return []string{"Logs not implemented yet"}, nil
}

// CreateDependency creates a new dependency for a Node.js app
func (s *NodeAppService) CreateDependency(ctx context.Context, appID, tenantID string, req *models.CreateNodeAppDependencyRequest) (*models.NodeAppDependency, error) {
	// Verify app exists and belongs to tenant
	_, err := s.Get(ctx, appID, tenantID)
	if err != nil {
		return nil, err
	}

	appUUID, err := uuid.Parse(appID)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	dep := &models.NodeAppDependency{
		ID:      uuid.New(),
		AppID:   appUUID,
		Name:    req.Name,
		Version: req.Version,
		IsDev:   req.IsDev,
	}

	if err := s.nodeAppRepo.CreateDependency(ctx, dep); err != nil {
		return nil, fmt.Errorf("failed to create dependency: %w", err)
	}

	s.logger.Info("Node.js dependency created",
		zap.String("id", dep.ID.String()),
		zap.String("name", dep.Name),
		zap.String("version", dep.Version),
	)

	return dep, nil
}

// ListDependencies lists all dependencies for a Node.js app
func (s *NodeAppService) ListDependencies(ctx context.Context, appID, tenantID string) ([]models.NodeAppDependency, error) {
	// Verify app exists and belongs to tenant
	_, err := s.Get(ctx, appID, tenantID)
	if err != nil {
		return nil, err
	}

	appUUID, err := uuid.Parse(appID)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	return s.nodeAppRepo.ListDependenciesByApp(ctx, appUUID)
}

// UpdateDependency updates a dependency
func (s *NodeAppService) UpdateDependency(ctx context.Context, id, tenantID string, req *models.UpdateNodeAppDependencyRequest) (*models.NodeAppDependency, error) {
	depUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid dependency ID: %w", err)
	}

	dep, err := s.nodeAppRepo.GetDependencyByID(ctx, depUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependency: %w", err)
	}

	// Verify app belongs to tenant
	_, err = s.Get(ctx, dep.AppID.String(), tenantID)
	if err != nil {
		return nil, err
	}

	dep.Version = req.Version
	if err := s.nodeAppRepo.UpdateDependency(ctx, dep); err != nil {
		return nil, fmt.Errorf("failed to update dependency: %w", err)
	}

	s.logger.Info("Node.js dependency updated",
		zap.String("id", dep.ID.String()),
		zap.String("name", dep.Name),
		zap.String("version", dep.Version),
	)

	return dep, nil
}

// DeleteDependency deletes a dependency
func (s *NodeAppService) DeleteDependency(ctx context.Context, id, tenantID string) error {
	depUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid dependency ID: %w", err)
	}

	dep, err := s.nodeAppRepo.GetDependencyByID(ctx, depUUID)
	if err != nil {
		return fmt.Errorf("failed to get dependency: %w", err)
	}

	// Verify app belongs to tenant
	_, err = s.Get(ctx, dep.AppID.String(), tenantID)
	if err != nil {
		return err
	}

	if err := s.nodeAppRepo.DeleteDependency(ctx, dep.ID); err != nil {
		return fmt.Errorf("failed to delete dependency: %w", err)
	}

	s.logger.Info("Node.js dependency deleted",
		zap.String("id", dep.ID.String()),
		zap.String("name", dep.Name),
	)

	return nil
}

// CreateEnvironment creates a new environment variable for a Node.js app
func (s *NodeAppService) CreateEnvironment(ctx context.Context, appID, tenantID string, req *models.CreateNodeAppEnvironmentRequest) (*models.NodeAppEnvironment, error) {
	// Verify app exists and belongs to tenant
	_, err := s.Get(ctx, appID, tenantID)
	if err != nil {
		return nil, err
	}

	appUUID, err := uuid.Parse(appID)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	env := &models.NodeAppEnvironment{
		ID:       uuid.New(),
		AppID:    appUUID,
		Key:      req.Key,
		Value:    req.Value,
		IsSecret: req.IsSecret,
	}

	if err := s.nodeAppRepo.CreateEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("failed to create environment variable: %w", err)
	}

	s.logger.Info("Node.js environment variable created",
		zap.String("id", env.ID.String()),
		zap.String("key", env.Key),
	)

	return env, nil
}

// ListEnvironments lists all environment variables for a Node.js app
func (s *NodeAppService) ListEnvironments(ctx context.Context, appID, tenantID string) ([]models.NodeAppEnvironment, error) {
	// Verify app exists and belongs to tenant
	_, err := s.Get(ctx, appID, tenantID)
	if err != nil {
		return nil, err
	}

	appUUID, err := uuid.Parse(appID)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	return s.nodeAppRepo.ListEnvironmentsByApp(ctx, appUUID)
}

// UpdateEnvironment updates an environment variable
func (s *NodeAppService) UpdateEnvironment(ctx context.Context, id, tenantID string, req *models.UpdateNodeAppEnvironmentRequest) (*models.NodeAppEnvironment, error) {
	envUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid environment ID: %w", err)
	}

	env, err := s.nodeAppRepo.GetEnvironmentByID(ctx, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variable: %w", err)
	}

	// Verify app belongs to tenant
	_, err = s.Get(ctx, env.AppID.String(), tenantID)
	if err != nil {
		return nil, err
	}

	env.Value = req.Value
	if req.IsSecret != nil {
		env.IsSecret = *req.IsSecret
	}

	if err := s.nodeAppRepo.UpdateEnvironment(ctx, env); err != nil {
		return nil, fmt.Errorf("failed to update environment variable: %w", err)
	}

	s.logger.Info("Node.js environment variable updated",
		zap.String("id", env.ID.String()),
		zap.String("key", env.Key),
	)

	return env, nil
}

// DeleteEnvironment deletes an environment variable
func (s *NodeAppService) DeleteEnvironment(ctx context.Context, id, tenantID string) error {
	envUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid environment ID: %w", err)
	}

	env, err := s.nodeAppRepo.GetEnvironmentByID(ctx, envUUID)
	if err != nil {
		return fmt.Errorf("failed to get environment variable: %w", err)
	}

	// Verify app belongs to tenant
	_, err = s.Get(ctx, env.AppID.String(), tenantID)
	if err != nil {
		return err
	}

	if err := s.nodeAppRepo.DeleteEnvironment(ctx, env.ID); err != nil {
		return fmt.Errorf("failed to delete environment variable: %w", err)
	}

	s.logger.Info("Node.js environment variable deleted",
		zap.String("id", env.ID.String()),
		zap.String("key", env.Key),
	)

	return nil
}
