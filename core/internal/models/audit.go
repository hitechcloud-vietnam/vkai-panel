package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID    *uuid.UUID `json:"user_id" db:"user_id"`
	Action    string     `json:"action" db:"action"`
	Resource  string     `json:"resource" db:"resource"`
	ResourceID *uuid.UUID `json:"resource_id" db:"resource_id"`
	Details   JSONMap    `json:"details" db:"details"`
	IPAddress string     `json:"ip_address" db:"ip_address"`
	UserAgent string     `json:"user_agent" db:"user_agent"`
	Status    string     `json:"status" db:"status"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// AuditLogSearchRequest represents an audit log search request
type AuditLogSearchRequest struct {
	UserID     *uuid.UUID `json:"user_id"`
	Action     string     `json:"action"`
	Resource   string     `json:"resource"`
	ResourceID *uuid.UUID `json:"resource_id"`
	Status     string     `json:"status"`
	Start      *time.Time `json:"start"`
	End        *time.Time `json:"end"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}

// AuditLogStats represents audit log statistics
type AuditLogStats struct {
	TotalLogs    int            `json:"total_logs"`
	ByAction     map[string]int `json:"by_action"`
	ByResource   map[string]int `json:"by_resource"`
	ByStatus     map[string]int `json:"by_status"`
	ByUser       map[string]int `json:"by_user"`
	RecentLogs   []AuditLog     `json:"recent_logs"`
}
