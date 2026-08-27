package models

import (
	"time"

	"github.com/google/uuid"
)

// WebsiteStats represents aggregated statistics for a website
type WebsiteStats struct {
	ID              uuid.UUID `json:"id" db:"id"`
	TenantID        uuid.UUID `json:"tenant_id" db:"tenant_id"`
	WebsiteID       uuid.UUID `json:"website_id" db:"website_id"`
	Date            time.Time `json:"date" db:"date"`
	PageViews       int64     `json:"page_views" db:"page_views"`
	UniqueVisitors  int64     `json:"unique_visitors" db:"unique_visitors"`
	TotalBandwidth  int64     `json:"total_bandwidth" db:"total_bandwidth"` // bytes
	AvgResponseTime float64   `json:"avg_response_time" db:"avg_response_time"` // ms
	BounceRate      float64   `json:"bounce_rate" db:"bounce_rate"` // percentage
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// WebsitePageStats represents per-page statistics
type WebsitePageStats struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	WebsiteID   uuid.UUID `json:"website_id" db:"website_id"`
	Path        string    `json:"path" db:"path"`
	PageViews   int64     `json:"page_views" db:"page_views"`
	UniqueViews int64     `json:"unique_views" db:"unique_views"`
	AvgTimeOnPage float64 `json:"avg_time_on_page" db:"avg_time_on_page"` // seconds
	Date        time.Time `json:"date" db:"date"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// WebsiteVisitorLog represents individual visitor logs
type WebsiteVisitorLog struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	WebsiteID   uuid.UUID `json:"website_id" db:"website_id"`
	VisitorIP   string    `json:"visitor_ip" db:"visitor_ip"`
	UserAgent   string    `json:"user_agent" db:"user_agent"`
	Path        string    `json:"path" db:"path"`
	Method      string    `json:"method" db:"method"`
	StatusCode  int       `json:"status_code" db:"status_code"`
	ResponseTime float64  `json:"response_time" db:"response_time"` // ms
	Referer     string    `json:"referer" db:"referer"`
	Country     string    `json:"country" db:"country"`
	Bandwidth   int64     `json:"bandwidth" db:"bandwidth"` // bytes
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// WebsiteReferrerStats represents referrer statistics
type WebsiteReferrerStats struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	WebsiteID uuid.UUID `json:"website_id" db:"website_id"`
	Referer   string    `json:"referer" db:"referer"`
	Visits    int64     `json:"visits" db:"visits"`
	Date      time.Time `json:"date" db:"date"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WebsiteCountryStats represents visitor statistics by country
type WebsiteCountryStats struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	WebsiteID uuid.UUID `json:"website_id" db:"website_id"`
	Country   string    `json:"country" db:"country"`
	Visitors  int64     `json:"visitors" db:"visitors"`
	Date      time.Time `json:"date" db:"date"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WebsiteStatsOverview represents the overview response
type WebsiteStatsOverview struct {
	TotalPageViews      int64   `json:"total_page_views"`
	TotalUniqueVisitors int64   `json:"total_unique_visitors"`
	TotalBandwidth      int64   `json:"total_bandwidth"`
	AvgResponseTime     float64 `json:"avg_response_time"`
	AvgBounceRate       float64 `json:"avg_bounce_rate"`
	TopPages            []struct {
		Path      string `json:"path"`
		PageViews int64  `json:"page_views"`
	} `json:"top_pages"`
	TopReferrers []struct {
		Referer string `json:"referer"`
		Visits  int64  `json:"visits"`
	} `json:"top_referrers"`
	TopCountries []struct {
		Country  string `json:"country"`
		Visitors int64  `json:"visitors"`
	} `json:"top_countries"`
	DailyStats []struct {
		Date           string  `json:"date"`
		PageViews      int64   `json:"page_views"`
		UniqueVisitors int64   `json:"unique_visitors"`
		Bandwidth      int64   `json:"bandwidth"`
	} `json:"daily_stats"`
	RecentVisitors []WebsiteVisitorLog `json:"recent_visitors"`
}

// WebsiteStatsRequest represents the request parameters for stats
type WebsiteStatsRequest struct {
	WebsiteID string `form:"website_id"`
	Days      int    `form:"days,default=30"`
}
