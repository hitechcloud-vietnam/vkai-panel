package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type WordPressService struct {
	repo   *repository.WordPressRepository
	logger *zap.Logger

	// rt is the host-side half of this service - WP-CLI, staging and the
	// runtime identity repository - built on first use. See
	// wordpress_runtime.go.
	rt wpRuntime
}

func NewWordPressService(repo *repository.WordPressRepository, logger *zap.Logger) *WordPressService {
	return &WordPressService{
		repo:   repo,
		logger: logger,
	}
}

// WordPress Site operations
func (s *WordPressService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateWordPressSiteRequest) (*models.WordPressSite, error) {
	serverID, err := uuid.Parse(req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("invalid server_id: %w", err)
	}

	site := &models.WordPressSite{
		TenantID:      tenantID,
		ServerID:      serverID,
		Name:          req.Name,
		Domain:        req.Domain,
		Path:          req.Path,
		DBName:        req.DBName,
		DBUser:        req.DBUser,
		DBPassword:    req.DBPassword,
		DBHost:        req.DBHost,
		DBPrefix:      req.DBPrefix,
		AdminUser:     req.AdminUser,
		AdminPassword: req.AdminPassword,
		AdminEmail:    req.AdminEmail,
		Version:       "latest",
		Status:        "active",
		IsActive:      true,
		AutoUpdate:    req.AutoUpdate,
	}

	if req.WebsiteID != "" {
		websiteID, err := uuid.Parse(req.WebsiteID)
		if err != nil {
			return nil, fmt.Errorf("invalid website_id: %w", err)
		}
		site.WebsiteID = &websiteID
	}

	if site.DBHost == "" {
		site.DBHost = "localhost"
	}
	if site.DBPrefix == "" {
		site.DBPrefix = "wp_"
	}

	if err := s.repo.Create(ctx, site); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress site created",
		zap.String("site_id", site.ID.String()),
		zap.String("name", site.Name),
		zap.String("domain", site.Domain),
	)

	return site, nil
}

func (s *WordPressService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.WordPressSite, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if site.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return site, nil
}

func (s *WordPressService) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.WordPressSite, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit, offset)
}

func (s *WordPressService) ListByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.WordPressSite, error) {
	sites, err := s.repo.ListByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Filter by tenant
	var result []models.WordPressSite
	for _, site := range sites {
		if site.TenantID == tenantID {
			result = append(result, site)
		}
	}
	return result, nil
}

func (s *WordPressService) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateWordPressSiteRequest) (*models.WordPressSite, error) {
	site, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		site.Name = req.Name
	}
	if req.Domain != "" {
		site.Domain = req.Domain
	}
	if req.AdminUser != "" {
		site.AdminUser = req.AdminUser
	}
	if req.AdminEmail != "" {
		site.AdminEmail = req.AdminEmail
	}
	if req.AutoUpdate != nil {
		site.AutoUpdate = *req.AutoUpdate
	}
	if req.IsActive != nil {
		site.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, site); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress site updated",
		zap.String("site_id", site.ID.String()),
		zap.String("name", site.Name),
	)

	return site, nil
}

func (s *WordPressService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.logger.Info("WordPress site deleted", zap.String("site_id", id.String()))
	return nil
}

// WordPress Plugin operations
func (s *WordPressService) InstallPlugin(ctx context.Context, tenantID, siteID uuid.UUID, req *models.InstallPluginRequest) (*models.WordPressPlugin, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}

	plugin := &models.WordPressPlugin{
		SiteID:     siteID,
		TenantID:   tenantID,
		Name:       req.Slug,
		Slug:       req.Slug,
		Version:    req.Version,
		Status:     "active",
		IsActive:   true,
		AutoUpdate: false,
	}

	if err := s.repo.CreatePlugin(ctx, plugin); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress plugin installed",
		zap.String("plugin_id", plugin.ID.String()),
		zap.String("site_id", siteID.String()),
		zap.String("slug", req.Slug),
	)

	return plugin, nil
}

func (s *WordPressService) ListPlugins(ctx context.Context, tenantID, siteID uuid.UUID) ([]models.WordPressPlugin, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListPluginsBySite(ctx, siteID)
}

func (s *WordPressService) UpdatePlugin(ctx context.Context, tenantID, pluginID uuid.UUID, req *models.InstallPluginRequest) (*models.WordPressPlugin, error) {
	plugin, err := s.repo.GetPluginByID(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	// Verify access
	_, err = s.GetByID(ctx, tenantID, plugin.SiteID)
	if err != nil {
		return nil, err
	}

	if req.Version != "" {
		plugin.Version = req.Version
	}

	if err := s.repo.UpdatePlugin(ctx, plugin); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress plugin updated",
		zap.String("plugin_id", plugin.ID.String()),
		zap.String("slug", plugin.Slug),
	)

	return plugin, nil
}

func (s *WordPressService) DeletePlugin(ctx context.Context, tenantID, pluginID uuid.UUID) error {
	plugin, err := s.repo.GetPluginByID(ctx, pluginID)
	if err != nil {
		return err
	}

	// Verify access
	_, err = s.GetByID(ctx, tenantID, plugin.SiteID)
	if err != nil {
		return err
	}

	if err := s.repo.DeletePlugin(ctx, pluginID); err != nil {
		return err
	}

	s.logger.Info("WordPress plugin deleted", zap.String("plugin_id", pluginID.String()))
	return nil
}

// WordPress Theme operations
func (s *WordPressService) InstallTheme(ctx context.Context, tenantID, siteID uuid.UUID, req *models.InstallThemeRequest) (*models.WordPressTheme, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}

	theme := &models.WordPressTheme{
		SiteID:     siteID,
		TenantID:   tenantID,
		Name:       req.Slug,
		Slug:       req.Slug,
		Version:    req.Version,
		IsActive:   false,
		AutoUpdate: false,
	}

	if err := s.repo.CreateTheme(ctx, theme); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress theme installed",
		zap.String("theme_id", theme.ID.String()),
		zap.String("site_id", siteID.String()),
		zap.String("slug", req.Slug),
	)

	return theme, nil
}

func (s *WordPressService) ListThemes(ctx context.Context, tenantID, siteID uuid.UUID) ([]models.WordPressTheme, error) {
	// Verify access
	_, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListThemesBySite(ctx, siteID)
}

func (s *WordPressService) UpdateTheme(ctx context.Context, tenantID, themeID uuid.UUID, req *models.InstallThemeRequest) (*models.WordPressTheme, error) {
	theme, err := s.repo.GetThemeByID(ctx, themeID)
	if err != nil {
		return nil, err
	}

	// Verify access
	_, err = s.GetByID(ctx, tenantID, theme.SiteID)
	if err != nil {
		return nil, err
	}

	if req.Version != "" {
		theme.Version = req.Version
	}

	if err := s.repo.UpdateTheme(ctx, theme); err != nil {
		return nil, err
	}

	s.logger.Info("WordPress theme updated",
		zap.String("theme_id", theme.ID.String()),
		zap.String("slug", theme.Slug),
	)

	return theme, nil
}

func (s *WordPressService) DeleteTheme(ctx context.Context, tenantID, themeID uuid.UUID) error {
	theme, err := s.repo.GetThemeByID(ctx, themeID)
	if err != nil {
		return err
	}

	// Verify access
	_, err = s.GetByID(ctx, tenantID, theme.SiteID)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteTheme(ctx, themeID); err != nil {
		return err
	}

	s.logger.Info("WordPress theme deleted", zap.String("theme_id", themeID.String()))
	return nil
}
