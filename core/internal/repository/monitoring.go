package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type MonitoringRepository struct {
	db *sqlx.DB
}

func NewMonitoringRepository(db *sqlx.DB) *MonitoringRepository {
	return &MonitoringRepository{db: db}
}

// Monitoring Metric operations
func (r *MonitoringRepository) CreateMetric(ctx context.Context, metric *models.MonitoringMetric) error {
	query := `
		INSERT INTO monitoring_metrics (id, server_id, tenant_id, metric, value, unit, tags, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	metric.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		metric.ID, metric.ServerID, metric.TenantID, metric.Metric,
		metric.Value, metric.Unit, metric.Tags, metric.Timestamp,
	).Scan(&metric.ID)
}

func (r *MonitoringRepository) ListMetricsByServer(ctx context.Context, serverID uuid.UUID, metric string, start, end time.Time, limit int) ([]models.MonitoringMetric, error) {
	var metrics []models.MonitoringMetric
	query := `
		SELECT * FROM monitoring_metrics 
		WHERE server_id = $1 AND metric = $2 AND timestamp BETWEEN $3 AND $4
		ORDER BY timestamp DESC LIMIT $5`
	if err := r.db.SelectContext(ctx, &metrics, query, serverID, metric, start, end, limit); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (r *MonitoringRepository) ListMetricsByTenant(ctx context.Context, tenantID uuid.UUID, metric string, start, end time.Time, limit int) ([]models.MonitoringMetric, error) {
	var metrics []models.MonitoringMetric
	query := `
		SELECT * FROM monitoring_metrics 
		WHERE tenant_id = $1 AND metric = $2 AND timestamp BETWEEN $3 AND $4
		ORDER BY timestamp DESC LIMIT $5`
	if err := r.db.SelectContext(ctx, &metrics, query, tenantID, metric, start, end, limit); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (r *MonitoringRepository) GetLatestMetric(ctx context.Context, serverID uuid.UUID, metric string) (*models.MonitoringMetric, error) {
	var m models.MonitoringMetric
	query := `
		SELECT * FROM monitoring_metrics 
		WHERE server_id = $1 AND metric = $2
		ORDER BY timestamp DESC LIMIT 1`
	if err := r.db.GetContext(ctx, &m, query, serverID, metric); err != nil {
		return nil, fmt.Errorf("metric not found: %w", err)
	}
	return &m, nil
}

func (r *MonitoringRepository) DeleteOldMetrics(ctx context.Context, before time.Time) error {
	query := `DELETE FROM monitoring_metrics WHERE timestamp < $1`
	_, err := r.db.ExecContext(ctx, query, before)
	return err
}

// Monitoring Alert operations
func (r *MonitoringRepository) CreateAlert(ctx context.Context, alert *models.MonitoringAlert) error {
	query := `
		INSERT INTO monitoring_alerts (id, tenant_id, server_id, name, description, metric, 
			condition, threshold, duration, severity, status, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at`

	alert.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		alert.ID, alert.TenantID, alert.ServerID, alert.Name, alert.Description,
		alert.Metric, alert.Condition, alert.Threshold, alert.Duration,
		alert.Severity, alert.Status, alert.IsActive,
	).Scan(&alert.CreatedAt, &alert.UpdatedAt)
}

func (r *MonitoringRepository) GetAlertByID(ctx context.Context, id uuid.UUID) (*models.MonitoringAlert, error) {
	var alert models.MonitoringAlert
	query := `SELECT * FROM monitoring_alerts WHERE id = $1`
	if err := r.db.GetContext(ctx, &alert, query, id); err != nil {
		return nil, fmt.Errorf("alert not found: %w", err)
	}
	return &alert, nil
}

func (r *MonitoringRepository) ListAlertsByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.MonitoringAlert, int, error) {
	var alerts []models.MonitoringAlert
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM monitoring_alerts WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get alerts
	query := `SELECT * FROM monitoring_alerts WHERE tenant_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &alerts, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return alerts, total, nil
}

func (r *MonitoringRepository) ListAlertsByServer(ctx context.Context, serverID uuid.UUID) ([]models.MonitoringAlert, error) {
	var alerts []models.MonitoringAlert
	query := `SELECT * FROM monitoring_alerts WHERE server_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &alerts, query, serverID); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (r *MonitoringRepository) UpdateAlert(ctx context.Context, alert *models.MonitoringAlert) error {
	query := `
		UPDATE monitoring_alerts 
		SET name = $2, description = $3, metric = $4, condition = $5, threshold = $6, 
			duration = $7, severity = $8, status = $9, is_active = $10, 
			last_triggered_at = $11, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		alert.ID, alert.Name, alert.Description, alert.Metric, alert.Condition,
		alert.Threshold, alert.Duration, alert.Severity, alert.Status,
		alert.IsActive, alert.LastTriggeredAt,
	)
	return err
}

func (r *MonitoringRepository) DeleteAlert(ctx context.Context, id uuid.UUID) error {
	// Delete related records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM monitoring_alert_logs WHERE alert_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM monitoring_alerts WHERE id = $1`, id)
	return err
}

// Monitoring Alert Log operations
func (r *MonitoringRepository) CreateAlertLog(ctx context.Context, log *models.MonitoringAlertLog) error {
	query := `
		INSERT INTO monitoring_alert_logs (id, alert_id, tenant_id, server_id, value, message, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	log.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		log.ID, log.AlertID, log.TenantID, log.ServerID,
		log.Value, log.Message, log.Status,
	).Scan(&log.CreatedAt)
}

func (r *MonitoringRepository) ListAlertLogsByAlert(ctx context.Context, alertID uuid.UUID, limit, offset int) ([]models.MonitoringAlertLog, int, error) {
	var logs []models.MonitoringAlertLog
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM monitoring_alert_logs WHERE alert_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, alertID); err != nil {
		return nil, 0, err
	}

	// Get logs
	query := `SELECT * FROM monitoring_alert_logs WHERE alert_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &logs, query, alertID, limit, offset); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *MonitoringRepository) ListAlertLogsByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.MonitoringAlertLog, int, error) {
	var logs []models.MonitoringAlertLog
	var total int

	// Get total count
	countQuery := `SELECT COUNT(*) FROM monitoring_alert_logs WHERE tenant_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	// Get logs
	query := `SELECT * FROM monitoring_alert_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &logs, query, tenantID, limit, offset); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// Monitoring Dashboard operations
func (r *MonitoringRepository) CreateDashboard(ctx context.Context, dashboard *models.MonitoringDashboard) error {
	query := `
		INSERT INTO monitoring_dashboards (id, tenant_id, name, description, layout, widgets, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`

	dashboard.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		dashboard.ID, dashboard.TenantID, dashboard.Name, dashboard.Description,
		dashboard.Layout, dashboard.Widgets, dashboard.IsDefault,
	).Scan(&dashboard.CreatedAt, &dashboard.UpdatedAt)
}

func (r *MonitoringRepository) GetDashboardByID(ctx context.Context, id uuid.UUID) (*models.MonitoringDashboard, error) {
	var dashboard models.MonitoringDashboard
	query := `SELECT * FROM monitoring_dashboards WHERE id = $1`
	if err := r.db.GetContext(ctx, &dashboard, query, id); err != nil {
		return nil, fmt.Errorf("dashboard not found: %w", err)
	}
	return &dashboard, nil
}

func (r *MonitoringRepository) ListDashboardsByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.MonitoringDashboard, error) {
	var dashboards []models.MonitoringDashboard
	query := `SELECT * FROM monitoring_dashboards WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &dashboards, query, tenantID); err != nil {
		return nil, err
	}
	return dashboards, nil
}

func (r *MonitoringRepository) UpdateDashboard(ctx context.Context, dashboard *models.MonitoringDashboard) error {
	query := `
		UPDATE monitoring_dashboards 
		SET name = $2, description = $3, layout = $4, widgets = $5, is_default = $6, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		dashboard.ID, dashboard.Name, dashboard.Description,
		dashboard.Layout, dashboard.Widgets, dashboard.IsDefault,
	)
	return err
}

func (r *MonitoringRepository) DeleteDashboard(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM monitoring_dashboards WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
