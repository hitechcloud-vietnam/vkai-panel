package models

import (
	"time"

	"github.com/google/uuid"
)

// Notification represents a notification
type Notification struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID    *uuid.UUID `json:"user_id" db:"user_id"`
	Type      string     `json:"type" db:"type"`
	Title     string     `json:"title" db:"title"`
	Message   string     `json:"message" db:"message"`
	Details   JSONMap    `json:"details" db:"details"`
	IsRead    bool       `json:"is_read" db:"is_read"`
	ReadAt    *time.Time `json:"read_at" db:"read_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name      string    `json:"name" db:"name"`
	Type      string    `json:"type" db:"type"`
	Subject   string    `json:"subject" db:"subject"`
	Body      string    `json:"body" db:"body"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// NotificationChannel represents a notification channel
type NotificationChannel struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name      string    `json:"name" db:"name"`
	Type      string    `json:"type" db:"type"`
	Config    JSONMap   `json:"config" db:"config"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// NotificationPreference represents user notification preferences
type NotificationPreference struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Type      string    `json:"type" db:"type"`
	Channel   string    `json:"channel" db:"channel"`
	Enabled   bool      `json:"enabled" db:"enabled"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CreateNotificationRequest represents a request to create a notification
type CreateNotificationRequest struct {
	UserID  *uuid.UUID `json:"user_id"`
	Type    string     `json:"type" binding:"required"`
	Title   string     `json:"title" binding:"required"`
	Message string     `json:"message" binding:"required"`
	Details JSONMap    `json:"details"`
}

// CreateNotificationTemplateRequest represents a request to create a notification template
type CreateNotificationTemplateRequest struct {
	Name    string `json:"name" binding:"required"`
	Type    string `json:"type" binding:"required"`
	Subject string `json:"subject" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

// UpdateNotificationTemplateRequest represents a request to update a notification template
type UpdateNotificationTemplateRequest struct {
	Name     *string `json:"name"`
	Type     *string `json:"type"`
	Subject  *string `json:"subject"`
	Body     *string `json:"body"`
	IsActive *bool   `json:"is_active"`
}

// CreateNotificationChannelRequest represents a request to create a notification channel
type CreateNotificationChannelRequest struct {
	Name   string  `json:"name" binding:"required"`
	Type   string  `json:"type" binding:"required"`
	Config JSONMap `json:"config" binding:"required"`
}

// UpdateNotificationChannelRequest represents a request to update a notification channel
type UpdateNotificationChannelRequest struct {
	Name     *string  `json:"name"`
	Type     *string  `json:"type"`
	Config   *JSONMap `json:"config"`
	IsActive *bool    `json:"is_active"`
}

// UpdateNotificationPreferenceRequest represents a request to update notification preferences
type UpdateNotificationPreferenceRequest struct {
	Type    string `json:"type" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Enabled bool   `json:"enabled"`
}
