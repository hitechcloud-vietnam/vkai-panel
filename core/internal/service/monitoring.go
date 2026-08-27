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

type MonitoringService struct {
	repo   *repository.MonitoringRepository
	logger *zap.Logger
}

func NewMonitoringService(repo *repository.MonitoringRepository, logger *zap.Logger) *MonitoringService {
	return &MonitoringService{
		repo:   repo,
		logger: logger,
	}
}

// Monitoring Metric operations
func (s *MonitoringService) RecordMetric(ctx context.Context, tenantID, serverID uuid.UUID, metric string, value float64, unit string, tags models.JSONMap) error {
	m := &models.MonitoringMetric{
		ServerID:  serverID,
		TenantID:  tenantID,
		Metric:    metric,
		Value:     value,
		Unit:      unit,
		Tags:      tags,
		Timestamp: time.Now(),
	}

	if err := s.repo.CreateMetric(ctx, m); err != nil {
		return err
	}

	// Check alerts
	go s.checkAlerts(serverID, tenantID, metric, value)

	return nil
}

func (s *MonitoringService) GetMetrics(ctx context.Context, tenantID, serverID uuid.UUID, metric string, start, end time.Time, limit int) ([]models.MonitoringMetric, error) {
	return s.repo.ListMetricsByServer(ctx, serverID, metric, start, end, limit)
}

func (s *MonitoringService) GetMetricsByTenant(ctx context.Context, tenantID uuid.UUID, metric string, start, end time.Time, limit int) ([]models.MonitoringMetric, error) {
	return s.repo.ListMetricsByTenant(ctx, tenantID, metric, start, end, limit)
}

func (s *MonitoringService) GetLatestMetric(ctx context.Context, serverID uuid.UUID, metric string) (*models.MonitoringMetric, error) {
	return s.repo.GetLatestMetric(ctx, serverID, metric)
}

func (s *MonitoringService) CleanupOldMetrics(ctx context.Context, days int) error {
	before := time.Now().AddDate(0, 0, -days)
	return s.repo.DeleteOldMetrics(ctx, before)
}

// Monitoring Alert operations
func (s *MonitoringService) CreateAlert(ctx context.Context, tenantID uuid.UUID, req *models.CreateMonitoringAlertRequest) (*models.MonitoringAlert, error) {
	alert := &models.MonitoringAlert{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Metric:      req.Metric,
		Condition:   req.Condition,
		Threshold:   req.Threshold,
		Duration:    req.Duration,
		Severity:    req.Severity,
		Status:      "active",
		IsActive:    true,
	}

	if req.ServerID != "" {
		serverID, err := uuid.Parse(req.ServerID)
		if err != nil {
			return nil, fmt.Errorf("invalid server_id: %w", err)
		}
		alert.ServerID = &serverID
	}

	if alert.Duration == 0 {
		alert.Duration = 300 // 5 minutes default
	}

	if err := s.repo.CreateAlert(ctx, alert); err != nil {
		return nil, err
	}

	s.logger.Info("Monitoring alert created",
		zap.String("alert_id", alert.ID.String()),
		zap.String("name", alert.Name),
		zap.String("metric", alert.Metric),
	)

	return alert, nil
}

func (s *MonitoringService) GetAlertByID(ctx context.Context, tenantID, id uuid.UUID) (*models.MonitoringAlert, error) {
	alert, err := s.repo.GetAlertByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if alert.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return alert, nil
}

func (s *MonitoringService) ListAlerts(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.MonitoringAlert, int, error) {
	return s.repo.ListAlertsByTenant(ctx, tenantID, limit, offset)
}

func (s *MonitoringService) ListAlertsByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.MonitoringAlert, error) {
	alerts, err := s.repo.ListAlertsByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Filter by tenant
	var result []models.MonitoringAlert
	for _, a := range alerts {
		if a.TenantID == tenantID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *MonitoringService) UpdateAlert(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateMonitoringAlertRequest) (*models.MonitoringAlert, error) {
	alert, err := s.GetAlertByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		alert.Name = req.Name
	}
	if req.Description != "" {
		alert.Description = req.Description
	}
	if req.Metric != "" {
		alert.Metric = req.Metric
	}
	if req.Condition != "" {
		alert.Condition = req.Condition
	}
	if req.Threshold != 0 {
		alert.Threshold = req.Threshold
	}
	if req.Duration > 0 {
		alert.Duration = req.Duration
	}
	if req.Severity != "" {
		alert.Severity = req.Severity
	}
	if req.IsActive != nil {
		alert.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateAlert(ctx, alert); err != nil {
		return nil, err
	}

	s.logger.Info("Monitoring alert updated",
		zap.String("alert_id", alert.ID.String()),
		zap.String("name", alert.Name),
	)

	return alert, nil
}

func (s *MonitoringService) DeleteAlert(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := s.GetAlertByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteAlert(ctx, id); err != nil {
		return err
	}

	s.logger.Info("Monitoring alert deleted", zap.String("alert_id", id.String()))
	return nil
}

// Monitoring Alert Log operations
func (s *MonitoringService) ListAlertLogs(ctx context.Context, tenantID, alertID uuid.UUID, limit, offset int) ([]models.MonitoringAlertLog, int, error) {
	// Verify access
	_, err := s.GetAlertByID(ctx, tenantID, alertID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListAlertLogsByAlert(ctx, alertID, limit, offset)
}

func (s *MonitoringService) ListAlertLogsByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.MonitoringAlertLog, int, error) {
	return s.repo.ListAlertLogsByTenant(ctx, tenantID, limit, offset)
}

// Monitoring Dashboard operations
func (s *MonitoringService) CreateDashboard(ctx context.Context, tenantID uuid.UUID, req *models.CreateMonitoringDashboardRequest) (*models.MonitoringDashboard, error) {
	dashboard := &models.MonitoringDashboard{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Layout:      req.Layout,
		Widgets:     req.Widgets,
		IsDefault:   req.IsDefault,
	}

	if err := s.repo.CreateDashboard(ctx, dashboard); err != nil {
		return nil, err
	}

	s.logger.Info("Monitoring dashboard created",
		zap.String("dashboard_id", dashboard.ID.String()),
		zap.String("name", dashboard.Name),
	)

	return dashboard, nil
}

func (s *MonitoringService) GetDashboardByID(ctx context.Context, tenantID, id uuid.UUID) (*models.MonitoringDashboard, error) {
	dashboard, err := s.repo.GetDashboardByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if dashboard.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return dashboard, nil
}

func (s *MonitoringService) ListDashboards(ctx context.Context, tenantID uuid.UUID) ([]models.MonitoringDashboard, error) {
	return s.repo.ListDashboardsByTenant(ctx, tenantID)
}

func (s *MonitoringService) UpdateDashboard(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateMonitoringDashboardRequest) (*models.MonitoringDashboard, error) {
	dashboard, err := s.GetDashboardByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		dashboard.Name = req.Name
	}
	if req.Description != "" {
		dashboard.Description = req.Description
	}
	if req.Layout != nil {
		dashboard.Layout = req.Layout
	}
	if req.Widgets != nil {
		dashboard.Widgets = req.Widgets
	}
	if req.IsDefault != nil {
		dashboard.IsDefault = *req.IsDefault
	}

	if err := s.repo.UpdateDashboard(ctx, dashboard); err != nil {
		return nil, err
	}

	s.logger.Info("Monitoring dashboard updated",
		zap.String("dashboard_id", dashboard.ID.String()),
		zap.String("name", dashboard.Name),
	)

	return dashboard, nil
}

func (s *MonitoringService) DeleteDashboard(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := s.GetDashboardByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteDashboard(ctx, id); err != nil {
		return err
	}

	s.logger.Info("Monitoring dashboard deleted", zap.String("dashboard_id", id.String()))
	return nil
}

// Helper methods
func (s *MonitoringService) checkAlerts(serverID, tenantID uuid.UUID, metric string, value float64) {
	ctx := context.Background()
	alerts, err := s.repo.ListAlertsByServer(ctx, serverID)
	if err != nil {
		s.logger.Error("Failed to list alerts for server", zap.Error(err))
		return
	}

	for _, alert := range alerts {
		if !alert.IsActive || alert.Metric != metric {
			continue
		}

		triggered := false
		switch alert.Condition {
		case "gt":
			triggered = value > alert.Threshold
		case "lt":
			triggered = value < alert.Threshold
		case "gte":
			triggered = value >= alert.Threshold
		case "lte":
			triggered = value <= alert.Threshold
		case "eq":
			triggered = value == alert.Threshold
		case "ne":
			triggered = value != alert.Threshold
		}

		if triggered {
			// Create alert log
			log := &models.MonitoringAlertLog{
				AlertID:  alert.ID,
				TenantID: tenantID,
				ServerID: &serverID,
				Value:    value,
				Message:  fmt.Sprintf("Alert triggered: %s %s %f (current: %f)", metric, alert.Condition, alert.Threshold, value),
				Status:   "triggered",
			}
			if err := s.repo.CreateAlertLog(ctx, log); err != nil {
				s.logger.Error("Failed to create alert log", zap.Error(err))
			}

			// Update alert last triggered time
			now := time.Now()
			alert.LastTriggeredAt = &now
			alert.Status = "triggered"
			if err := s.repo.UpdateAlert(ctx, &alert); err != nil {
				s.logger.Error("Failed to update alert", zap.Error(err))
			}

			s.logger.Warn("Alert triggered",
				zap.String("alert_id", alert.ID.String()),
				zap.String("name", alert.Name),
				zap.String("metric", metric),
				zap.Float64("value", value),
				zap.Float64("threshold", alert.Threshold),
			)
		}
	}
}
