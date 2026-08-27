package models

import (
	"time"

	"github.com/google/uuid"
)

// SecurityScan represents a security scan
type SecurityScan struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ServerID    uuid.UUID  `json:"server_id" db:"server_id"`
	ScanType    string     `json:"scan_type" db:"scan_type"`
	Status      string     `json:"status" db:"status"`
	StartedAt   time.Time  `json:"started_at" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
	Score       int        `json:"score" db:"score"`
	TotalChecks int        `json:"total_checks" db:"total_checks"`
	PassedChecks int       `json:"passed_checks" db:"passed_checks"`
	FailedChecks int       `json:"failed_checks" db:"failed_checks"`
	Warnings    int        `json:"warnings" db:"warnings"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// SecurityVulnerability represents a security vulnerability
type SecurityVulnerability struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	ScanID      uuid.UUID  `json:"scan_id" db:"scan_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Severity    string     `json:"severity" db:"severity"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description" db:"description"`
	Affected    string     `json:"affected" db:"affected"`
	Solution    string     `json:"solution" db:"solution"`
	CVE         string     `json:"cve" db:"cve"`
	CVSS        float64    `json:"cvss" db:"cvss"`
	Status      string     `json:"status" db:"status"`
	ResolvedAt  *time.Time `json:"resolved_at" db:"resolved_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// SecurityCheck represents a security check item
type SecurityCheck struct {
	ID          uuid.UUID `json:"id" db:"id"`
	ScanID      uuid.UUID `json:"scan_id" db:"scan_id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Category    string    `json:"category" db:"category"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Status      string    `json:"status" db:"status"`
	Details     string    `json:"details" db:"details"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// SecurityPolicy represents a security policy
type SecurityPolicy struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Category    string    `json:"category" db:"category"`
	Rules       JSONMap   `json:"rules" db:"rules"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateSecurityScanRequest represents a request to create a security scan
type CreateSecurityScanRequest struct {
	ServerID string `json:"server_id" binding:"required"`
	ScanType string `json:"scan_type" binding:"required"`
}

// UpdateSecurityVulnerabilityRequest represents a request to update a vulnerability
type UpdateSecurityVulnerabilityRequest struct {
	Status string `json:"status" binding:"required"`
}

// CreateSecurityPolicyRequest represents a request to create a security policy
type CreateSecurityPolicyRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Category    string `json:"category" binding:"required"`
	Rules       JSONMap `json:"rules" binding:"required"`
}

// UpdateSecurityPolicyRequest represents a request to update a security policy
type UpdateSecurityPolicyRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Rules       JSONMap `json:"rules"`
	IsActive    *bool  `json:"is_active"`
}
