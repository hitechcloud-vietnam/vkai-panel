package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Daily Report
type DailyReport struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ReportDate  string     `json:"report_date" db:"report_date"` // YYYY-MM-DD
	ReportType  string     `json:"report_type" db:"report_type"` // daily, weekly, monthly
	Title       string     `json:"title" db:"title"`
	Summary     string     `json:"summary" db:"summary"`
	Sections    []ReportSection `json:"sections"`
	Status      string     `json:"status" db:"status"` // draft, generated, sent
	SentAt      *time.Time `json:"sent_at" db:"sent_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

type ReportSection struct {
	ID         uuid.UUID `json:"id" db:"id"`
	ReportID   uuid.UUID `json:"report_id" db:"report_id"`
	SectionKey string    `json:"section_key" db:"section_key"`
	Title      string    `json:"title" db:"title"`
	Content    string    `json:"content" db:"content"`
	DataJSON   string    `json:"data_json" db:"data_json"`
	SortOrder  int       `json:"sort_order" db:"sort_order"`
}

// Report schedule configuration
type ReportSchedule struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	ReportType  string     `json:"report_type" db:"report_type"` // daily, weekly, monthly
	Frequency   string     `json:"frequency" db:"frequency"`     // cron expression
	Recipients  pq.StringArray `json:"recipients" db:"recipients"`
	Sections    pq.StringArray `json:"sections" db:"sections"` // which sections to include
	IsActive    bool       `json:"is_active" db:"is_active"`
	LastSentAt  *time.Time `json:"last_sent_at" db:"last_sent_at"`
	NextSendAt  *time.Time `json:"next_send_at" db:"next_send_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// Report delivery log
type ReportDelivery struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	ReportID   uuid.UUID  `json:"report_id" db:"report_id"`
	ScheduleID *uuid.UUID `json:"schedule_id" db:"schedule_id"`
	Recipient  string     `json:"recipient" db:"recipient"`
	Channel    string     `json:"channel" db:"channel"` // email, webhook, slack
	Status     string     `json:"status" db:"status"`   // sent, failed, pending
	Error      string     `json:"error" db:"error"`
	SentAt     *time.Time `json:"sent_at" db:"sent_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// Requests
type CreateScheduleRequest struct {
	Name       string   `json:"name" binding:"required"`
	ReportType string   `json:"report_type" binding:"required"`
	Frequency  string   `json:"frequency" binding:"required"`
	Recipients []string `json:"recipients" binding:"required,min=1"`
	Sections   []string `json:"sections"`
}

type UpdateScheduleRequest struct {
	Name       string   `json:"name"`
	ReportType string   `json:"report_type"`
	Frequency  string   `json:"frequency"`
	Recipients []string `json:"recipients"`
	Sections   []string `json:"sections"`
	IsActive   *bool    `json:"is_active"`
}

// Stats
type DailyReportStats struct {
	TotalReports    int64  `json:"total_reports" db:"total_reports"`
	ReportsThisMonth int64 `json:"reports_this_month" db:"reports_this_month"`
	ActiveSchedules int64  `json:"active_schedules" db:"active_schedules"`
	TotalDeliveries int64  `json:"total_deliveries" db:"total_deliveries"`
	FailedDeliveries int64 `json:"failed_deliveries" db:"failed_deliveries"`
	LastReportDate  string `json:"last_report_date" db:"last_report_date"`
}

// Report data aggregations
type ServerHealthSummary struct {
	TotalServers  int     `json:"total_servers"`
	OnlineServers int     `json:"online_servers"`
	AvgCPU        float64 `json:"avg_cpu"`
	AvgMemory     float64 `json:"avg_memory"`
	AvgDisk       float64 `json:"avg_disk"`
}

type WebsiteSummary struct {
	TotalWebsites  int `json:"total_websites"`
	ActiveWebsites int `json:"active_websites"`
	SSLEnabled     int `json:"ssl_enabled"`
	SSLError       int `json:"ssl_error"`
}

type SecuritySummary struct {
	WAFBlocked     int `json:"waf_blocked"`
	FirewallDrops  int `json:"firewall_drops"`
	FailedLogins   int `json:"failed_logins"`
	Vulnerabilities int `json:"vulnerabilities"`
}

type BackupSummary struct {
	TotalBackups  int    `json:"total_backups"`
	Successful    int    `json:"successful"`
	Failed        int    `json:"failed"`
	TotalSize     string `json:"total_size"`
}

type ReportData struct {
	Server   ServerHealthSummary `json:"server"`
	Website  WebsiteSummary      `json:"website"`
	Security SecuritySummary     `json:"security"`
	Backup   BackupSummary       `json:"backup"`
}
