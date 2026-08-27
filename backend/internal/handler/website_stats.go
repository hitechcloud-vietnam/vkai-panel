package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"go.uber.org/zap"
)

type WebsiteStatsHandler struct {
	service *service.WebsiteStatsService
	logger  *zap.Logger
}

func NewWebsiteStatsHandler(service *service.WebsiteStatsService, logger *zap.Logger) *WebsiteStatsHandler {
	return &WebsiteStatsHandler{service: service, logger: logger}
}

// GetOverview returns aggregated statistics for a website
func (h *WebsiteStatsHandler) GetOverview(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	websiteIDStr := c.Query("website_id")
	if websiteIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "website_id is required"})
		return
	}

	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid website_id"})
		return
	}

	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 {
		days = 30
	}

	overview, err := h.service.GetOverview(c.Request.Context(), tenantID, websiteID, days)
	if err != nil {
		h.logger.Error("Failed to get website stats overview", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get website statistics"})
		return
	}

	c.JSON(http.StatusOK, overview)
}

// ListVisitorLogs returns visitor logs with pagination
func (h *WebsiteStatsHandler) ListVisitorLogs(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	websiteIDStr := c.Query("website_id")
	if websiteIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "website_id is required"})
		return
	}

	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid website_id"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}

	offsetStr := c.DefaultQuery("offset", "0")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	logs, err := h.service.ListVisitorLogs(c.Request.Context(), tenantID, websiteID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list visitor logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list visitor logs"})
		return
	}

	total, err := h.service.GetVisitorLogCount(c.Request.Context(), tenantID, websiteID)
	if err != nil {
		h.logger.Error("Failed to get visitor log count", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get visitor log count"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
	})
}

// RecordVisitorLog records an individual visitor log entry
func (h *WebsiteStatsHandler) RecordVisitorLog(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		WebsiteID    string  `json:"website_id" binding:"required"`
		VisitorIP    string  `json:"visitor_ip"`
		UserAgent    string  `json:"user_agent"`
		Path         string  `json:"path" binding:"required"`
		Method       string  `json:"method"`
		StatusCode   int     `json:"status_code"`
		ResponseTime float64 `json:"response_time"`
		Referer      string  `json:"referer"`
		Country      string  `json:"country"`
		Bandwidth    int64   `json:"bandwidth"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	websiteID, err := uuid.Parse(req.WebsiteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid website_id"})
		return
	}

	log := &models.WebsiteVisitorLog{
		TenantID:     tenantID,
		WebsiteID:    websiteID,
		VisitorIP:    req.VisitorIP,
		UserAgent:    req.UserAgent,
		Path:         req.Path,
		Method:       req.Method,
		StatusCode:   req.StatusCode,
		ResponseTime: req.ResponseTime,
		Referer:      req.Referer,
		Country:      req.Country,
		Bandwidth:    req.Bandwidth,
	}

	if err := h.service.RecordVisitorLog(c.Request.Context(), log); err != nil {
		h.logger.Error("Failed to record visitor log", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record visitor log"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": log.ID, "created_at": log.CreatedAt})
}
