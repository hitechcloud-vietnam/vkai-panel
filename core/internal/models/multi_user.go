package models

import (
	"time"

	"github.com/google/uuid"
)

// Role management
type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=100"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"` // ["resource:action", ...]
}

type UpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type RoleWithPermissions struct {
	Role
	Permissions []Permission `json:"permissions"`
}

// User-Role assignment
type AssignRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// User session
type UserSession struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	UserID       uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID     uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	TokenHash    string     `json:"-" db:"token_hash"`
	IPAddress    string     `json:"ip_address" db:"ip_address"`
	UserAgent    string     `json:"user_agent" db:"user_agent"`
	ExpiresAt    time.Time  `json:"expires_at" db:"expires_at"`
	LastActiveAt time.Time  `json:"last_active_at" db:"last_active_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// User activity log
type UserActivity struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Action    string     `json:"action" db:"action"`
	Resource  string     `json:"resource" db:"resource"`
	Details   string     `json:"details" db:"details"`
	IPAddress string     `json:"ip_address" db:"ip_address"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// User with roles
type UserWithRoles struct {
	User
	Roles []Role `json:"roles"`
}

// Multi-user stats
type MultiUserStats struct {
	TotalUsers     int64 `json:"total_users" db:"total_users"`
	ActiveUsers    int64 `json:"active_users" db:"active_users"`
	OnlineUsers    int64 `json:"online_users" db:"online_users"`
	TotalRoles     int64 `json:"total_roles" db:"total_roles"`
	TotalSessions  int64 `json:"total_sessions" db:"total_sessions"`
	ActivitiesToday int64 `json:"activities_today" db:"activities_today"`
}
