package rbac

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// System roles
const (
	RoleSuperAdmin  = "super_admin"
	RoleAdmin       = "admin"
	RoleServerAdmin = "server_admin"
	RoleWebAdmin    = "web_admin"
	RoleDBAdmin     = "database_admin"
	RoleDeveloper   = "developer"
	RoleOperator    = "operator"
	RoleViewer      = "viewer"
)

// Permissions
const (
	PermServerRead    = "server.read"
	PermServerWrite   = "server.write"
	PermWebsiteRead   = "website.read"
	PermWebsiteWrite  = "website.write"
	PermDBRead        = "database.read"
	PermDBWrite       = "database.write"
	PermDNSRead       = "dns.read"
	PermDNSWrite      = "dns.write"
	PermSSLRead       = "ssl.read"
	PermSSLWrite      = "ssl.write"
	PermDockerRead    = "docker.read"
	PermDockerWrite   = "docker.write"
	PermTerminalExec  = "terminal.execute"
	PermBackupRead    = "backup.read"
	PermBackupWrite   = "backup.write"
	PermSettingsWrite = "settings.write"
	PermUserRead      = "user.read"
	PermUserWrite     = "user.write"
	PermAuditRead     = "audit.read"

	// Panel access settings: the port, the security entrance, the IP allow
	// list, the pinned host name and the panel's own TLS certificate. These are
	// held apart from settings.write because they govern who can reach the
	// panel at all - a mistake here locks every administrator out of the
	// machine, so they are granted to administrators only.
	PermPanelRead  = "panel.read"
	PermPanelWrite = "panel.write"
)

// Permission is one row of the permissions table.
type Permission struct {
	Resource    string `json:"resource" db:"resource"`
	Action      string `json:"action" db:"action"`
	Description string `json:"description" db:"description"`

	// AdminOnly marks a permission that must never be granted to a
	// non-administrative role, however a custom role is assembled.
	AdminOnly bool `json:"admin_only" db:"-"`
}

// Name is the "resource.action" form used by the permission checks, matching
// how the database composes it.
func (p Permission) Name() string { return p.Resource + "." + p.Action }

// SystemPermissions is the catalogue of built-in permissions. It is the source
// of truth for what may appear in the permissions table.
var SystemPermissions = []Permission{
	{Resource: "server", Action: "read", Description: "View servers and their metrics"},
	{Resource: "server", Action: "write", Description: "Create, change and remove servers"},
	{Resource: "website", Action: "read", Description: "View websites and their files"},
	{Resource: "website", Action: "write", Description: "Create, change and remove websites"},
	{Resource: "database", Action: "read", Description: "View database servers and databases"},
	{Resource: "database", Action: "write", Description: "Create, change and remove databases"},
	{Resource: "dns", Action: "read", Description: "View DNS zones and records"},
	{Resource: "dns", Action: "write", Description: "Create, change and remove DNS records"},
	{Resource: "ssl", Action: "read", Description: "View website certificates"},
	{Resource: "ssl", Action: "write", Description: "Issue, upload and remove website certificates"},
	{Resource: "docker", Action: "read", Description: "View containers, images, networks and volumes"},
	{Resource: "docker", Action: "write", Description: "Run and remove containers and images"},
	{Resource: "terminal", Action: "execute", Description: "Run commands on the host"},
	{Resource: "backup", Action: "read", Description: "View backup jobs and records"},
	{Resource: "backup", Action: "write", Description: "Create and run backups, and restore from them"},
	{Resource: "settings", Action: "write", Description: "Change application settings"},
	{Resource: "user", Action: "read", Description: "View users and API keys"},
	{Resource: "user", Action: "write", Description: "Create, change and remove users and API keys"},
	{Resource: "audit", Action: "read", Description: "Read the audit log"},
	{Resource: "panel", Action: "read", Description: "View the panel access settings", AdminOnly: true},
	{Resource: "panel", Action: "write", Description: "Change the panel port, security entrance, IP allow list and TLS", AdminOnly: true},
}

// AdminOnlyPermissions lists the permissions reserved for administrative roles.
func AdminOnlyPermissions() []Permission {
	out := make([]Permission, 0, 2)
	for _, permission := range SystemPermissions {
		if permission.AdminOnly {
			out = append(out, permission)
		}
	}
	return out
}

type PermissionChecker struct {
	db *sqlx.DB
}

func NewPermissionChecker(db *sqlx.DB) *PermissionChecker {
	return &PermissionChecker{db: db}
}

func (pc *PermissionChecker) HasPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM role_permissions rp
		JOIN user_roles ur ON ur.role_id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1 AND p.resource || '.' || p.action = $2
	`
	err := pc.db.QueryRowContext(ctx, query, userID, permission).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (pc *PermissionChecker) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var permissions []string
	query := `
		SELECT DISTINCT p.resource || '.' || p.action
		FROM role_permissions rp
		JOIN user_roles ur ON ur.role_id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
	`
	err := pc.db.SelectContext(ctx, &permissions, query, userID)
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

func (pc *PermissionChecker) AssignRole(ctx context.Context, userID, roleID uuid.UUID) error {
	query := `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := pc.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (pc *PermissionChecker) RemoveRole(ctx context.Context, userID, roleID uuid.UUID) error {
	query := `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`
	_, err := pc.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (pc *PermissionChecker) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var roles []string
	query := `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
	`
	err := pc.db.SelectContext(ctx, &roles, query, userID)
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// RequirePermission is a helper to check permission and return error
func (pc *PermissionChecker) RequirePermission(ctx context.Context, userID uuid.UUID, permission string) error {
	has, err := pc.HasPermission(ctx, userID, permission)
	if err != nil {
		return err
	}
	if !has {
		return ErrPermissionDenied
	}
	return nil
}

var ErrPermissionDenied = &PermissionError{"permission denied"}

type PermissionError struct {
	Message string
}

func (e *PermissionError) Error() string {
	return e.Message
}
