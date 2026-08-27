package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type SSLRepository struct {
	db *sqlx.DB
}

func NewSSLRepository(db *sqlx.DB) *SSLRepository {
	return &SSLRepository{db: db}
}

func (r *SSLRepository) Create(ctx context.Context, cert *models.SSLCertificate) error {
	query := `
		INSERT INTO ssl_certificates (id, tenant_id, website_id, domain, issuer, certificate, private_key, chain_cert, not_before, not_after, status, auto_renew, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING created_at, updated_at`

	cert.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		cert.ID, cert.TenantID, cert.WebsiteID, cert.Domain, cert.Issuer,
		cert.Certificate, cert.PrivateKey, cert.ChainCert,
		cert.NotBefore, cert.NotAfter, cert.Status, cert.AutoRenew, cert.Source,
	).Scan(&cert.CreatedAt, &cert.UpdatedAt)
}

func (r *SSLRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.SSLCertificate, error) {
	var cert models.SSLCertificate
	query := `SELECT * FROM ssl_certificates WHERE id = $1`
	if err := r.db.GetContext(ctx, &cert, query, id); err != nil {
		return nil, fmt.Errorf("certificate not found: %w", err)
	}
	return &cert, nil
}

func (r *SSLRepository) GetByDomain(ctx context.Context, domain string) (*models.SSLCertificate, error) {
	var cert models.SSLCertificate
	query := `SELECT * FROM ssl_certificates WHERE domain = $1 ORDER BY created_at DESC LIMIT 1`
	if err := r.db.GetContext(ctx, &cert, query, domain); err != nil {
		return nil, fmt.Errorf("certificate not found: %w", err)
	}
	return &cert, nil
}

func (r *SSLRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.SSLCertificate, error) {
	var certs []models.SSLCertificate
	query := `SELECT * FROM ssl_certificates WHERE tenant_id = $1 ORDER BY domain`
	if err := r.db.SelectContext(ctx, &certs, query, tenantID); err != nil {
		return nil, err
	}
	return certs, nil
}

func (r *SSLRepository) Update(ctx context.Context, cert *models.SSLCertificate) error {
	query := `
		UPDATE ssl_certificates SET
			issuer = $2, certificate = $3, private_key = $4, chain_cert = $5,
			not_before = $6, not_after = $7, status = $8, auto_renew = $9, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		cert.ID, cert.Issuer, cert.Certificate, cert.PrivateKey, cert.ChainCert,
		cert.NotBefore, cert.NotAfter, cert.Status, cert.AutoRenew,
	)
	return err
}

func (r *SSLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM ssl_certificates WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *SSLRepository) GetExpiringSoon(ctx context.Context, tenantID uuid.UUID, days int) ([]models.SSLCertificate, error) {
	var certs []models.SSLCertificate
	query := `SELECT * FROM ssl_certificates WHERE tenant_id = $1 AND not_after <= NOW() + INTERVAL '%d days' AND status = 'valid' ORDER BY not_after`
	if err := r.db.SelectContext(ctx, &certs, fmt.Sprintf(query, days), tenantID); err != nil {
		return nil, err
	}
	return certs, nil
}
