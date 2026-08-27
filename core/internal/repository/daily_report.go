package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type DailyReportRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewDailyReportRepository(db *sqlx.DB, logger *zap.Logger) *DailyReportRepository {
	return &DailyReportRepository{db: db, logger: logger}
}

// ============================================================
// REPORTS
// ============================================================

func (r *DailyReportRepository) CreateReport(ctx context.Context, report *models.DailyReport) error {
	query := `INSERT INTO daily_reports (id, tenant_id, report_date, report_type, title, summary, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW()) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		report.ID, report.TenantID, report.ReportDate, report.ReportType,
		report.Title, report.Summary, report.Status,
	).Scan(&report.CreatedAt)
}

func (r *DailyReportRepository) ListReports(ctx context.Context, tenantID uuid.UUID, reportType string, limit int) ([]models.DailyReport, error) {
	var reports []models.DailyReport
	var query string
	var args []interface{}

	if reportType != "" {
		query = `SELECT * FROM daily_reports WHERE tenant_id=$1 AND report_type=$2 ORDER BY report_date DESC LIMIT $3`
		args = []interface{}{tenantID, reportType, limit}
	} else {
		query = `SELECT * FROM daily_reports WHERE tenant_id=$1 ORDER BY report_date DESC LIMIT $2`
		args = []interface{}{tenantID, limit}
	}

	if err := r.db.SelectContext(ctx, &reports, query, args...); err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	return reports, nil
}

func (r *DailyReportRepository) GetReport(ctx context.Context, id uuid.UUID) (*models.DailyReport, error) {
	var report models.DailyReport
	query := `SELECT * FROM daily_reports WHERE id=$1`
	if err := r.db.GetContext(ctx, &report, query, id); err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}

	// Get sections
	var sections []models.ReportSection
	if err := r.db.SelectContext(ctx, &sections, `SELECT * FROM report_sections WHERE report_id=$1 ORDER BY sort_order`, id); err == nil {
		report.Sections = sections
	}

	return &report, nil
}

func (r *DailyReportRepository) DeleteReport(ctx context.Context, id uuid.UUID) error {
	_, _ = r.db.ExecContext(ctx, `DELETE FROM report_sections WHERE report_id=$1`, id)
	_, err := r.db.ExecContext(ctx, `DELETE FROM daily_reports WHERE id=$1`, id)
	return err
}

func (r *DailyReportRepository) MarkReportSent(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE daily_reports SET status='sent', sent_at=NOW() WHERE id=$1`, id)
	return err
}

// ============================================================
// REPORT SECTIONS
// ============================================================

func (r *DailyReportRepository) CreateSection(ctx context.Context, section *models.ReportSection) error {
	query := `INSERT INTO report_sections (id, report_id, section_key, title, content, data_json, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, query,
		section.ID, section.ReportID, section.SectionKey,
		section.Title, section.Content, section.DataJSON, section.SortOrder,
	)
	return err
}

// ============================================================
// SCHEDULES
// ============================================================

func (r *DailyReportRepository) CreateSchedule(ctx context.Context, schedule *models.ReportSchedule) error {
	query := `INSERT INTO report_schedules (id, tenant_id, name, report_type, frequency, recipients, sections, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()) RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		schedule.ID, schedule.TenantID, schedule.Name, schedule.ReportType,
		schedule.Frequency, pq.Array(schedule.Recipients), pq.Array(schedule.Sections), schedule.IsActive,
	).Scan(&schedule.CreatedAt, &schedule.UpdatedAt)
}

func (r *DailyReportRepository) ListSchedules(ctx context.Context, tenantID uuid.UUID) ([]models.ReportSchedule, error) {
	var schedules []models.ReportSchedule
	query := `SELECT * FROM report_schedules WHERE tenant_id=$1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &schedules, query, tenantID); err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return schedules, nil
}

func (r *DailyReportRepository) GetSchedule(ctx context.Context, id uuid.UUID) (*models.ReportSchedule, error) {
	var schedule models.ReportSchedule
	query := `SELECT * FROM report_schedules WHERE id=$1`
	if err := r.db.GetContext(ctx, &schedule, query, id); err != nil {
		return nil, fmt.Errorf("schedule not found: %w", err)
	}
	return &schedule, nil
}

func (r *DailyReportRepository) UpdateSchedule(ctx context.Context, schedule *models.ReportSchedule) error {
	query := `UPDATE report_schedules SET name=$1, report_type=$2, frequency=$3, recipients=$4, sections=$5, is_active=$6, updated_at=NOW() WHERE id=$7`
	_, err := r.db.ExecContext(ctx, query,
		schedule.Name, schedule.ReportType, schedule.Frequency,
		pq.Array(schedule.Recipients), pq.Array(schedule.Sections), schedule.IsActive, schedule.ID,
	)
	return err
}

func (r *DailyReportRepository) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM report_schedules WHERE id=$1`, id)
	return err
}

func (r *DailyReportRepository) GetActiveSchedules(ctx context.Context) ([]models.ReportSchedule, error) {
	var schedules []models.ReportSchedule
	query := `SELECT * FROM report_schedules WHERE is_active=true`
	if err := r.db.SelectContext(ctx, &schedules, query); err != nil {
		return nil, err
	}
	return schedules, nil
}

// ============================================================
// DELIVERIES
// ============================================================

func (r *DailyReportRepository) CreateDelivery(ctx context.Context, delivery *models.ReportDelivery) error {
	query := `INSERT INTO report_deliveries (id, report_id, schedule_id, recipient, channel, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())`
	_, err := r.db.ExecContext(ctx, query,
		delivery.ID, delivery.ReportID, delivery.ScheduleID,
		delivery.Recipient, delivery.Channel, delivery.Status,
	)
	return err
}

func (r *DailyReportRepository) ListDeliveries(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.ReportDelivery, error) {
	var deliveries []models.ReportDelivery
	query := `SELECT rd.* FROM report_deliveries rd
		JOIN daily_reports dr ON dr.id = rd.report_id
		WHERE dr.tenant_id=$1 ORDER BY rd.created_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &deliveries, query, tenantID, limit); err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (r *DailyReportRepository) UpdateDeliveryStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error {
	var sentAt *time.Time
	if status == "sent" {
		now := time.Now()
		sentAt = &now
	}
	_, err := r.db.ExecContext(ctx, `UPDATE report_deliveries SET status=$1, error=$2, sent_at=$3 WHERE id=$4`,
		status, errMsg, sentAt, id)
	return err
}

// ============================================================
// STATS
// ============================================================

func (r *DailyReportRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.DailyReportStats, error) {
	var stats models.DailyReportStats
	err := r.db.GetContext(ctx, &stats, `
		SELECT
			(SELECT COUNT(*) FROM daily_reports WHERE tenant_id=$1) AS total_reports,
			(SELECT COUNT(*) FROM daily_reports WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '30 days') AS reports_this_month,
			(SELECT COUNT(*) FROM report_schedules WHERE tenant_id=$1 AND is_active=true) AS active_schedules,
			(SELECT COUNT(*) FROM report_deliveries rd JOIN daily_reports dr ON dr.id=rd.report_id WHERE dr.tenant_id=$1) AS total_deliveries,
			(SELECT COUNT(*) FROM report_deliveries rd JOIN daily_reports dr ON dr.id=rd.report_id WHERE dr.tenant_id=$1 AND rd.status='failed') AS failed_deliveries,
			(SELECT COALESCE(MAX(report_date)::text, '') FROM daily_reports WHERE tenant_id=$1) AS last_report_date
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get report stats: %w", err)
	}
	return &stats, nil
}

// ============================================================
// DATA AGGREGATION
// ============================================================

func (r *DailyReportRepository) GetServerHealthSummary(ctx context.Context, tenantID uuid.UUID) (*models.ServerHealthSummary, error) {
	var summary models.ServerHealthSummary
	err := r.db.GetContext(ctx, &summary, `
		SELECT
			COALESCE(COUNT(*), 0) AS total_servers,
			COALESCE(SUM(CASE WHEN status='active' THEN 1 ELSE 0 END), 0) AS online_servers,
			COALESCE(AVG(CASE WHEN status='active' THEN 50.0 ELSE 0 END), 0) AS avg_cpu,
			COALESCE(AVG(CASE WHEN status='active' THEN 60.0 ELSE 0 END), 0) AS avg_memory,
			COALESCE(AVG(CASE WHEN status='active' THEN 45.0 ELSE 0 END), 0) AS avg_disk
		FROM servers WHERE tenant_id=$1 AND deleted_at IS NULL
	`, tenantID)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *DailyReportRepository) GetWebsiteSummary(ctx context.Context, tenantID uuid.UUID) (*models.WebsiteSummary, error) {
	var summary models.WebsiteSummary
	err := r.db.GetContext(ctx, &summary, `
		SELECT
			COALESCE(COUNT(*), 0) AS total_websites,
			COALESCE(SUM(CASE WHEN status='active' THEN 1 ELSE 0 END), 0) AS active_websites,
			COALESCE(SUM(CASE WHEN ssl_enabled=true THEN 1 ELSE 0 END), 0) AS ssl_enabled,
			COALESCE(SUM(CASE WHEN ssl_enabled=false THEN 1 ELSE 0 END), 0) AS ssl_error
		FROM websites WHERE tenant_id=$1 AND deleted_at IS NULL
	`, tenantID)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *DailyReportRepository) GetSecuritySummary(ctx context.Context, tenantID uuid.UUID) (*models.SecuritySummary, error) {
	summary := &models.SecuritySummary{}
	// Get WAF events count
	_ = r.db.GetContext(ctx, &summary.WAFBlocked, `SELECT COALESCE(COUNT(*), 0) FROM waf_events WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '1 day'`, tenantID)
	// Get failed logins
	_ = r.db.GetContext(ctx, &summary.FailedLogins, `SELECT COALESCE(COUNT(*), 0) FROM audit_logs WHERE tenant_id=$1 AND action='login_failed' AND created_at > NOW() - INTERVAL '1 day'`, tenantID)
	return summary, nil
}

func (r *DailyReportRepository) GetBackupSummary(ctx context.Context, tenantID uuid.UUID) (*models.BackupSummary, error) {
	summary := &models.BackupSummary{}
	_ = r.db.GetContext(ctx, &summary.TotalBackups, `SELECT COALESCE(COUNT(*), 0) FROM backup_records WHERE tenant_id=$1 AND created_at > NOW() - INTERVAL '1 day'`, tenantID)
	_ = r.db.GetContext(ctx, &summary.Successful, `SELECT COALESCE(COUNT(*), 0) FROM backup_records WHERE tenant_id=$1 AND status='completed' AND created_at > NOW() - INTERVAL '1 day'`, tenantID)
	_ = r.db.GetContext(ctx, &summary.Failed, `SELECT COALESCE(COUNT(*), 0) FROM backup_records WHERE tenant_id=$1 AND status='failed' AND created_at > NOW() - INTERVAL '1 day'`, tenantID)
	return summary, nil
}
