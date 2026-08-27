package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ProtectedPath represents a file/directory under tamper protection
type ProtectedPath struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Path          string         `json:"path" db:"path"`
	PathType      string         `json:"path_type" db:"path_type"` // file, directory
	Recursive     bool           `json:"recursive" db:"recursive"`
	Algorithm     string         `json:"algorithm" db:"algorithm"` // sha256, sha512, md5
	IsEnabled     bool           `json:"is_enabled" db:"is_enabled"`
	AlertOnChange bool           `json:"alert_on_change" db:"alert_on_change"`
	AlertOnDelete bool           `json:"alert_on_delete" db:"alert_on_delete"`
	AlertOnCreate bool           `json:"alert_on_create" db:"alert_on_create"`
	IgnorePatterns pq.StringArray `json:"ignore_patterns" db:"ignore_patterns"` // glob patterns to ignore
	Description   string         `json:"description" db:"description"`
	FileCount     int            `json:"file_count" db:"file_count"`
	LastScanAt    *time.Time     `json:"last_scan_at" db:"last_scan_at"`
	LastAlertAt   *time.Time     `json:"last_alert_at" db:"last_alert_at"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
}

// FileBaseline stores the checksum of a monitored file
type FileBaseline struct {
	ID           uuid.UUID `json:"id" db:"id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ProtectedID  uuid.UUID `json:"protected_id" db:"protected_id"`
	FilePath     string    `json:"file_path" db:"file_path"`
	Checksum     string    `json:"checksum" db:"checksum"`
	FileSize     int64     `json:"file_size" db:"file_size"`
	FileMode     string    `json:"file_mode" db:"file_mode"`
	OwnerUser    string    `json:"owner_user" db:"owner_user"`
	OwnerGroup   string    `json:"owner_group" db:"owner_group"`
	ModTime      time.Time `json:"mod_time" db:"mod_time"`
	ScannedAt    time.Time `json:"scanned_at" db:"scanned_at"`
}

// TamperAlert represents an integrity violation alert
type TamperAlert struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProtectedID uuid.UUID  `json:"protected_id" db:"protected_id"`
	FilePath    string     `json:"file_path" db:"file_path"`
	AlertType   string     `json:"alert_type" db:"alert_type"` // modified, deleted, created, permission_changed
	Severity    string     `json:"severity" db:"severity"`     // low, medium, high, critical
	OldChecksum string     `json:"old_checksum" db:"old_checksum"`
	NewChecksum string     `json:"new_checksum" db:"new_checksum"`
	OldSize     int64      `json:"old_size" db:"old_size"`
	NewSize     int64      `json:"new_size" db:"new_size"`
	OldMode     string     `json:"old_mode" db:"old_mode"`
	NewMode     string     `json:"new_mode" db:"new_mode"`
	IsResolved  bool       `json:"is_resolved" db:"is_resolved"`
	ResolvedBy  string     `json:"resolved_by" db:"resolved_by"`
	ResolvedAt  *time.Time `json:"resolved_at" db:"resolved_at"`
	Notes       string     `json:"notes" db:"notes"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// TamperAuditLog represents an audit trail entry
type TamperAuditLog struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Action    string    `json:"action" db:"action"` // scan, baseline_update, alert_resolved, path_added, path_removed, path_updated
	Target    string    `json:"target" db:"target"`
	Details   string    `json:"details" db:"details"`
	IPAddress string    `json:"ip_address" db:"ip_address"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Username  string    `json:"username" db:"username"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// TamperScanResult represents the result of an integrity scan
type TamperScanResult struct {
	ID           uuid.UUID `json:"id" db:"id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ProtectedID  uuid.UUID `json:"protected_id" db:"protected_id"`
	Status       string    `json:"status" db:"status"` // clean, violations_found, error
	TotalFiles   int       `json:"total_files" db:"total_files"`
	ScannedFiles int       `json:"scanned_files" db:"scanned_files"`
	Violations   int       `json:"violations" db:"violations"`
	NewFiles     int       `json:"new_files" db:"new_files"`
	DeletedFiles int       `json:"deleted_files" db:"deleted_files"`
	ModifiedFiles int      `json:"modified_files" db:"modified_files"`
	Duration     int       `json:"duration" db:"duration"` // milliseconds
	ScanLog      string    `json:"scan_log" db:"scan_log"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Request types
type CreateProtectedPathRequest struct {
	Path          string   `json:"path" binding:"required"`
	PathType      string   `json:"path_type" binding:"required"`
	Recursive     bool     `json:"recursive"`
	Algorithm     string   `json:"algorithm"`
	AlertOnChange bool     `json:"alert_on_change"`
	AlertOnDelete bool     `json:"alert_on_delete"`
	AlertOnCreate bool     `json:"alert_on_create"`
	IgnorePatterns []string `json:"ignore_patterns"`
	Description   string   `json:"description"`
}

type UpdateProtectedPathRequest struct {
	Path          *string  `json:"path"`
	Recursive     *bool    `json:"recursive"`
	Algorithm     *string  `json:"algorithm"`
	IsEnabled     *bool    `json:"is_enabled"`
	AlertOnChange *bool    `json:"alert_on_change"`
	AlertOnDelete *bool    `json:"alert_on_delete"`
	AlertOnCreate *bool    `json:"alert_on_create"`
	IgnorePatterns []string `json:"ignore_patterns"`
	Description   *string  `json:"description"`
}

type ResolveAlertRequest struct {
	Notes string `json:"notes"`
}

// Stats
type TamperStats struct {
	ProtectedPaths   int     `json:"protected_paths" db:"protected_paths"`
	EnabledPaths     int     `json:"enabled_paths" db:"enabled_paths"`
	TotalFiles       int     `json:"total_files" db:"total_files"`
	ActiveAlerts     int     `json:"active_alerts" db:"active_alerts"`
	ResolvedAlerts   int     `json:"resolved_alerts" db:"resolved_alerts"`
	AlertsToday      int     `json:"alerts_today" db:"alerts_today"`
	LastScanAt       *time.Time `json:"last_scan_at" db:"last_scan_at"`
	TotalScans       int     `json:"total_scans" db:"total_scans"`
	CleanScans       int     `json:"clean_scans" db:"clean_scans"`
	ViolationScans   int     `json:"violation_scans" db:"violation_scans"`
}
