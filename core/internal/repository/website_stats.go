package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type WebsiteStatsRepository struct {
	db *sqlx.DB
}

func NewWebsiteStatsRepository(db *sqlx.DB) *WebsiteStatsRepository {
	return &WebsiteStatsRepository{db: db}
}

// GetOverview returns aggregated statistics for a website
func (r *WebsiteStatsRepository) GetOverview(ctx context.Context, tenantID, websiteID uuid.UUID, since time.Time) (*models.WebsiteStatsOverview, error) {
	overview := &models.WebsiteStatsOverview{}

	// Get totals
	err := r.db.QueryRowContext(ctx, `
		SELECT 
			COALESCE(SUM(page_views), 0),
			COALESCE(SUM(unique_visitors), 0),
			COALESCE(SUM(total_bandwidth), 0),
			COALESCE(AVG(avg_response_time), 0),
			COALESCE(AVG(bounce_rate), 0)
		FROM website_stats 
		WHERE tenant_id = $1 AND website_id = $2 AND date >= $3
	`, tenantID, websiteID, since).Scan(
		&overview.TotalPageViews,
		&overview.TotalUniqueVisitors,
		&overview.TotalBandwidth,
		&overview.AvgResponseTime,
		&overview.AvgBounceRate,
	)
	if err != nil {
		return nil, err
	}

	// Get top pages
	rows, err := r.db.QueryContext(ctx, `
		SELECT path, SUM(page_views) as page_views
		FROM website_page_stats 
		WHERE tenant_id = $1 AND website_id = $2 AND date >= $3
		GROUP BY path 
		ORDER BY page_views DESC 
		LIMIT 10
	`, tenantID, websiteID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var page struct {
			Path      string `json:"path"`
			PageViews int64  `json:"page_views"`
		}
		if err := rows.Scan(&page.Path, &page.PageViews); err != nil {
			return nil, err
		}
		overview.TopPages = append(overview.TopPages, page)
	}

	// Get top referrers
	rows2, err := r.db.QueryContext(ctx, `
		SELECT referer, SUM(visits) as visits
		FROM website_referrer_stats 
		WHERE tenant_id = $1 AND website_id = $2 AND date >= $3
		GROUP BY referer 
		ORDER BY visits DESC 
		LIMIT 10
	`, tenantID, websiteID, since)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var ref struct {
			Referer string `json:"referer"`
			Visits  int64  `json:"visits"`
		}
		if err := rows2.Scan(&ref.Referer, &ref.Visits); err != nil {
			return nil, err
		}
		overview.TopReferrers = append(overview.TopReferrers, ref)
	}

	// Get top countries
	rows3, err := r.db.QueryContext(ctx, `
		SELECT country, SUM(visitors) as visitors
		FROM website_country_stats 
		WHERE tenant_id = $1 AND website_id = $2 AND date >= $3
		GROUP BY country 
		ORDER BY visitors DESC 
		LIMIT 10
	`, tenantID, websiteID, since)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()

	for rows3.Next() {
		var country struct {
			Country  string `json:"country"`
			Visitors int64  `json:"visitors"`
		}
		if err := rows3.Scan(&country.Country, &country.Visitors); err != nil {
			return nil, err
		}
		overview.TopCountries = append(overview.TopCountries, country)
	}

	// Get daily stats
	rows4, err := r.db.QueryContext(ctx, `
		SELECT date, page_views, unique_visitors, total_bandwidth
		FROM website_stats 
		WHERE tenant_id = $1 AND website_id = $2 AND date >= $3
		ORDER BY date ASC
	`, tenantID, websiteID, since)
	if err != nil {
		return nil, err
	}
	defer rows4.Close()

	for rows4.Next() {
		var daily struct {
			Date           string `json:"date"`
			PageViews      int64  `json:"page_views"`
			UniqueVisitors int64  `json:"unique_visitors"`
			Bandwidth      int64  `json:"bandwidth"`
		}
		var date time.Time
		if err := rows4.Scan(&date, &daily.PageViews, &daily.UniqueVisitors, &daily.Bandwidth); err != nil {
			return nil, err
		}
		daily.Date = date.Format("2006-01-02")
		overview.DailyStats = append(overview.DailyStats, daily)
	}

	// Get recent visitors
	rows5, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, website_id, visitor_ip, user_agent, path, method, 
			   status_code, response_time, referer, country, bandwidth, created_at
		FROM website_visitor_logs 
		WHERE tenant_id = $1 AND website_id = $2
		ORDER BY created_at DESC 
		LIMIT 20
	`, tenantID, websiteID)
	if err != nil {
		return nil, err
	}
	defer rows5.Close()

	for rows5.Next() {
		var log models.WebsiteVisitorLog
		if err := rows5.Scan(
			&log.ID, &log.TenantID, &log.WebsiteID, &log.VisitorIP, &log.UserAgent,
			&log.Path, &log.Method, &log.StatusCode, &log.ResponseTime,
			&log.Referer, &log.Country, &log.Bandwidth, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		overview.RecentVisitors = append(overview.RecentVisitors, log)
	}

	return overview, nil
}

// RecordVisitorLog records an individual visitor log entry
func (r *WebsiteStatsRepository) RecordVisitorLog(ctx context.Context, log *models.WebsiteVisitorLog) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO website_visitor_logs (tenant_id, website_id, visitor_ip, user_agent, path, method, 
			   status_code, response_time, referer, country, bandwidth)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`, log.TenantID, log.WebsiteID, log.VisitorIP, log.UserAgent, log.Path, log.Method,
		log.StatusCode, log.ResponseTime, log.Referer, log.Country, log.Bandwidth).
		Scan(&log.ID, &log.CreatedAt)
}

// UpdateDailyStats updates or inserts daily aggregated statistics
func (r *WebsiteStatsRepository) UpdateDailyStats(ctx context.Context, stats *models.WebsiteStats) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO website_stats (tenant_id, website_id, date, page_views, unique_visitors, total_bandwidth, avg_response_time, bounce_rate)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, website_id, date) 
		DO UPDATE SET 
			page_views = website_stats.page_views + EXCLUDED.page_views,
			unique_visitors = website_stats.unique_visitors + EXCLUDED.unique_visitors,
			total_bandwidth = website_stats.total_bandwidth + EXCLUDED.total_bandwidth,
			avg_response_time = (website_stats.avg_response_time + EXCLUDED.avg_response_time) / 2,
			bounce_rate = (website_stats.bounce_rate + EXCLUDED.bounce_rate) / 2
	`, stats.TenantID, stats.WebsiteID, stats.Date, stats.PageViews, stats.UniqueVisitors,
		stats.TotalBandwidth, stats.AvgResponseTime, stats.BounceRate)
	return err
}

// UpdatePageStats updates or inserts per-page statistics
func (r *WebsiteStatsRepository) UpdatePageStats(ctx context.Context, stats *models.WebsitePageStats) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO website_page_stats (tenant_id, website_id, path, page_views, unique_views, avg_time_on_page, date)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, website_id, path, date) 
		DO UPDATE SET 
			page_views = website_page_stats.page_views + EXCLUDED.page_views,
			unique_views = website_page_stats.unique_views + EXCLUDED.unique_views,
			avg_time_on_page = (website_page_stats.avg_time_on_page + EXCLUDED.avg_time_on_page) / 2
	`, stats.TenantID, stats.WebsiteID, stats.Path, stats.PageViews, stats.UniqueViews,
		stats.AvgTimeOnPage, stats.Date)
	return err
}

// UpdateReferrerStats updates or inserts referrer statistics
func (r *WebsiteStatsRepository) UpdateReferrerStats(ctx context.Context, stats *models.WebsiteReferrerStats) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO website_referrer_stats (tenant_id, website_id, referer, visits, date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, website_id, referer, date) 
		DO UPDATE SET visits = website_referrer_stats.visits + EXCLUDED.visits
	`, stats.TenantID, stats.WebsiteID, stats.Referer, stats.Visits, stats.Date)
	return err
}

// UpdateCountryStats updates or inserts country statistics
func (r *WebsiteStatsRepository) UpdateCountryStats(ctx context.Context, stats *models.WebsiteCountryStats) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO website_country_stats (tenant_id, website_id, country, visitors, date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, website_id, country, date) 
		DO UPDATE SET visitors = website_country_stats.visitors + EXCLUDED.visitors
	`, stats.TenantID, stats.WebsiteID, stats.Country, stats.Visitors, stats.Date)
	return err
}

// ListVisitorLogs returns visitor logs with pagination
func (r *WebsiteStatsRepository) ListVisitorLogs(ctx context.Context, tenantID, websiteID uuid.UUID, limit, offset int) ([]models.WebsiteVisitorLog, error) {
	var logs []models.WebsiteVisitorLog
	err := r.db.SelectContext(ctx, &logs, `
		SELECT id, tenant_id, website_id, visitor_ip, user_agent, path, method, 
			   status_code, response_time, referer, country, bandwidth, created_at
		FROM website_visitor_logs 
		WHERE tenant_id = $1 AND website_id = $2
		ORDER BY created_at DESC 
		LIMIT $3 OFFSET $4
	`, tenantID, websiteID, limit, offset)
	return logs, err
}

// GetVisitorLogCount returns total count of visitor logs
func (r *WebsiteStatsRepository) GetVisitorLogCount(ctx context.Context, tenantID, websiteID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM website_visitor_logs 
		WHERE tenant_id = $1 AND website_id = $2
	`, tenantID, websiteID)
	return count, err
}
