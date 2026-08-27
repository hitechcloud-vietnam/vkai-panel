package models

import (
	"time"

	"github.com/google/uuid"
)

// WordPressSite represents a WordPress installation
type WordPressSite struct {
	ID            uuid.UUID `json:"id" db:"id"`
	TenantID      uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID      uuid.UUID `json:"server_id" db:"server_id"`
	WebsiteID     *uuid.UUID `json:"website_id" db:"website_id"`
	Name          string    `json:"name" db:"name"`
	Domain        string    `json:"domain" db:"domain"`
	Path          string    `json:"path" db:"path"`
	DBName        string    `json:"db_name" db:"db_name"`
	DBUser        string    `json:"db_user" db:"db_user"`
	DBPassword    string    `json:"db_password" db:"db_password"`
	DBHost        string    `json:"db_host" db:"db_host"`
	DBPrefix      string    `json:"db_prefix" db:"db_prefix"`
	AdminUser     string    `json:"admin_user" db:"admin_user"`
	AdminPassword string    `json:"admin_password" db:"admin_password"`
	AdminEmail    string    `json:"admin_email" db:"admin_email"`
	Version       string    `json:"version" db:"version"`
	Status        string    `json:"status" db:"status"`
	IsActive      bool      `json:"is_active" db:"is_active"`
	AutoUpdate    bool      `json:"auto_update" db:"auto_update"`
	LastUpdateAt  *time.Time `json:"last_update_at" db:"last_update_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// WordPressPlugin represents a WordPress plugin
type WordPressPlugin struct {
	ID          uuid.UUID `json:"id" db:"id"`
	SiteID      uuid.UUID `json:"site_id" db:"site_id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Version     string    `json:"version" db:"version"`
	Status      string    `json:"status" db:"status"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	AutoUpdate  bool      `json:"auto_update" db:"auto_update"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// WordPressTheme represents a WordPress theme
type WordPressTheme struct {
	ID         uuid.UUID `json:"id" db:"id"`
	SiteID     uuid.UUID `json:"site_id" db:"site_id"`
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name       string    `json:"name" db:"name"`
	Slug       string    `json:"slug" db:"slug"`
	Version    string    `json:"version" db:"version"`
	IsActive   bool      `json:"is_active" db:"is_active"`
	AutoUpdate bool      `json:"auto_update" db:"auto_update"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// CreateWordPressSiteRequest represents a request to create a WordPress site
type CreateWordPressSiteRequest struct {
	ServerID      string `json:"server_id" binding:"required"`
	WebsiteID     string `json:"website_id"`
	Name          string `json:"name" binding:"required"`
	Domain        string `json:"domain" binding:"required"`
	Path          string `json:"path" binding:"required"`
	DBName        string `json:"db_name" binding:"required"`
	DBUser        string `json:"db_user" binding:"required"`
	DBPassword    string `json:"db_password" binding:"required"`
	DBHost        string `json:"db_host"`
	DBPrefix      string `json:"db_prefix"`
	AdminUser     string `json:"admin_user" binding:"required"`
	AdminPassword string `json:"admin_password" binding:"required"`
	AdminEmail    string `json:"admin_email" binding:"required"`
	AutoUpdate    bool   `json:"auto_update"`
}

// UpdateWordPressSiteRequest represents a request to update a WordPress site
type UpdateWordPressSiteRequest struct {
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	AdminUser  string `json:"admin_user"`
	AdminEmail string `json:"admin_email"`
	AutoUpdate *bool  `json:"auto_update"`
	IsActive   *bool  `json:"is_active"`
}

// InstallPluginRequest represents a request to install a WordPress plugin
type InstallPluginRequest struct {
	Slug    string `json:"slug" binding:"required"`
	Version string `json:"version"`
}

// InstallThemeRequest represents a request to install a WordPress theme
type InstallThemeRequest struct {
	Slug    string `json:"slug" binding:"required"`
	Version string `json:"version"`
}
