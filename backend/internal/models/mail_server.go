package models

import (
	"time"

	"github.com/google/uuid"
)

// MailDomain represents a mail domain
type MailDomain struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	TenantID    uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	Domain      string     `db:"domain" json:"domain"`
	IsVerified  bool       `db:"is_verified" json:"is_verified"`
	MXRecord    string     `db:"mx_record" json:"mx_record"`
	SPFRecord   string     `db:"spf_record" json:"spf_record"`
	DKIMEnabled bool       `db:"dkim_enabled" json:"dkim_enabled"`
	DMARCRecord string     `db:"dmarc_record" json:"dmarc_record"`
	IsActive    bool       `db:"is_active" json:"is_active"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

// MailAccount represents an email account
type MailAccount struct {
	ID            uuid.UUID  `db:"id" json:"id"`
	TenantID      uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	DomainID      uuid.UUID  `db:"domain_id" json:"domain_id"`
	Email         string     `db:"email" json:"email"`
	Password      string     `db:"password" json:"-"`
	QuotaMB       int        `db:"quota_mb" json:"quota_mb"`
	UsedMB        int        `db:"used_mb" json:"used_mb"`
	IsActive      bool       `db:"is_active" json:"is_active"`
	ForwardTo     string     `db:"forward_to" json:"forward_to"`
	AutoReply     bool       `db:"auto_reply" json:"auto_reply"`
	AutoReplyMsg  string     `db:"auto_reply_msg" json:"auto_reply_msg"`
	LastLoginAt   *time.Time `db:"last_login_at" json:"last_login_at"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

// MailAlias represents a mail alias/forwarding rule
type MailAlias struct {
	ID          uuid.UUID `db:"id" json:"id"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	DomainID    uuid.UUID `db:"domain_id" json:"domain_id"`
	Source      string    `db:"source" json:"source"`
	Destination string    `db:"destination" json:"destination"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// MailDKIMKey represents DKIM configuration for a domain
type MailDKIMKey struct {
	ID        uuid.UUID `db:"id" json:"id"`
	DomainID  uuid.UUID `db:"domain_id" json:"domain_id"`
	Selector  string    `db:"selector" json:"selector"`
	PublicKey string    `db:"public_key" json:"public_key"`
	PrivateKey string   `db:"private_key" json:"-"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// MailQueueItem represents a mail in the queue
type MailQueueItem struct {
	ID         uuid.UUID `db:"id" json:"id"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenant_id"`
	From       string    `db:"from_address" json:"from"`
	To         string    `db:"to_address" json:"to"`
	Subject    string    `db:"subject" json:"subject"`
	Status     string    `db:"status" json:"status"` // queued, sent, failed, deferred
	RetryCount int       `db:"retry_count" json:"retry_count"`
	LastError  string    `db:"last_error" json:"last_error"`
	ScheduledAt *time.Time `db:"scheduled_at" json:"scheduled_at"`
	SentAt     *time.Time `db:"sent_at" json:"sent_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
}

// MailSpamFilter represents spam filter settings
type MailSpamFilter struct {
	ID             uuid.UUID `db:"id" json:"id"`
	TenantID       uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Enabled        bool      `db:"enabled" json:"enabled"`
	SpamThreshold  float64   `db:"spam_threshold" json:"spam_threshold"`
	RejectScore    float64   `db:"reject_score" json:"reject_score"`
	Greylisting    bool      `db:"greylisting" json:"greylisting"`
	Blacklist      []string  `db:"blacklist" json:"blacklist"`
	Whitelist      []string  `db:"whitelist" json:"whitelist"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// MailServerConfig represents mail server configuration
type MailServerConfig struct {
	ID              uuid.UUID `db:"id" json:"id"`
	TenantID        uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Hostname        string    `db:"hostname" json:"hostname"`
	SMTPPort        int       `db:"smtp_port" json:"smtp_port"`
	SMTPSPort       int       `db:"smtps_port" json:"smtps_port"`
	IMAPPort        int       `db:"imap_port" json:"imap_port"`
	IMAPSPort       int       `db:"imaps_port" json:"imaps_port"`
	MaxMessageSize  int       `db:"max_message_size" json:"max_message_size"` // in MB
	MaxMailboxes    int       `db:"max_mailboxes" json:"max_mailboxes"`
	TLSEnabled      bool      `db:"tls_enabled" json:"tls_enabled"`
	CertPath        string    `db:"cert_path" json:"cert_path"`
	KeyPath         string    `db:"key_path" json:"key_path"`
	IsRunning       bool      `db:"is_running" json:"is_running"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

// Request/Response types
type CreateDomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

type CreateAccountRequest struct {
	DomainID uuid.UUID `json:"domain_id" binding:"required"`
	Email    string    `json:"email" binding:"required"`
	Password string    `json:"password" binding:"required,min=6"`
	QuotaMB  int       `json:"quota_mb"`
}

type UpdateAccountRequest struct {
	QuotaMB      int    `json:"quota_mb"`
	IsActive     *bool  `json:"is_active"`
	ForwardTo    string `json:"forward_to"`
	AutoReply    *bool  `json:"auto_reply"`
	AutoReplyMsg string `json:"auto_reply_msg"`
}

type CreateAliasRequest struct {
	DomainID    uuid.UUID `json:"domain_id" binding:"required"`
	Source      string    `json:"source" binding:"required"`
	Destination string    `json:"destination" binding:"required"`
}

type UpdateSpamFilterRequest struct {
	Enabled       *bool    `json:"enabled"`
	SpamThreshold *float64 `json:"spam_threshold"`
	RejectScore   *float64 `json:"reject_score"`
	Greylisting   *bool    `json:"greylisting"`
	Blacklist     []string `json:"blacklist"`
	Whitelist     []string `json:"whitelist"`
}

type UpdateServerConfigRequest struct {
	Hostname       string `json:"hostname"`
	SMTPPort       *int   `json:"smtp_port"`
	SMTPSPort      *int   `json:"smtps_port"`
	IMAPPort       *int   `json:"imap_port"`
	IMAPSPort      *int   `json:"imaps_port"`
	MaxMessageSize *int   `json:"max_message_size"`
	TLSEnabled     *bool  `json:"tls_enabled"`
	CertPath       string `json:"cert_path"`
	KeyPath        string `json:"key_path"`
}

type MailStats struct {
	TotalDomains   int `json:"total_domains"`
	TotalAccounts  int `json:"total_accounts"`
	TotalAliases   int `json:"total_aliases"`
	QueueSize      int `json:"queue_size"`
	SentToday      int `json:"sent_today"`
	ReceivedToday  int `json:"received_today"`
	FailedToday    int `json:"failed_today"`
	StorageUsedMB  int `json:"storage_used_mb"`
}
