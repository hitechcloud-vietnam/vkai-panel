package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type TenantRepository struct {
	db *sqlx.DB
}

func NewTenantRepository(db *sqlx.DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (id, name, slug, domain, status, plan, max_servers, max_websites, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		tenant.ID, tenant.Name, tenant.Slug, tenant.Domain,
		tenant.Status, tenant.Plan, tenant.MaxServers, tenant.MaxWebsites,
	).Scan(&tenant.CreatedAt, &tenant.UpdatedAt)
}

func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	var tenant models.Tenant
	query := `SELECT * FROM tenants WHERE id = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &tenant, query, id)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}
	return &tenant, nil
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	var tenant models.Tenant
	query := `SELECT * FROM tenants WHERE slug = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &tenant, query, slug)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}
	return &tenant, nil
}

func (r *TenantRepository) List(ctx context.Context, page, perPage int) ([]models.Tenant, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM tenants WHERE deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, err
	}

	var tenants []models.Tenant
	offset := (page - 1) * perPage
	query := `SELECT * FROM tenants WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	if err := r.db.SelectContext(ctx, &tenants, query, perPage, offset); err != nil {
		return nil, 0, err
	}

	return tenants, total, nil
}

func (r *TenantRepository) Update(ctx context.Context, tenant *models.Tenant) error {
	query := `
		UPDATE tenants SET name=$1, slug=$2, domain=$3, status=$4, plan=$5,
		max_servers=$6, max_websites=$7, updated_at=NOW()
		WHERE id=$8 AND deleted_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query,
		tenant.Name, tenant.Slug, tenant.Domain, tenant.Status,
		tenant.Plan, tenant.MaxServers, tenant.MaxWebsites, tenant.ID,
	)
	return err
}

func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE tenants SET deleted_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
