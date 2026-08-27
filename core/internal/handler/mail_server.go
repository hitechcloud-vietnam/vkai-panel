package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type MailServerHandler struct {
	service *service.MailServerService
	logger  *zap.Logger
}

func NewMailServerHandler(service *service.MailServerService, logger *zap.Logger) *MailServerHandler {
	return &MailServerHandler{service: service, logger: logger}
}

// --- Domains ---

func (h *MailServerHandler) CreateDomain(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.CreateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	domain, err := h.service.CreateDomain(c.Request.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to create mail domain", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create domain"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"domain": domain})
}

func (h *MailServerHandler) ListDomains(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	domains, err := h.service.ListDomains(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list mail domains", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list domains"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"domains": domains})
}

func (h *MailServerHandler) GetDomain(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}
	domain, err := h.service.GetDomain(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"domain": domain})
}

func (h *MailServerHandler) DeleteDomain(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid domain ID"})
		return
	}
	if err := h.service.DeleteDomain(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete domain"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Domain deleted"})
}

// --- Accounts ---

func (h *MailServerHandler) CreateAccount(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	account, err := h.service.CreateAccount(c.Request.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to create mail account", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": account})
}

func (h *MailServerHandler) ListAccounts(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	accounts, err := h.service.ListAccounts(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list mail accounts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list accounts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": accounts})
}

func (h *MailServerHandler) GetAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}
	account, err := h.service.GetAccount(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": account})
}

func (h *MailServerHandler) UpdateAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}
	var req models.UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	account, err := h.service.UpdateAccount(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update account"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": account})
}

func (h *MailServerHandler) DeleteAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}
	if err := h.service.DeleteAccount(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

// --- Aliases ---

func (h *MailServerHandler) CreateAlias(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.CreateAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alias, err := h.service.CreateAlias(c.Request.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("Failed to create mail alias", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create alias"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alias": alias})
}

func (h *MailServerHandler) ListAliases(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	aliases, err := h.service.ListAliases(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list mail aliases", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list aliases"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

func (h *MailServerHandler) DeleteAlias(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alias ID"})
		return
	}
	if err := h.service.DeleteAlias(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete alias"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Alias deleted"})
}

// --- Queue ---

func (h *MailServerHandler) ListQueue(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	items, err := h.service.ListQueueItems(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list mail queue", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list queue"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queue": items})
}

func (h *MailServerHandler) DeleteQueueItem(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue item ID"})
		return
	}
	if err := h.service.DeleteQueueItem(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete queue item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Queue item deleted"})
}

func (h *MailServerHandler) FlushQueue(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if err := h.service.FlushQueue(c.Request.Context(), tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to flush queue"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Queue flushed"})
}

// --- Spam Filter ---

func (h *MailServerHandler) GetSpamFilter(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	sf, err := h.service.GetSpamFilter(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get spam filter"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"spam_filter": sf})
}

func (h *MailServerHandler) UpdateSpamFilter(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.UpdateSpamFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sf, err := h.service.UpdateSpamFilter(c.Request.Context(), tenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update spam filter"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"spam_filter": sf})
}

// --- Server Config ---

func (h *MailServerHandler) GetServerConfig(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	cfg, err := h.service.GetServerConfig(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get server config"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": cfg})
}

func (h *MailServerHandler) UpdateServerConfig(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.UpdateServerConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.service.UpdateServerConfig(c.Request.Context(), tenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update server config"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": cfg})
}

// --- Stats ---

func (h *MailServerHandler) GetStats(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	stats, err := h.service.GetStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get stats"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
