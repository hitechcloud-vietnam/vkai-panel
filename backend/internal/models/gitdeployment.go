package models

import (
	"time"

	"github.com/google/uuid"
)

// GitDeployment represents a Git deployment configuration
type GitDeployment struct {
	ID             uuid.UUID `json:"id" db:"id"`
	TenantID       uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID       uuid.UUID `json:"server_id" db:"server_id"`
	WebsiteID      *uuid.UUID `json:"website_id" db:"website_id"`
	Name           string    `json:"name" db:"name"`
	RepositoryURL  string    `json:"repository_url" db:"repository_url"`
	Branch         string    `json:"branch" db:"branch"`
	DeployPath     string    `json:"deploy_path" db:"deploy_path"`
	DeployKey      string    `json:"deploy_key" db:"deploy_key"`
	WebhookSecret  string    `json:"webhook_secret" db:"webhook_secret"`
	WebhookURL     string    `json:"webhook_url" db:"webhook_url"`
	AutoDeploy     bool      `json:"auto_deploy" db:"auto_deploy"`
	DeployScript   string    `json:"deploy_script" db:"deploy_script"`
	PreDeployHook  string    `json:"pre_deploy_hook" db:"pre_deploy_hook"`
	PostDeployHook string    `json:"post_deploy_hook" db:"post_deploy_hook"`
	Environment    JSONMap   `json:"environment" db:"environment"`
	Status         string    `json:"status" db:"status"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	LastDeployAt   *time.Time `json:"last_deploy_at" db:"last_deploy_at"`
	LastCommitHash string    `json:"last_commit_hash" db:"last_commit_hash"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// GitDeploymentLog represents a deployment log entry
type GitDeploymentLog struct {
	ID           uuid.UUID `json:"id" db:"id"`
	DeploymentID uuid.UUID `json:"deployment_id" db:"deployment_id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	CommitHash   string    `json:"commit_hash" db:"commit_hash"`
	CommitMsg    string    `json:"commit_msg" db:"commit_msg"`
	Author       string    `json:"author" db:"author"`
	Status       string    `json:"status" db:"status"`
	Output       string    `json:"output" db:"output"`
	Error        string    `json:"error" db:"error"`
	Duration     int       `json:"duration" db:"duration"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// CreateGitDeploymentRequest represents a request to create a Git deployment
type CreateGitDeploymentRequest struct {
	ServerID       string `json:"server_id" binding:"required"`
	WebsiteID      string `json:"website_id"`
	Name           string `json:"name" binding:"required"`
	RepositoryURL  string `json:"repository_url" binding:"required"`
	Branch         string `json:"branch"`
	DeployPath     string `json:"deploy_path" binding:"required"`
	DeployKey      string `json:"deploy_key"`
	WebhookSecret  string `json:"webhook_secret"`
	AutoDeploy     bool   `json:"auto_deploy"`
	DeployScript   string `json:"deploy_script"`
	PreDeployHook  string `json:"pre_deploy_hook"`
	PostDeployHook string `json:"post_deploy_hook"`
	Environment    JSONMap `json:"environment"`
}

// UpdateGitDeploymentRequest represents a request to update a Git deployment
type UpdateGitDeploymentRequest struct {
	Name           string `json:"name"`
	RepositoryURL  string `json:"repository_url"`
	Branch         string `json:"branch"`
	DeployPath     string `json:"deploy_path"`
	DeployKey      string `json:"deploy_key"`
	WebhookSecret  string `json:"webhook_secret"`
	AutoDeploy     *bool  `json:"auto_deploy"`
	DeployScript   string `json:"deploy_script"`
	PreDeployHook  string `json:"pre_deploy_hook"`
	PostDeployHook string `json:"post_deploy_hook"`
	Environment    JSONMap `json:"environment"`
	IsActive       *bool  `json:"is_active"`
}

// DeployRequest represents a request to trigger a deployment
type DeployRequest struct {
	CommitHash string `json:"commit_hash"`
	Force      bool   `json:"force"`
}
