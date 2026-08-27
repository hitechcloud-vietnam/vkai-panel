package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type AuditHandler struct {
	service *service.AuditService
	logger  *zap.Logger
}

func NewAuditHandler(service *service.AuditService, logger *zap.Logger) *AuditHandler {
	return &AuditHandler{
		service: service,
		logger:  logger,
	}
}

// Service exposes the audit service so other handlers constructed inside the
// router can record entries without changing the router's constructor
// signature, which the API entry point passes positionally.
func (h *AuditHandler) Service() *service.AuditService {
	if h == nil {
		return nil
	}
	return h.service
}

func (h *AuditHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit log id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	log, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, log)
}

func (h *AuditHandler) Search(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	req := &models.AuditLogSearchRequest{
		Action:   c.Query("action"),
		Resource: c.Query("resource"),
		Status:   c.Query("status"),
	}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err == nil {
			req.UserID = &userID
		}
	}

	if resourceIDStr := c.Query("resource_id"); resourceIDStr != "" {
		resourceID, err := uuid.Parse(resourceIDStr)
		if err == nil {
			req.ResourceID = &resourceID
		}
	}

	if startStr := c.Query("start"); startStr != "" {
		start, err := time.Parse(time.RFC3339, startStr)
		if err == nil {
			req.Start = &start
		}
	}

	if endStr := c.Query("end"); endStr != "" {
		end, err := time.Parse(time.RFC3339, endStr)
		if err == nil {
			req.End = &end
		}
	}

	req.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "100"))
	req.Offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.service.Search(c.Request.Context(), tenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  req.Limit,
		"offset": req.Offset,
	})
}

func (h *AuditHandler) GetStats(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	stats, err := h.service.GetStats(c.Request.Context(), tenantID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *AuditHandler) CleanupOld(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))

	deleted, err := h.service.CleanupOld(c.Request.Context(), tenantID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "old audit logs cleaned up",
		"deleted": deleted,
	})
}
