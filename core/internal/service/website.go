package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
)

type WebsiteService struct {
	websiteRepo *repository.WebsiteRepository
	serverRepo  *repository.ServerRepository
	registry    *webserver.Registry
}

func NewWebsiteService(
	websiteRepo *repository.WebsiteRepository,
	serverRepo *repository.ServerRepository,
	registry *webserver.Registry,
) *WebsiteService {
	return &WebsiteService{
		websiteRepo: websiteRepo,
		serverRepo:  serverRepo,
		registry:    registry,
	}
}

func (s *WebsiteService) Create(ctx context.Context, req *models.CreateWebsiteRequest, tenantID uuid.UUID) (*models.Website, error) {
	// The domain becomes part of a vhost file path and of the vhost body, so it
	// is validated before anything touches the filesystem.
	if err := webserver.ValidateSiteDomain(req.Domain); err != nil {
		return nil, err
	}

	// Check if domain already exists
	existing, _ := s.websiteRepo.GetByDomain(ctx, req.Domain)
	if existing != nil {
		return nil, fmt.Errorf("domain %s already exists", req.Domain)
	}

	// Get server to determine web server type
	server, err := s.serverRepo.GetByID(ctx, tenantID, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}

	// Default root dir. A supplied one must be an absolute, traversal-free path
	// inside the web root; anything else falls back to the default rather than
	// being trusted.
	rootDir, err := config.SiteRoot(req.Domain)
	if err != nil {
		return nil, err
	}
	if req.RootDir != "" {
		if err := utils.ValidateAbsolutePath(req.RootDir, "root_dir"); err != nil {
			return nil, err
		}
		clean, err := utils.EnsureWithinRoot(config.WebRoot(), req.RootDir)
		if err != nil {
			return nil, err
		}
		rootDir = clean
	}

	website := &models.Website{
		TenantID:      tenantID,
		ServerID:      req.ServerID,
		Domain:        req.Domain,
		RootDir:       rootDir,
		WebServerType: server.WebServerType,
		PHPVersion:    req.PHPVersion,
		SiteType:      req.SiteType,
		Status:        "pending",
		SSLEnabled:    false,
	}

	if err := s.websiteRepo.Create(ctx, website); err != nil {
		return nil, fmt.Errorf("failed to create website: %w", err)
	}

	// Create root directory
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to create root dir %s: %v\n", rootDir, err)
	}

	// Generate web server config
	adapter, ok := s.registry.Get(server.WebServerType)
	if ok {
		siteConfig := &webserver.SiteConfig{
			Domain:     req.Domain,
			RootDir:    rootDir,
			SSLEnabled: false,
			PHPVersion: req.PHPVersion,
		}

		if err := adapter.CreateSite(ctx, siteConfig); err != nil {
			fmt.Printf("Warning: failed to create site config: %v\n", err)
		} else {
			_ = adapter.EnableSite(ctx, req.Domain)
			_ = adapter.Reload(ctx)
			website.Status = "active"
			_ = s.websiteRepo.UpdateStatus(ctx, website.ID, "active")
		}
	}

	return website, nil
}

func (s *WebsiteService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Website, error) {
	return s.websiteRepo.GetByID(ctx, tenantID, id)
}

func (s *WebsiteService) ListByTenant(ctx context.Context, tenantID uuid.UUID, params *models.PaginationParams) ([]models.Website, int, error) {
	return s.websiteRepo.ListByTenant(ctx, tenantID, params)
}

func (s *WebsiteService) Update(ctx context.Context, website *models.Website) error {
	if err := webserver.ValidateSiteDomain(website.Domain); err != nil {
		return err
	}
	if err := utils.ValidateAbsolutePath(website.RootDir, "root_dir"); err != nil {
		return err
	}
	if _, err := utils.EnsureWithinRoot(config.WebRoot(), website.RootDir); err != nil {
		return err
	}
	return s.websiteRepo.Update(ctx, website)
}

func (s *WebsiteService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	website, err := s.websiteRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	// Disable site in web server
	server, err := s.serverRepo.GetByID(ctx, website.TenantID, website.ServerID)
	if err == nil {
		adapter, ok := s.registry.Get(server.WebServerType)
		if ok {
			_ = adapter.DisableSite(ctx, website.Domain)
			_ = adapter.DeleteSite(ctx, website.Domain)
			_ = adapter.Reload(ctx)
		}
	}

	return s.websiteRepo.Delete(ctx, tenantID, id)
}

func (s *WebsiteService) EnableSSL(ctx context.Context, tenantID, id uuid.UUID, cert, key, chain string) error {
	website, err := s.websiteRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	// Re-validate the stored domain: it is about to become a directory name.
	if err := webserver.ValidateSiteDomain(website.Domain); err != nil {
		return err
	}

	server, err := s.serverRepo.GetByID(ctx, website.TenantID, website.ServerID)
	if err != nil {
		return err
	}

	adapter, ok := s.registry.Get(server.WebServerType)
	if !ok {
		return fmt.Errorf("web server adapter not found: %s", server.WebServerType)
	}

	// Save certificates
	certDir, err := config.SiteSSLDir(website.Domain)
	if err != nil {
		return err
	}
	os.MkdirAll(certDir, 0700)

	certPath := filepath.Join(certDir, "fullchain.pem")
	keyPath := filepath.Join(certDir, "privkey.pem")
	chainPath := filepath.Join(certDir, "chain.pem")

	os.WriteFile(certPath, []byte(cert), 0600)
	os.WriteFile(keyPath, []byte(key), 0600)
	if chain != "" {
		os.WriteFile(chainPath, []byte(chain), 0600)
	}

	// Update site config with SSL
	siteConfig := &webserver.SiteConfig{
		Domain:     website.Domain,
		RootDir:    website.RootDir,
		SSLEnabled: true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		PHPVersion: website.PHPVersion,
	}

	if err := adapter.CreateSite(ctx, siteConfig); err != nil {
		return fmt.Errorf("failed to update site config: %w", err)
	}

	_ = adapter.Reload(ctx)

	website.SSLEnabled = true
	return s.websiteRepo.Update(ctx, website)
}

func (s *WebsiteService) AddDomain(ctx context.Context, tenantID, websiteID uuid.UUID, domainName, domainType string) (*models.Domain, error) {
	if err := webserver.ValidateSiteDomain(domainName); err != nil {
		return nil, err
	}

	website, err := s.websiteRepo.GetByID(ctx, tenantID, websiteID)
	if err != nil {
		return nil, err
	}

	domain := &models.Domain{
		TenantID:  website.TenantID,
		WebsiteID: &websiteID,
		Name:      domainName,
		Type:      domainType,
		Status:    "active",
	}

	if err := s.websiteRepo.CreateDomain(ctx, domain); err != nil {
		return nil, err
	}

	return domain, nil
}

func (s *WebsiteService) ListDomains(ctx context.Context, tenantID, websiteID uuid.UUID) ([]models.Domain, error) {
	if _, err := s.websiteRepo.GetByID(ctx, tenantID, websiteID); err != nil {
		return nil, err
	}
	return s.websiteRepo.ListDomains(ctx, websiteID)
}

func (s *WebsiteService) DeleteDomain(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.websiteRepo.DeleteDomain(ctx, tenantID, id)
}

// GenerateDefaultIndex creates a default index.html for new sites
func (s *WebsiteService) GenerateDefaultIndex(rootDir, domain string) error {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Domain}}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #0f172a; color: #e2e8f0; }
        .container { text-align: center; }
        h1 { font-size: 2rem; margin-bottom: 0.5rem; }
        p { color: #94a3b8; }
        .badge { display: inline-block; padding: 0.25rem 0.75rem; background: #1e40af; border-radius: 9999px; font-size: 0.875rem; margin-top: 1rem; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Domain}}</h1>
        <p>This site is managed by VKAI Panel</p>
        <div class="badge">HiTechCloud</div>
    </div>
</body>
</html>`

	t, err := template.New("index").Parse(tmpl)
	if err != nil {
		return err
	}

	indexPath := filepath.Join(rootDir, "index.html")
	f, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, struct{ Domain string }{Domain: domain})
}
