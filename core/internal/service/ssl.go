package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
)

type SSLService struct {
	sslRepo     *repository.SSLRepository
	websiteRepo *repository.WebsiteRepository
}

func NewSSLService(sslRepo *repository.SSLRepository, websiteRepo *repository.WebsiteRepository) *SSLService {
	return &SSLService{
		sslRepo:     sslRepo,
		websiteRepo: websiteRepo,
	}
}

func (s *SSLService) IssueLetsEncrypt(ctx context.Context, tenantID uuid.UUID, domain string, webroot string) (*models.SSLCertificate, error) {
	// The domain becomes both a certbot argument and a path component under
	// /etc/letsencrypt/live, and the webroot is passed straight to certbot.
	if err := webserver.ValidateSiteDomain(domain); err != nil {
		return nil, err
	}
	if err := utils.ValidateAbsolutePath(webroot, "webroot"); err != nil {
		return nil, err
	}

	// Check if certbot is installed
	if _, err := exec.LookPath("certbot"); err != nil {
		return nil, fmt.Errorf("certbot is not installed. Install with: apt install certbot python3-certbot-nginx")
	}

	// Build certbot command
	args := []string{
		"certonly",
		"--webroot",
		"-w", webroot,
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
		"--register-unsafely-without-email",
		"--no-eff-email",
	}

	cmd := exec.CommandContext(ctx, "certbot", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("certbot failed: %s: %w", string(output), err)
	}

	// Read the generated certificates
	certDir := filepath.Join("/etc/letsencrypt/live", domain)
	certPEM, err := os.ReadFile(filepath.Join(certDir, "fullchain.pem"))
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(filepath.Join(certDir, "privkey.pem"))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	chainPEM, _ := os.ReadFile(filepath.Join(certDir, "chain.pem"))

	cert := &models.SSLCertificate{
		TenantID:    tenantID,
		Domain:      domain,
		Issuer:      "Let's Encrypt",
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
		ChainCert:   string(chainPEM),
		Status:      "valid",
		AutoRenew:   true,
		Source:      "letsencrypt",
	}

	if err := s.sslRepo.Create(ctx, cert); err != nil {
		return nil, fmt.Errorf("failed to save certificate: %w", err)
	}

	return cert, nil
}

func (s *SSLService) UploadCustom(ctx context.Context, tenantID uuid.UUID, domain, certPEM, keyPEM, chainPEM string) (*models.SSLCertificate, error) {
	cert := &models.SSLCertificate{
		TenantID:    tenantID,
		Domain:      domain,
		Issuer:      "Custom",
		Certificate: certPEM,
		PrivateKey:  keyPEM,
		ChainCert:   chainPEM,
		Status:      "valid",
		AutoRenew:   false,
		Source:      "custom",
	}

	if err := s.sslRepo.Create(ctx, cert); err != nil {
		return nil, fmt.Errorf("failed to save certificate: %w", err)
	}

	return cert, nil
}

func (s *SSLService) GetByID(ctx context.Context, id uuid.UUID) (*models.SSLCertificate, error) {
	return s.sslRepo.GetByID(ctx, id)
}

func (s *SSLService) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.SSLCertificate, error) {
	return s.sslRepo.ListByTenant(ctx, tenantID)
}

func (s *SSLService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.sslRepo.Delete(ctx, id)
}

func (s *SSLService) RenewAll(ctx context.Context, tenantID uuid.UUID) ([]string, []error) {
	certs, err := s.sslRepo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, []error{err}
	}

	var renewed []string
	var errs []error

	for _, cert := range certs {
		if cert.AutoRenew && cert.Source == "letsencrypt" {
			if err := webserver.ValidateSiteDomain(cert.Domain); err != nil {
				continue
			}
			cmd := exec.CommandContext(ctx, "certbot", "renew", "--cert-name", cert.Domain, "--non-interactive")
			output, err := cmd.CombinedOutput()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to renew %s: %s: %w", cert.Domain, string(output), err))
				continue
			}
			renewed = append(renewed, cert.Domain)
		}
	}

	return renewed, errs
}

func (s *SSLService) GetExpiringSoon(ctx context.Context, tenantID uuid.UUID, days int) ([]models.SSLCertificate, error) {
	return s.sslRepo.GetExpiringSoon(ctx, tenantID, days)
}

// SetupAutoRenewal creates a cron job for automatic certificate renewal
func (s *SSLService) SetupAutoRenewal() error {
	cronEntry := "0 3 * * * certbot renew --quiet --deploy-hook 'systemctl reload nginx'\n"

	cronPath := "/etc/cron.d/vkai-ssl-renew"
	return os.WriteFile(cronPath, []byte(cronEntry), 0644)
}

// GetCertInfo returns certificate information from PEM data
func (s *SSLService) GetCertInfo(certPEM string) (map[string]interface{}, error) {
	// Parse certificate info using openssl
	cmd := exec.Command("openssl", "x509", "-noout", "-subject", "-issuer", "-dates", "-text")
	cmd.Stdin = strings.NewReader(certPEM)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return map[string]interface{}{
		"info": string(output),
	}, nil
}
