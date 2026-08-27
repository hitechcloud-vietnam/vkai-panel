package models

import (
	"time"

	"github.com/google/uuid"
)

// LogEntry represents a log entry
type LogEntry struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	ServerID  uuid.UUID  `json:"server_id" db:"server_id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Source    string     `json:"source" db:"source"`
	Level     string     `json:"level" db:"level"`
	Message   string     `json:"message" db:"message"`
	Details   JSONMap    `json:"details" db:"details"`
	Timestamp time.Time  `json:"timestamp" db:"timestamp"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// LogSource represents a log source configuration
type LogSource struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID    uuid.UUID  `json:"server_id" db:"server_id"`
	Name        string     `json:"name" db:"name"`
	Type        string     `json:"type" db:"type"`
	Path        string     `json:"path" db:"path"`
	Format      string     `json:"format" db:"format"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	LastReadAt  *time.Time `json:"last_read_at" db:"last_read_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// LogRotation represents a log rotation policy
type LogRotation struct {
	ID           uuid.UUID `json:"id" db:"id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID     uuid.UUID `json:"server_id" db:"server_id"`
	Source       string    `json:"source" db:"source"`
	MaxSizeMB    int       `json:"max_size_mb" db:"max_size_mb"`
	MaxAgeDays   int       `json:"max_age_days" db:"max_age_days"`
	MaxFiles     int       `json:"max_files" db:"max_files"`
	CompressOld  bool      `json:"compress_old" db:"compress_old"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// LogSearchRequest represents a log search request
type LogSearchRequest struct {
	ServerID *uuid.UUID `json:"server_id"`
	Source   string     `json:"source"`
	Level    string     `json:"level"`
	Query    string     `json:"query"`
	Start    *time.Time `json:"start"`
	End      *time.Time `json:"end"`
	Limit    int        `json:"limit"`
	Offset   int        `json:"offset"`
}

// CreateLogSourceRequest represents a request to create a log source
type CreateLogSourceRequest struct {
	ServerID uuid.UUID `json:"server_id" binding:"required"`
	Name     string    `json:"name" binding:"required"`
	Type     string    `json:"type" binding:"required"`
	Path     string    `json:"path" binding:"required"`
	Format   string    `json:"format"`
}

// UpdateLogSourceRequest represents a request to update a log source
type UpdateLogSourceRequest struct {
	Name     *string `json:"name"`
	Type     *string `json:"type"`
	Path     *string `json:"path"`
	Format   *string `json:"format"`
	IsActive *bool   `json:"is_active"`
}

// CreateLogRotationRequest represents a request to create a log rotation policy
type CreateLogRotationRequest struct {
	ServerID    uuid.UUID `json:"server_id" binding:"required"`
	Source      string    `json:"source" binding:"required"`
	MaxSizeMB   int       `json:"max_size_mb" binding:"required,min=1"`
	MaxAgeDays  int       `json:"max_age_days" binding:"required,min=1"`
	MaxFiles    int       `json:"max_files" binding:"required,min=1"`
	CompressOld bool      `json:"compress_old"`
}

// UpdateLogRotationRequest represents a request to update a log rotation policy
type UpdateLogRotationRequest struct {
	MaxSizeMB   *int  `json:"max_size_mb"`
	MaxAgeDays  *int  `json:"max_age_days"`
	MaxFiles    *int  `json:"max_files"`
	CompressOld *bool `json:"compress_old"`
	IsActive    *bool `json:"is_active"`
}
