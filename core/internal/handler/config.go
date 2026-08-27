package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"go.uber.org/zap"
)

// ConfigHandler handles config HTTP requests
type ConfigHandler struct {
	service *service.ConfigService
	logger  *zap.Logger
}

// NewConfigHandler creates a new config handler
func NewConfigHandler(service *service.ConfigService, logger *zap.Logger) *ConfigHandler {
	return &ConfigHandler{
		service: service,
		logger:  logger,
	}
}

// CreateSnapshot handles POST /config/snapshots
func (h *ConfigHandler) CreateSnapshot(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var snapshot config.ConfigSnapshot
	if err := c.ShouldBindJSON(&snapshot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Get user ID from context
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err == nil {
			snapshot.UserID = &userID
		}
	}

	if err := h.service.CreateSnapshot(c.Request.Context(), tenantID, &snapshot); err != nil {
		h.logger.Error("Failed to create config snapshot", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create config snapshot"})
		return
	}

	c.JSON(http.StatusCreated, snapshot)
}

// GetSnapshot handles GET /config/snapshots/:id
func (h *ConfigHandler) GetSnapshot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID"})
		return
	}

	snapshot, err := h.service.GetSnapshot(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Snapshot not found"})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

// ListSnapshots handles GET /config/snapshots
func (h *ConfigHandler) ListSnapshots(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	filter := &config.ConfigFilter{
		ConfigType: config.ConfigType(c.Query("config_type")),
		Name:       c.Query("name"),
	}

	if serverIDStr := c.Query("server_id"); serverIDStr != "" {
		serverID, err := uuid.Parse(serverIDStr)
		if err == nil {
			filter.ServerID = &serverID
		}
	}

	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		isActive := isActiveStr == "true"
		filter.IsActive = &isActive
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	filter.Page = page
	filter.PageSize = pageSize

	snapshots, total, err := h.service.ListSnapshots(c.Request.Context(), tenantID, filter)
	if err != nil {
		h.logger.Error("Failed to list config snapshots", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list config snapshots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      snapshots,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Rollback handles POST /config/rollback
func (h *ConfigHandler) Rollback(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req config.ConfigRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	snapshot, err := h.service.Rollback(c.Request.Context(), tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to rollback config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rollback config"})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

// GetDiff handles GET /config/diff
func (h *ConfigHandler) GetDiff(c *gin.Context) {
	id1Str := c.Query("id1")
	id2Str := c.Query("id2")

	id1, err := uuid.Parse(id1Str)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID 1"})
		return
	}

	id2, err := uuid.Parse(id2Str)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID 2"})
		return
	}

	diff, err := h.service.GetDiff(c.Request.Context(), id1, id2)
	if err != nil {
		h.logger.Error("Failed to get config diff", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config diff"})
		return
	}

	c.JSON(http.StatusOK, diff)
}

// GetSnapshotHistory handles GET /config/history
func (h *ConfigHandler) GetSnapshotHistory(c *gin.Context) {
	configType := config.ConfigType(c.Query("config_type"))
	name := c.Query("name")
	serverIDStr := c.Query("server_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	snapshots, err := h.service.GetSnapshotHistory(c.Request.Context(), configType, name, serverID, limit)
	if err != nil {
		h.logger.Error("Failed to get config history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config history"})
		return
	}

	c.JSON(http.StatusOK, snapshots)
}

// GetConfigStats handles GET /config/stats
func (h *ConfigHandler) GetConfigStats(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	stats, err := h.service.GetConfigStats(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get config stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// DeleteSnapshot handles DELETE /config/snapshots/:id
func (h *ConfigHandler) DeleteSnapshot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID"})
		return
	}

	if err := h.service.DeleteSnapshot(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to delete config snapshot", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete config snapshot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Snapshot deleted"})
}

// CleanupOldSnapshots handles POST /config/cleanup
func (h *ConfigHandler) CleanupOldSnapshots(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	keepVersions, _ := strconv.Atoi(c.DefaultQuery("keep_versions", "10"))

	count, err := h.service.CleanupOldSnapshots(c.Request.Context(), tenantID, keepVersions)
	if err != nil {
		h.logger.Error("Failed to cleanup old snapshots", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup old snapshots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Old snapshots cleaned up",
		"count":   count,
	})
}

// ValidateConfig handles POST /config/validate
func (h *ConfigHandler) ValidateConfig(c *gin.Context) {
	var req struct {
		ConfigType config.ConfigType `json:"config_type"`
		Content    string            `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	validation := h.service.ValidateConfig(req.ConfigType, req.Content)
	c.JSON(http.StatusOK, validation)
}

// Template handlers

// CreateTemplate handles POST /config/templates
func (h *ConfigHandler) CreateTemplate(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var template config.ConfigTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := h.service.CreateTemplate(c.Request.Context(), tenantID, &template); err != nil {
		h.logger.Error("Failed to create config template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create config template"})
		return
	}

	c.JSON(http.StatusCreated, template)
}

// GetTemplate handles GET /config/templates/:id
func (h *ConfigHandler) GetTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	template, err := h.service.GetTemplate(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// ListTemplates handles GET /config/templates
func (h *ConfigHandler) ListTemplates(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	configType := config.ConfigType(c.Query("config_type"))

	templates, err := h.service.ListTemplates(c.Request.Context(), tenantID, configType)
	if err != nil {
		h.logger.Error("Failed to list config templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list config templates"})
		return
	}

	c.JSON(http.StatusOK, templates)
}

// UpdateTemplate handles PUT /config/templates/:id
func (h *ConfigHandler) UpdateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var template config.ConfigTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	template.ID = id
	if err := h.service.UpdateTemplate(c.Request.Context(), &template); err != nil {
		h.logger.Error("Failed to update config template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config template"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// DeleteTemplate handles DELETE /config/templates/:id
func (h *ConfigHandler) DeleteTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	if err := h.service.DeleteTemplate(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to delete config template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete config template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted"})
}
