package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

type TamperProofRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewTamperProofRepository(db *sqlx.DB, logger *zap.Logger) *TamperProofRepository {
	return &TamperProofRepository{db: db, logger: logger}
}

// Protected Paths
func (r *TamperProofRepository) CreateProtectedPath(ctx context.Context, tenantID uuid.UUID, req models.CreateProtectedPathRequest) (*models.ProtectedPath, error) {
	var p models.ProtectedPath
	algo := req.Algorithm
	if algo == "" {
		algo = "sha256"
	}
	err := r.db.GetContext(ctx, &p, `
		INSERT INTO tamper_protected_paths (tenant_id, path, path_type, recursive, algorithm, is_enabled, alert_on_change, alert_on_delete, alert_on_create, ignore_patterns, description)
		VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, path, path_type, recursive, algorithm, is_enabled, alert_on_change, alert_on_delete, alert_on_create, ignore_patterns, description, file_count, last_scan_at, last_alert_at, created_at, updated_at
	`, tenantID, req.Path, req.PathType, req.Recursive, algo, req.AlertOnChange, req.AlertOnDelete, req.AlertOnCreate, pq.Array(req.IgnorePatterns), req.Description)
	if err != nil {
		return nil, fmt.Errorf("create protected path: %w", err)
	}
	return &p, nil
}

func (r *TamperProofRepository) ListProtectedPaths(ctx context.Context, tenantID uuid.UUID) ([]models.ProtectedPath, error) {
	var paths []models.ProtectedPath
	err := r.db.SelectContext(ctx, &paths, `
		SELECT id, tenant_id, path, path_type, recursive, algorithm, is_enabled, alert_on_change, alert_on_delete, alert_on_create, ignore_patterns, description, file_count, last_scan_at, last_alert_at, created_at, updated_at
		FROM tamper_protected_paths WHERE tenant_id=$1 ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list protected paths: %w", err)
	}
	return paths, nil
}

func (r *TamperProofRepository) GetProtectedPath(ctx context.Context, tenantID, id uuid.UUID) (*models.ProtectedPath, error) {
	var p models.ProtectedPath
	err := r.db.GetContext(ctx, &p, `
		SELECT id, tenant_id, path, path_type, recursive, algorithm, is_enabled, alert_on_change, alert_on_delete, alert_on_create, ignore_patterns, description, file_count, last_scan_at, last_alert_at, created_at, updated_at
		FROM tamper_protected_paths WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("get protected path: %w", err)
	}
	return &p, nil
}

func (r *TamperProofRepository) UpdateProtectedPath(ctx context.Context, tenantID, id uuid.UUID, req models.UpdateProtectedPathRequest) (*models.ProtectedPath, error) {
	existing, err := r.GetProtectedPath(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if req.Path != nil { existing.Path = *req.Path }
	if req.Recursive != nil { existing.Recursive = *req.Recursive }
	if req.Algorithm != nil { existing.Algorithm = *req.Algorithm }
	if req.IsEnabled != nil { existing.IsEnabled = *req.IsEnabled }
	if req.AlertOnChange != nil { existing.AlertOnChange = *req.AlertOnChange }
	if req.AlertOnDelete != nil { existing.AlertOnDelete = *req.AlertOnDelete }
	if req.AlertOnCreate != nil { existing.AlertOnCreate = *req.AlertOnCreate }
	if req.IgnorePatterns != nil { existing.IgnorePatterns = req.IgnorePatterns }
	if req.Description != nil { existing.Description = *req.Description }

	_, err = r.db.ExecContext(ctx, `
		UPDATE tamper_protected_paths SET path=$3, path_type=$4, recursive=$5, algorithm=$6, is_enabled=$7, alert_on_change=$8, alert_on_delete=$9, alert_on_create=$10, ignore_patterns=$11, description=$12, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, existing.Path, existing.PathType, existing.Recursive, existing.Algorithm, existing.IsEnabled, existing.AlertOnChange, existing.AlertOnDelete, existing.AlertOnCreate, pq.Array(existing.IgnorePatterns), existing.Description)
	if err != nil {
		return nil, fmt.Errorf("update protected path: %w", err)
	}
	return r.GetProtectedPath(ctx, tenantID, id)
}

func (r *TamperProofRepository) DeleteProtectedPath(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tamper_protected_paths WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete protected path: %w", err)
	}
	// Also delete baselines
	_, _ = r.db.ExecContext(ctx, `DELETE FROM tamper_baselines WHERE tenant_id=$1 AND protected_id=$2`, tenantID, id)
	return nil
}

func (r *TamperProofRepository) GetEnabledPaths(ctx context.Context, tenantID uuid.UUID) ([]models.ProtectedPath, error) {
	var paths []models.ProtectedPath
	err := r.db.SelectContext(ctx, &paths, `
		SELECT id, tenant_id, path, path_type, recursive, algorithm, is_enabled, alert_on_change, alert_on_delete, alert_on_create, ignore_patterns, description, file_count, last_scan_at, last_alert_at, created_at, updated_at
		FROM tamper_protected_paths WHERE tenant_id=$1 AND is_enabled=true ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get enabled paths: %w", err)
	}
	return paths, nil
}

func (r *TamperProofRepository) UpdatePathScanInfo(ctx context.Context, tenantID, id uuid.UUID, fileCount int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamper_protected_paths SET file_count=$3, last_scan_at=NOW(), updated_at=NOW() WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, fileCount)
	return err
}

func (r *TamperProofRepository) UpdatePathAlertInfo(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamper_protected_paths SET last_alert_at=NOW(), updated_at=NOW() WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return err
}

// Baselines
func (r *TamperProofRepository) UpsertBaseline(ctx context.Context, b *models.FileBaseline) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamper_baselines (id, tenant_id, protected_id, file_path, checksum, file_size, file_mode, owner_user, owner_group, mod_time, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (tenant_id, protected_id, file_path) DO UPDATE SET checksum=$5, file_size=$6, file_mode=$7, owner_user=$8, owner_group=$9, mod_time=$10, scanned_at=$11
	`, b.ID, b.TenantID, b.ProtectedID, b.FilePath, b.Checksum, b.FileSize, b.FileMode, b.OwnerUser, b.OwnerGroup, b.ModTime, b.ScannedAt)
	if err != nil {
		return fmt.Errorf("upsert baseline: %w", err)
	}
	return nil
}

func (r *TamperProofRepository) GetBaselines(ctx context.Context, tenantID, protectedID uuid.UUID) ([]models.FileBaseline, error) {
	var baselines []models.FileBaseline
	err := r.db.SelectContext(ctx, &baselines, `
		SELECT id, tenant_id, protected_id, file_path, checksum, file_size, file_mode, owner_user, owner_group, mod_time, scanned_at
		FROM tamper_baselines WHERE tenant_id=$1 AND protected_id=$2 ORDER BY file_path
	`, tenantID, protectedID)
	if err != nil {
		return nil, fmt.Errorf("get baselines: %w", err)
	}
	return baselines, nil
}

func (r *TamperProofRepository) GetBaseline(ctx context.Context, tenantID, protectedID uuid.UUID, filePath string) (*models.FileBaseline, error) {
	var b models.FileBaseline
	err := r.db.GetContext(ctx, &b, `
		SELECT id, tenant_id, protected_id, file_path, checksum, file_size, file_mode, owner_user, owner_group, mod_time, scanned_at
		FROM tamper_baselines WHERE tenant_id=$1 AND protected_id=$2 AND file_path=$3
	`, tenantID, protectedID, filePath)
	if err != nil {
		return nil, fmt.Errorf("get baseline: %w", err)
	}
	return &b, nil
}

func (r *TamperProofRepository) DeleteBaseline(ctx context.Context, tenantID, protectedID uuid.UUID, filePath string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tamper_baselines WHERE tenant_id=$1 AND protected_id=$2 AND file_path=$3`, tenantID, protectedID, filePath)
	return err
}

func (r *TamperProofRepository) DeleteBaselinesForPath(ctx context.Context, tenantID, protectedID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tamper_baselines WHERE tenant_id=$1 AND protected_id=$2`, tenantID, protectedID)
	return err
}

// Alerts
func (r *TamperProofRepository) CreateAlert(ctx context.Context, a *models.TamperAlert) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamper_alerts (id, tenant_id, protected_id, file_path, alert_type, severity, old_checksum, new_checksum, old_size, new_size, old_mode, new_mode, is_resolved, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, false, '')
	`, a.ID, a.TenantID, a.ProtectedID, a.FilePath, a.AlertType, a.Severity, a.OldChecksum, a.NewChecksum, a.OldSize, a.NewSize, a.OldMode, a.NewMode)
	if err != nil {
		return fmt.Errorf("create alert: %w", err)
	}
	return nil
}

func (r *TamperProofRepository) ListAlerts(ctx context.Context, tenantID uuid.UUID, resolved *bool) ([]models.TamperAlert, error) {
	query := `SELECT id, tenant_id, protected_id, file_path, alert_type, severity, old_checksum, new_checksum, old_size, new_size, old_mode, new_mode, is_resolved, resolved_by, resolved_at, notes, created_at FROM tamper_alerts WHERE tenant_id=$1`
	args := []interface{}{tenantID}
	if resolved != nil {
		query += " AND is_resolved=$2"
		args = append(args, *resolved)
	}
	query += " ORDER BY created_at DESC LIMIT 200"
	var alerts []models.TamperAlert
	err := r.db.SelectContext(ctx, &alerts, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	return alerts, nil
}

func (r *TamperProofRepository) GetAlert(ctx context.Context, tenantID, id uuid.UUID) (*models.TamperAlert, error) {
	var a models.TamperAlert
	err := r.db.GetContext(ctx, &a, `
		SELECT id, tenant_id, protected_id, file_path, alert_type, severity, old_checksum, new_checksum, old_size, new_size, old_mode, new_mode, is_resolved, resolved_by, resolved_at, notes, created_at
		FROM tamper_alerts WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("get alert: %w", err)
	}
	return &a, nil
}

func (r *TamperProofRepository) ResolveAlert(ctx context.Context, tenantID, id uuid.UUID, resolvedBy, notes string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamper_alerts SET is_resolved=true, resolved_by=$3, resolved_at=NOW(), notes=$4 WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, resolvedBy, notes)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}
	return nil
}

func (r *TamperProofRepository) GetActiveAlertCount(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM tamper_alerts WHERE tenant_id=$1 AND is_resolved=false`, tenantID)
	return count, err
}

// Scan Results
func (r *TamperProofRepository) CreateScanResult(ctx context.Context, sr *models.TamperScanResult) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamper_scan_results (id, tenant_id, protected_id, status, total_files, scanned_files, violations, new_files, deleted_files, modified_files, duration, scan_log, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, sr.ID, sr.TenantID, sr.ProtectedID, sr.Status, sr.TotalFiles, sr.ScannedFiles, sr.Violations, sr.NewFiles, sr.DeletedFiles, sr.ModifiedFiles, sr.Duration, sr.ScanLog, sr.CreatedAt)
	if err != nil {
		return fmt.Errorf("create scan result: %w", err)
	}
	return nil
}

func (r *TamperProofRepository) ListScanResults(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TamperScanResult, error) {
	if limit <= 0 { limit = 50 }
	var results []models.TamperScanResult
	err := r.db.SelectContext(ctx, &results, `
		SELECT id, tenant_id, protected_id, status, total_files, scanned_files, violations, new_files, deleted_files, modified_files, duration, scan_log, created_at
		FROM tamper_scan_results WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list scan results: %w", err)
	}
	return results, nil
}

// Audit Log
func (r *TamperProofRepository) CreateAuditLog(ctx context.Context, al *models.TamperAuditLog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamper_audit_logs (id, tenant_id, action, target, details, ip_address, user_id, username, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, al.ID, al.TenantID, al.Action, al.Target, al.Details, al.IPAddress, al.UserID, al.Username, al.CreatedAt)
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

func (r *TamperProofRepository) ListAuditLogs(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.TamperAuditLog, error) {
	if limit <= 0 { limit = 100 }
	var logs []models.TamperAuditLog
	err := r.db.SelectContext(ctx, &logs, `
		SELECT id, tenant_id, action, target, details, ip_address, user_id, username, created_at
		FROM tamper_audit_logs WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	return logs, nil
}

// Stats
func (r *TamperProofRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.TamperStats, error) {
	var stats models.TamperStats
	err := r.db.GetContext(ctx, &stats, `
		SELECT
			(SELECT COUNT(*) FROM tamper_protected_paths WHERE tenant_id=$1) AS protected_paths,
			(SELECT COUNT(*) FROM tamper_protected_paths WHERE tenant_id=$1 AND is_enabled=true) AS enabled_paths,
			(SELECT COALESCE(SUM(file_count), 0) FROM tamper_protected_paths WHERE tenant_id=$1) AS total_files,
			(SELECT COUNT(*) FROM tamper_alerts WHERE tenant_id=$1 AND is_resolved=false) AS active_alerts,
			(SELECT COUNT(*) FROM tamper_alerts WHERE tenant_id=$1 AND is_resolved=true) AS resolved_alerts,
			(SELECT COUNT(*) FROM tamper_alerts WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '1 day') AS alerts_today,
			(SELECT MAX(created_at) FROM tamper_scan_results WHERE tenant_id=$1) AS last_scan_at,
			(SELECT COUNT(*) FROM tamper_scan_results WHERE tenant_id=$1) AS total_scans,
			(SELECT COUNT(*) FROM tamper_scan_results WHERE tenant_id=$1 AND status='clean') AS clean_scans,
			(SELECT COUNT(*) FROM tamper_scan_results WHERE tenant_id=$1 AND status='violations_found') AS violation_scans
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get tamper stats: %w", err)
	}
	return &stats, nil
}

// Cleanup
func (r *TamperProofRepository) CleanupOldAlerts(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM tamper_alerts WHERE tenant_id=$1 AND is_resolved=true AND resolved_at < NOW() - ($2 || ' days')::INTERVAL
	`, tenantID, days)
	if err != nil {
		return 0, fmt.Errorf("cleanup old alerts: %w", err)
	}
	return result.RowsAffected()
}

func (r *TamperProofRepository) CleanupOldScanResults(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM tamper_scan_results WHERE tenant_id=$1 AND created_at < NOW() - ($2 || ' days')::INTERVAL
	`, tenantID, days)
	if err != nil {
		return 0, fmt.Errorf("cleanup old scan results: %w", err)
	}
	return result.RowsAffected()
}

func (r *TamperProofRepository) CleanupOldAuditLogs(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM tamper_audit_logs WHERE tenant_id=$1 AND created_at < NOW() - ($2 || ' days')::INTERVAL
	`, tenantID, days)
	if err != nil {
		return 0, fmt.Errorf("cleanup old audit logs: %w", err)
	}
	return result.RowsAffected()
}

// Helper
func timePtr(t time.Time) *time.Time { return &t }
