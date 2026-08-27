package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type ServerHandler struct {
	serverService *service.ServerService
	logger        *zap.Logger
}

func NewServerHandler(serverService *service.ServerService, logger *zap.Logger) *ServerHandler {
	return &ServerHandler{serverService: serverService, logger: logger}
}

func (h *ServerHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	server, err := h.serverService.Create(c.Request.Context(), tenantID, req)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, server)
}

func (h *ServerHandler) Get(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	server, err := h.serverService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		utils.NotFound(c, "Server not found")
		return
	}

	utils.Success(c, server)
}

func (h *ServerHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		params.Page = 1
		params.PerPage = 20
	}
	params.Normalize()

	servers, total, err := h.serverService.ListByTenant(c.Request.Context(), tenantID, params.Page, params.PerPage)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Paginated(c, servers, total, params.Page, params.PerPage)
}

func (h *ServerHandler) Update(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	server, err := h.serverService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		utils.NotFound(c, "Server not found")
		return
	}

	var req struct {
		Hostname  string   `json:"hostname"`
		IPAddress string   `json:"ip_address"`
		SSHPort   int      `json:"ssh_port"`
		Location  string   `json:"location"`
		Tags      []string `json:"tags"`
		Role      string   `json:"role"`
		Status    string   `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Hostname != "" {
		server.Hostname = req.Hostname
	}
	if req.IPAddress != "" {
		server.IPAddress = req.IPAddress
	}
	if req.SSHPort > 0 {
		server.SSHPort = req.SSHPort
	}
	if req.Location != "" {
		server.Location = req.Location
	}
	if req.Tags != nil {
		server.Tags = req.Tags
	}
	if req.Role != "" {
		server.Role = req.Role
	}
	if req.Status != "" {
		server.Status = req.Status
	}

	if err := h.serverService.Update(c.Request.Context(), server); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	if err := h.serverService.Delete(c.Request.Context(), tenantID, id); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

func (h *ServerHandler) GetMetrics(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	metrics, err := h.serverService.GetMetrics(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Metrics not found")
		return
	}

	utils.Success(c, metrics)
}
