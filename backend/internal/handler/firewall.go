package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type FirewallHandler struct {
	firewallService *service.FirewallService
}

func NewFirewallHandler(firewallService *service.FirewallService) *FirewallHandler {
	return &FirewallHandler{firewallService: firewallService}
}

func (h *FirewallHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	rule, err := h.firewallService.Create(c.Request.Context(), &req, tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, rule)
}

func (h *FirewallHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid rule ID")
		return
	}

	rule, err := h.firewallService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Firewall rule not found")
		return
	}

	utils.Success(c, rule)
}

func (h *FirewallHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	rules, err := h.firewallService.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, rules)
}

func (h *FirewallHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid rule ID")
		return
	}

	rule, err := h.firewallService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Firewall rule not found")
		return
	}

	var req struct {
		Protocol  string `json:"protocol"`
		Port      string `json:"port"`
		Source    string `json:"source"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
		Status    string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Protocol != "" {
		rule.Protocol = req.Protocol
	}
	if req.Port != "" {
		rule.Port = req.Port
	}
	if req.Source != "" {
		rule.Source = req.Source
	}
	if req.Action != "" {
		rule.Action = req.Action
	}
	if req.Direction != "" {
		rule.Direction = req.Direction
	}
	if req.Status != "" {
		rule.Status = req.Status
	}

	if err := h.firewallService.Update(c.Request.Context(), rule); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, rule)
}

func (h *FirewallHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid rule ID")
		return
	}

	if err := h.firewallService.Delete(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Firewall rule deleted"})
}

func (h *FirewallHandler) GetActiveRules(c *gin.Context) {
	rules, err := h.firewallService.GetActiveRules(c.Request.Context())
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"rules": rules})
}

func (h *FirewallHandler) SaveRules(c *gin.Context) {
	if err := h.firewallService.SaveRules(c.Request.Context()); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Firewall rules saved"})
}
