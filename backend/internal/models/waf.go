package models

import (
	"time"

	"github.com/google/uuid"
)

// WAFRule represents a Web Application Firewall rule
type WAFRule struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	RuleType    string     `json:"rule_type" db:"rule_type"` // modsecurity, custom, owasp
	Severity    string     `json:"severity" db:"severity"`   // low, medium, high, critical
	Action      string     `json:"action" db:"action"`       // block, log, allow
	Pattern     string     `json:"pattern" db:"pattern"`
	Enabled     bool       `json:"enabled" db:"enabled"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// WAFPolicy represents a WAF policy configuration
type WAFPolicy struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name            string     `json:"name" db:"name"`
	Description     string     `json:"description" db:"description"`
	Mode            string     `json:"mode" db:"mode"` // detection, prevention
	ParanoiaLevel   int        `json:"paranoia_level" db:"paranoia_level"` // 1-4
	AnomalyThreshold int       `json:"anomaly_threshold" db:"anomaly_threshold"`
	Enabled         bool       `json:"enabled" db:"enabled"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt       *time.Time `json:"-" db:"deleted_at"`
}

// WAFEvent represents a WAF event/attack log
type WAFEvent struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	RuleID      *uuid.UUID `json:"rule_id" db:"rule_id"`
	WebsiteID   *uuid.UUID `json:"website_id" db:"website_id"`
	SourceIP    string    `json:"source_ip" db:"source_ip"`
	Method      string    `json:"method" db:"method"`
	Path        string    `json:"path" db:"path"`
	UserAgent   string    `json:"user_agent" db:"user_agent"`
	AttackType  string    `json:"attack_type" db:"attack_type"`
	Severity    string    `json:"severity" db:"severity"`
	Action      string    `json:"action" db:"action"`
	Details     string    `json:"details" db:"details"`
	Blocked     bool      `json:"blocked" db:"blocked"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// WAFStats represents WAF statistics
type WAFStats struct {
	TotalRequests   int64 `json:"total_requests"`
	BlockedRequests int64 `json:"blocked_requests"`
	AllowedRequests int64 `json:"allowed_requests"`
	TopAttackTypes  []struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	} `json:"top_attack_types"`
	TopSourceIPs []struct {
		IP    string `json:"ip"`
		Count int64  `json:"count"`
	} `json:"top_source_ips"`
	RecentEvents []WAFEvent `json:"recent_events"`
}
