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

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
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

// panelTLSRiskPayload is the body of the 409 returned when a certificate would
// be served but should not be accepted without the operator saying so.
type panelTLSRiskPayload struct {
	AcknowledgementRequired bool                          `json:"acknowledgement_required"`
	Check                   string                        `json:"check"`
	Message                 string                        `json:"message"`
	Field                   string                        `json:"field"`
	Certificate             *config.CertificateInspection `json:"certificate"`
}

// panelRollbackPayload is the body of the 409 returned when a change was
// applied, could not be reached afterwards, and was undone by the panel itself.
type panelRollbackPayload struct {
	RolledBack bool                         `json:"rolled_back"`
	Reason     string                       `json:"reason"`
	Changes    []service.PanelSettingChange `json:"changes"`
	AccessURL  string                       `json:"access_url"`
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
// Everything an operator can change here now takes effect in this process
// before the response is written: the port is rebound, the access gate is
// rebuilt, the certificate manager is replaced. Anything that could not be
// applied is named in the response, with the reason, rather than reported as a
// success that quietly did not happen.
//
// Three answers other than 200 are part of the contract:
//
//	409 CONFIRMATION_REQUIRED                 the change moves the panel; repeat
//	                                          with "confirm": true
//	409 TLS_RISK_ACKNOWLEDGEMENT_REQUIRED     the certificate would be served
//	                                          but browsers will object; repeat
//	                                          with "tls_accept_risk": true
//	409 ROLLED_BACK                           the change was applied, the panel
//	                                          could not be reached afterwards
//	                                          and it has been undone
//	409 NOT_APPLIED                           the change could not be applied at
//	                                          all - the port is taken, the
//	                                          certificate does not load - and
//	                                          nothing about the panel changed
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

	// A change that was applied, failed its reachability check and was undone
	// is a 409 and not a 500: nothing is broken, the panel is exactly as it
	// was, and the operator has to be told that rather than left wondering
	// whether their change half-happened.
	var rolledBack *service.PanelSettingsRollbackError
	if errors.As(err, &rolledBack) {
		c.JSON(http.StatusConflict, utils.APIResponse{
			Success: false,
			Data: panelRollbackPayload{
				RolledBack: true,
				Reason:     rolledBack.Reason,
				Changes:    rolledBack.Changes,
				AccessURL:  rolledBack.AccessURL,
			},
			Error: &utils.APIError{
				Code:    "ROLLED_BACK",
				Message: "The change was applied and then undone, because the panel could not be reached afterwards. It is still running exactly as it was.",
				Details: rolledBack.Reason,
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	// A change that could not be applied is not an internal error: the request
	// was well formed, the panel is intact, and the operator needs the reason -
	// "something else is already listening on that port" - not a generic 500.
	var notApplied *service.PanelSettingsApplyError
	if errors.As(err, &notApplied) {
		c.JSON(http.StatusConflict, utils.APIResponse{
			Success: false,
			Data: panelRollbackPayload{
				RolledBack: false,
				Reason:     notApplied.Reason,
				Changes:    notApplied.Changes,
				AccessURL:  notApplied.AccessURL,
			},
			Error: &utils.APIError{
				Code:    "NOT_APPLIED",
				Message: "The change was not applied. The panel is still running, and still reachable, exactly as it was.",
				Details: notApplied.Reason,
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	var risk *service.PanelTLSRiskError
	if errors.As(err, &risk) {
		c.JSON(http.StatusConflict, utils.APIResponse{
			Success: false,
			Data: panelTLSRiskPayload{
				AcknowledgementRequired: true,
				Check:                   risk.Check,
				Message:                 risk.Message,
				Field:                   "tls_certificate",
				Certificate:             risk.Inspection,
			},
			Error: &utils.APIError{
				Code:    "TLS_RISK_ACKNOWLEDGEMENT_REQUIRED",
				Message: risk.Message,
				Details: "Repeat the request with \"tls_accept_risk\": true once you have understood what this certificate will and will not do.",
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	var validation *service.PanelSettingsValidationError
	if errors.As(err, &validation) {
		details := validation.Field
		if validation.Check != "" {
			details = validation.Field + " (" + validation.Check + ")"
		}
		c.JSON(http.StatusBadRequest, utils.APIResponse{
			Success: false,
			Error: &utils.APIError{
				Code:    "VALIDATION_ERROR",
				Message: validation.Message,
				Details: details,
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	h.logger.Error("panel settings request failed", zap.Error(err))
	utils.InternalError(c, err)
}
