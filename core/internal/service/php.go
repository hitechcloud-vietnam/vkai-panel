package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type PHPService struct {
	phpRepo *repository.PHPRepository
	logger  *zap.Logger

	// rt is the host-side half of this service - the FPM manager and the
	// per-pool settings repository - built on first use. See php_runtime.go.
	rt phpRuntime
}

func NewPHPService(phpRepo *repository.PHPRepository, logger *zap.Logger) *PHPService {
	return &PHPService{
		phpRepo: phpRepo,
		logger:  logger,
	}
}

// CreatePHPVersion creates a new PHP version
func (s *PHPService) CreatePHPVersion(ctx context.Context, req *models.CreatePHPVersionRequest, tenantID string) (*models.PHPVersion, error) {
	php := &models.PHPVersion{
		ID:         uuid.New().String(),
		Version:    req.Version,
		Path:       req.Path,
		FPMPath:    req.FPMPath,
		FPMConfig:  req.FPMConfig,
		IniPath:    req.IniPath,
		Extensions: []string{},
		IsActive:   true,
		IsDefault:  false,
		ServerID:   req.ServerID,
		TenantID:   tenantID,
	}

	if err := s.phpRepo.CreatePHPVersion(ctx, php); err != nil {
		return nil, fmt.Errorf("failed to create PHP version: %w", err)
	}

	s.logger.Info("PHP version created",
		zap.String("id", php.ID),
		zap.String("version", php.Version),
		zap.String("server_id", php.ServerID),
	)

	return php, nil
}

// GetPHPVersion gets a PHP version by ID
func (s *PHPService) GetPHPVersion(ctx context.Context, id, tenantID string) (*models.PHPVersion, error) {
	return s.phpRepo.GetPHPVersion(ctx, id, tenantID)
}

// ListPHPVersions lists all PHP versions for a tenant
func (s *PHPService) ListPHPVersions(ctx context.Context, tenantID, serverID string) ([]*models.PHPVersion, error) {
	return s.phpRepo.ListPHPVersions(ctx, tenantID, serverID)
}

// UpdatePHPVersion updates a PHP version
func (s *PHPService) UpdatePHPVersion(ctx context.Context, id, tenantID string, req *models.UpdatePHPVersionRequest) (*models.PHPVersion, error) {
	php, err := s.phpRepo.GetPHPVersion(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP version: %w", err)
	}

	if req.IsActive != nil {
		php.IsActive = *req.IsActive
	}
	if req.IsDefault != nil {
		php.IsDefault = *req.IsDefault
	}

	if err := s.phpRepo.UpdatePHPVersion(ctx, php); err != nil {
		return nil, fmt.Errorf("failed to update PHP version: %w", err)
	}

	s.logger.Info("PHP version updated",
		zap.String("id", php.ID),
		zap.String("version", php.Version),
	)

	return php, nil
}

// DeletePHPVersion deletes a PHP version
func (s *PHPService) DeletePHPVersion(ctx context.Context, id, tenantID string) error {
	if err := s.phpRepo.DeletePHPVersion(ctx, id, tenantID); err != nil {
		return fmt.Errorf("failed to delete PHP version: %w", err)
	}

	s.logger.Info("PHP version deleted", zap.String("id", id))
	return nil
}

// CreatePHPPool creates a new PHP-FPM pool
func (s *PHPService) CreatePHPPool(ctx context.Context, req *models.CreatePHPPoolRequest, tenantID string) (*models.PHPPool, error) {
	pool := &models.PHPPool{
		ID:                    uuid.New().String(),
		Name:                  req.Name,
		PHPVersionID:          req.PHPVersionID,
		User:                  req.User,
		Group:                 req.Group,
		Listen:                req.Listen,
		ListenOwner:           req.ListenOwner,
		ListenGroup:           req.ListenGroup,
		ListenMode:            req.ListenMode,
		PM:                    req.PM,
		PMMaxChildren:         req.PMMaxChildren,
		PMStartServers:        req.PMStartServers,
		PMMinSpareServers:     req.PMMinSpareServers,
		PMMaxSpareServers:     req.PMMaxSpareServers,
		PMMaxRequests:         req.PMMaxRequests,
		PMProcessIdleTimeout:  req.PMProcessIdleTimeout,
		StatusPath:            req.StatusPath,
		AccessLog:             req.AccessLog,
		ErrorLog:              req.ErrorLog,
		PhpAdminFlag:          req.PhpAdminFlag,
		PhpValue:              req.PhpValue,
		PhpAdminValue:         req.PhpAdminValue,
		Env:                   req.Env,
		IsActive:              true,
		WebsiteID:             req.WebsiteID,
		ServerID:              req.ServerID,
		TenantID:              tenantID,
	}

	if err := s.phpRepo.CreatePHPPool(ctx, pool); err != nil {
		return nil, fmt.Errorf("failed to create PHP-FPM pool: %w", err)
	}

	s.logger.Info("PHP-FPM pool created",
		zap.String("id", pool.ID),
		zap.String("name", pool.Name),
		zap.String("php_version_id", pool.PHPVersionID),
	)

	return pool, nil
}

// GetPHPPool gets a PHP-FPM pool by ID
func (s *PHPService) GetPHPPool(ctx context.Context, id, tenantID string) (*models.PHPPool, error) {
	return s.phpRepo.GetPHPPool(ctx, id, tenantID)
}

// ListPHPPools lists all PHP-FPM pools for a tenant
func (s *PHPService) ListPHPPools(ctx context.Context, tenantID, serverID, websiteID string) ([]*models.PHPPool, error) {
	return s.phpRepo.ListPHPPools(ctx, tenantID, serverID, websiteID)
}

// UpdatePHPPool updates a PHP-FPM pool
func (s *PHPService) UpdatePHPPool(ctx context.Context, id, tenantID string, req *models.UpdatePHPPoolRequest) (*models.PHPPool, error) {
	pool, err := s.phpRepo.GetPHPPool(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP-FPM pool: %w", err)
	}

	if req.User != nil {
		pool.User = *req.User
	}
	if req.Group != nil {
		pool.Group = *req.Group
	}
	if req.Listen != nil {
		pool.Listen = *req.Listen
	}
	if req.ListenOwner != nil {
		pool.ListenOwner = *req.ListenOwner
	}
	if req.ListenGroup != nil {
		pool.ListenGroup = *req.ListenGroup
	}
	if req.ListenMode != nil {
		pool.ListenMode = *req.ListenMode
	}
	if req.PM != nil {
		pool.PM = *req.PM
	}
	if req.PMMaxChildren != nil {
		pool.PMMaxChildren = *req.PMMaxChildren
	}
	if req.PMStartServers != nil {
		pool.PMStartServers = *req.PMStartServers
	}
	if req.PMMinSpareServers != nil {
		pool.PMMinSpareServers = *req.PMMinSpareServers
	}
	if req.PMMaxSpareServers != nil {
		pool.PMMaxSpareServers = *req.PMMaxSpareServers
	}
	if req.PMMaxRequests != nil {
		pool.PMMaxRequests = *req.PMMaxRequests
	}
	if req.PMProcessIdleTimeout != nil {
		pool.PMProcessIdleTimeout = *req.PMProcessIdleTimeout
	}
	if req.StatusPath != nil {
		pool.StatusPath = *req.StatusPath
	}
	if req.AccessLog != nil {
		pool.AccessLog = *req.AccessLog
	}
	if req.ErrorLog != nil {
		pool.ErrorLog = *req.ErrorLog
	}
	if req.PhpAdminFlag != nil {
		pool.PhpAdminFlag = req.PhpAdminFlag
	}
	if req.PhpValue != nil {
		pool.PhpValue = req.PhpValue
	}
	if req.PhpAdminValue != nil {
		pool.PhpAdminValue = req.PhpAdminValue
	}
	if req.Env != nil {
		pool.Env = req.Env
	}
	if req.IsActive != nil {
		pool.IsActive = *req.IsActive
	}

	if err := s.phpRepo.UpdatePHPPool(ctx, pool); err != nil {
		return nil, fmt.Errorf("failed to update PHP-FPM pool: %w", err)
	}

	s.logger.Info("PHP-FPM pool updated",
		zap.String("id", pool.ID),
		zap.String("name", pool.Name),
	)

	return pool, nil
}

// DeletePHPPool deletes a PHP-FPM pool
func (s *PHPService) DeletePHPPool(ctx context.Context, id, tenantID string) error {
	if err := s.phpRepo.DeletePHPPool(ctx, id, tenantID); err != nil {
		return fmt.Errorf("failed to delete PHP-FPM pool: %w", err)
	}

	s.logger.Info("PHP-FPM pool deleted", zap.String("id", id))
	return nil
}

// InstallPHPExtension installs a PHP extension
func (s *PHPService) InstallPHPExtension(ctx context.Context, req *models.InstallPHPExtensionRequest, tenantID string) (*models.PHPExtension, error) {
	ext := &models.PHPExtension{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Version:      "latest",
		Description:  "",
		IsInstalled:  true,
		IsEnabled:    true,
		PHPVersionID: req.PHPVersionID,
		ServerID:     req.ServerID,
		TenantID:     tenantID,
	}

	if err := s.phpRepo.CreatePHPExtension(ctx, ext); err != nil {
		return nil, fmt.Errorf("failed to install PHP extension: %w", err)
	}

	s.logger.Info("PHP extension installed",
		zap.String("id", ext.ID),
		zap.String("name", ext.Name),
		zap.String("php_version_id", ext.PHPVersionID),
	)

	return ext, nil
}

// ListPHPExtensions lists all PHP extensions for a PHP version
func (s *PHPService) ListPHPExtensions(ctx context.Context, phpVersionID, tenantID string) ([]*models.PHPExtension, error) {
	return s.phpRepo.ListPHPExtensions(ctx, phpVersionID, tenantID)
}

// UpdatePHPExtension updates a PHP extension
func (s *PHPService) UpdatePHPExtension(ctx context.Context, id, tenantID string, isEnabled bool) (*models.PHPExtension, error) {
	ext, err := s.phpRepo.GetPHPExtension(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP extension: %w", err)
	}

	ext.IsEnabled = isEnabled

	if err := s.phpRepo.UpdatePHPExtension(ctx, ext); err != nil {
		return nil, fmt.Errorf("failed to update PHP extension: %w", err)
	}

	s.logger.Info("PHP extension updated",
		zap.String("id", ext.ID),
		zap.String("name", ext.Name),
		zap.Bool("is_enabled", ext.IsEnabled),
	)

	return ext, nil
}

// DeletePHPExtension deletes a PHP extension
func (s *PHPService) DeletePHPExtension(ctx context.Context, id, tenantID string) error {
	if err := s.phpRepo.DeletePHPExtension(ctx, id, tenantID); err != nil {
		return fmt.Errorf("failed to delete PHP extension: %w", err)
	}

	s.logger.Info("PHP extension deleted", zap.String("id", id))
	return nil
}

// GetPHPConfig gets PHP configuration for a PHP version
func (s *PHPService) GetPHPConfig(ctx context.Context, phpVersionID, tenantID string) (*models.PHPConfig, error) {
	return s.phpRepo.GetPHPConfig(ctx, phpVersionID, tenantID)
}

// UpdatePHPConfig updates PHP configuration
func (s *PHPService) UpdatePHPConfig(ctx context.Context, phpVersionID, tenantID string, req *models.UpdatePHPConfigRequest) (*models.PHPConfig, error) {
	config, err := s.phpRepo.GetPHPConfig(ctx, phpVersionID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP config: %w", err)
	}

	if req.MemoryLimit != nil {
		config.MemoryLimit = *req.MemoryLimit
	}
	if req.MaxExecutionTime != nil {
		config.MaxExecutionTime = *req.MaxExecutionTime
	}
	if req.MaxInputTime != nil {
		config.MaxInputTime = *req.MaxInputTime
	}
	if req.PostMaxSize != nil {
		config.PostMaxSize = *req.PostMaxSize
	}
	if req.UploadMaxFilesize != nil {
		config.UploadMaxFilesize = *req.UploadMaxFilesize
	}
	if req.MaxFileUploads != nil {
		config.MaxFileUploads = *req.MaxFileUploads
	}
	if req.ErrorReporting != nil {
		config.ErrorReporting = *req.ErrorReporting
	}
	if req.DisplayErrors != nil {
		config.DisplayErrors = *req.DisplayErrors
	}
	if req.LogErrors != nil {
		config.LogErrors = *req.LogErrors
	}
	if req.ErrorLog != nil {
		config.ErrorLog = *req.ErrorLog
	}
	if req.DateFormat != nil {
		config.DateFormat = *req.DateFormat
	}
	if req.Timezone != nil {
		config.Timezone = *req.Timezone
	}
	if req.OPcacheEnabled != nil {
		config.OPcacheEnabled = *req.OPcacheEnabled
	}
	if req.OPcacheMemory != nil {
		config.OPcacheMemory = *req.OPcacheMemory
	}
	if req.OPcacheMaxFiles != nil {
		config.OPcacheMaxFiles = *req.OPcacheMaxFiles
	}
	if req.OPcacheRevalidateFreq != nil {
		config.OPcacheRevalidateFreq = *req.OPcacheRevalidateFreq
	}
	if req.CustomSettings != nil {
		config.CustomSettings = req.CustomSettings
	}

	if err := s.phpRepo.UpdatePHPConfig(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to update PHP config: %w", err)
	}

	s.logger.Info("PHP config updated",
		zap.String("php_version_id", config.PHPVersionID),
	)

	return config, nil
}

// DeletePHPConfig deletes PHP configuration
func (s *PHPService) DeletePHPConfig(ctx context.Context, id, tenantID string) error {
	if err := s.phpRepo.DeletePHPConfig(ctx, id, tenantID); err != nil {
		return fmt.Errorf("failed to delete PHP config: %w", err)
	}

	s.logger.Info("PHP config deleted", zap.String("id", id))
	return nil
}

// GetPHPExtension gets a PHP extension by ID
func (s *PHPService) GetPHPExtension(ctx context.Context, id, tenantID string) (*models.PHPExtension, error) {
	return s.phpRepo.GetPHPExtension(ctx, id, tenantID)
}
