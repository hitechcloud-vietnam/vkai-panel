package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type TenantHandler struct {
	tenantService *service.TenantService
	logger        *zap.Logger
}

func NewTenantHandler(tenantService *service.TenantService, logger *zap.Logger) *TenantHandler {
	return &TenantHandler{tenantService: tenantService, logger: logger}
}

func (h *TenantHandler) Create(c *gin.Context) {
	var req models.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	tenant, err := h.tenantService.Create(c.Request.Context(), req)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, tenant)
}

func (h *TenantHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid tenant ID")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Tenant not found")
		return
	}

	utils.Success(c, tenant)
}

func (h *TenantHandler) List(c *gin.Context) {
	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		params.Page = 1
		params.PerPage = 20
	}
	params.Normalize()

	tenants, total, err := h.tenantService.List(c.Request.Context(), params.Page, params.PerPage)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Paginated(c, tenants, total, params.Page, params.PerPage)
}

func (h *TenantHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid tenant ID")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Tenant not found")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Domain      string `json:"domain"`
		Status      string `json:"status"`
		Plan        string `json:"plan"`
		MaxServers  int    `json:"max_servers"`
		MaxWebsites int    `json:"max_websites"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Name != "" {
		tenant.Name = req.Name
	}
	if req.Domain != "" {
		tenant.Domain = req.Domain
	}
	if req.Status != "" {
		tenant.Status = req.Status
	}
	if req.Plan != "" {
		tenant.Plan = req.Plan
	}
	if req.MaxServers > 0 {
		tenant.MaxServers = req.MaxServers
	}
	if req.MaxWebsites > 0 {
		tenant.MaxWebsites = req.MaxWebsites
	}

	if err := h.tenantService.Update(c.Request.Context(), tenant); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, tenant)
}

func (h *TenantHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid tenant ID")
		return
	}

	if err := h.tenantService.Delete(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}
