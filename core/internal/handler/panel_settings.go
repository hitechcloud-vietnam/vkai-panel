package handler

// HTTP surface for the panel's own access settings.
//
// Everything here is administrator-only. The interesting part is the
// confirmation contract: a change that would move or restrict the panel's
// entrance is answered with 409 and the URL the caller must use afterwards,
// and is applied only when the request repeats itself with "confirm": true.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// PanelSettingsHandler serves /api/v1/panel/settings.
type PanelSettingsHandler struct {
	service *service.PanelSettingsService
	logger  *zap.Logger
}

// NewPanelSettingsHandler wires the handler to the service.
func NewPanelSettingsHandler(svc *service.PanelSettingsService, logger *zap.Logger) *PanelSettingsHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PanelSettingsHandler{service: svc, logger: logger}
}

// panelConfirmationPayload is the body of a 409. It travels in the "data" field
// so a client can act on it without parsing an error string.
type panelConfirmationPayload struct {
	ConfirmationRequired bool                              `json:"confirmation_required"`
	Reasons              []service.PanelConfirmationReason `json:"reasons"`
	NewURL               string                            `json:"new_url"`
	Changes              []service.PanelSettingChange      `json:"changes"`
}

// panelEntranceRegenerateRequest is the body of the regenerate endpoint.
type panelEntranceRegenerateRequest struct {
	Confirm bool `json:"confirm"`
}

// Get returns the current panel access settings plus the values derived from
// them: the access URL, the certificate fingerprint and expiry, and whether a
// restart is still owed.
//
// GET /api/v1/panel/settings
func (h *PanelSettingsHandler) Get(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	utils.Success(c, h.service.Get(h.caller(c)))
}

// Update applies a partial change to the settings.
//
// PUT /api/v1/panel/settings
func (h *PanelSettingsHandler) Update(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	var req service.PanelSettingsUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "The request body is not valid JSON.")
		return
	}

	result, err := h.service.Update(c.Request.Context(), h.caller(c), &req)
	if err != nil {
		h.respondError(c, err)
		return
	}

	utils.Success(c, result)
}

// RegenerateEntrance replaces the security entrance with a fresh random one.
//
// POST /api/v1/panel/settings/entrance/regenerate
func (h *PanelSettingsHandler) RegenerateEntrance(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	// The body is optional: an empty POST is a request for the confirmation
	// preview rather than a malformed call.
	var req panelEntranceRegenerateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, "The request body is not valid JSON.")
			return
		}
	}

	result, err := h.service.RegenerateEntrance(c.Request.Context(), h.caller(c), req.Confirm)
	if err != nil {
		h.respondError(c, err)
		return
	}

	utils.Success(c, result)
}

// ReissueCertificate forces a new self-signed certificate for the panel.
//
// POST /api/v1/panel/settings/tls/reissue
func (h *PanelSettingsHandler) ReissueCertificate(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	result, err := h.service.ReissueCertificate(c.Request.Context(), h.caller(c))
	if err != nil {
		h.respondError(c, err)
		return
	}

	utils.Success(c, result)
}

// caller captures who is asking and from where. Both are needed: the lockout
// checks are relative to this request's own address and host.
func (h *PanelSettingsHandler) caller(c *gin.Context) service.PanelSettingsCaller {
	return service.PanelSettingsCaller{
		ClientIP:  c.ClientIP(),
		Host:      c.Request.Host,
		UserAgent: c.Request.UserAgent(),
		UserID:    middleware.GetUserID(c),
		TenantID:  middleware.GetTenantID(c),
	}
}

// ready guards against a build in which the settings service could not be
// constructed, so the route answers honestly instead of panicking.
func (h *PanelSettingsHandler) ready(c *gin.Context) bool {
	if h == nil || h.service == nil {
		utils.ServiceUnavailable(c, "Panel access settings are not available on this instance.")
		return false
	}
	return true
}

// respondError maps the service's typed errors onto status codes. A rejected
// value is a 400 that names the field; a lockout is a 409 that carries the URL
// to use afterwards; anything else is an internal error and is logged rather
// than echoed back.
func (h *PanelSettingsHandler) respondError(c *gin.Context, err error) {
	var confirmation *service.PanelSettingsConfirmationError
	if errors.As(err, &confirmation) {
		c.JSON(http.StatusConflict, utils.APIResponse{
			Success: false,
			Data: panelConfirmationPayload{
				ConfirmationRequired: true,
				Reasons:              confirmation.Reasons,
				NewURL:               confirmation.NewURL,
				Changes:              confirmation.Changes,
			},
			Error: &utils.APIError{
				Code:    "CONFIRMATION_REQUIRED",
				Message: "This change moves the panel. Repeat the request with \"confirm\": true once you have saved the new URL.",
				Details: confirmation.Error(),
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	var validation *service.PanelSettingsValidationError
	if errors.As(err, &validation) {
		c.JSON(http.StatusBadRequest, utils.APIResponse{
			Success: false,
			Error: &utils.APIError{
				Code:    "VALIDATION_ERROR",
				Message: validation.Message,
				Details: validation.Field,
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	h.logger.Error("panel settings request failed", zap.Error(err))
	utils.InternalError(c, err)
}
