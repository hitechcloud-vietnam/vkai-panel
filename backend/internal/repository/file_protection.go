package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type FileProtectionRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewFileProtectionRepository(db *sqlx.DB, logger *zap.Logger) *FileProtectionRepository {
	return &FileProtectionRepository{db: db, logger: logger}
}

// --- Rules ---

func (r *FileProtectionRepository) CreateRule(ctx context.Context, tenantID uuid.UUID, req models.CreateProtectionRuleRequest) (*models.FileProtectionRule, error) {
	rule := &models.FileProtectionRule{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        req.Name,
		Path:        req.Path,
		Recursive:   req.Recursive,
		FilePattern: req.FilePattern,
		WatchCreate: req.WatchCreate,
		WatchModify: req.WatchModify,
		WatchDelete: req.WatchDelete,
		WatchPerms:  req.WatchPerms,
		IsActive:    true,
	}
	if rule.FilePattern == "" {
		rule.FilePattern = "*"
	}
	if !rule.WatchCreate && !rule.WatchModify && !rule.WatchDelete && !rule.WatchPerms {
		rule.WatchModify = true
		rule.WatchDelete = true
	}

	query := `INSERT INTO file_protection_rules (id, tenant_id, name, path, recursive, file_pattern, watch_create, watch_modify, watch_delete, watch_permissions, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, query, rule.ID, rule.TenantID, rule.Name, rule.Path, rule.Recursive, rule.FilePattern,
		rule.WatchCreate, rule.WatchModify, rule.WatchDelete, rule.WatchPerms, rule.IsActive)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *FileProtectionRepository) ListRules(ctx context.Context, tenantID uuid.UUID) ([]models.FileProtectionRule, error) {
	var rules []models.FileProtectionRule
	query := `SELECT * FROM file_protection_rules WHERE tenant_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &rules, query, tenantID)
	return rules, err
}

func (r *FileProtectionRepository) GetRule(ctx context.Context, id uuid.UUID) (*models.FileProtectionRule, error) {
	var rule models.FileProtectionRule
	query := `SELECT * FROM file_protection_rules WHERE id = $1`
	err := r.db.GetContext(ctx, &rule, query, id)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *FileProtectionRepository) UpdateRule(ctx context.Context, id uuid.UUID, req models.UpdateProtectionRuleRequest) (*models.FileProtectionRule, error) {
	rule, err := r.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.Recursive != nil {
		rule.Recursive = *req.Recursive
	}
	if req.FilePattern != nil {
		rule.FilePattern = *req.FilePattern
	}
	if req.WatchCreate != nil {
		rule.WatchCreate = *req.WatchCreate
	}
	if req.WatchModify != nil {
		rule.WatchModify = *req.WatchModify
	}
	if req.WatchDelete != nil {
		rule.WatchDelete = *req.WatchDelete
	}
	if req.WatchPerms != nil {
		rule.WatchPerms = *req.WatchPerms
	}
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	query := `UPDATE file_protection_rules SET name=$1, recursive=$2, file_pattern=$3, watch_create=$4, watch_modify=$5, watch_delete=$6, watch_permissions=$7, is_active=$8, updated_at=NOW() WHERE id=$9`
	_, err = r.db.ExecContext(ctx, query, rule.Name, rule.Recursive, rule.FilePattern, rule.WatchCreate, rule.WatchModify, rule.WatchDelete, rule.WatchPerms, rule.IsActive, id)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *FileProtectionRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM file_protection_rules WHERE id = $1`, id)
	return err
}

func (r *FileProtectionRepository) ToggleRule(ctx context.Context, id uuid.UUID) (*models.FileProtectionRule, error) {
	rule, err := r.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	rule.IsActive = !rule.IsActive
	_, err = r.db.ExecContext(ctx, `UPDATE file_protection_rules SET is_active = $1, updated_at = NOW() WHERE id = $2`, rule.IsActive, id)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// --- Integrity Records ---

func (r *FileProtectionRepository) CreateIntegrityRecord(ctx context.Context, rec *models.FileIntegrityRecord) error {
	rec.ID = uuid.New()
	rec.ScannedAt = time.Now()
	query := `INSERT INTO file_integrity_records (id, rule_id, tenant_id, file_path, sha256_hash, file_size, file_mode, owner, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, query, rec.ID, rec.RuleID, rec.TenantID, rec.FilePath, rec.SHA256Hash, rec.FileSize, rec.FileMode, rec.Owner, rec.ScannedAt)
	return err
}

func (r *FileProtectionRepository) GetIntegrityRecord(ctx context.Context, ruleID uuid.UUID, filePath string) (*models.FileIntegrityRecord, error) {
	var rec models.FileIntegrityRecord
	query := `SELECT * FROM file_integrity_records WHERE rule_id = $1 AND file_path = $2`
	err := r.db.GetContext(ctx, &rec, query, ruleID, filePath)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *FileProtectionRepository) UpdateIntegrityRecord(ctx context.Context, rec *models.FileIntegrityRecord) error {
	query := `UPDATE file_integrity_records SET sha256_hash=$1, file_size=$2, file_mode=$3, owner=$4, scanned_at=NOW() WHERE id=$5`
	_, err := r.db.ExecContext(ctx, query, rec.SHA256Hash, rec.FileSize, rec.FileMode, rec.Owner, rec.ID)
	return err
}

func (r *FileProtectionRepository) ListIntegrityRecords(ctx context.Context, ruleID uuid.UUID) ([]models.FileIntegrityRecord, error) {
	var records []models.FileIntegrityRecord
	query := `SELECT * FROM file_integrity_records WHERE rule_id = $1 ORDER BY file_path`
	err := r.db.SelectContext(ctx, &records, query, ruleID)
	return records, err
}

// --- Change Events ---

func (r *FileProtectionRepository) CreateChangeEvent(ctx context.Context, event *models.FileChangeEvent) error {
	event.ID = uuid.New()
	event.CreatedAt = time.Now()
	query := `INSERT INTO file_change_events (id, rule_id, tenant_id, file_path, event_type, old_hash, new_hash, old_mode, new_mode, details, severity, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, false, $12)`
	_, err := r.db.ExecContext(ctx, query, event.ID, event.RuleID, event.TenantID, event.FilePath, event.EventType,
		event.OldHash, event.NewHash, event.OldMode, event.NewMode, event.Details, event.Severity, event.CreatedAt)
	return err
}

func (r *FileProtectionRepository) ListChangeEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.FileChangeEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []models.FileChangeEvent
	query := `SELECT * FROM file_change_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`
	err := r.db.SelectContext(ctx, &events, query, tenantID, limit)
	return events, err
}

func (r *FileProtectionRepository) MarkEventRead(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE file_change_events SET is_read = true WHERE id = $1`, id)
	return err
}

func (r *FileProtectionRepository) MarkAllEventsRead(ctx context.Context, tenantID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE file_change_events SET is_read = true WHERE tenant_id = $1`, tenantID)
	return err
}

// --- Quarantine ---

func (r *FileProtectionRepository) CreateQuarantineItem(ctx context.Context, item *models.QuarantineItem) error {
	item.ID = uuid.New()
	item.CreatedAt = time.Now()
	query := `INSERT INTO file_quarantine (id, tenant_id, original_path, quarantine_path, sha256_hash, file_size, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, query, item.ID, item.TenantID, item.OriginalPath, item.QuarantinePath, item.SHA256Hash, item.FileSize, item.Reason, item.CreatedAt)
	return err
}

func (r *FileProtectionRepository) ListQuarantineItems(ctx context.Context, tenantID uuid.UUID) ([]models.QuarantineItem, error) {
	var items []models.QuarantineItem
	query := `SELECT * FROM file_quarantine WHERE tenant_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &items, query, tenantID)
	return items, err
}

func (r *FileProtectionRepository) RestoreQuarantineItem(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE file_quarantine SET restored_at = $1 WHERE id = $2`, now, id)
	return err
}

func (r *FileProtectionRepository) DeleteQuarantineItem(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM file_quarantine WHERE id = $1`, id)
	return err
}

// --- Stats ---

func (r *FileProtectionRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.FileProtectionStats, error) {
	stats := &models.FileProtectionStats{}
	r.db.GetContext(ctx, &stats.TotalRules, `SELECT COUNT(*) FROM file_protection_rules WHERE tenant_id = $1`, tenantID)
	r.db.GetContext(ctx, &stats.ActiveRules, `SELECT COUNT(*) FROM file_protection_rules WHERE tenant_id = $1 AND is_active = true`, tenantID)
	r.db.GetContext(ctx, &stats.TotalFiles, `SELECT COUNT(*) FROM file_integrity_records WHERE tenant_id = $1`, tenantID)

	today := time.Now().Truncate(24 * time.Hour)
	r.db.GetContext(ctx, &stats.ChangesToday, `SELECT COUNT(*) FROM file_change_events WHERE tenant_id = $1 AND created_at >= $2`, tenantID, today)
	r.db.GetContext(ctx, &stats.QuarantinedFiles, `SELECT COUNT(*) FROM file_quarantine WHERE tenant_id = $1 AND restored_at IS NULL`, tenantID)
	r.db.GetContext(ctx, &stats.UnreadAlerts, `SELECT COUNT(*) FROM file_change_events WHERE tenant_id = $1 AND is_read = false`, tenantID)

	return stats, nil
}
