package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type NodeAppRepository struct {
	db *sqlx.DB
}

func NewNodeAppRepository(db *sqlx.DB) *NodeAppRepository {
	return &NodeAppRepository{db: db}
}

// NodeApp operations
func (r *NodeAppRepository) Create(ctx context.Context, app *models.NodeApp) error {
	query := `
		INSERT INTO node_apps (id, tenant_id, server_id, website_id, name, description, path, port, 
			node_version, npm_version, start_script, stop_script, restart_script, env_file, log_file, 
			pid_file, status, is_active, auto_restart, max_restarts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING created_at, updated_at`

	app.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		app.ID, app.TenantID, app.ServerID, app.WebsiteID, app.Name, app.Description,
		app.Path, app.Port, app.NodeVersion, app.NPMVersion, app.StartScript,
		app.StopScript, app.RestartScript, app.EnvFile, app.LogFile, app.PIDFile,
		app.Status, app.IsActive, app.AutoRestart, app.MaxRestarts,
	).Scan(&app.CreatedAt, &app.UpdatedAt)
}

func (r *NodeAppRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.NodeApp, error) {
	var app models.NodeApp
	query := `SELECT * FROM node_apps WHERE id = $1`
	if err := r.db.GetContext(ctx, &app, query, id); err != nil {
		return nil, fmt.Errorf("node app not found: %w", err)
	}
	return &app, nil
}

func (r *NodeAppRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.NodeApp, int, error) {
	var apps []models.NodeApp
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM node_apps WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get apps
	query := `SELECT * FROM node_apps WHERE tenant_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &apps, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

func (r *NodeAppRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.NodeApp, error) {
	var apps []models.NodeApp
	query := `SELECT * FROM node_apps WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &apps, query, serverID); err != nil {
		return nil, err
	}
	return apps, nil
}

func (r *NodeAppRepository) Update(ctx context.Context, app *models.NodeApp) error {
	query := `
		UPDATE node_apps 
		SET name = $2, description = $3, port = $4, node_version = $5, start_script = $6, 
			stop_script = $7, restart_script = $8, auto_restart = $9, max_restarts = $10, 
			is_active = $11, status = $12, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		app.ID, app.Name, app.Description, app.Port, app.NodeVersion,
		app.StartScript, app.StopScript, app.RestartScript,
		app.AutoRestart, app.MaxRestarts, app.IsActive, app.Status,
	)
	return err
}

func (r *NodeAppRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM node_app_dependencies WHERE app_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM node_app_environments WHERE app_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM node_apps WHERE id = $1`, id)
	return err
}

// NodeAppDependency operations
func (r *NodeAppRepository) CreateDependency(ctx context.Context, dep *models.NodeAppDependency) error {
	query := `
		INSERT INTO node_app_dependencies (id, app_id, name, version, is_dev)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`

	dep.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		dep.ID, dep.AppID, dep.Name, dep.Version, dep.IsDev,
	).Scan(&dep.CreatedAt, &dep.UpdatedAt)
}

func (r *NodeAppRepository) GetDependencyByID(ctx context.Context, id uuid.UUID) (*models.NodeAppDependency, error) {
	var dep models.NodeAppDependency
	query := `SELECT * FROM node_app_dependencies WHERE id = $1`
	if err := r.db.GetContext(ctx, &dep, query, id); err != nil {
		return nil, fmt.Errorf("dependency not found: %w", err)
	}
	return &dep, nil
}

func (r *NodeAppRepository) ListDependenciesByApp(ctx context.Context, appID uuid.UUID) ([]models.NodeAppDependency, error) {
	var deps []models.NodeAppDependency
	query := `SELECT * FROM node_app_dependencies WHERE app_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &deps, query, appID); err != nil {
		return nil, err
	}
	return deps, nil
}

func (r *NodeAppRepository) UpdateDependency(ctx context.Context, dep *models.NodeAppDependency) error {
	query := `UPDATE node_app_dependencies SET version = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, dep.ID, dep.Version)
	return err
}

func (r *NodeAppRepository) DeleteDependency(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM node_app_dependencies WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// NodeAppEnvironment operations
func (r *NodeAppRepository) CreateEnvironment(ctx context.Context, env *models.NodeAppEnvironment) error {
	query := `
		INSERT INTO node_app_environments (id, app_id, key, value, is_secret)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`

	env.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		env.ID, env.AppID, env.Key, env.Value, env.IsSecret,
	).Scan(&env.CreatedAt, &env.UpdatedAt)
}

func (r *NodeAppRepository) GetEnvironmentByID(ctx context.Context, id uuid.UUID) (*models.NodeAppEnvironment, error) {
	var env models.NodeAppEnvironment
	query := `SELECT * FROM node_app_environments WHERE id = $1`
	if err := r.db.GetContext(ctx, &env, query, id); err != nil {
		return nil, fmt.Errorf("environment variable not found: %w", err)
	}
	return &env, nil
}

func (r *NodeAppRepository) ListEnvironmentsByApp(ctx context.Context, appID uuid.UUID) ([]models.NodeAppEnvironment, error) {
	var envs []models.NodeAppEnvironment
	query := `SELECT * FROM node_app_environments WHERE app_id = $1 ORDER BY key`
	if err := r.db.SelectContext(ctx, &envs, query, appID); err != nil {
		return nil, err
	}
	return envs, nil
}

func (r *NodeAppRepository) UpdateEnvironment(ctx context.Context, env *models.NodeAppEnvironment) error {
	query := `UPDATE node_app_environments SET value = $2, is_secret = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, env.ID, env.Value, env.IsSecret)
	return err
}

func (r *NodeAppRepository) DeleteEnvironment(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM node_app_environments WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
