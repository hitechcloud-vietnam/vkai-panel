package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailCampaign represents an email marketing campaign
type EmailCampaign struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Subject     string     `json:"subject" db:"subject"`
	HTMLContent string     `json:"html_content" db:"html_content"`
	PlainText   string     `json:"plain_text" db:"plain_text"`
	Status      string     `json:"status" db:"status"` // draft, scheduled, sending, sent, paused, cancelled
	ScheduledAt *time.Time `json:"scheduled_at" db:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at" db:"sent_at"`
	TotalRecipients int    `json:"total_recipients" db:"total_recipients"`
	SentCount       int    `json:"sent_count" db:"sent_count"`
	OpenCount       int    `json:"open_count" db:"open_count"`
	ClickCount      int    `json:"click_count" db:"click_count"`
	BounceCount     int    `json:"bounce_count" db:"bounce_count"`
	UnsubscribeCount int   `json:"unsubscribe_count" db:"unsubscribe_count"`
	FromName    string     `json:"from_name" db:"from_name"`
	FromEmail   string     `json:"from_email" db:"from_email"`
	ReplyTo     string     `json:"reply_to" db:"reply_to"`
	Tags        []string   `json:"tags" db:"tags"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// EmailContact represents an email subscriber/contact
type EmailContact struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Email       string     `json:"email" db:"email"`
	FirstName   string     `json:"first_name" db:"first_name"`
	LastName    string     `json:"last_name" db:"last_name"`
	Status      string     `json:"status" db:"status"` // active, unsubscribed, bounced, complained
	Source      string     `json:"source" db:"source"` // manual, import, api, signup_form
	Tags        []string   `json:"tags" db:"tags"`
	Metadata    map[string]string `json:"metadata" db:"metadata"`
	ConfirmedAt *time.Time `json:"confirmed_at" db:"confirmed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// EmailList represents a mailing list
type EmailList struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	ContactCount int       `json:"contact_count" db:"contact_count"`
	DoubleOptIn bool       `json:"double_opt_in" db:"double_opt_in"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// EmailListContact is the join table between lists and contacts
type EmailListContact struct {
	ListID    uuid.UUID `json:"list_id" db:"list_id"`
	ContactID uuid.UUID `json:"contact_id" db:"contact_id"`
	AddedAt   time.Time `json:"added_at" db:"added_at"`
}

// EmailTemplate represents a reusable email template
type EmailTemplate struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Subject     string     `json:"subject" db:"subject"`
	HTMLContent string     `json:"html_content" db:"html_content"`
	Category    string     `json:"category" db:"category"`
	Thumbnail   string     `json:"thumbnail" db:"thumbnail"`
	IsDefault   bool       `json:"is_default" db:"is_default"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// EmailSendLog tracks individual email sends
type EmailSendLog struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	CampaignID uuid.UUID  `json:"campaign_id" db:"campaign_id"`
	ContactID  uuid.UUID  `json:"contact_id" db:"contact_id"`
	Email      string     `json:"email" db:"email"`
	Status     string     `json:"status" db:"status"` // queued, sent, delivered, opened, clicked, bounced, complained
	SentAt     *time.Time `json:"sent_at" db:"sent_at"`
	OpenedAt   *time.Time `json:"opened_at" db:"opened_at"`
	ClickedAt  *time.Time `json:"clicked_at" db:"clicked_at"`
	BouncedAt  *time.Time `json:"bounced_at" db:"bounced_at"`
	ErrorMsg   string     `json:"error_msg" db:"error_msg"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// EmailAutomation represents an automated email workflow
type EmailAutomation struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	TriggerType string     `json:"trigger_type" db:"trigger_type"` // contact_added, tag_added, date_based, api
	TriggerConfig string   `json:"trigger_config" db:"trigger_config"`
	Status      string     `json:"status" db:"status"` // active, paused, draft
	EmailsSent  int        `json:"emails_sent" db:"emails_sent"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

// Request/Response types
type CreateCampaignRequest struct {
	Name        string   `json:"name" binding:"required"`
	Subject     string   `json:"subject" binding:"required"`
	HTMLContent string   `json:"html_content" binding:"required"`
	PlainText   string   `json:"plain_text"`
	FromName    string   `json:"from_name" binding:"required"`
	FromEmail   string   `json:"from_email" binding:"required,email"`
	ReplyTo     string   `json:"reply_to"`
	Tags        []string `json:"tags"`
	ListIDs     []string `json:"list_ids"`
	ScheduledAt *string  `json:"scheduled_at"`
}

type UpdateCampaignRequest struct {
	Name        *string  `json:"name"`
	Subject     *string  `json:"subject"`
	HTMLContent *string  `json:"html_content"`
	PlainText   *string  `json:"plain_text"`
	FromName    *string  `json:"from_name"`
	FromEmail   *string  `json:"from_email"`
	ReplyTo     *string  `json:"reply_to"`
	Tags        []string `json:"tags"`
	ScheduledAt *string  `json:"scheduled_at"`
}

type CreateContactRequest struct {
	Email     string            `json:"email" binding:"required,email"`
	FirstName string            `json:"first_name"`
	LastName  string            `json:"last_name"`
	Tags      []string          `json:"tags"`
	ListIDs   []string          `json:"list_ids"`
	Source    string            `json:"source"`
	Metadata  map[string]string `json:"metadata"`
}

type CreateListRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	DoubleOptIn bool   `json:"double_opt_in"`
}

type CreateTemplateRequest struct {
	Name        string `json:"name" binding:"required"`
	Subject     string `json:"subject" binding:"required"`
	HTMLContent string `json:"html_content" binding:"required"`
	Category    string `json:"category"`
}

type EmailStats struct {
	TotalCampaigns   int     `json:"total_campaigns"`
	TotalContacts    int     `json:"total_contacts"`
	TotalLists       int     `json:"total_lists"`
	TotalSent        int     `json:"total_sent"`
	TotalOpened      int     `json:"total_opened"`
	TotalClicked     int     `json:"total_clicked"`
	TotalBounced     int     `json:"total_bounced"`
	AvgOpenRate      float64 `json:"avg_open_rate"`
	AvgClickRate     float64 `json:"avg_click_rate"`
	AvgBounceRate    float64 `json:"avg_bounce_rate"`
}
