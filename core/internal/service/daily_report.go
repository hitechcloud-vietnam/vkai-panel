package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type DailyReportService struct {
	repo   *repository.DailyReportRepository
	logger *zap.Logger
}

func NewDailyReportService(repo *repository.DailyReportRepository, logger *zap.Logger) *DailyReportService {
	return &DailyReportService{repo: repo, logger: logger}
}

// Reports
func (s *DailyReportService) ListReports(ctx context.Context, tenantID uuid.UUID, reportType string, limit int) ([]models.DailyReport, error) {
	if limit <= 0 {
		limit = 30
	}
	return s.repo.ListReports(ctx, tenantID, reportType, limit)
}

func (s *DailyReportService) GetReport(ctx context.Context, id uuid.UUID) (*models.DailyReport, error) {
	return s.repo.GetReport(ctx, id)
}

func (s *DailyReportService) DeleteReport(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteReport(ctx, id)
}

// Generate report
func (s *DailyReportService) GenerateReport(ctx context.Context, tenantID uuid.UUID, reportType string) (*models.DailyReport, error) {
	reportDate := time.Now().Format("2006-01-02")

	// Gather data
	serverHealth, err := s.repo.GetServerHealthSummary(ctx, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get server health summary", zap.Error(err))
		serverHealth = &models.ServerHealthSummary{}
	}
	websiteSummary, err := s.repo.GetWebsiteSummary(ctx, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get website summary", zap.Error(err))
		websiteSummary = &models.WebsiteSummary{}
	}
	securitySummary, err := s.repo.GetSecuritySummary(ctx, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get security summary", zap.Error(err))
		securitySummary = &models.SecuritySummary{}
	}
	backupSummary, err := s.repo.GetBackupSummary(ctx, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get backup summary", zap.Error(err))
		backupSummary = &models.BackupSummary{}
	}

	reportData := models.ReportData{
		Server:   *serverHealth,
		Website:  *websiteSummary,
		Security: *securitySummary,
		Backup:   *backupSummary,
	}

	dataJSON, _ := json.Marshal(reportData)

	// Build summary
	summary := fmt.Sprintf("Servers: %d/%d online | Websites: %d active | WAF blocked: %d | Backups: %d/%d successful",
		reportData.Server.OnlineServers, reportData.Server.TotalServers,
		reportData.Website.ActiveWebsites,
		reportData.Security.WAFBlocked,
		reportData.Backup.Successful, reportData.Backup.TotalBackups,
	)

	report := &models.DailyReport{
		ID:         uuid.New(),
		TenantID:   tenantID,
		ReportDate: reportDate,
		ReportType: reportType,
		Title:      fmt.Sprintf("%s Report - %s", capitalize(reportType), reportDate),
		Summary:    summary,
		Status:     "generated",
	}

	if err := s.repo.CreateReport(ctx, report); err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}

	// Create sections
	sections := []struct {
		key     string
		title   string
		content string
	}{
		{"server_health", "Server Health", fmt.Sprintf("Total: %d servers, %d online. Avg CPU: %.1f%%, Memory: %.1f%%, Disk: %.1f%%",
			reportData.Server.TotalServers, reportData.Server.OnlineServers,
			reportData.Server.AvgCPU, reportData.Server.AvgMemory, reportData.Server.AvgDisk)},
		{"website_status", "Website Status", fmt.Sprintf("Total: %d websites, %d active. SSL enabled: %d",
			reportData.Website.TotalWebsites, reportData.Website.ActiveWebsites, reportData.Website.SSLEnabled)},
		{"security", "Security Overview", fmt.Sprintf("WAF blocked: %d threats, Failed logins: %d",
			reportData.Security.WAFBlocked, reportData.Security.FailedLogins)},
		{"backups", "Backup Status", fmt.Sprintf("Total: %d, Successful: %d, Failed: %d",
			reportData.Backup.TotalBackups, reportData.Backup.Successful, reportData.Backup.Failed)},
	}

	for i, sec := range sections {
		section := &models.ReportSection{
			ID:         uuid.New(),
			ReportID:   report.ID,
			SectionKey: sec.key,
			Title:      sec.title,
			Content:    sec.content,
			DataJSON:   string(dataJSON),
			SortOrder:  i,
		}
		_ = s.repo.CreateSection(ctx, section)
	}

	report.Sections = make([]models.ReportSection, len(sections))
	return report, nil
}

// Schedules
func (s *DailyReportService) CreateSchedule(ctx context.Context, tenantID uuid.UUID, req models.CreateScheduleRequest) (*models.ReportSchedule, error) {
	schedule := &models.ReportSchedule{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Name:       req.Name,
		ReportType: req.ReportType,
		Frequency:  req.Frequency,
		Recipients: pq.StringArray(req.Recipients),
		Sections:   pq.StringArray(req.Sections),
		IsActive:   true,
	}
	if err := s.repo.CreateSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *DailyReportService) ListSchedules(ctx context.Context, tenantID uuid.UUID) ([]models.ReportSchedule, error) {
	return s.repo.ListSchedules(ctx, tenantID)
}

func (s *DailyReportService) GetSchedule(ctx context.Context, id uuid.UUID) (*models.ReportSchedule, error) {
	return s.repo.GetSchedule(ctx, id)
}

func (s *DailyReportService) UpdateSchedule(ctx context.Context, id uuid.UUID, req models.UpdateScheduleRequest) (*models.ReportSchedule, error) {
	schedule, err := s.repo.GetSchedule(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		schedule.Name = req.Name
	}
	if req.ReportType != "" {
		schedule.ReportType = req.ReportType
	}
	if req.Frequency != "" {
		schedule.Frequency = req.Frequency
	}
	if req.Recipients != nil {
		schedule.Recipients = req.Recipients
	}
	if req.Sections != nil {
		schedule.Sections = req.Sections
	}
	if req.IsActive != nil {
		schedule.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *DailyReportService) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSchedule(ctx, id)
}

// Deliveries
func (s *DailyReportService) ListDeliveries(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.ReportDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListDeliveries(ctx, tenantID, limit)
}

// Stats
func (s *DailyReportService) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.DailyReportStats, error) {
	return s.repo.GetStats(ctx, tenantID)
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
