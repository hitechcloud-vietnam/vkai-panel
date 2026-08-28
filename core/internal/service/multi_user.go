package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type MultiUserService struct {
	repo   *repository.MultiUserRepository
	logger *zap.Logger
}

func NewMultiUserService(repo *repository.MultiUserRepository, logger *zap.Logger) *MultiUserService {
	return &MultiUserService{repo: repo, logger: logger}
}

// Roles
func (s *MultiUserService) CreateRole(ctx context.Context, tenantID uuid.UUID, req models.CreateRoleRequest) (*models.Role, error) {
	role := &models.Role{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		IsSystem:    false,
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}

	// Set permissions if provided
	if len(req.Permissions) > 0 {
		var permIDs []uuid.UUID
		for _, p := range req.Permissions {
			// Format: "resource:action"
			parts := splitPermission(p)
			if len(parts) == 2 {
				perm, err := s.repo.GetPermissionByName(ctx, parts[0], parts[1])
				if err == nil {
					permIDs = append(permIDs, perm.ID)
				}
			}
		}
		if len(permIDs) > 0 {
			_ = s.repo.SetRolePermissions(ctx, role.ID, permIDs)
		}
	}

	return role, nil
}

func (s *MultiUserService) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]models.RoleWithPermissions, error) {
	roles, err := s.repo.ListRoles(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var result []models.RoleWithPermissions
	for _, r := range roles {
		perms, _ := s.repo.GetRolePermissions(ctx, r.ID)
		result = append(result, models.RoleWithPermissions{Role: r, Permissions: perms})
	}
	return result, nil
}

func (s *MultiUserService) GetRole(ctx context.Context, id uuid.UUID) (*models.RoleWithPermissions, error) {
	role, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return nil, err
	}
	perms, _ := s.repo.GetRolePermissions(ctx, role.ID)
	return &models.RoleWithPermissions{Role: *role, Permissions: perms}, nil
}

func (s *MultiUserService) UpdateRole(ctx context.Context, id uuid.UUID, req models.UpdateRoleRequest) (*models.Role, error) {
	role, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Description != "" {
		role.Description = req.Description
	}
	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}

	// Update permissions if provided
	if req.Permissions != nil {
		var permIDs []uuid.UUID
		for _, p := range req.Permissions {
			parts := splitPermission(p)
			if len(parts) == 2 {
				perm, err := s.repo.GetPermissionByName(ctx, parts[0], parts[1])
				if err == nil {
					permIDs = append(permIDs, perm.ID)
				}
			}
		}
		_ = s.repo.SetRolePermissions(ctx, role.ID, permIDs)
	}

	return role, nil
}

func (s *MultiUserService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteRole(ctx, id)
}

// Permissions
func (s *MultiUserService) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	return s.repo.ListPermissions(ctx)
}

// User-Role
func (s *MultiUserService) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.repo.AssignUserRole(ctx, userID, roleID)
}

func (s *MultiUserService) RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.repo.RemoveUserRole(ctx, userID, roleID)
}

func (s *MultiUserService) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	return s.repo.GetUserRoles(ctx, userID)
}

func (s *MultiUserService) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	return s.repo.GetUserPermissions(ctx, userID)
}

// Sessions
func (s *MultiUserService) ListActiveSessions(ctx context.Context, tenantID uuid.UUID) ([]models.UserSession, error) {
	return s.repo.ListActiveSessions(ctx, tenantID)
}

func (s *MultiUserService) DeleteSession(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSession(ctx, id)
}

func (s *MultiUserService) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteUserSessions(ctx, userID)
}

// Activity
func (s *MultiUserService) LogActivity(ctx context.Context, tenantID, userID uuid.UUID, action, resource, details, ip string) error {
	activity := &models.UserActivity{
		ID:        uuid.New(),
		UserID:    userID,
		TenantID:  tenantID,
		Action:    action,
		Resource:  resource,
		Details:   details,
		IPAddress: ip,
	}
	return s.repo.LogActivity(ctx, activity)
}

func (s *MultiUserService) ListActivities(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, limit int) ([]models.UserActivity, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListActivities(ctx, tenantID, userID, limit)
}

// API Keys
//
// This used to mint its own key material. It generated "vkai_" plus two UUIDs,
// took the first EIGHT characters as the lookup prefix - a prefix every key
// shared, so no key could be found by it - and stored the key under a
// "hashAPIKey" that returned the key unchanged. Every key minted through this
// path was therefore a live credential sitting in plain text in a column that
// goes into every backup and every database dump.
//
// It now uses the one minting path this process has (service/apikey.go), which
// generates 128 bits of entropy, stores an HMAC-SHA-256 digest under a
// server-side pepper, and uses the 12 character prefix the authentication
// lookup actually searches by.
//
// The scopes are validated here too, because a key minted through this path
// authenticates through the same middleware as any other and must not be able
// to carry a grant that path would have refused.
func (s *MultiUserService) CreateAPIKey(ctx context.Context, tenantID, userID uuid.UUID, req models.CreateAPIKeyRequest) (*models.APIKey, string, error) {
	scopes, err := auth.ParseScopeSet(req.Scopes)
	if err != nil {
		return nil, "", err
	}

	rawKey, keyHash, keyPrefix, err := MintAPIKeyMaterial()
	if err != nil {
		return nil, "", err
	}

	expiresAt := req.ExpiresAt
	if expiresAt == nil {
		deadline := time.Now().Add(DefaultAPIKeyLifetime)
		expiresAt = &deadline
	}

	apiKey := &models.APIKey{
		ID:        uuid.New(),
		UserID:    userID,
		TenantID:  tenantID,
		Name:      req.Name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Scopes:    scopes.Strings(),
		ExpiresAt: expiresAt,
		Status:    "active",
	}
	if err := s.repo.CreateAPIKey(ctx, apiKey); err != nil {
		return nil, "", err
	}
	return apiKey, rawKey, nil
}

func (s *MultiUserService) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error) {
	return s.repo.ListAPIKeys(ctx, userID)
}

func (s *MultiUserService) DeleteAPIKey(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.DeleteAPIKey(ctx, id, userID)
}

// Stats
func (s *MultiUserService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.MultiUserStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}

// Helpers
func splitPermission(p string) []string {
	for i := 0; i < len(p); i++ {
		if p[i] == ':' {
			return []string{p[:i], p[i+1:]}
		}
	}
	return nil
}
