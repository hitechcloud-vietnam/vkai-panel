package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type MultiUserRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewMultiUserRepository(db *sqlx.DB, logger *zap.Logger) *MultiUserRepository {
	return &MultiUserRepository{db: db, logger: logger}
}

// ============================================================
// ROLE MANAGEMENT
// ============================================================

func (r *MultiUserRepository) CreateRole(ctx context.Context, role *models.Role) error {
	query := `INSERT INTO roles (id, tenant_id, name, description, is_system, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		role.ID, role.TenantID, role.Name, role.Description, role.IsSystem,
	).Scan(&role.CreatedAt, &role.UpdatedAt)
}

func (r *MultiUserRepository) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]models.Role, error) {
	var roles []models.Role
	query := `SELECT * FROM roles WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &roles, query, tenantID); err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	return roles, nil
}

func (r *MultiUserRepository) GetRole(ctx context.Context, id uuid.UUID) (*models.Role, error) {
	var role models.Role
	query := `SELECT * FROM roles WHERE id = $1`
	if err := r.db.GetContext(ctx, &role, query, id); err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}
	return &role, nil
}

func (r *MultiUserRepository) UpdateRole(ctx context.Context, role *models.Role) error {
	query := `UPDATE roles SET name=$1, description=$2, updated_at=NOW() WHERE id=$3`
	_, err := r.db.ExecContext(ctx, query, role.Name, role.Description, role.ID)
	return err
}

func (r *MultiUserRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	// Delete role permissions first
	_, _ = r.db.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, id)
	// Delete user roles
	_, _ = r.db.ExecContext(ctx, `DELETE FROM user_roles WHERE role_id=$1`, id)
	// Delete role
	_, err := r.db.ExecContext(ctx, `DELETE FROM roles WHERE id=$1 AND is_system=false`, id)
	return err
}

// ============================================================
// PERMISSION MANAGEMENT
// ============================================================

func (r *MultiUserRepository) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	var perms []models.Permission
	query := `SELECT * FROM permissions ORDER BY resource, action`
	if err := r.db.SelectContext(ctx, &perms, query); err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return perms, nil
}

func (r *MultiUserRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	var perms []models.Permission
	query := `SELECT p.* FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = $1 ORDER BY p.resource, p.action`
	if err := r.db.SelectContext(ctx, &perms, query, roleID); err != nil {
		return nil, fmt.Errorf("get role permissions: %w", err)
	}
	return perms, nil
}

func (r *MultiUserRepository) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, roleID); err != nil {
		return err
	}

	// Insert new
	for _, pid := range permissionIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`,
			roleID, pid,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *MultiUserRepository) GetPermissionByName(ctx context.Context, resource, action string) (*models.Permission, error) {
	var perm models.Permission
	query := `SELECT * FROM permissions WHERE resource=$1 AND action=$2`
	if err := r.db.GetContext(ctx, &perm, query, resource, action); err != nil {
		return nil, err
	}
	return &perm, nil
}

// ============================================================
// USER-ROLE ASSIGNMENT
// ============================================================

func (r *MultiUserRepository) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	query := `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (r *MultiUserRepository) RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	query := `DELETE FROM user_roles WHERE user_id=$1 AND role_id=$2`
	_, err := r.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (r *MultiUserRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	var roles []models.Role
	query := `SELECT r.* FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = $1 ORDER BY r.name`
	if err := r.db.SelectContext(ctx, &roles, query, userID); err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	return roles, nil
}

func (r *MultiUserRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	var perms []models.Permission
	query := `SELECT DISTINCT p.* FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN user_roles ur ON ur.role_id = rp.role_id
		WHERE ur.user_id = $1 ORDER BY p.resource, p.action`
	if err := r.db.SelectContext(ctx, &perms, query, userID); err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	return perms, nil
}

// ============================================================
// USER SESSIONS
// ============================================================

func (r *MultiUserRepository) CreateSession(ctx context.Context, session *models.UserSession) error {
	query := `INSERT INTO user_sessions (id, user_id, tenant_id, token_hash, ip_address, user_agent, expires_at, last_active_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.TenantID, session.TokenHash,
		session.IPAddress, session.UserAgent, session.ExpiresAt,
	)
	return err
}

func (r *MultiUserRepository) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]models.UserSession, error) {
	var sessions []models.UserSession
	query := `SELECT * FROM user_sessions WHERE user_id=$1 AND expires_at > NOW() ORDER BY last_active_at DESC`
	if err := r.db.SelectContext(ctx, &sessions, query, userID); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *MultiUserRepository) ListActiveSessions(ctx context.Context, tenantID uuid.UUID) ([]models.UserSession, error) {
	var sessions []models.UserSession
	query := `SELECT * FROM user_sessions WHERE tenant_id=$1 AND expires_at > NOW() ORDER BY last_active_at DESC`
	if err := r.db.SelectContext(ctx, &sessions, query, tenantID); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *MultiUserRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE id=$1`, id)
	return err
}

func (r *MultiUserRepository) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id=$1`, userID)
	return err
}

func (r *MultiUserRepository) CleanExpiredSessions(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE expires_at < NOW()`)
	return err
}

// ============================================================
// USER ACTIVITY LOG
// ============================================================

func (r *MultiUserRepository) LogActivity(ctx context.Context, activity *models.UserActivity) error {
	query := `INSERT INTO user_activities (id, user_id, tenant_id, action, resource, details, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`
	_, err := r.db.ExecContext(ctx, query,
		activity.ID, activity.UserID, activity.TenantID,
		activity.Action, activity.Resource, activity.Details, activity.IPAddress,
	)
	return err
}

func (r *MultiUserRepository) ListActivities(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, limit int) ([]models.UserActivity, error) {
	var activities []models.UserActivity
	var args []interface{}
	var conditions []string

	conditions = append(conditions, "tenant_id = $1")
	args = append(args, tenantID)

	if userID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, *userID)
	}

	query := fmt.Sprintf(`SELECT * FROM user_activities WHERE %s ORDER BY created_at DESC LIMIT $%d`,
		strings.Join(conditions, " AND "), len(args)+1)
	args = append(args, limit)

	if err := r.db.SelectContext(ctx, &activities, query, args...); err != nil {
		return nil, err
	}
	return activities, nil
}

// ============================================================
// API KEYS
// ============================================================

func (r *MultiUserRepository) CreateAPIKey(ctx context.Context, key *models.APIKey) error {
	query := `INSERT INTO api_keys (id, user_id, tenant_id, name, key_hash, key_prefix, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW()) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		key.ID, key.UserID, key.TenantID, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt,
	).Scan(&key.CreatedAt)
}

func (r *MultiUserRepository) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error) {
	var keys []models.APIKey
	query := `SELECT * FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &keys, query, userID); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *MultiUserRepository) DeleteAPIKey(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (r *MultiUserRepository) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	var key models.APIKey
	query := `SELECT * FROM api_keys WHERE key_hash=$1 AND (expires_at IS NULL OR expires_at > NOW())`
	if err := r.db.GetContext(ctx, &key, query, keyHash); err != nil {
		return nil, err
	}
	return &key, nil
}

// ============================================================
// STATS
// ============================================================

func (r *MultiUserRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.MultiUserStats, error) {
	var stats models.MultiUserStats

	err := r.db.GetContext(ctx, &stats, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE tenant_id=$1 AND deleted_at IS NULL) AS total_users,
			(SELECT COUNT(*) FROM users WHERE tenant_id=$1 AND status='active' AND deleted_at IS NULL) AS active_users,
			(SELECT COUNT(*) FROM user_sessions WHERE tenant_id=$1 AND expires_at > NOW()) AS online_users,
			(SELECT COUNT(*) FROM roles WHERE tenant_id=$1) AS total_roles,
			(SELECT COUNT(*) FROM user_sessions WHERE tenant_id=$1 AND expires_at > NOW()) AS total_sessions,
			(SELECT COUNT(*) FROM user_activities WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '1 day') AS activities_today
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get multi-user stats: %w", err)
	}
	return &stats, nil
}
