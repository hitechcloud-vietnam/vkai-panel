package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"go.uber.org/zap"
)

// ConfigService handles config business logic
type ConfigService struct {
	repo   *repository.ConfigRepository
	logger *zap.Logger
}

// NewConfigService creates a new config service
func NewConfigService(repo *repository.ConfigRepository, logger *zap.Logger) *ConfigService {
	return &ConfigService{
		repo:   repo,
		logger: logger,
	}
}

// CreateSnapshot creates a new config snapshot
func (s *ConfigService) CreateSnapshot(ctx context.Context, tenantID uuid.UUID, snapshot *config.ConfigSnapshot) error {
	snapshot.ID = uuid.New()
	snapshot.TenantID = tenantID
	snapshot.IsActive = true

	// Deactivate previous active snapshot
	active, err := s.repo.GetActiveSnapshot(ctx, snapshot.ConfigType, snapshot.Name, snapshot.ServerID)
	if err == nil && active != nil {
		active.IsActive = false
		if err := s.repo.SetActiveSnapshot(ctx, active.ID); err != nil {
			s.logger.Warn("Failed to deactivate previous snapshot", zap.Error(err))
		}
	}

	return s.repo.CreateSnapshot(ctx, snapshot)
}

// GetSnapshot retrieves a snapshot by ID
func (s *ConfigService) GetSnapshot(ctx context.Context, id uuid.UUID) (*config.ConfigSnapshot, error) {
	return s.repo.GetSnapshot(ctx, id)
}

// GetActiveSnapshot retrieves the active snapshot
func (s *ConfigService) GetActiveSnapshot(ctx context.Context, configType config.ConfigType, name string, serverID uuid.UUID) (*config.ConfigSnapshot, error) {
	return s.repo.GetActiveSnapshot(ctx, configType, name, serverID)
}

// ListSnapshots lists snapshots with filters
func (s *ConfigService) ListSnapshots(ctx context.Context, tenantID uuid.UUID, filter *config.ConfigFilter) ([]*config.ConfigSnapshot, int, error) {
	return s.repo.ListSnapshots(ctx, tenantID, filter)
}

// Rollback performs a rollback to a specific snapshot
func (s *ConfigService) Rollback(ctx context.Context, tenantID uuid.UUID, req *config.ConfigRollbackRequest) (*config.ConfigSnapshot, error) {
	// Get the target snapshot
	snapshot, err := s.repo.GetSnapshot(ctx, req.SnapshotID)
	if err != nil {
		return nil, fmt.Errorf("snapshot not found: %w", err)
	}

	// Verify tenant ownership
	if snapshot.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}

	// Create a new snapshot with the rolled-back content
	newSnapshot := &config.ConfigSnapshot{
		ConfigType:  snapshot.ConfigType,
		Name:        snapshot.Name,
		Path:        snapshot.Path,
		Content:     snapshot.Content,
		IsActive:    true,
		IsAutomatic: false,
		Description: fmt.Sprintf("Rollback to version %d: %s", snapshot.Version, req.Reason),
		ServerID:    snapshot.ServerID,
	}

	if err := s.CreateSnapshot(ctx, tenantID, newSnapshot); err != nil {
		return nil, fmt.Errorf("failed to create rollback snapshot: %w", err)
	}

	s.logger.Info("Config rolled back",
		zap.String("config_type", string(snapshot.ConfigType)),
		zap.String("name", snapshot.Name),
		zap.Int("target_version", snapshot.Version),
		zap.String("reason", req.Reason),
	)

	return newSnapshot, nil
}

// GetDiff returns differences between two config versions
func (s *ConfigService) GetDiff(ctx context.Context, id1, id2 uuid.UUID) (*config.ConfigDiff, error) {
	snapshot1, err := s.repo.GetSnapshot(ctx, id1)
	if err != nil {
		return nil, fmt.Errorf("snapshot 1 not found: %w", err)
	}

	snapshot2, err := s.repo.GetSnapshot(ctx, id2)
	if err != nil {
		return nil, fmt.Errorf("snapshot 2 not found: %w", err)
	}

	diff := &config.ConfigDiff{
		OldVersion: snapshot1.Version,
		NewVersion: snapshot2.Version,
		OldContent: snapshot1.Content,
		NewContent: snapshot2.Content,
		CreatedAt:  snapshot2.CreatedAt,
	}

	// Simple line-by-line diff
	oldLines := strings.Split(snapshot1.Content, "\n")
	newLines := strings.Split(snapshot2.Content, "\n")

	oldSet := make(map[string]bool)
	for _, line := range oldLines {
		oldSet[line] = true
	}

	newSet := make(map[string]bool)
	for _, line := range newLines {
		newSet[line] = true
	}

	// Find additions (in new but not in old)
	for _, line := range newLines {
		if !oldSet[line] {
			diff.Additions = append(diff.Additions, line)
		}
	}

	// Find deletions (in old but not in new)
	for _, line := range oldLines {
		if !newSet[line] {
			diff.Deletions = append(diff.Deletions, line)
		}
	}

	return diff, nil
}

// GetSnapshotHistory returns version history
func (s *ConfigService) GetSnapshotHistory(ctx context.Context, configType config.ConfigType, name string, serverID uuid.UUID, limit int) ([]*config.ConfigSnapshot, error) {
	return s.repo.GetSnapshotHistory(ctx, configType, name, serverID, limit)
}

// GetConfigStats returns config statistics
func (s *ConfigService) GetConfigStats(ctx context.Context, tenantID uuid.UUID) (*config.ConfigStats, error) {
	return s.repo.GetConfigStats(ctx, tenantID)
}

// DeleteSnapshot deletes a snapshot
func (s *ConfigService) DeleteSnapshot(ctx context.Context, id uuid.UUID) error {
	snapshot, err := s.repo.GetSnapshot(ctx, id)
	if err != nil {
		return err
	}

	if snapshot.IsActive {
		return fmt.Errorf("cannot delete active snapshot")
	}

	return s.repo.DeleteSnapshot(ctx, id)
}

// CleanupOldSnapshots cleans up old snapshots
func (s *ConfigService) CleanupOldSnapshots(ctx context.Context, tenantID uuid.UUID, keepVersions int) (int64, error) {
	return s.repo.CleanupOldSnapshots(ctx, tenantID, keepVersions)
}

// ValidateConfig validates configuration content
func (s *ConfigService) ValidateConfig(configType config.ConfigType, content string) *config.ConfigValidation {
	validation := &config.ConfigValidation{
		IsValid:  true,
		Errors:   []string{},
		Warnings: []string{},
	}

	switch configType {
	case config.ConfigTypeNginx:
		s.validateNginx(content, validation)
	case config.ConfigTypeApache:
		s.validateApache(content, validation)
	case config.ConfigTypePHP:
		s.validatePHP(content, validation)
	case config.ConfigTypeMySQL:
		s.validateMySQL(content, validation)
	case config.ConfigTypePostgreSQL:
		s.validatePostgreSQL(content, validation)
	}

	return validation
}

// validateNginx validates nginx configuration
func (s *ConfigService) validateNginx(content string, validation *config.ConfigValidation) {
	// Check for common nginx config issues
	if !strings.Contains(content, "server {") && !strings.Contains(content, "http {") {
		validation.Warnings = append(validation.Warnings, "No server or http block found")
	}

	// Check for unclosed braces
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		validation.Errors = append(validation.Errors, "Mismatched braces")
		validation.IsValid = false
	}
}

// validateApache validates apache configuration
func (s *ConfigService) validateApache(content string, validation *config.ConfigValidation) {
	// Check for common apache config issues
	if !strings.Contains(content, "<VirtualHost") {
		validation.Warnings = append(validation.Warnings, "No VirtualHost block found")
	}

	// Check for unclosed tags
	openTags := strings.Count(content, "<")
	closeTags := strings.Count(content, "</")
	if openTags != closeTags*2 {
		validation.Warnings = append(validation.Warnings, "Possible unclosed XML tags")
	}
}

// validatePHP validates PHP configuration
func (s *ConfigService) validatePHP(content string, validation *config.ConfigValidation) {
	// Check for common PHP config issues
	if !strings.Contains(content, "[PHP]") {
		validation.Warnings = append(validation.Warnings, "No [PHP] section found")
	}
}

// validateMySQL validates MySQL configuration
func (s *ConfigService) validateMySQL(content string, validation *config.ConfigValidation) {
	// Check for common MySQL config issues
	if !strings.Contains(content, "[mysqld]") {
		validation.Warnings = append(validation.Warnings, "No [mysqld] section found")
	}
}

// validatePostgreSQL validates PostgreSQL configuration
func (s *ConfigService) validatePostgreSQL(content string, validation *config.ConfigValidation) {
	// Check for common PostgreSQL config issues
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			validation.Warnings = append(validation.Warnings, fmt.Sprintf("Line may not be a valid config: %s", line))
		}
	}
}

// Template operations
func (s *ConfigService) CreateTemplate(ctx context.Context, tenantID uuid.UUID, template *config.ConfigTemplate) error {
	template.ID = uuid.New()
	template.TenantID = tenantID
	return s.repo.CreateTemplate(ctx, template)
}

func (s *ConfigService) GetTemplate(ctx context.Context, id uuid.UUID) (*config.ConfigTemplate, error) {
	return s.repo.GetTemplate(ctx, id)
}

func (s *ConfigService) ListTemplates(ctx context.Context, tenantID uuid.UUID, configType config.ConfigType) ([]*config.ConfigTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID, configType)
}

func (s *ConfigService) UpdateTemplate(ctx context.Context, template *config.ConfigTemplate) error {
	return s.repo.UpdateTemplate(ctx, template)
}

func (s *ConfigService) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, id)
}
