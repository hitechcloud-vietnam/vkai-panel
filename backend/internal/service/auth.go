package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type AuthService struct {
	userRepo    *repository.UserRepository
	tenantRepo  *repository.TenantRepository
	jwtManager  *auth.JWTManager
	logger      *zap.Logger
}

func NewAuthService(
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		jwtManager: jwtManager,
		logger:     logger,
	}
}

func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, ip string) (*models.LoginResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if user.Status != "active" {
		return nil, fmt.Errorf("account is not active")
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Get role IDs
	roleIDs := []string{} // TODO: fetch from RBAC

	tokenPair, err := s.jwtManager.GenerateTokenPair(
		user.ID, user.TenantID, user.Username, user.Email, roleIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID, ip)

	return &models.LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    tokenPair.TokenType,
		User:         *user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.TokenPair, error) {
	claims, err := s.jwtManager.ValidateToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	roleIDs := []string{} // TODO: fetch from RBAC

	return s.jwtManager.GenerateTokenPair(
		user.ID, user.TenantID, user.Username, user.Email, roleIDs,
	)
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}
