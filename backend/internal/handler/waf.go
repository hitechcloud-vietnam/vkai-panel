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

type WAFHandler struct {
	service *service.WAFService
	logger  *zap.Logger
}

func NewWAFHandler(service *service.WAFService, logger *zap.Logger) *WAFHandler {
	return &WAFHandler{service: service, logger: logger}
}

// Rules

func (h *WAFHandler) ListRules(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	rules, err := h.service.ListRules(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list WAF rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list WAF rules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *WAFHandler) GetRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	rule, err := h.service.GetRule(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "WAF rule not found"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *WAFHandler) CreateRule(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var rule models.WAFRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	rule.TenantID = tenantID
	if err := h.service.CreateRule(c.Request.Context(), &rule); err != nil {
		h.logger.Error("Failed to create WAF rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create WAF rule"})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *WAFHandler) UpdateRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	var rule models.WAFRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	rule.ID = id
	if err := h.service.UpdateRule(c.Request.Context(), &rule); err != nil {
		h.logger.Error("Failed to update WAF rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update WAF rule"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *WAFHandler) DeleteRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	if err := h.service.DeleteRule(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to delete WAF rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete WAF rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "WAF rule deleted"})
}

func (h *WAFHandler) ToggleRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := h.service.ToggleRule(c.Request.Context(), id, req.Enabled); err != nil {
		h.logger.Error("Failed to toggle WAF rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle WAF rule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "WAF rule toggled"})
}

// Policies

func (h *WAFHandler) ListPolicies(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	policies, err := h.service.ListPolicies(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list WAF policies", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list WAF policies"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

func (h *WAFHandler) GetPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	policy, err := h.service.GetPolicy(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "WAF policy not found"})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *WAFHandler) CreatePolicy(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var policy models.WAFPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	policy.TenantID = tenantID
	if err := h.service.CreatePolicy(c.Request.Context(), &policy); err != nil {
		h.logger.Error("Failed to create WAF policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create WAF policy"})
		return
	}
	c.JSON(http.StatusCreated, policy)
}

func (h *WAFHandler) UpdatePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	var policy models.WAFPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	policy.ID = id
	if err := h.service.UpdatePolicy(c.Request.Context(), &policy); err != nil {
		h.logger.Error("Failed to update WAF policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update WAF policy"})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *WAFHandler) DeletePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	if err := h.service.DeletePolicy(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to delete WAF policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete WAF policy"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "WAF policy deleted"})
}

// Events

func (h *WAFHandler) ListEvents(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	events, err := h.service.ListEvents(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list WAF events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list WAF events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

// Stats

func (h *WAFHandler) GetStats(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	stats, err := h.service.GetStats(c.Request.Context(), tenantID, days)
	if err != nil {
		h.logger.Error("Failed to get WAF stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get WAF stats"})
		return
	}
	c.JSON(http.StatusOK, stats)
}
