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

// APIKeyHandler handles API key HTTP requests
type APIKeyHandler struct {
	service *service.APIKeyService
	logger  *zap.Logger
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(service *service.APIKeyService, logger *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{
		service: service,
		logger:  logger,
	}
}

// Create handles POST /api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req models.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	result, err := h.service.CreateAPIKey(c.Request.Context(), tenantID, userID, &req)
	if err != nil {
		h.logger.Error("Failed to create API key", zap.Error(err))
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, result)
}

// Get handles GET /api-keys/:id
func (h *APIKeyHandler) Get(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	apiKey, err := h.service.Get(c.Request.Context(), tenantID, id)
	if err != nil {
		utils.NotFound(c, "API key not found")
		return
	}

	utils.Success(c, apiKey)
}

// List handles GET /api-keys
func (h *APIKeyHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	params := models.NewPaginationParams(c)

	keys, total, err := h.service.List(c.Request.Context(), tenantID, params.Page, params.PerPage)
	if err != nil {
		h.logger.Error("Failed to list API keys", zap.Error(err))
		utils.InternalError(c, err.Error())
		return
	}

	utils.Paginated(c, keys, int64(total), params.Page, params.PerPage)
}

// Update handles PUT /api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	var req models.UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	apiKey, err := h.service.Update(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		utils.NotFound(c, "API key not found")
		return
	}

	utils.Success(c, apiKey)
}

// Delete handles DELETE /api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		h.logger.Error("Failed to delete API key", zap.Error(err))
		utils.InternalError(c, err.Error())
		return
	}

	utils.NoContent(c)
}
