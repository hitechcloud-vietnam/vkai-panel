package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type DatabaseHandler struct {
	dbService *service.DatabaseService
}

func NewDatabaseHandler(dbService *service.DatabaseService) *DatabaseHandler {
	return &DatabaseHandler{dbService: dbService}
}

func (h *DatabaseHandler) CreateServer(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateDBServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	server, err := h.dbService.CreateServer(c.Request.Context(), &req, tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, server)
}

func (h *DatabaseHandler) GetServer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	server, err := h.dbService.GetServerByID(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Database server not found")
		return
	}

	utils.Success(c, server)
}

func (h *DatabaseHandler) ListServers(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	servers, err := h.dbService.ListServersByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, servers)
}

func (h *DatabaseHandler) DeleteServer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return
	}

	if err := h.dbService.DeleteServer(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Database server deleted"})
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateDBEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	entry, err := h.dbService.CreateDatabase(c.Request.Context(), &req, tenantID)
	if err != nil {
		if WriteQuotaError(c, err) {
			return
		}
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, entry)
}

func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	entries, err := h.dbService.ListEntriesByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, entries)
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid database ID")
		return
	}

	if err := h.dbService.DeleteEntry(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Database deleted"})
}

func (h *DatabaseHandler) ChangePassword(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid database ID")
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.dbService.ChangePassword(c.Request.Context(), middleware.GetTenantID(c), id, req.Password); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Password changed"})
}
