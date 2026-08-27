package models

import (
	"time"

	"github.com/google/uuid"
)

// ReverseProxy represents a reverse proxy configuration
type ReverseProxy struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID    uuid.UUID `json:"server_id" db:"server_id"`
	WebsiteID   *uuid.UUID `json:"website_id" db:"website_id"`
	Name        string    `json:"name" db:"name"`
	Domain      string    `json:"domain" db:"domain"`
	ListenPort  int       `json:"listen_port" db:"listen_port"`
	TargetURL   string    `json:"target_url" db:"target_url"`
	TargetHost  string    `json:"target_host" db:"target_host"`
	TargetPort  int       `json:"target_port" db:"target_port"`
	Protocol    string    `json:"protocol" db:"protocol"`
	SSLEnabled  bool      `json:"ssl_enabled" db:"ssl_enabled"`
	SSLRedirect bool      `json:"ssl_redirect" db:"ssl_redirect"`
	SSLCertPath string    `json:"ssl_cert_path" db:"ssl_cert_path"`
	SSLKeyPath  string    `json:"ssl_key_path" db:"ssl_key_path"`
	Headers     JSONMap   `json:"headers" db:"headers"`
	WebSocket   bool      `json:"websocket" db:"websocket"`
	LoadBalancer bool     `json:"load_balancer" db:"load_balancer"`
	BackendServers JSONMap `json:"backend_servers" db:"backend_servers"`
	HealthCheck string    `json:"health_check" db:"health_check"`
	HealthInterval int    `json:"health_interval" db:"health_interval"`
	Status      string    `json:"status" db:"status"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ReverseProxyAccessLog represents an access log entry
type ReverseProxyAccessLog struct {
	ID           uuid.UUID `json:"id" db:"id"`
	ProxyID      uuid.UUID `json:"proxy_id" db:"proxy_id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	RemoteAddr   string    `json:"remote_addr" db:"remote_addr"`
	Method       string    `json:"method" db:"method"`
	RequestURI   string    `json:"request_uri" db:"request_uri"`
	Status       int       `json:"status" db:"status"`
	BodyBytes    int64     `json:"body_bytes" db:"body_bytes"`
	Referer      string    `json:"referer" db:"referer"`
	UserAgent    string    `json:"user_agent" db:"user_agent"`
	ResponseTime float64   `json:"response_time" db:"response_time"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// CreateReverseProxyRequest represents a request to create a reverse proxy
type CreateReverseProxyRequest struct {
	ServerID       string `json:"server_id" binding:"required"`
	WebsiteID      string `json:"website_id"`
	Name           string `json:"name" binding:"required"`
	Domain         string `json:"domain" binding:"required"`
	ListenPort     int    `json:"listen_port"`
	TargetURL      string `json:"target_url"`
	TargetHost     string `json:"target_host" binding:"required"`
	TargetPort     int    `json:"target_port" binding:"required"`
	Protocol       string `json:"protocol"`
	SSLEnabled     bool   `json:"ssl_enabled"`
	SSLRedirect    bool   `json:"ssl_redirect"`
	SSLCertPath    string `json:"ssl_cert_path"`
	SSLKeyPath     string `json:"ssl_key_path"`
	Headers        JSONMap `json:"headers"`
	WebSocket      bool   `json:"websocket"`
	LoadBalancer   bool   `json:"load_balancer"`
	BackendServers JSONMap `json:"backend_servers"`
	HealthCheck    string `json:"health_check"`
	HealthInterval int    `json:"health_interval"`
}

// UpdateReverseProxyRequest represents a request to update a reverse proxy
type UpdateReverseProxyRequest struct {
	Name           string `json:"name"`
	Domain         string `json:"domain"`
	ListenPort     int    `json:"listen_port"`
	TargetURL      string `json:"target_url"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	Protocol       string `json:"protocol"`
	SSLEnabled     *bool  `json:"ssl_enabled"`
	SSLRedirect    *bool  `json:"ssl_redirect"`
	SSLCertPath    string `json:"ssl_cert_path"`
	SSLKeyPath     string `json:"ssl_key_path"`
	Headers        JSONMap `json:"headers"`
	WebSocket      *bool  `json:"websocket"`
	LoadBalancer   *bool  `json:"load_balancer"`
	BackendServers JSONMap `json:"backend_servers"`
	HealthCheck    string `json:"health_check"`
	HealthInterval int    `json:"health_interval"`
	IsActive       *bool  `json:"is_active"`
}
