package rbac

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// System roles
const (
	RoleSuperAdmin    = "super_admin"
	RoleAdmin         = "admin"
	RoleServerAdmin   = "server_admin"
	RoleWebAdmin      = "web_admin"
	RoleDBAdmin       = "database_admin"
	RoleDeveloper     = "developer"
	RoleOperator      = "operator"
	RoleViewer        = "viewer"
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
)

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
