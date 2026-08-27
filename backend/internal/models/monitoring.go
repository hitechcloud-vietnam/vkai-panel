package models

import (
	"time"

	"github.com/google/uuid"
)

// MonitoringMetric represents a monitoring metric data point
type MonitoringMetric struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ServerID  uuid.UUID `json:"server_id" db:"server_id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Metric    string    `json:"metric" db:"metric"`
	Value     float64   `json:"value" db:"value"`
	Unit      string    `json:"unit" db:"unit"`
	Tags      JSONMap   `json:"tags" db:"tags"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

// MonitoringAlert represents a monitoring alert
type MonitoringAlert struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID    *uuid.UUID `json:"server_id" db:"server_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Metric      string    `json:"metric" db:"metric"`
	Condition   string    `json:"condition" db:"condition"`
	Threshold   float64   `json:"threshold" db:"threshold"`
	Duration    int       `json:"duration" db:"duration"`
	Severity    string    `json:"severity" db:"severity"`
	Status      string    `json:"status" db:"status"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	LastTriggeredAt *time.Time `json:"last_triggered_at" db:"last_triggered_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// MonitoringAlertLog represents an alert log entry
type MonitoringAlertLog struct {
	ID        uuid.UUID `json:"id" db:"id"`
	AlertID   uuid.UUID `json:"alert_id" db:"alert_id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ServerID  *uuid.UUID `json:"server_id" db:"server_id"`
	Value     float64   `json:"value" db:"value"`
	Message   string    `json:"message" db:"message"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// MonitoringDashboard represents a monitoring dashboard
type MonitoringDashboard struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Layout      JSONMap   `json:"layout" db:"layout"`
	Widgets     JSONMap   `json:"widgets" db:"widgets"`
	IsDefault   bool      `json:"is_default" db:"is_default"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateMonitoringAlertRequest represents a request to create a monitoring alert
type CreateMonitoringAlertRequest struct {
	ServerID    string  `json:"server_id"`
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Metric      string  `json:"metric" binding:"required"`
	Condition   string  `json:"condition" binding:"required"`
	Threshold   float64 `json:"threshold" binding:"required"`
	Duration    int     `json:"duration"`
	Severity    string  `json:"severity" binding:"required"`
}

// UpdateMonitoringAlertRequest represents a request to update a monitoring alert
type UpdateMonitoringAlertRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Metric      string  `json:"metric"`
	Condition   string  `json:"condition"`
	Threshold   float64 `json:"threshold"`
	Duration    int     `json:"duration"`
	Severity    string  `json:"severity"`
	IsActive    *bool   `json:"is_active"`
}

// CreateMonitoringDashboardRequest represents a request to create a monitoring dashboard
type CreateMonitoringDashboardRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Layout      JSONMap `json:"layout"`
	Widgets     JSONMap `json:"widgets"`
	IsDefault   bool   `json:"is_default"`
}

// UpdateMonitoringDashboardRequest represents a request to update a monitoring dashboard
type UpdateMonitoringDashboardRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Layout      JSONMap `json:"layout"`
	Widgets     JSONMap `json:"widgets"`
	IsDefault   *bool  `json:"is_default"`
}
