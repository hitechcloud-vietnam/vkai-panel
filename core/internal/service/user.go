package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type UserService struct {
	userRepo   *repository.UserRepository
	tenantRepo *repository.TenantRepository
	logger     *zap.Logger
}

func NewUserService(userRepo *repository.UserRepository, tenantRepo *repository.TenantRepository, logger *zap.Logger) *UserService {
	return &UserService{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		logger:     logger,
	}
}

// Create provisions a user. callerTenantID is the tenant of the authenticated
// caller: a non-administrator may only create users inside it.
func (s *UserService) Create(ctx context.Context, req models.CreateUserRequest, callerTenantID uuid.UUID, callerIsAdmin bool) (*models.User, error) {
	if req.TenantID == uuid.Nil {
		req.TenantID = callerTenantID
	}
	if !callerIsAdmin && req.TenantID != callerTenantID {
		return nil, fmt.Errorf("cannot create a user in another tenant")
	}

	if err := utils.ValidatePasswordStrength(req.Password, "password"); err != nil {
		return nil, fmt.Errorf("%s", err.Message)
	}

	// Check if username exists
	existing, _ := s.userRepo.GetByUsername(ctx, req.Username)
	if existing != nil {
		return nil, fmt.Errorf("username already exists")
	}

	// Check if email exists
	existing, _ = s.userRepo.GetByEmail(ctx, req.Email)
	if existing != nil {
		return nil, fmt.Errorf("email already exists")
	}

	// Verify tenant exists
	_, err := s.tenantRepo.GetByID(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found")
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &models.User{
		ID:           uuid.New(),
		TenantID:     req.TenantID,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Status:       "active",
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Assign roles
	// TODO: implement role assignment

	return user, nil
}

func (s *UserService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByIDInTenant(ctx, tenantID, id)
}

func (s *UserService) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error) {
	return s.userRepo.ListByTenant(ctx, tenantID, page, perPage)
}

func (s *UserService) Update(ctx context.Context, user *models.User) error {
	return s.userRepo.Update(ctx, user)
}

func (s *UserService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.userRepo.Delete(ctx, tenantID, id)
}

func (s *UserService) ChangePassword(ctx context.Context, tenantID, id uuid.UUID, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByIDInTenant(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if !utils.CheckPassword(oldPassword, user.PasswordHash) {
		return fmt.Errorf("invalid current password")
	}

	if err := utils.ValidatePasswordStrength(newPassword, "password"); err != nil {
		return fmt.Errorf("%s", err.Message)
	}

	hash, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.userRepo.UpdatePassword(ctx, tenantID, id, hash)
}
