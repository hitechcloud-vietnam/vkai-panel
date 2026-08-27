package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"go.uber.org/zap"
)

type WebsiteStatsService struct {
	repo   *repository.WebsiteStatsRepository
	logger *zap.Logger
}

func NewWebsiteStatsService(repo *repository.WebsiteStatsRepository, logger *zap.Logger) *WebsiteStatsService {
	return &WebsiteStatsService{repo: repo, logger: logger}
}

// GetOverview returns aggregated statistics for a website
func (s *WebsiteStatsService) GetOverview(ctx context.Context, tenantID, websiteID uuid.UUID, days int) (*models.WebsiteStatsOverview, error) {
	since := time.Now().AddDate(0, 0, -days)
	return s.repo.GetOverview(ctx, tenantID, websiteID, since)
}

// RecordVisitorLog records an individual visitor log entry
func (s *WebsiteStatsService) RecordVisitorLog(ctx context.Context, log *models.WebsiteVisitorLog) error {
	return s.repo.RecordVisitorLog(ctx, log)
}

// UpdateDailyStats updates or inserts daily aggregated statistics
func (s *WebsiteStatsService) UpdateDailyStats(ctx context.Context, stats *models.WebsiteStats) error {
	return s.repo.UpdateDailyStats(ctx, stats)
}

// UpdatePageStats updates or inserts per-page statistics
func (s *WebsiteStatsService) UpdatePageStats(ctx context.Context, stats *models.WebsitePageStats) error {
	return s.repo.UpdatePageStats(ctx, stats)
}

// UpdateReferrerStats updates or inserts referrer statistics
func (s *WebsiteStatsService) UpdateReferrerStats(ctx context.Context, stats *models.WebsiteReferrerStats) error {
	return s.repo.UpdateReferrerStats(ctx, stats)
}

// UpdateCountryStats updates or inserts country statistics
func (s *WebsiteStatsService) UpdateCountryStats(ctx context.Context, stats *models.WebsiteCountryStats) error {
	return s.repo.UpdateCountryStats(ctx, stats)
}

// ListVisitorLogs returns visitor logs with pagination
func (s *WebsiteStatsService) ListVisitorLogs(ctx context.Context, tenantID, websiteID uuid.UUID, limit, offset int) ([]models.WebsiteVisitorLog, error) {
	return s.repo.ListVisitorLogs(ctx, tenantID, websiteID, limit, offset)
}

// GetVisitorLogCount returns total count of visitor logs
func (s *WebsiteStatsService) GetVisitorLogCount(ctx context.Context, tenantID, websiteID uuid.UUID) (int64, error) {
	return s.repo.GetVisitorLogCount(ctx, tenantID, websiteID)
}
