package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// APIKeyService handles API key business logic
type APIKeyService struct {
	repo   *repository.APIKeyRepository
	logger *zap.Logger
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(repo *repository.APIKeyRepository, logger *zap.Logger) *APIKeyService {
	return &APIKeyService{
		repo:   repo,
		logger: logger,
	}
}

// APIKeyWithPlaintext holds an API key with its plaintext value (returned only once)
type APIKeyWithPlaintext struct {
	*models.APIKey
	Key string `json:"key"`
}

// CreateAPIKey generates a new API key, hashes it, stores it, and returns the full key once
func (s *APIKeyService) CreateAPIKey(ctx context.Context, tenantID, userID uuid.UUID, req *models.CreateAPIKeyRequest) (*APIKeyWithPlaintext, error) {
	// Generate random key: vk_live_ + 32 hex characters
	randomBytes := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	fullKey := "vk_live_" + hex.EncodeToString(randomBytes)

	// Key prefix is the first 12 characters: vk_live_xxxx
	keyPrefix := fullKey[:12]

	// Hash the full key with SHA256
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Set default status
	status := "active"

	apiKey := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		Name:      req.Name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		Status:    status,
	}

	if err := s.repo.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	s.logger.Info("API key created",
		zap.String("id", apiKey.ID.String()),
		zap.String("name", apiKey.Name),
		zap.String("tenant_id", tenantID.String()),
	)

	return &APIKeyWithPlaintext{
		APIKey: apiKey,
		Key:    fullKey,
	}, nil
}

// Get retrieves an API key by ID
func (s *APIKeyService) Get(ctx context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// List retrieves all API keys for a tenant with pagination
func (s *APIKeyService) List(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.APIKey, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage
	return s.repo.ListByTenant(ctx, tenantID, perPage, offset)
}

// Update updates an existing API key
func (s *APIKeyService) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateAPIKeyRequest) (*models.APIKey, error) {
	apiKey, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("API key not found: %w", err)
	}

	if req.Name != "" {
		apiKey.Name = req.Name
	}
	if req.Scopes != nil {
		apiKey.Scopes = req.Scopes
	}
	if req.Status != "" {
		apiKey.Status = req.Status
	}
	if req.ExpiresAt != nil {
		apiKey.ExpiresAt = req.ExpiresAt
	}

	if err := s.repo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to update API key: %w", err)
	}

	return apiKey, nil
}

// Delete removes an API key
func (s *APIKeyService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.Delete(ctx, tenantID, id)
}

// ValidateKey validates a full API key and returns the associated API key record
func (s *APIKeyService) ValidateKey(ctx context.Context, fullKey string) (*models.APIKey, error) {
	// Extract prefix (first 12 characters)
	if len(fullKey) < 12 {
		return nil, fmt.Errorf("invalid API key format")
	}
	keyPrefix := fullKey[:12]

	// Look up by prefix
	apiKey, err := s.repo.GetByKeyPrefix(ctx, keyPrefix)
	if err != nil {
		return nil, fmt.Errorf("API key not found")
	}

	// Check if key is expired
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Verify the full key against the stored hash
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])
	if keyHash != apiKey.KeyHash {
		return nil, fmt.Errorf("invalid API key")
	}

	// Update last used timestamp
	if err := s.repo.UpdateLastUsed(ctx, apiKey.ID); err != nil {
		s.logger.Warn("Failed to update API key last_used", zap.Error(err))
	}

	return apiKey, nil
}
