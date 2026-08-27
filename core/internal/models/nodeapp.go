package models

import (
	"time"

	"github.com/google/uuid"
)

// NodeApp represents a Node.js application
type NodeApp struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID    uuid.UUID `json:"server_id" db:"server_id"`
	WebsiteID   *uuid.UUID `json:"website_id" db:"website_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Path        string    `json:"path" db:"path"`
	Port        int       `json:"port" db:"port"`
	NodeVersion string    `json:"node_version" db:"node_version"`
	NPMVersion  string    `json:"npm_version" db:"npm_version"`
	StartScript string    `json:"start_script" db:"start_script"`
	StopScript  string    `json:"stop_script" db:"stop_script"`
	RestartScript string  `json:"restart_script" db:"restart_script"`
	EnvFile     string    `json:"env_file" db:"env_file"`
	LogFile     string    `json:"log_file" db:"log_file"`
	PIDFile     string    `json:"pid_file" db:"pid_file"`
	Status      string    `json:"status" db:"status"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	AutoRestart bool      `json:"auto_restart" db:"auto_restart"`
	MaxRestarts int       `json:"max_restarts" db:"max_restarts"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// NodeAppDependency represents a Node.js app dependency
type NodeAppDependency struct {
	ID        uuid.UUID `json:"id" db:"id"`
	AppID     uuid.UUID `json:"app_id" db:"app_id"`
	Name      string    `json:"name" db:"name"`
	Version   string    `json:"version" db:"version"`
	IsDev     bool      `json:"is_dev" db:"is_dev"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// NodeAppEnvironment represents a Node.js app environment variable
type NodeAppEnvironment struct {
	ID        uuid.UUID `json:"id" db:"id"`
	AppID     uuid.UUID `json:"app_id" db:"app_id"`
	Key       string    `json:"key" db:"key"`
	Value     string    `json:"value" db:"value"`
	IsSecret  bool      `json:"is_secret" db:"is_secret"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CreateNodeAppRequest represents a request to create a Node.js app
type CreateNodeAppRequest struct {
	ServerID      string `json:"server_id" binding:"required"`
	WebsiteID     string `json:"website_id"`
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description"`
	Path          string `json:"path" binding:"required"`
	Port          int    `json:"port" binding:"required"`
	NodeVersion   string `json:"node_version"`
	StartScript   string `json:"start_script"`
	StopScript    string `json:"stop_script"`
	RestartScript string `json:"restart_script"`
	AutoRestart   bool   `json:"auto_restart"`
	MaxRestarts   int    `json:"max_restarts"`
}

// UpdateNodeAppRequest represents a request to update a Node.js app
type UpdateNodeAppRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Port          int    `json:"port"`
	NodeVersion   string `json:"node_version"`
	StartScript   string `json:"start_script"`
	StopScript    string `json:"stop_script"`
	RestartScript string `json:"restart_script"`
	AutoRestart   *bool  `json:"auto_restart"`
	MaxRestarts   int    `json:"max_restarts"`
	IsActive      *bool  `json:"is_active"`
}

// CreateNodeAppDependencyRequest represents a request to create a dependency
type CreateNodeAppDependencyRequest struct {
	Name    string `json:"name" binding:"required"`
	Version string `json:"version" binding:"required"`
	IsDev   bool   `json:"is_dev"`
}

// UpdateNodeAppDependencyRequest represents a request to update a dependency
type UpdateNodeAppDependencyRequest struct {
	Version string `json:"version" binding:"required"`
}

// CreateNodeAppEnvironmentRequest represents a request to create an environment variable
type CreateNodeAppEnvironmentRequest struct {
	Key      string `json:"key" binding:"required"`
	Value    string `json:"value" binding:"required"`
	IsSecret bool   `json:"is_secret"`
}

// UpdateNodeAppEnvironmentRequest represents a request to update an environment variable
type UpdateNodeAppEnvironmentRequest struct {
	Value    string `json:"value" binding:"required"`
	IsSecret *bool  `json:"is_secret"`
}
