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

type EmailMarketingHandler struct {
	service *service.EmailMarketingService
	logger  *zap.Logger
}

func NewEmailMarketingHandler(service *service.EmailMarketingService, logger *zap.Logger) *EmailMarketingHandler {
	return &EmailMarketingHandler{service: service, logger: logger}
}

// ============ Campaigns ============

func (h *EmailMarketingHandler) CreateCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	campaign := &models.EmailCampaign{
		TenantID:    tenantID,
		Name:        req.Name,
		Subject:     req.Subject,
		HTMLContent: req.HTMLContent,
		PlainText:   req.PlainText,
		FromName:    req.FromName,
		FromEmail:   req.FromEmail,
		ReplyTo:     req.ReplyTo,
		Tags:        req.Tags,
	}

	if err := h.service.CreateCampaign(c.Request.Context(), campaign); err != nil {
		h.logger.Error("Failed to create campaign", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaign": campaign})
}

func (h *EmailMarketingHandler) ListCampaigns(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	campaigns, total, err := h.service.ListCampaigns(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list campaigns", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list campaigns"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaigns": campaigns, "total": total})
}

func (h *EmailMarketingHandler) GetCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	campaign, err := h.service.GetCampaign(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Campaign not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"campaign": campaign})
}

func (h *EmailMarketingHandler) UpdateCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	var req models.UpdateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateCampaign(c.Request.Context(), tenantID, id, &req); err != nil {
		h.logger.Error("Failed to update campaign", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Campaign updated"})
}

func (h *EmailMarketingHandler) DeleteCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	if err := h.service.DeleteCampaign(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Campaign deleted"})
}

func (h *EmailMarketingHandler) SendCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	if err := h.service.SendCampaign(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Campaign sending started"})
}

func (h *EmailMarketingHandler) PauseCampaign(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	if err := h.service.PauseCampaign(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to pause campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Campaign paused"})
}

// ============ Contacts ============

func (h *EmailMarketingHandler) CreateContact(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	contact := &models.EmailContact{
		TenantID:  tenantID,
		Email:     req.Email,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Tags:      req.Tags,
		Source:    req.Source,
		Metadata:  req.Metadata,
	}

	if err := h.service.CreateContact(c.Request.Context(), contact); err != nil {
		h.logger.Error("Failed to create contact", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create contact"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contact": contact})
}

func (h *EmailMarketingHandler) ListContacts(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	search := c.Query("search")

	contacts, total, err := h.service.ListContacts(c.Request.Context(), tenantID, limit, offset, search)
	if err != nil {
		h.logger.Error("Failed to list contacts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list contacts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contacts": contacts, "total": total})
}

func (h *EmailMarketingHandler) DeleteContact(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid contact ID"})
		return
	}

	if err := h.service.DeleteContact(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete contact"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact deleted"})
}

// ============ Lists ============

func (h *EmailMarketingHandler) CreateList(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	list := &models.EmailList{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		DoubleOptIn: req.DoubleOptIn,
	}

	if err := h.service.CreateList(c.Request.Context(), list); err != nil {
		h.logger.Error("Failed to create list", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"list": list})
}

func (h *EmailMarketingHandler) ListLists(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	lists, err := h.service.ListLists(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list lists", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list lists"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"lists": lists})
}

func (h *EmailMarketingHandler) DeleteList(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid list ID"})
		return
	}

	if err := h.service.DeleteList(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "List deleted"})
}

// ============ Templates ============

func (h *EmailMarketingHandler) CreateTemplate(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template := &models.EmailTemplate{
		TenantID:    tenantID,
		Name:        req.Name,
		Subject:     req.Subject,
		HTMLContent: req.HTMLContent,
		Category:    req.Category,
	}

	if err := h.service.CreateTemplate(c.Request.Context(), template); err != nil {
		h.logger.Error("Failed to create template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"template": template})
}

func (h *EmailMarketingHandler) ListTemplates(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	templates, err := h.service.ListTemplates(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list templates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

func (h *EmailMarketingHandler) DeleteTemplate(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	if err := h.service.DeleteTemplate(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted"})
}

// ============ Stats ============

func (h *EmailMarketingHandler) GetStats(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	stats, err := h.service.GetStats(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get email stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
