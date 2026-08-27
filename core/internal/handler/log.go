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

type LogHandler struct {
	service *service.LogService
	logger  *zap.Logger
}

func NewLogHandler(service *service.LogService, logger *zap.Logger) *LogHandler {
	return &LogHandler{
		service: service,
		logger:  logger,
	}
}

// Log entries
func (h *LogHandler) SearchEntries(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	req := &models.LogSearchRequest{
		Source: c.Query("source"),
		Level:  c.Query("level"),
		Query:  c.Query("query"),
	}

	if serverIDStr := c.Query("server_id"); serverIDStr != "" {
		serverID, err := uuid.Parse(serverIDStr)
		if err == nil {
			req.ServerID = &serverID
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

	entries, total, err := h.service.SearchEntries(c.Request.Context(), tenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
		"limit":   req.Limit,
		"offset":  req.Offset,
	})
}

func (h *LogHandler) RecordEntry(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("server_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	var req struct {
		Source  string        `json:"source" binding:"required"`
		Level   string        `json:"level" binding:"required"`
		Message string        `json:"message" binding:"required"`
		Details models.JSONMap `json:"details"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.RecordEntry(c.Request.Context(), tenantID, serverID, req.Source, req.Level, req.Message, req.Details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "log entry recorded"})
}

func (h *LogHandler) CleanupOldEntries(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	retentionDays, _ := strconv.Atoi(c.DefaultQuery("retention_days", "30"))

	deleted, err := h.service.CleanupOldEntries(c.Request.Context(), tenantID, retentionDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "old entries cleaned up",
		"deleted": deleted,
	})
}

// Log sources
func (h *LogHandler) CreateSource(c *gin.Context) {
	var req models.CreateLogSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	source, err := h.service.CreateSource(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, source)
}

func (h *LogHandler) GetSource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	source, err := h.service.GetSourceByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, source)
}

func (h *LogHandler) ListSources(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	var serverID *uuid.UUID
	if serverIDStr := c.Query("server_id"); serverIDStr != "" {
		sid, err := uuid.Parse(serverIDStr)
		if err == nil {
			serverID = &sid
		}
	}

	sources, err := h.service.ListSources(c.Request.Context(), tenantID, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sources": sources})
}

func (h *LogHandler) UpdateSource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source id"})
		return
	}

	var req models.UpdateLogSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	source, err := h.service.UpdateSource(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, source)
}

func (h *LogHandler) DeleteSource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteSource(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "source deleted"})
}

// Log rotation
func (h *LogHandler) CreateRotation(c *gin.Context) {
	var req models.CreateLogRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	rotation, err := h.service.CreateRotation(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rotation)
}

func (h *LogHandler) GetRotation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	rotation, err := h.service.GetRotationByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rotation)
}

func (h *LogHandler) ListRotations(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	var serverID *uuid.UUID
	if serverIDStr := c.Query("server_id"); serverIDStr != "" {
		sid, err := uuid.Parse(serverIDStr)
		if err == nil {
			serverID = &sid
		}
	}

	rotations, err := h.service.ListRotations(c.Request.Context(), tenantID, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rotations": rotations})
}

func (h *LogHandler) UpdateRotation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation id"})
		return
	}

	var req models.UpdateLogRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	rotation, err := h.service.UpdateRotation(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rotation)
}

func (h *LogHandler) DeleteRotation(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rotation id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteRotation(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rotation deleted"})
}
