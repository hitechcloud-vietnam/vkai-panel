package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type WebsiteRepository struct {
	db *sqlx.DB
}

func NewWebsiteRepository(db *sqlx.DB) *WebsiteRepository {
	return &WebsiteRepository{db: db}
}

func (r *WebsiteRepository) Create(ctx context.Context, w *models.Website) error {
	query := `
		INSERT INTO websites (id, tenant_id, server_id, domain, root_dir, web_server_type, php_version, site_type, status, ssl_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING created_at, updated_at`

	w.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		w.ID, w.TenantID, w.ServerID, w.Domain, w.RootDir,
		w.WebServerType, w.PHPVersion, w.SiteType, w.Status, w.SSLEnabled,
	).Scan(&w.CreatedAt, &w.UpdatedAt)
}

func (r *WebsiteRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Website, error) {
	var w models.Website
	query := `SELECT * FROM websites WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &w, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("website not found: %w", err)
	}
	return &w, nil
}

func (r *WebsiteRepository) GetByDomain(ctx context.Context, domain string) (*models.Website, error) {
	var w models.Website
	query := `SELECT * FROM websites WHERE domain = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &w, query, domain); err != nil {
		return nil, fmt.Errorf("website not found: %w", err)
	}
	return &w, nil
}

func (r *WebsiteRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, params *models.PaginationParams) ([]models.Website, int, error) {
	var websites []models.Website
	var total int

	countQuery := `SELECT COUNT(*) FROM websites WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	query := `SELECT * FROM websites WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &websites, query, tenantID, params.PerPage, params.Offset()); err != nil {
		return nil, 0, err
	}

	return websites, total, nil
}

func (r *WebsiteRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.Website, error) {
	var websites []models.Website
	query := `SELECT * FROM websites WHERE server_id = $1 AND deleted_at IS NULL ORDER BY domain`
	if err := r.db.SelectContext(ctx, &websites, query, serverID); err != nil {
		return nil, err
	}
	return websites, nil
}

func (r *WebsiteRepository) Update(ctx context.Context, w *models.Website) error {
	query := `
		UPDATE websites SET
			domain = $2, root_dir = $3, web_server_type = $4, php_version = $5,
			site_type = $6, status = $7, ssl_enabled = $8, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $9 AND deleted_at IS NULL
		RETURNING updated_at`
	return r.db.QueryRowContext(ctx, query,
		w.ID, w.Domain, w.RootDir, w.WebServerType,
		w.PHPVersion, w.SiteType, w.Status, w.SSLEnabled, w.TenantID,
	).Scan(&w.UpdatedAt)
}

func (r *WebsiteRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `UPDATE websites SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *WebsiteRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE websites SET status = $2, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}

// Domain operations
func (r *WebsiteRepository) CreateDomain(ctx context.Context, d *models.Domain) error {
	query := `
		INSERT INTO domains (id, tenant_id, website_id, name, type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING created_at, updated_at`

	d.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		d.ID, d.TenantID, d.WebsiteID, d.Name, d.Type, d.Status,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
}

func (r *WebsiteRepository) ListDomains(ctx context.Context, websiteID uuid.UUID) ([]models.Domain, error) {
	var domains []models.Domain
	query := `SELECT * FROM domains WHERE website_id = $1 ORDER BY type, name`
	if err := r.db.SelectContext(ctx, &domains, query, websiteID); err != nil {
		return nil, err
	}
	return domains, nil
}

func (r *WebsiteRepository) DeleteDomain(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM domains WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}
