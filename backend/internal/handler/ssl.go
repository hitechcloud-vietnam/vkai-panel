package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type SSLHandler struct {
	sslService *service.SSLService
}

func NewSSLHandler(sslService *service.SSLService) *SSLHandler {
	return &SSLHandler{sslService: sslService}
}

func (h *SSLHandler) IssueLetsEncrypt(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Domain  string `json:"domain" binding:"required"`
		Webroot string `json:"webroot" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	cert, err := h.sslService.IssueLetsEncrypt(c.Request.Context(), tenantID, req.Domain, req.Webroot)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, cert)
}

func (h *SSLHandler) UploadCustom(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Domain      string `json:"domain" binding:"required"`
		Certificate string `json:"certificate" binding:"required"`
		PrivateKey  string `json:"private_key" binding:"required"`
		ChainCert   string `json:"chain_cert"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	cert, err := h.sslService.UploadCustom(c.Request.Context(), tenantID, req.Domain, req.Certificate, req.PrivateKey, req.ChainCert)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, cert)
}

func (h *SSLHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid certificate ID")
		return
	}

	cert, err := h.sslService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Certificate not found")
		return
	}

	utils.Success(c, cert)
}

func (h *SSLHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	certs, err := h.sslService.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, certs)
}

func (h *SSLHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid certificate ID")
		return
	}

	if err := h.sslService.Delete(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Certificate deleted"})
}

func (h *SSLHandler) RenewAll(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	renewed, errs := h.sslService.RenewAll(c.Request.Context(), tenantID)

	utils.Success(c, gin.H{
		"renewed": renewed,
		"errors":  errs,
	})
}

func (h *SSLHandler) GetExpiringSoon(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	certs, err := h.sslService.GetExpiringSoon(c.Request.Context(), tenantID, 30)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, certs)
}
