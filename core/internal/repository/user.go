package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, tenant_id, username, email, password_hash, first_name, last_name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		user.ID, user.TenantID, user.Username, user.Email,
		user.PasswordHash, user.FirstName, user.LastName, user.Status,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM users WHERE username = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &user, query, username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	var users []models.User
	offset := (page - 1) * perPage
	query := `SELECT * FROM users WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &users, query, tenantID, perPage, offset); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// GetByIDInTenant looks a user up within one tenant. Every request-driven
// lookup uses this so a caller cannot address a user in another tenant.
func (r *UserRepository) GetByIDInTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error) {
	var user models.User
	query := `SELECT * FROM users WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &user, query, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users SET username=$1, email=$2, first_name=$3, last_name=$4,
		status=$5, updated_at=NOW()
		WHERE id=$6 AND tenant_id=$7 AND deleted_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query,
		user.Username, user.Email, user.FirstName, user.LastName,
		user.Status, user.ID, user.TenantID,
	)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, tenantID, id uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2 AND tenant_id=$3`
	_, err := r.db.ExecContext(ctx, query, passwordHash, id, tenantID)
	return err
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID, ip string) error {
	query := `UPDATE users SET last_login_at=NOW(), last_login_ip=$1 WHERE id=$2`
	_, err := r.db.ExecContext(ctx, query, ip, id)
	return err
}

func (r *UserRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `UPDATE users SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// GetRoleNames returns the names of the roles assigned to a user. Used to fill
// the role claim of a freshly issued token.
func (r *UserRepository) GetRoleNames(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var roles []string
	query := `SELECT r.name FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = $1 ORDER BY r.name`
	if err := r.db.SelectContext(ctx, &roles, query, userID); err != nil {
		return nil, fmt.Errorf("failed to load user roles: %w", err)
	}
	return roles, nil
}

// GetPermissionNames returns the effective "resource.action" permissions a user
// holds through their roles.
func (r *UserRepository) GetPermissionNames(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var perms []string
	query := `SELECT DISTINCT p.resource || '.' || p.action
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN user_roles ur ON ur.role_id = rp.role_id
		WHERE ur.user_id = $1`
	if err := r.db.SelectContext(ctx, &perms, query, userID); err != nil {
		return nil, fmt.Errorf("failed to load user permissions: %w", err)
	}
	return perms, nil
}
