package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type GitDeploymentService struct {
	repo   *repository.GitDeploymentRepository
	logger *zap.Logger
}

func NewGitDeploymentService(repo *repository.GitDeploymentRepository, logger *zap.Logger) *GitDeploymentService {
	return &GitDeploymentService{
		repo:   repo,
		logger: logger,
	}
}

// GitDeployment operations
func (s *GitDeploymentService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateGitDeploymentRequest) (*models.GitDeployment, error) {
	serverID, err := uuid.Parse(req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("invalid server_id: %w", err)
	}

	deployment := &models.GitDeployment{
		TenantID:       tenantID,
		ServerID:       serverID,
		Name:           req.Name,
		RepositoryURL:  req.RepositoryURL,
		Branch:         req.Branch,
		DeployPath:     req.DeployPath,
		DeployKey:      req.DeployKey,
		WebhookSecret:  req.WebhookSecret,
		AutoDeploy:     req.AutoDeploy,
		DeployScript:   req.DeployScript,
		PreDeployHook:  req.PreDeployHook,
		PostDeployHook: req.PostDeployHook,
		Environment:    req.Environment,
		Status:         "active",
		IsActive:       true,
	}

	if req.WebsiteID != "" {
		websiteID, err := uuid.Parse(req.WebsiteID)
		if err != nil {
			return nil, fmt.Errorf("invalid website_id: %w", err)
		}
		deployment.WebsiteID = &websiteID
	}

	if deployment.Branch == "" {
		deployment.Branch = "main"
	}

	if err := s.repo.Create(ctx, deployment); err != nil {
		return nil, err
	}

	s.logger.Info("Git deployment created",
		zap.String("deployment_id", deployment.ID.String()),
		zap.String("name", deployment.Name),
		zap.String("repository", deployment.RepositoryURL),
	)

	return deployment, nil
}

func (s *GitDeploymentService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.GitDeployment, error) {
	deployment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if deployment.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return deployment, nil
}

func (s *GitDeploymentService) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.GitDeployment, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit, offset)
}

func (s *GitDeploymentService) ListByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.GitDeployment, error) {
	deployments, err := s.repo.ListByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Filter by tenant
	var result []models.GitDeployment
	for _, d := range deployments {
		if d.TenantID == tenantID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s *GitDeploymentService) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateGitDeploymentRequest) (*models.GitDeployment, error) {
	deployment, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		deployment.Name = req.Name
	}
	if req.RepositoryURL != "" {
		deployment.RepositoryURL = req.RepositoryURL
	}
	if req.Branch != "" {
		deployment.Branch = req.Branch
	}
	if req.DeployPath != "" {
		deployment.DeployPath = req.DeployPath
	}
	if req.DeployKey != "" {
		deployment.DeployKey = req.DeployKey
	}
	if req.WebhookSecret != "" {
		deployment.WebhookSecret = req.WebhookSecret
	}
	if req.AutoDeploy != nil {
		deployment.AutoDeploy = *req.AutoDeploy
	}
	if req.DeployScript != "" {
		deployment.DeployScript = req.DeployScript
	}
	if req.PreDeployHook != "" {
		deployment.PreDeployHook = req.PreDeployHook
	}
	if req.PostDeployHook != "" {
		deployment.PostDeployHook = req.PostDeployHook
	}
	if req.Environment != nil {
		deployment.Environment = req.Environment
	}
	if req.IsActive != nil {
		deployment.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, deployment); err != nil {
		return nil, err
	}

	s.logger.Info("Git deployment updated",
		zap.String("deployment_id", deployment.ID.String()),
		zap.String("name", deployment.Name),
	)

	return deployment, nil
}

func (s *GitDeploymentService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.logger.Info("Git deployment deleted", zap.String("deployment_id", id.String()))
	return nil
}

// Deploy operations
func (s *GitDeploymentService) Deploy(ctx context.Context, tenantID, id uuid.UUID, req *models.DeployRequest) (*models.GitDeploymentLog, error) {
	deployment, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	// Create deployment log
	log := &models.GitDeploymentLog{
		DeploymentID: deployment.ID,
		TenantID:     tenantID,
		CommitHash:   req.CommitHash,
		Status:       "running",
	}

	if err := s.repo.CreateDeploymentLog(ctx, log); err != nil {
		return nil, err
	}

	// Run deployment in background
	go s.executeDeployment(deployment, log, req.Force)

	return log, nil
}

func (s *GitDeploymentService) executeDeployment(deployment *models.GitDeployment, log *models.GitDeploymentLog, force bool) {
	startTime := time.Now()
	var output strings.Builder

	// Pre-deploy hook
	if deployment.PreDeployHook != "" {
		output.WriteString("Running pre-deploy hook...\n")
		cmd := exec.Command("bash", "-c", deployment.PreDeployHook)
		cmd.Dir = deployment.DeployPath
		out, err := cmd.CombinedOutput()
		output.WriteString(string(out))
		if err != nil {
			log.Status = "failed"
			log.Error = fmt.Sprintf("Pre-deploy hook failed: %v", err)
			log.Output = output.String()
			log.Duration = int(time.Since(startTime).Seconds())
			s.repo.CreateDeploymentLog(context.Background(), log)
			return
		}
	}

	// Git pull
	output.WriteString("Pulling latest changes...\n")
	gitArgs := []string{"-C", deployment.DeployPath, "pull", "origin", deployment.Branch}
	if force {
		gitArgs = []string{"-C", deployment.DeployPath, "fetch", "origin", deployment.Branch, "--force"}
	}
	cmd := exec.Command("git", gitArgs...)
	out, err := cmd.CombinedOutput()
	output.WriteString(string(out))
	if err != nil {
		log.Status = "failed"
		log.Error = fmt.Sprintf("Git pull failed: %v", err)
		log.Output = output.String()
		log.Duration = int(time.Since(startTime).Seconds())
		s.repo.CreateDeploymentLog(context.Background(), log)
		return
	}

	// Deploy script
	if deployment.DeployScript != "" {
		output.WriteString("Running deploy script...\n")
		cmd = exec.Command("bash", "-c", deployment.DeployScript)
		cmd.Dir = deployment.DeployPath
		out, err = cmd.CombinedOutput()
		output.WriteString(string(out))
		if err != nil {
			log.Status = "failed"
			log.Error = fmt.Sprintf("Deploy script failed: %v", err)
			log.Output = output.String()
			log.Duration = int(time.Since(startTime).Seconds())
			s.repo.CreateDeploymentLog(context.Background(), log)
			return
		}
	}

	// Post-deploy hook
	if deployment.PostDeployHook != "" {
		output.WriteString("Running post-deploy hook...\n")
		cmd = exec.Command("bash", "-c", deployment.PostDeployHook)
		cmd.Dir = deployment.DeployPath
		out, err = cmd.CombinedOutput()
		output.WriteString(string(out))
		if err != nil {
			log.Status = "failed"
			log.Error = fmt.Sprintf("Post-deploy hook failed: %v", err)
			log.Output = output.String()
			log.Duration = int(time.Since(startTime).Seconds())
			s.repo.CreateDeploymentLog(context.Background(), log)
			return
		}
	}

	// Get current commit hash
	cmd = exec.Command("git", "-C", deployment.DeployPath, "rev-parse", "HEAD")
	out, err = cmd.CombinedOutput()
	if err == nil {
		deployment.LastCommitHash = strings.TrimSpace(string(out))
	}

	// Update deployment status
	now := time.Now()
	deployment.LastDeployAt = &now
	deployment.Status = "deployed"
	s.repo.Update(context.Background(), deployment)

	log.Status = "success"
	log.Output = output.String()
	log.Duration = int(time.Since(startTime).Seconds())
	s.repo.CreateDeploymentLog(context.Background(), log)

	s.logger.Info("Deployment completed",
		zap.String("deployment_id", deployment.ID.String()),
		zap.String("commit", deployment.LastCommitHash),
	)
}

// Deployment Log operations
func (s *GitDeploymentService) ListDeploymentLogs(ctx context.Context, tenantID, deploymentID uuid.UUID, limit, offset int) ([]models.GitDeploymentLog, int, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, deploymentID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListDeploymentLogsByDeployment(ctx, deploymentID, limit, offset)
}

func (s *GitDeploymentService) ClearDeploymentLogs(ctx context.Context, tenantID, deploymentID uuid.UUID) error {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, deploymentID)
	if err != nil {
		return err
	}
	return s.repo.DeleteDeploymentLogsByDeployment(ctx, deploymentID)
}
