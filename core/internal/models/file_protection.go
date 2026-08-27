package models

import (
	"time"

	"github.com/google/uuid"
)

// FileProtectionRule represents a file monitoring rule
type FileProtectionRule struct {
	ID          uuid.UUID `db:"id" json:"id"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name        string    `db:"name" json:"name"`
	Path        string    `db:"path" json:"path"`
	Recursive   bool      `db:"recursive" json:"recursive"`
	FilePattern string    `db:"file_pattern" json:"file_pattern"` // glob pattern, e.g. "*.conf", "*" for all
	WatchCreate bool      `db:"watch_create" json:"watch_create"`
	WatchModify bool      `db:"watch_modify" json:"watch_modify"`
	WatchDelete bool      `db:"watch_delete" json:"watch_delete"`
	WatchPerms  bool      `db:"watch_permissions" json:"watch_permissions"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// FileIntegrityRecord stores baseline hash of a monitored file
type FileIntegrityRecord struct {
	ID         uuid.UUID `db:"id" json:"id"`
	RuleID     uuid.UUID `db:"rule_id" json:"rule_id"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenant_id"`
	FilePath   string    `db:"file_path" json:"file_path"`
	SHA256Hash string    `db:"sha256_hash" json:"sha256_hash"`
	FileSize   int64     `db:"file_size" json:"file_size"`
	FileMode   string    `db:"file_mode" json:"file_mode"`
	Owner      string    `db:"owner" json:"owner"`
	ScannedAt  time.Time `db:"scanned_at" json:"scanned_at"`
}

// FileChangeEvent logs a detected file change
type FileChangeEvent struct {
	ID        uuid.UUID `db:"id" json:"id"`
	RuleID    uuid.UUID `db:"rule_id" json:"rule_id"`
	TenantID  uuid.UUID `db:"tenant_id" json:"tenant_id"`
	FilePath  string    `db:"file_path" json:"file_path"`
	EventType string    `db:"event_type" json:"event_type"` // created, modified, deleted, permissions_changed
	OldHash   string    `db:"old_hash" json:"old_hash"`
	NewHash   string    `db:"new_hash" json:"new_hash"`
	OldMode   string    `db:"old_mode" json:"old_mode"`
	NewMode   string    `db:"new_mode" json:"new_mode"`
	Details   string    `db:"details" json:"details"`
	Severity  string    `db:"severity" json:"severity"` // low, medium, high, critical
	IsRead    bool      `db:"is_read" json:"is_read"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// QuarantineItem represents a quarantined file
type QuarantineItem struct {
	ID           uuid.UUID `db:"id" json:"id"`
	TenantID     uuid.UUID `db:"tenant_id" json:"tenant_id"`
	OriginalPath string    `db:"original_path" json:"original_path"`
	QuarantinePath string  `db:"quarantine_path" json:"quarantine_path"`
	SHA256Hash   string    `db:"sha256_hash" json:"sha256_hash"`
	FileSize     int64     `db:"file_size" json:"file_size"`
	Reason       string    `db:"reason" json:"reason"`
	RestoredAt   *time.Time `db:"restored_at" json:"restored_at"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// Request types
type CreateProtectionRuleRequest struct {
	Name        string `json:"name" binding:"required"`
	Path        string `json:"path" binding:"required"`
	Recursive   bool   `json:"recursive"`
	FilePattern string `json:"file_pattern"`
	WatchCreate bool   `json:"watch_create"`
	WatchModify bool   `json:"watch_modify"`
	WatchDelete bool   `json:"watch_delete"`
	WatchPerms  bool   `json:"watch_permissions"`
}

type UpdateProtectionRuleRequest struct {
	Name        *string `json:"name"`
	Recursive   *bool   `json:"recursive"`
	FilePattern *string `json:"file_pattern"`
	WatchCreate *bool   `json:"watch_create"`
	WatchModify *bool   `json:"watch_modify"`
	WatchDelete *bool   `json:"watch_delete"`
	WatchPerms  *bool   `json:"watch_permissions"`
	IsActive    *bool   `json:"is_active"`
}

type FileProtectionStats struct {
	TotalRules       int `json:"total_rules"`
	ActiveRules      int `json:"active_rules"`
	TotalFiles       int `json:"total_files"`
	ChangesToday     int `json:"changes_today"`
	QuarantinedFiles int `json:"quarantined_files"`
	UnreadAlerts     int `json:"unread_alerts"`
}
