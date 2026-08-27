package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type ReverseProxyRepository struct {
	db *sqlx.DB
}

func NewReverseProxyRepository(db *sqlx.DB) *ReverseProxyRepository {
	return &ReverseProxyRepository{db: db}
}

// ReverseProxy operations
func (r *ReverseProxyRepository) Create(ctx context.Context, proxy *models.ReverseProxy) error {
	query := `
		INSERT INTO reverse_proxies (id, tenant_id, server_id, website_id, name, domain, listen_port, 
			target_url, target_host, target_port, protocol, ssl_enabled, ssl_redirect, ssl_cert_path, 
			ssl_key_path, headers, websocket, load_balancer, backend_servers, health_check, 
			health_interval, status, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		RETURNING created_at, updated_at`

	proxy.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		proxy.ID, proxy.TenantID, proxy.ServerID, proxy.WebsiteID, proxy.Name, proxy.Domain,
		proxy.ListenPort, proxy.TargetURL, proxy.TargetHost, proxy.TargetPort, proxy.Protocol,
		proxy.SSLEnabled, proxy.SSLRedirect, proxy.SSLCertPath, proxy.SSLKeyPath, proxy.Headers,
		proxy.WebSocket, proxy.LoadBalancer, proxy.BackendServers, proxy.HealthCheck,
		proxy.HealthInterval, proxy.Status, proxy.IsActive,
	).Scan(&proxy.CreatedAt, &proxy.UpdatedAt)
}

func (r *ReverseProxyRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ReverseProxy, error) {
	var proxy models.ReverseProxy
	query := `SELECT * FROM reverse_proxies WHERE id = $1`
	if err := r.db.GetContext(ctx, &proxy, query, id); err != nil {
		return nil, fmt.Errorf("reverse proxy not found: %w", err)
	}
	return &proxy, nil
}

func (r *ReverseProxyRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.ReverseProxy, int, error) {
	var proxies []models.ReverseProxy
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM reverse_proxies WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get proxies
	query := `SELECT * FROM reverse_proxies WHERE tenant_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &proxies, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return proxies, total, nil
}

func (r *ReverseProxyRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.ReverseProxy, error) {
	var proxies []models.ReverseProxy
	query := `SELECT * FROM reverse_proxies WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &proxies, query, serverID); err != nil {
		return nil, err
	}
	return proxies, nil
}

func (r *ReverseProxyRepository) Update(ctx context.Context, proxy *models.ReverseProxy) error {
	query := `
		UPDATE reverse_proxies 
		SET name = $2, domain = $3, listen_port = $4, target_url = $5, target_host = $6, 
			target_port = $7, protocol = $8, ssl_enabled = $9, ssl_redirect = $10, 
			ssl_cert_path = $11, ssl_key_path = $12, headers = $13, websocket = $14, 
			load_balancer = $15, backend_servers = $16, health_check = $17, 
			health_interval = $18, status = $19, is_active = $20, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		proxy.ID, proxy.Name, proxy.Domain, proxy.ListenPort, proxy.TargetURL,
		proxy.TargetHost, proxy.TargetPort, proxy.Protocol, proxy.SSLEnabled,
		proxy.SSLRedirect, proxy.SSLCertPath, proxy.SSLKeyPath, proxy.Headers,
		proxy.WebSocket, proxy.LoadBalancer, proxy.BackendServers, proxy.HealthCheck,
		proxy.HealthInterval, proxy.Status, proxy.IsActive,
	)
	return err
}

func (r *ReverseProxyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM reverse_proxy_access_logs WHERE proxy_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM reverse_proxies WHERE id = $1`, id)
	return err
}

// Access Log operations
func (r *ReverseProxyRepository) CreateAccessLog(ctx context.Context, log *models.ReverseProxyAccessLog) error {
	query := `
		INSERT INTO reverse_proxy_access_logs (id, proxy_id, tenant_id, remote_addr, method, 
			request_uri, status, body_bytes, referer, user_agent, response_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at`

	log.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		log.ID, log.ProxyID, log.TenantID, log.RemoteAddr, log.Method,
		log.RequestURI, log.Status, log.BodyBytes, log.Referer,
		log.UserAgent, log.ResponseTime,
	).Scan(&log.CreatedAt)
}

func (r *ReverseProxyRepository) ListAccessLogsByProxy(ctx context.Context, proxyID uuid.UUID, limit, offset int) ([]models.ReverseProxyAccessLog, int, error) {
	var logs []models.ReverseProxyAccessLog
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM reverse_proxy_access_logs WHERE proxy_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, proxyID); err != nil {
		return nil, 0, err
	}

	// Get logs
	query := `SELECT * FROM reverse_proxy_access_logs WHERE proxy_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &logs, query, proxyID, limit, offset); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *ReverseProxyRepository) DeleteAccessLogsByProxy(ctx context.Context, proxyID uuid.UUID) error {
	query := `DELETE FROM reverse_proxy_access_logs WHERE proxy_id = $1`
	_, err := r.db.ExecContext(ctx, query, proxyID)
	return err
}
