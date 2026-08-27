package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type WebsiteHandler struct {
	websiteService *service.WebsiteService
}

func NewWebsiteHandler(websiteService *service.WebsiteService) *WebsiteHandler {
	return &WebsiteHandler{websiteService: websiteService}
}

func (h *WebsiteHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	website, err := h.websiteService.Create(c.Request.Context(), &req, tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, website)
}

func (h *WebsiteHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	website, err := h.websiteService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Website not found")
		return
	}

	utils.Success(c, website)
}

func (h *WebsiteHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	params := models.NewPaginationParams(c)

	websites, total, err := h.websiteService.ListByTenant(c.Request.Context(), tenantID, params)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Paginated(c, websites, total, params.Page, params.PerPage)
}

func (h *WebsiteHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	website, err := h.websiteService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Website not found")
		return
	}

	var req struct {
		Domain     string `json:"domain"`
		RootDir    string `json:"root_dir"`
		PHPVersion string `json:"php_version"`
		SiteType   string `json:"site_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Domain != "" {
		website.Domain = req.Domain
	}
	if req.RootDir != "" {
		website.RootDir = req.RootDir
	}
	if req.PHPVersion != "" {
		website.PHPVersion = req.PHPVersion
	}
	if req.SiteType != "" {
		website.SiteType = req.SiteType
	}

	if err := h.websiteService.Update(c.Request.Context(), website); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, website)
}

func (h *WebsiteHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	if err := h.websiteService.Delete(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Website deleted"})
}

func (h *WebsiteHandler) EnableSSL(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	var req struct {
		Certificate string `json:"certificate" binding:"required"`
		PrivateKey  string `json:"private_key" binding:"required"`
		ChainCert   string `json:"chain_cert"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.websiteService.EnableSSL(c.Request.Context(), id, req.Certificate, req.PrivateKey, req.ChainCert); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "SSL enabled"})
}

func (h *WebsiteHandler) AddDomain(c *gin.Context) {
	websiteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
		Type string `json:"type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	domain, err := h.websiteService.AddDomain(c.Request.Context(), websiteID, req.Name, req.Type)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, domain)
}

func (h *WebsiteHandler) ListDomains(c *gin.Context) {
	websiteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid website ID")
		return
	}

	domains, err := h.websiteService.ListDomains(c.Request.Context(), websiteID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, domains)
}

func (h *WebsiteHandler) DeleteDomain(c *gin.Context) {
	domainID, err := uuid.Parse(c.Param("domainId"))
	if err != nil {
		utils.BadRequest(c, "Invalid domain ID")
		return
	}

	if err := h.websiteService.DeleteDomain(c.Request.Context(), domainID); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Domain deleted"})
}
