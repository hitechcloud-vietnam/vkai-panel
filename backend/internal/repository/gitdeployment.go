package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type GitDeploymentRepository struct {
	db *sqlx.DB
}

func NewGitDeploymentRepository(db *sqlx.DB) *GitDeploymentRepository {
	return &GitDeploymentRepository{db: db}
}

// GitDeployment operations
func (r *GitDeploymentRepository) Create(ctx context.Context, deployment *models.GitDeployment) error {
	query := `
		INSERT INTO git_deployments (id, tenant_id, server_id, website_id, name, repository_url, 
			branch, deploy_path, deploy_key, webhook_secret, webhook_url, auto_deploy, deploy_script, 
			pre_deploy_hook, post_deploy_hook, environment, status, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING created_at, updated_at`

	deployment.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		deployment.ID, deployment.TenantID, deployment.ServerID, deployment.WebsiteID,
		deployment.Name, deployment.RepositoryURL, deployment.Branch, deployment.DeployPath,
		deployment.DeployKey, deployment.WebhookSecret, deployment.WebhookURL,
		deployment.AutoDeploy, deployment.DeployScript, deployment.PreDeployHook,
		deployment.PostDeployHook, deployment.Environment, deployment.Status, deployment.IsActive,
	).Scan(&deployment.CreatedAt, &deployment.UpdatedAt)
}

func (r *GitDeploymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.GitDeployment, error) {
	var deployment models.GitDeployment
	query := `SELECT * FROM git_deployments WHERE id = $1`
	if err := r.db.GetContext(ctx, &deployment, query, id); err != nil {
		return nil, fmt.Errorf("git deployment not found: %w", err)
	}
	return &deployment, nil
}

func (r *GitDeploymentRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.GitDeployment, int, error) {
	var deployments []models.GitDeployment
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM git_deployments WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get deployments
	query := `SELECT * FROM git_deployments WHERE tenant_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &deployments, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return deployments, total, nil
}

func (r *GitDeploymentRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.GitDeployment, error) {
	var deployments []models.GitDeployment
	query := `SELECT * FROM git_deployments WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &deployments, query, serverID); err != nil {
		return nil, err
	}
	return deployments, nil
}

func (r *GitDeploymentRepository) Update(ctx context.Context, deployment *models.GitDeployment) error {
	query := `
		UPDATE git_deployments 
		SET name = $2, repository_url = $3, branch = $4, deploy_path = $5, deploy_key = $6, 
			webhook_secret = $7, webhook_url = $8, auto_deploy = $9, deploy_script = $10, 
			pre_deploy_hook = $11, post_deploy_hook = $12, environment = $13, status = $14, 
			is_active = $15, last_deploy_at = $16, last_commit_hash = $17, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		deployment.ID, deployment.Name, deployment.RepositoryURL, deployment.Branch,
		deployment.DeployPath, deployment.DeployKey, deployment.WebhookSecret,
		deployment.WebhookURL, deployment.AutoDeploy, deployment.DeployScript,
		deployment.PreDeployHook, deployment.PostDeployHook, deployment.Environment,
		deployment.Status, deployment.IsActive, deployment.LastDeployAt,
		deployment.LastCommitHash,
	)
	return err
}

func (r *GitDeploymentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM git_deployment_logs WHERE deployment_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM git_deployments WHERE id = $1`, id)
	return err
}

// Deployment Log operations
func (r *GitDeploymentRepository) CreateDeploymentLog(ctx context.Context, log *models.GitDeploymentLog) error {
	query := `
		INSERT INTO git_deployment_logs (id, deployment_id, tenant_id, commit_hash, commit_msg, 
			author, status, output, error, duration)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`

	log.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		log.ID, log.DeploymentID, log.TenantID, log.CommitHash, log.CommitMsg,
		log.Author, log.Status, log.Output, log.Error, log.Duration,
	).Scan(&log.CreatedAt)
}

func (r *GitDeploymentRepository) ListDeploymentLogsByDeployment(ctx context.Context, deploymentID uuid.UUID, limit, offset int) ([]models.GitDeploymentLog, int, error) {
	var logs []models.GitDeploymentLog
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM git_deployment_logs WHERE deployment_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, deploymentID); err != nil {
		return nil, 0, err
	}

	// Get logs
	query := `SELECT * FROM git_deployment_logs WHERE deployment_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &logs, query, deploymentID, limit, offset); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *GitDeploymentRepository) DeleteDeploymentLogsByDeployment(ctx context.Context, deploymentID uuid.UUID) error {
	query := `DELETE FROM git_deployment_logs WHERE deployment_id = $1`
	_, err := r.db.ExecContext(ctx, query, deploymentID)
	return err
}
