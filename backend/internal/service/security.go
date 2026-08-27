package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type SecurityService struct {
	securityRepo *repository.SecurityRepository
	logger       *zap.Logger
}

func NewSecurityService(securityRepo *repository.SecurityRepository, logger *zap.Logger) *SecurityService {
	return &SecurityService{
		securityRepo: securityRepo,
		logger:       logger,
	}
}

// CreateScan creates a new security scan
func (s *SecurityService) CreateScan(ctx context.Context, req *models.CreateSecurityScanRequest, tenantID string) (*models.SecurityScan, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	serverUUID, err := uuid.Parse(req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("invalid server ID: %w", err)
	}

	scan := &models.SecurityScan{
		ID:       uuid.New(),
		TenantID: tenantUUID,
		ServerID: serverUUID,
		ScanType: req.ScanType,
		Status:   "running",
		StartedAt: time.Now(),
	}

	if err := s.securityRepo.CreateScan(ctx, scan); err != nil {
		return nil, fmt.Errorf("failed to create security scan: %w", err)
	}

	s.logger.Info("Security scan created",
		zap.String("id", scan.ID.String()),
		zap.String("type", scan.ScanType),
		zap.String("server_id", scan.ServerID.String()),
	)

	// TODO: Start actual security scan in background
	go s.runScan(context.Background(), scan)

	return scan, nil
}

// GetScan gets a security scan by ID
func (s *SecurityService) GetScan(ctx context.Context, id, tenantID string) (*models.SecurityScan, error) {
	scanUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid scan ID: %w", err)
	}

	scan, err := s.securityRepo.GetScanByID(ctx, scanUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get security scan: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if scan.TenantID != tenantUUID {
		return nil, fmt.Errorf("scan not found")
	}

	return scan, nil
}

// ListScans lists all security scans for a tenant
func (s *SecurityService) ListScans(ctx context.Context, tenantID string, page, perPage int) ([]models.SecurityScan, int, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid tenant ID: %w", err)
	}

	offset := (page - 1) * perPage
	return s.securityRepo.ListScansByTenant(ctx, tenantUUID, perPage, offset)
}

// DeleteScan deletes a security scan
func (s *SecurityService) DeleteScan(ctx context.Context, id, tenantID string) error {
	scan, err := s.GetScan(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.securityRepo.DeleteScan(ctx, scan.ID); err != nil {
		return fmt.Errorf("failed to delete security scan: %w", err)
	}

	s.logger.Info("Security scan deleted",
		zap.String("id", scan.ID.String()),
	)

	return nil
}

// GetVulnerability gets a vulnerability by ID
func (s *SecurityService) GetVulnerability(ctx context.Context, id, tenantID string) (*models.SecurityVulnerability, error) {
	vulnUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid vulnerability ID: %w", err)
	}

	vuln, err := s.securityRepo.GetVulnerabilityByID(ctx, vulnUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vulnerability: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if vuln.TenantID != tenantUUID {
		return nil, fmt.Errorf("vulnerability not found")
	}

	return vuln, nil
}

// ListVulnerabilitiesByScan lists all vulnerabilities for a scan
func (s *SecurityService) ListVulnerabilitiesByScan(ctx context.Context, scanID, tenantID string) ([]models.SecurityVulnerability, error) {
	scanUUID, err := uuid.Parse(scanID)
	if err != nil {
		return nil, fmt.Errorf("invalid scan ID: %w", err)
	}

	// Verify scan belongs to tenant
	scan, err := s.GetScan(ctx, scanID, tenantID)
	if err != nil {
		return nil, err
	}

	if scan.TenantID.String() != tenantID {
		return nil, fmt.Errorf("scan not found")
	}

	return s.securityRepo.ListVulnerabilitiesByScan(ctx, scanUUID)
}

// ListVulnerabilitiesByTenant lists all vulnerabilities for a tenant
func (s *SecurityService) ListVulnerabilitiesByTenant(ctx context.Context, tenantID, severity string, page, perPage int) ([]models.SecurityVulnerability, int, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid tenant ID: %w", err)
	}

	offset := (page - 1) * perPage
	return s.securityRepo.ListVulnerabilitiesByTenant(ctx, tenantUUID, severity, perPage, offset)
}

// UpdateVulnerability updates a vulnerability
func (s *SecurityService) UpdateVulnerability(ctx context.Context, id, tenantID string, req *models.UpdateSecurityVulnerabilityRequest) (*models.SecurityVulnerability, error) {
	vuln, err := s.GetVulnerability(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	vuln.Status = req.Status
	if req.Status == "resolved" {
		now := time.Now()
		vuln.ResolvedAt = &now
	}

	if err := s.securityRepo.UpdateVulnerability(ctx, vuln); err != nil {
		return nil, fmt.Errorf("failed to update vulnerability: %w", err)
	}

	s.logger.Info("Vulnerability updated",
		zap.String("id", vuln.ID.String()),
		zap.String("status", vuln.Status),
	)

	return vuln, nil
}

// DeleteVulnerability deletes a vulnerability
func (s *SecurityService) DeleteVulnerability(ctx context.Context, id, tenantID string) error {
	vuln, err := s.GetVulnerability(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.securityRepo.DeleteVulnerability(ctx, vuln.ID); err != nil {
		return fmt.Errorf("failed to delete vulnerability: %w", err)
	}

	s.logger.Info("Vulnerability deleted",
		zap.String("id", vuln.ID.String()),
	)

	return nil
}

// ListChecksByScan lists all checks for a scan
func (s *SecurityService) ListChecksByScan(ctx context.Context, scanID, tenantID string) ([]models.SecurityCheck, error) {
	scanUUID, err := uuid.Parse(scanID)
	if err != nil {
		return nil, fmt.Errorf("invalid scan ID: %w", err)
	}

	// Verify scan belongs to tenant
	scan, err := s.GetScan(ctx, scanID, tenantID)
	if err != nil {
		return nil, err
	}

	if scan.TenantID.String() != tenantID {
		return nil, fmt.Errorf("scan not found")
	}

	return s.securityRepo.ListChecksByScan(ctx, scanUUID)
}

// CreatePolicy creates a new security policy
func (s *SecurityService) CreatePolicy(ctx context.Context, req *models.CreateSecurityPolicyRequest, tenantID string) (*models.SecurityPolicy, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	policy := &models.SecurityPolicy{
		ID:          uuid.New(),
		TenantID:    tenantUUID,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Rules:       req.Rules,
		IsActive:    true,
	}

	if err := s.securityRepo.CreatePolicy(ctx, policy); err != nil {
		return nil, fmt.Errorf("failed to create security policy: %w", err)
	}

	s.logger.Info("Security policy created",
		zap.String("id", policy.ID.String()),
		zap.String("name", policy.Name),
	)

	return policy, nil
}

// GetPolicy gets a security policy by ID
func (s *SecurityService) GetPolicy(ctx context.Context, id, tenantID string) (*models.SecurityPolicy, error) {
	policyUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid policy ID: %w", err)
	}

	policy, err := s.securityRepo.GetPolicyByID(ctx, policyUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get security policy: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if policy.TenantID != tenantUUID {
		return nil, fmt.Errorf("policy not found")
	}

	return policy, nil
}

// ListPolicies lists all security policies for a tenant
func (s *SecurityService) ListPolicies(ctx context.Context, tenantID string) ([]models.SecurityPolicy, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	return s.securityRepo.ListPoliciesByTenant(ctx, tenantUUID)
}

// UpdatePolicy updates a security policy
func (s *SecurityService) UpdatePolicy(ctx context.Context, id, tenantID string, req *models.UpdateSecurityPolicyRequest) (*models.SecurityPolicy, error) {
	policy, err := s.GetPolicy(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		policy.Name = req.Name
	}
	if req.Description != "" {
		policy.Description = req.Description
	}
	if req.Category != "" {
		policy.Category = req.Category
	}
	if req.Rules != nil {
		policy.Rules = req.Rules
	}
	if req.IsActive != nil {
		policy.IsActive = *req.IsActive
	}

	if err := s.securityRepo.UpdatePolicy(ctx, policy); err != nil {
		return nil, fmt.Errorf("failed to update security policy: %w", err)
	}

	s.logger.Info("Security policy updated",
		zap.String("id", policy.ID.String()),
		zap.String("name", policy.Name),
	)

	return policy, nil
}

// DeletePolicy deletes a security policy
func (s *SecurityService) DeletePolicy(ctx context.Context, id, tenantID string) error {
	policy, err := s.GetPolicy(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.securityRepo.DeletePolicy(ctx, policy.ID); err != nil {
		return fmt.Errorf("failed to delete security policy: %w", err)
	}

	s.logger.Info("Security policy deleted",
		zap.String("id", policy.ID.String()),
		zap.String("name", policy.Name),
	)

	return nil
}

// runScan runs a security scan in the background
func (s *SecurityService) runScan(ctx context.Context, scan *models.SecurityScan) {
	s.logger.Info("Starting security scan",
		zap.String("id", scan.ID.String()),
		zap.String("type", scan.ScanType),
	)

	// TODO: Implement actual security scanning logic
	// This would include:
	// - Port scanning
	// - Vulnerability detection
	// - Configuration checks
	// - SSL/TLS checks
	// - Firewall rules analysis
	// - File permission checks
	// - User account security
	// - Software version checks

	// Simulate scan completion
	time.Sleep(5 * time.Second)

	// Update scan status
	now := time.Now()
	scan.Status = "completed"
	scan.CompletedAt = &now
	scan.Score = 85
	scan.TotalChecks = 50
	scan.PassedChecks = 42
	scan.FailedChecks = 3
	scan.Warnings = 5

	if err := s.securityRepo.UpdateScan(ctx, scan); err != nil {
		s.logger.Error("Failed to update scan status", zap.Error(err))
		return
	}

	s.logger.Info("Security scan completed",
		zap.String("id", scan.ID.String()),
		zap.Int("score", scan.Score),
	)
}
