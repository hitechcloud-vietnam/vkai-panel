package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type SecurityRepository struct {
	db *sqlx.DB
}

func NewSecurityRepository(db *sqlx.DB) *SecurityRepository {
	return &SecurityRepository{db: db}
}

// Security Scan operations
func (r *SecurityRepository) CreateScan(ctx context.Context, scan *models.SecurityScan) error {
	query := `
		INSERT INTO security_scans (id, tenant_id, server_id, scan_type, status, started_at, score, total_checks, passed_checks, failed_checks, warnings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	scan.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		scan.ID, scan.TenantID, scan.ServerID, scan.ScanType, scan.Status,
		scan.StartedAt, scan.Score, scan.TotalChecks, scan.PassedChecks,
		scan.FailedChecks, scan.Warnings,
	).Scan(&scan.CreatedAt, &scan.UpdatedAt)
}

func (r *SecurityRepository) GetScanByID(ctx context.Context, id uuid.UUID) (*models.SecurityScan, error) {
	var scan models.SecurityScan
	query := `SELECT * FROM security_scans WHERE id = $1`
	if err := r.db.GetContext(ctx, &scan, query, id); err != nil {
		return nil, fmt.Errorf("security scan not found: %w", err)
	}
	return &scan, nil
}

func (r *SecurityRepository) ListScansByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.SecurityScan, int, error) {
	var scans []models.SecurityScan
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM security_scans WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get scans
	query := `SELECT * FROM security_scans WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &scans, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return scans, total, nil
}

func (r *SecurityRepository) UpdateScan(ctx context.Context, scan *models.SecurityScan) error {
	query := `
		UPDATE security_scans 
		SET status = $2, completed_at = $3, score = $4, total_checks = $5, 
			passed_checks = $6, failed_checks = $7, warnings = $8, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		scan.ID, scan.Status, scan.CompletedAt, scan.Score,
		scan.TotalChecks, scan.PassedChecks, scan.FailedChecks, scan.Warnings,
	)
	return err
}

func (r *SecurityRepository) DeleteScan(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM security_checks WHERE scan_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM security_vulnerabilities WHERE scan_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM security_scans WHERE id = $1`, id)
	return err
}

// Security Vulnerability operations
func (r *SecurityRepository) CreateVulnerability(ctx context.Context, vuln *models.SecurityVulnerability) error {
	query := `
		INSERT INTO security_vulnerabilities (id, scan_id, tenant_id, severity, title, description, affected, solution, cve, cvss, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	vuln.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		vuln.ID, vuln.ScanID, vuln.TenantID, vuln.Severity, vuln.Title,
		vuln.Description, vuln.Affected, vuln.Solution, vuln.CVE, vuln.CVSS, vuln.Status,
	).Scan(&vuln.CreatedAt, &vuln.UpdatedAt)
}

func (r *SecurityRepository) GetVulnerabilityByID(ctx context.Context, id uuid.UUID) (*models.SecurityVulnerability, error) {
	var vuln models.SecurityVulnerability
	query := `SELECT * FROM security_vulnerabilities WHERE id = $1`
	if err := r.db.GetContext(ctx, &vuln, query, id); err != nil {
		return nil, fmt.Errorf("vulnerability not found: %w", err)
	}
	return &vuln, nil
}

func (r *SecurityRepository) ListVulnerabilitiesByScan(ctx context.Context, scanID uuid.UUID) ([]models.SecurityVulnerability, error) {
	var vulns []models.SecurityVulnerability
	query := `SELECT * FROM security_vulnerabilities WHERE scan_id = $1 ORDER BY severity DESC, created_at DESC`
	if err := r.db.SelectContext(ctx, &vulns, query, scanID); err != nil {
		return nil, err
	}
	return vulns, nil
}

func (r *SecurityRepository) ListVulnerabilitiesByTenant(ctx context.Context, tenantID uuid.UUID, severity string, limit, offset int) ([]models.SecurityVulnerability, int, error) {
	var vulns []models.SecurityVulnerability
	var total int

	// Build query based on severity filter
	var countQuery, query string
	var args []interface{}

	if severity != "" {
		countQuery = `SELECT COUNT(*) FROM security_vulnerabilities WHERE tenant_id = $1 AND severity = $2`
		query = `SELECT * FROM security_vulnerabilities WHERE tenant_id = $1 AND severity = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		args = []interface{}{tenantID, severity, limit, offset}
	} else {
		countQuery = `SELECT COUNT(*) FROM security_vulnerabilities WHERE tenant_id = $1`
		query = `SELECT * FROM security_vulnerabilities WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []interface{}{tenantID, limit, offset}
	}

	// Get total count
	if err := r.db.GetContext(ctx, &total, countQuery, args[:len(args)-2]...); err != nil {
		return nil, 0, err
	}

	// Get vulnerabilities
	if err := r.db.SelectContext(ctx, &vulns, query, args...); err != nil {
		return nil, 0, err
	}

	return vulns, total, nil
}

func (r *SecurityRepository) UpdateVulnerability(ctx context.Context, vuln *models.SecurityVulnerability) error {
	query := `UPDATE security_vulnerabilities SET status = $2, resolved_at = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, vuln.ID, vuln.Status, vuln.ResolvedAt)
	return err
}

func (r *SecurityRepository) DeleteVulnerability(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM security_vulnerabilities WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// Security Check operations
func (r *SecurityRepository) CreateCheck(ctx context.Context, check *models.SecurityCheck) error {
	query := `
		INSERT INTO security_checks (id, scan_id, tenant_id, category, name, description, status, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	check.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		check.ID, check.ScanID, check.TenantID, check.Category,
		check.Name, check.Description, check.Status, check.Details,
	).Scan(&check.CreatedAt)
}

func (r *SecurityRepository) ListChecksByScan(ctx context.Context, scanID uuid.UUID) ([]models.SecurityCheck, error) {
	var checks []models.SecurityCheck
	query := `SELECT * FROM security_checks WHERE scan_id = $1 ORDER BY category, name`
	if err := r.db.SelectContext(ctx, &checks, query, scanID); err != nil {
		return nil, err
	}
	return checks, nil
}

// Security Policy operations
func (r *SecurityRepository) CreatePolicy(ctx context.Context, policy *models.SecurityPolicy) error {
	query := `
		INSERT INTO security_policies (id, tenant_id, name, description, category, rules, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`

	policy.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		policy.ID, policy.TenantID, policy.Name, policy.Description,
		policy.Category, policy.Rules, policy.IsActive,
	).Scan(&policy.CreatedAt, &policy.UpdatedAt)
}

func (r *SecurityRepository) GetPolicyByID(ctx context.Context, id uuid.UUID) (*models.SecurityPolicy, error) {
	var policy models.SecurityPolicy
	query := `SELECT * FROM security_policies WHERE id = $1`
	if err := r.db.GetContext(ctx, &policy, query, id); err != nil {
		return nil, fmt.Errorf("security policy not found: %w", err)
	}
	return &policy, nil
}

func (r *SecurityRepository) ListPoliciesByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.SecurityPolicy, error) {
	var policies []models.SecurityPolicy
	query := `SELECT * FROM security_policies WHERE tenant_id = $1 ORDER BY category, name`
	if err := r.db.SelectContext(ctx, &policies, query, tenantID); err != nil {
		return nil, err
	}
	return policies, nil
}

func (r *SecurityRepository) UpdatePolicy(ctx context.Context, policy *models.SecurityPolicy) error {
	query := `
		UPDATE security_policies 
		SET name = $2, description = $3, category = $4, rules = $5, is_active = $6, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		policy.ID, policy.Name, policy.Description, policy.Category,
		policy.Rules, policy.IsActive,
	)
	return err
}

func (r *SecurityRepository) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM security_policies WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
