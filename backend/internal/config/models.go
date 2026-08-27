package config

import (
	"time"

	"github.com/google/uuid"
)

// ConfigType represents the type of configuration
type ConfigType string

const (
	ConfigTypeWebserver    ConfigType = "webserver"
	ConfigTypePHP          ConfigType = "php"
	ConfigTypeNginx        ConfigType = "nginx"
	ConfigTypeApache       ConfigType = "apache"
	ConfigTypeOpenLiteSpeed ConfigType = "openlitespeed"
	ConfigTypeLiteSpeed    ConfigType = "litespeed"
	ConfigTypeCaddy        ConfigType = "caddy"
	ConfigTypeTraefik      ConfigType = "traefik"
	ConfigTypeMySQL        ConfigType = "mysql"
	ConfigTypePostgreSQL   ConfigType = "postgresql"
	ConfigTypeRedis        ConfigType = "redis"
	ConfigTypeFirewall     ConfigType = "firewall"
	ConfigTypeDNS          ConfigType = "dns"
	ConfigTypeSSL          ConfigType = "ssl"
	ConfigTypeCron         ConfigType = "cron"
	ConfigTypeSystemd      ConfigType = "systemd"
)

// ConfigSnapshot represents a snapshot of configuration
type ConfigSnapshot struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	ConfigType  ConfigType `json:"config_type" db:"config_type"`
	Name        string     `json:"name" db:"name"`
	Path        string     `json:"path" db:"path"`
	Content     string     `json:"content" db:"content"`
	Checksum    string     `json:"checksum" db:"checksum"`
	Version     int        `json:"version" db:"version"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	IsAutomatic bool       `json:"is_automatic" db:"is_automatic"`
	Description string     `json:"description" db:"description"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID    uuid.UUID  `json:"server_id" db:"server_id"`
	UserID      *uuid.UUID `json:"user_id" db:"user_id"`
}

// ConfigDiff represents differences between two config versions
type ConfigDiff struct {
	OldVersion int        `json:"old_version"`
	NewVersion int        `json:"new_version"`
	OldContent string     `json:"old_content"`
	NewContent string     `json:"new_content"`
	Additions  []string   `json:"additions"`
	Deletions  []string   `json:"deletions"`
	Changes    []string   `json:"changes"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ConfigRollbackRequest represents a rollback request
type ConfigRollbackRequest struct {
	SnapshotID uuid.UUID `json:"snapshot_id" binding:"required"`
	Reason     string    `json:"reason"`
	Force      bool      `json:"force"`
}

// ConfigFilter represents filters for querying config snapshots
type ConfigFilter struct {
	ConfigType ConfigType `json:"config_type"`
	Name       string     `json:"name"`
	ServerID   *uuid.UUID `json:"server_id"`
	IsActive   *bool      `json:"is_active"`
	From       *time.Time `json:"from"`
	To         *time.Time `json:"to"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
}

// ConfigStats represents configuration statistics
type ConfigStats struct {
	TotalSnapshots  int            `json:"total_snapshots"`
	ByType          map[string]int `json:"by_type"`
	ByServer        map[string]int `json:"by_server"`
	ActiveConfigs   int            `json:"active_configs"`
	LastSnapshot    *time.Time     `json:"last_snapshot"`
	StorageUsed     int64          `json:"storage_used_bytes"`
}

// ConfigValidation represents validation result
type ConfigValidation struct {
	IsValid  bool     `json:"is_valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// ConfigTemplate represents a configuration template
type ConfigTemplate struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	ConfigType  ConfigType `json:"config_type" db:"config_type"`
	Content     string     `json:"content" db:"content"`
	Description string     `json:"description" db:"description"`
	Variables   []string   `json:"variables" db:"variables"`
	IsDefault   bool       `json:"is_default" db:"is_default"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
}
