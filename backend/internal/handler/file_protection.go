package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type FileProtectionHandler struct {
	service *service.FileProtectionService
	logger  *zap.Logger
}

func NewFileProtectionHandler(service *service.FileProtectionService, logger *zap.Logger) *FileProtectionHandler {
	return &FileProtectionHandler{service: service, logger: logger}
}

// --- Rules ---

func (h *FileProtectionHandler) CreateRule(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.CreateProtectionRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule, err := h.service.CreateRule(c.Request.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to create protection rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

func (h *FileProtectionHandler) ListRules(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	rules, err := h.service.ListRules(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list protection rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list rules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *FileProtectionHandler) GetRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}
	rule, err := h.service.GetRule(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

func (h *FileProtectionHandler) UpdateRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}
	var req models.UpdateProtectionRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule, err := h.service.UpdateRule(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

func (h *FileProtectionHandler) DeleteRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}
	if err := h.service.DeleteRule(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted"})
}

func (h *FileProtectionHandler) ToggleRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}
	rule, err := h.service.ToggleRule(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// --- Change Events ---

func (h *FileProtectionHandler) ListEvents(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	events, err := h.service.ListChangeEvents(c.Request.Context(), tenantID, limit)
	if err != nil {
		h.logger.Error("Failed to list change events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func (h *FileProtectionHandler) MarkEventRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}
	if err := h.service.MarkEventRead(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark event read"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Event marked as read"})
}

func (h *FileProtectionHandler) MarkAllEventsRead(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if err := h.service.MarkAllEventsRead(c.Request.Context(), tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark events read"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "All events marked as read"})
}

// --- Quarantine ---

func (h *FileProtectionHandler) ListQuarantine(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	items, err := h.service.ListQuarantineItems(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list quarantine items", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list quarantine"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"quarantine": items})
}

func (h *FileProtectionHandler) RestoreQuarantine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quarantine ID"})
		return
	}
	if err := h.service.RestoreQuarantineItem(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Item restored"})
}

func (h *FileProtectionHandler) DeleteQuarantine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quarantine ID"})
		return
	}
	if err := h.service.DeleteQuarantineItem(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Quarantine item deleted"})
}

// --- Stats ---

func (h *FileProtectionHandler) GetStats(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	stats, err := h.service.GetStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get stats"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
