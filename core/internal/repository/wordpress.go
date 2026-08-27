package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type WordPressRepository struct {
	db *sqlx.DB
}

func NewWordPressRepository(db *sqlx.DB) *WordPressRepository {
	return &WordPressRepository{db: db}
}

// WordPress Site operations
func (r *WordPressRepository) Create(ctx context.Context, site *models.WordPressSite) error {
	query := `
		INSERT INTO wordpress_sites (id, tenant_id, server_id, website_id, name, domain, path, 
			db_name, db_user, db_password, db_host, db_prefix, admin_user, admin_password, 
			admin_email, version, status, is_active, auto_update)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING created_at, updated_at`

	site.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		site.ID, site.TenantID, site.ServerID, site.WebsiteID, site.Name, site.Domain,
		site.Path, site.DBName, site.DBUser, site.DBPassword, site.DBHost, site.DBPrefix,
		site.AdminUser, site.AdminPassword, site.AdminEmail, site.Version, site.Status,
		site.IsActive, site.AutoUpdate,
	).Scan(&site.CreatedAt, &site.UpdatedAt)
}

func (r *WordPressRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.WordPressSite, error) {
	var site models.WordPressSite
	query := `SELECT * FROM wordpress_sites WHERE id = $1`
	if err := r.db.GetContext(ctx, &site, query, id); err != nil {
		return nil, fmt.Errorf("wordpress site not found: %w", err)
	}
	return &site, nil
}

func (r *WordPressRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.WordPressSite, int, error) {
	var sites []models.WordPressSite
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM wordpress_sites WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get sites
	query := `SELECT * FROM wordpress_sites WHERE tenant_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &sites, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return sites, total, nil
}

func (r *WordPressRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.WordPressSite, error) {
	var sites []models.WordPressSite
	query := `SELECT * FROM wordpress_sites WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &sites, query, serverID); err != nil {
		return nil, err
	}
	return sites, nil
}

func (r *WordPressRepository) Update(ctx context.Context, site *models.WordPressSite) error {
	query := `
		UPDATE wordpress_sites 
		SET name = $2, domain = $3, path = $4, db_name = $5, db_user = $6, db_password = $7, 
			db_host = $8, db_prefix = $9, admin_user = $10, admin_password = $11, admin_email = $12, 
			version = $13, status = $14, is_active = $15, auto_update = $16, last_update_at = $17, 
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		site.ID, site.Name, site.Domain, site.Path, site.DBName, site.DBUser,
		site.DBPassword, site.DBHost, site.DBPrefix, site.AdminUser, site.AdminPassword,
		site.AdminEmail, site.Version, site.Status, site.IsActive, site.AutoUpdate,
		site.LastUpdateAt,
	)
	return err
}

func (r *WordPressRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM wordpress_plugins WHERE site_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM wordpress_themes WHERE site_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM wordpress_sites WHERE id = $1`, id)
	return err
}

// WordPress Plugin operations
func (r *WordPressRepository) CreatePlugin(ctx context.Context, plugin *models.WordPressPlugin) error {
	query := `
		INSERT INTO wordpress_plugins (id, site_id, tenant_id, name, slug, version, status, is_active, auto_update)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	plugin.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		plugin.ID, plugin.SiteID, plugin.TenantID, plugin.Name, plugin.Slug,
		plugin.Version, plugin.Status, plugin.IsActive, plugin.AutoUpdate,
	).Scan(&plugin.CreatedAt, &plugin.UpdatedAt)
}

func (r *WordPressRepository) GetPluginByID(ctx context.Context, id uuid.UUID) (*models.WordPressPlugin, error) {
	var plugin models.WordPressPlugin
	query := `SELECT * FROM wordpress_plugins WHERE id = $1`
	if err := r.db.GetContext(ctx, &plugin, query, id); err != nil {
		return nil, fmt.Errorf("plugin not found: %w", err)
	}
	return &plugin, nil
}

func (r *WordPressRepository) ListPluginsBySite(ctx context.Context, siteID uuid.UUID) ([]models.WordPressPlugin, error) {
	var plugins []models.WordPressPlugin
	query := `SELECT * FROM wordpress_plugins WHERE site_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &plugins, query, siteID); err != nil {
		return nil, err
	}
	return plugins, nil
}

func (r *WordPressRepository) UpdatePlugin(ctx context.Context, plugin *models.WordPressPlugin) error {
	query := `
		UPDATE wordpress_plugins 
		SET name = $2, slug = $3, version = $4, status = $5, is_active = $6, auto_update = $7, 
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		plugin.ID, plugin.Name, plugin.Slug, plugin.Version, plugin.Status,
		plugin.IsActive, plugin.AutoUpdate,
	)
	return err
}

func (r *WordPressRepository) DeletePlugin(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM wordpress_plugins WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// WordPress Theme operations
func (r *WordPressRepository) CreateTheme(ctx context.Context, theme *models.WordPressTheme) error {
	query := `
		INSERT INTO wordpress_themes (id, site_id, tenant_id, name, slug, version, is_active, auto_update)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	theme.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		theme.ID, theme.SiteID, theme.TenantID, theme.Name, theme.Slug,
		theme.Version, theme.IsActive, theme.AutoUpdate,
	).Scan(&theme.CreatedAt, &theme.UpdatedAt)
}

func (r *WordPressRepository) GetThemeByID(ctx context.Context, id uuid.UUID) (*models.WordPressTheme, error) {
	var theme models.WordPressTheme
	query := `SELECT * FROM wordpress_themes WHERE id = $1`
	if err := r.db.GetContext(ctx, &theme, query, id); err != nil {
		return nil, fmt.Errorf("theme not found: %w", err)
	}
	return &theme, nil
}

func (r *WordPressRepository) ListThemesBySite(ctx context.Context, siteID uuid.UUID) ([]models.WordPressTheme, error) {
	var themes []models.WordPressTheme
	query := `SELECT * FROM wordpress_themes WHERE site_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &themes, query, siteID); err != nil {
		return nil, err
	}
	return themes, nil
}

func (r *WordPressRepository) UpdateTheme(ctx context.Context, theme *models.WordPressTheme) error {
	query := `
		UPDATE wordpress_themes 
		SET name = $2, slug = $3, version = $4, is_active = $5, auto_update = $6, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		theme.ID, theme.Name, theme.Slug, theme.Version, theme.IsActive, theme.AutoUpdate,
	)
	return err
}

func (r *WordPressRepository) DeleteTheme(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM wordpress_themes WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
