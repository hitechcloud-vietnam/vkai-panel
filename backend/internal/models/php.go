package models

import (
	"time"
)

// PHPVersion represents a PHP version installed on the system
type PHPVersion struct {
	ID          string    `json:"id" db:"id"`
	Version     string    `json:"version" db:"version"`
	Path        string    `json:"path" db:"path"`
	FPMPath     string    `json:"fpm_path" db:"fpm_path"`
	FPMConfig   string    `json:"fpm_config" db:"fpm_config"`
	IniPath     string    `json:"ini_path" db:"ini_path"`
	Extensions  []string  `json:"extensions" db:"extensions"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	IsDefault   bool      `json:"is_default" db:"is_default"`
	ServerID    string    `json:"server_id" db:"server_id"`
	TenantID    string    `json:"tenant_id" db:"tenant_id"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// PHPPool represents a PHP-FPM pool configuration
type PHPPool struct {
	ID              string    `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	PHPVersionID    string    `json:"php_version_id" db:"php_version_id"`
	User            string    `json:"user" db:"user"`
	Group           string    `json:"group" db:"group"`
	Listen          string    `json:"listen" db:"listen"`
	ListenOwner     string    `json:"listen_owner" db:"listen_owner"`
	ListenGroup     string    `json:"listen_group" db:"listen_group"`
	ListenMode      string    `json:"listen_mode" db:"listen_mode"`
	PM              string    `json:"pm" db:"pm"`
	PMMaxChildren   int       `json:"pm_max_children" db:"pm_max_children"`
	PMStartServers  int       `json:"pm_start_servers" db:"pm_start_servers"`
	PMMinSpareServers int     `json:"pm_min_spare_servers" db:"pm_min_spare_servers"`
	PMMaxSpareServers int     `json:"pm_max_spare_servers" db:"pm_max_spare_servers"`
	PMMaxRequests   int       `json:"pm_max_requests" db:"pm_max_requests"`
	PMProcessIdleTimeout string `json:"pm_process_idle_timeout" db:"pm_process_idle_timeout"`
	StatusPath      string    `json:"status_path" db:"status_path"`
	AccessLog       string    `json:"access_log" db:"access_log"`
	ErrorLog        string    `json:"error_log" db:"error_log"`
	PhpAdminFlag    map[string]string `json:"php_admin_flag" db:"php_admin_flag"`
	PhpValue        map[string]string `json:"php_value" db:"php_value"`
	PhpAdminValue   map[string]string `json:"php_admin_value" db:"php_admin_value"`
	Env             map[string]string `json:"env" db:"env"`
	IsActive        bool      `json:"is_active" db:"is_active"`
	WebsiteID       string    `json:"website_id" db:"website_id"`
	ServerID        string    `json:"server_id" db:"server_id"`
	TenantID        string    `json:"tenant_id" db:"tenant_id"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// PHPExtension represents a PHP extension
type PHPExtension struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Version     string    `json:"version" db:"version"`
	Description string    `json:"description" db:"description"`
	IsInstalled bool      `json:"is_installed" db:"is_installed"`
	IsEnabled   bool      `json:"is_enabled" db:"is_enabled"`
	PHPVersionID string   `json:"php_version_id" db:"php_version_id"`
	ServerID    string    `json:"server_id" db:"server_id"`
	TenantID    string    `json:"tenant_id" db:"tenant_id"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// PHPConfig represents PHP configuration
type PHPConfig struct {
	ID              string            `json:"id" db:"id"`
	PHPVersionID    string            `json:"php_version_id" db:"php_version_id"`
	MemoryLimit     string            `json:"memory_limit" db:"memory_limit"`
	MaxExecutionTime int              `json:"max_execution_time" db:"max_execution_time"`
	MaxInputTime    int              `json:"max_input_time" db:"max_input_time"`
	PostMaxSize     string            `json:"post_max_size" db:"post_max_size"`
	UploadMaxFilesize string          `json:"upload_max_filesize" db:"upload_max_filesize"`
	MaxFileUploads  int              `json:"max_file_uploads" db:"max_file_uploads"`
	ErrorReporting  string            `json:"error_reporting" db:"error_reporting"`
	DisplayErrors   bool             `json:"display_errors" db:"display_errors"`
	LogErrors       bool             `json:"log_errors" db:"log_errors"`
	ErrorLog        string           `json:"error_log" db:"error_log"`
	DateFormat      string           `json:"date_format" db:"date_format"`
	Timezone        string           `json:"timezone" db:"timezone"`
	OPcacheEnabled  bool             `json:"opcache_enabled" db:"opcache_enabled"`
	OPcacheMemory   int              `json:"opcache_memory" db:"opcache_memory"`
	OPcacheMaxFiles int              `json:"opcache_max_files" db:"opcache_max_files"`
	OPcacheRevalidateFreq int        `json:"opcache_revalidate_freq" db:"opcache_revalidate_freq"`
	CustomSettings  map[string]string `json:"custom_settings" db:"custom_settings"`
	ServerID        string           `json:"server_id" db:"server_id"`
	TenantID        string           `json:"tenant_id" db:"tenant_id"`
	CreatedAt       time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at" db:"updated_at"`
}

// CreatePHPVersionRequest represents a request to create a PHP version
type CreatePHPVersionRequest struct {
	Version   string `json:"version" binding:"required"`
	Path      string `json:"path" binding:"required"`
	FPMPath   string `json:"fpm_path"`
	FPMConfig string `json:"fpm_config"`
	IniPath   string `json:"ini_path"`
	ServerID  string `json:"server_id" binding:"required"`
}

// UpdatePHPVersionRequest represents a request to update a PHP version
type UpdatePHPVersionRequest struct {
	IsActive  *bool  `json:"is_active"`
	IsDefault *bool  `json:"is_default"`
}

// CreatePHPPoolRequest represents a request to create a PHP-FPM pool
type CreatePHPPoolRequest struct {
	Name            string            `json:"name" binding:"required"`
	PHPVersionID    string            `json:"php_version_id" binding:"required"`
	User            string            `json:"user" binding:"required"`
	Group           string            `json:"group" binding:"required"`
	Listen          string            `json:"listen" binding:"required"`
	ListenOwner     string            `json:"listen_owner"`
	ListenGroup     string            `json:"listen_group"`
	ListenMode      string            `json:"listen_mode"`
	PM              string            `json:"pm" binding:"required"`
	PMMaxChildren   int               `json:"pm_max_children" binding:"required"`
	PMStartServers  int               `json:"pm_start_servers"`
	PMMinSpareServers int             `json:"pm_min_spare_servers"`
	PMMaxSpareServers int             `json:"pm_max_spare_servers"`
	PMMaxRequests   int               `json:"pm_max_requests"`
	PMProcessIdleTimeout string        `json:"pm_process_idle_timeout"`
	StatusPath      string            `json:"status_path"`
	AccessLog       string            `json:"access_log"`
	ErrorLog        string            `json:"error_log"`
	PhpAdminFlag    map[string]string `json:"php_admin_flag"`
	PhpValue        map[string]string `json:"php_value"`
	PhpAdminValue   map[string]string `json:"php_admin_value"`
	Env             map[string]string `json:"env"`
	WebsiteID       string            `json:"website_id"`
	ServerID        string            `json:"server_id" binding:"required"`
}

// UpdatePHPPoolRequest represents a request to update a PHP-FPM pool
type UpdatePHPPoolRequest struct {
	User            *string           `json:"user"`
	Group           *string           `json:"group"`
	Listen          *string           `json:"listen"`
	ListenOwner     *string           `json:"listen_owner"`
	ListenGroup     *string           `json:"listen_group"`
	ListenMode      *string           `json:"listen_mode"`
	PM              *string           `json:"pm"`
	PMMaxChildren   *int              `json:"pm_max_children"`
	PMStartServers  *int              `json:"pm_start_servers"`
	PMMinSpareServers *int            `json:"pm_min_spare_servers"`
	PMMaxSpareServers *int            `json:"pm_max_spare_servers"`
	PMMaxRequests   *int              `json:"pm_max_requests"`
	PMProcessIdleTimeout *string      `json:"pm_process_idle_timeout"`
	StatusPath      *string           `json:"status_path"`
	AccessLog       *string           `json:"access_log"`
	ErrorLog        *string           `json:"error_log"`
	PhpAdminFlag    map[string]string `json:"php_admin_flag"`
	PhpValue        map[string]string `json:"php_value"`
	PhpAdminValue   map[string]string `json:"php_admin_value"`
	Env             map[string]string `json:"env"`
	IsActive        *bool             `json:"is_active"`
}

// InstallPHPExtensionRequest represents a request to install a PHP extension
type InstallPHPExtensionRequest struct {
	Name        string `json:"name" binding:"required"`
	PHPVersionID string `json:"php_version_id" binding:"required"`
	ServerID    string `json:"server_id" binding:"required"`
}

// UpdatePHPConfigRequest represents a request to update PHP configuration
type UpdatePHPConfigRequest struct {
	MemoryLimit     *string           `json:"memory_limit"`
	MaxExecutionTime *int             `json:"max_execution_time"`
	MaxInputTime    *int              `json:"max_input_time"`
	PostMaxSize     *string           `json:"post_max_size"`
	UploadMaxFilesize *string         `json:"upload_max_filesize"`
	MaxFileUploads  *int              `json:"max_file_uploads"`
	ErrorReporting  *string           `json:"error_reporting"`
	DisplayErrors   *bool             `json:"display_errors"`
	LogErrors       *bool             `json:"log_errors"`
	ErrorLog        *string           `json:"error_log"`
	DateFormat      *string           `json:"date_format"`
	Timezone        *string           `json:"timezone"`
	OPcacheEnabled  *bool             `json:"opcache_enabled"`
	OPcacheMemory   *int              `json:"opcache_memory"`
	OPcacheMaxFiles *int              `json:"opcache_max_files"`
	OPcacheRevalidateFreq *int        `json:"opcache_revalidate_freq"`
	CustomSettings  map[string]string `json:"custom_settings"`
}
