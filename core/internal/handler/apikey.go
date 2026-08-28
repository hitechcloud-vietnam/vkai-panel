package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
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
	if logger == nil {
		logger = zap.NewNop()
	}
	return &APIKeyHandler{
		service: service,
		logger:  logger,
	}
}

// Service exposes the API key service, so the route registration can build the
// validator that authenticates a presented key without a second instance of
// the service existing.
func (h *APIKeyHandler) Service() *service.APIKeyService {
	if h == nil {
		return nil
	}
	return h.service
}

// fail maps a service error onto a status code. The unavailable case gets its
// own answer: a 503 naming the cause is a configuration problem an operator can
// fix, where a 500 is a mystery and a 404 reads as "this panel has no API
// keys".
func (h *APIKeyHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAPIKeyUnavailable):
		utils.ServiceUnavailable(c, err.Error())
	case errors.Is(err, service.ErrAPIKeyNotFound):
		utils.NotFound(c, "API key not found")
	case errors.Is(err, service.ErrAPIKeyStorage):
		// The store said something. It said it to the log, not to the caller.
		utils.InternalError(c, err)
	default:
		utils.BadRequest(c, err.Error())
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
		h.logger.Warn("Failed to create API key", zap.Error(err))
		h.fail(c, err)
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
		h.fail(c, err)
		return
	}

	utils.Paginated(c, keys, int64(total), params.Page, params.PerPage)
}

// Update handles PUT /api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	actorID := middleware.GetUserID(c)

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

	apiKey, err := h.service.Update(c.Request.Context(), tenantID, actorID, id, &req)
	if err != nil {
		h.fail(c, err)
		return
	}

	utils.Success(c, apiKey)
}

// Delete handles DELETE /api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	actorID := middleware.GetUserID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, actorID, id); err != nil {
		h.fail(c, err)
		return
	}

	utils.NoContent(c)
}

// Rotate handles POST /api-keys/:id/rotate.
//
// It answers with the replacement key - shown once, like any other new key -
// and the instant the key being replaced stops working, so the operator knows
// exactly how long they have to redeploy.
func (h *APIKeyHandler) Rotate(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	actorID := middleware.GetUserID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	var req models.RotateAPIKeyRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
	}

	result, err := h.service.Rotate(c.Request.Context(), tenantID, actorID, id, &req)
	if err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, utils.APIResponse{
		Success:   true,
		Data:      result,
		RequestID: utils.GetRequestID(c),
	})
}

// Revoke handles POST /api-keys/:id/revoke. It takes effect on the next
// request that presents the key, not at its next expiry.
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	actorID := middleware.GetUserID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid API key ID")
		return
	}

	var req models.RevokeAPIKeyRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
	}

	if err := h.service.Revoke(c.Request.Context(), tenantID, actorID, id, req.Reason); err != nil {
		h.fail(c, err)
		return
	}

	utils.Success(c, gin.H{
		"id":      id,
		"status":  "revoked",
		"message": "The key is refused from the next request onwards.",
	})
}

// Scopes handles GET /access/scopes: the catalogue an interface needs to offer
// a scope picker, plus the grammar, so an operator is not guessing.
func (h *APIKeyHandler) Scopes(c *gin.Context) {
	utils.Success(c, gin.H{
		"modules": auth.Modules(),
		"actions": []string{auth.ActionRead, auth.ActionWrite},
		"grammar": "[<tenant>/]<module>:<action> - the tenant is optional and " +
			"defaults to the key's own tenant; \"*\" is accepted in any of the three positions",
		"examples": []string{
			"website:read",
			"*:read",
			"database:write",
			"*/monitoring:read",
		},
		"max_scopes_per_key": auth.MaxScopesPerKey,
	})
}

// WhoAmI handles GET /integration/whoami: what the presented key is and what
// it may do. An integration can call it to find out why it is being refused
// without an operator having to read the panel's logs.
func (h *APIKeyHandler) WhoAmI(c *gin.Context) {
	scopes, present := middleware.APIKeyScopes(c)
	if !present {
		utils.Unauthorized(c, "This endpoint requires an API key")
		return
	}
	utils.Success(c, gin.H{
		"api_key_id": c.GetString(middleware.APIKeyIDKey),
		"tenant_id":  middleware.GetTenantID(c),
		"user_id":    middleware.GetUserID(c),
		"scopes":     scopes,
	})
}
