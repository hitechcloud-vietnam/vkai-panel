package handler

// The operator's view of their own sessions.
//
// Three things an administrative panel has to be able to answer and, until
// now, could not:
//
//	"Where am I signed in?"          GET    /api/v1/sessions
//	"End that one."                  DELETE /api/v1/sessions/:id
//	"End every session on this       DELETE /api/v1/sessions/user/:id
//	 account, right now."            (administrator only)
//
// Plus the one that keeps the binding policy usable rather than infuriating:
//
//	"My address changed; here is     POST   /api/v1/sessions/current/reauthenticate
//	 my password, carry on."
//
// Ending the session the request is being made with is allowed, and the answer
// says so, because "sign me out here too" is precisely what an operator wants
// when they think something is wrong.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// SessionHandler serves the session endpoints.
type SessionHandler struct {
	service *service.SessionService
	logger  *zap.Logger
}

// NewSessionHandler builds the handler. A nil service is a real state - a
// panel whose session store could not be opened - and every route then answers
// 503 with the reason rather than 404.
func NewSessionHandler(sessions *service.SessionService, logger *zap.Logger) *SessionHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SessionHandler{service: sessions, logger: logger}
}

// Service exposes the session service so the route registration can install
// the same instance as the request-path guard.
func (h *SessionHandler) Service() *service.SessionService {
	if h == nil {
		return nil
	}
	return h.service
}

func (h *SessionHandler) available(c *gin.Context) bool {
	if h == nil || h.service == nil || !h.service.Enforcing() {
		utils.ServiceUnavailable(c,
			"Session management is unavailable on this panel: no session store is wired, "+
				"so sessions cannot be listed or ended before their token expires")
		return false
	}
	return true
}

func (h *SessionHandler) currentTokenID(c *gin.Context) string {
	return middleware.CurrentTokenID(middleware.GetClaims(c))
}

// List handles GET /sessions.
func (h *SessionHandler) List(c *gin.Context) {
	if !h.available(c) {
		return
	}

	views, err := h.service.List(c.Request.Context(),
		middleware.GetTenantID(c), middleware.GetUserID(c), h.currentTokenID(c))
	if err != nil {
		h.logger.Error("failed to list sessions", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, gin.H{
		"sessions": views,
		"policy": gin.H{
			"ip_binding":     h.service.Policy().IPMode,
			"device_binding": h.service.Policy().DeviceBinding,
		},
	})
}

// Terminate handles DELETE /sessions/:id.
func (h *SessionHandler) Terminate(c *gin.Context) {
	if !h.available(c) {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid session ID")
		return
	}

	self, err := h.service.Terminate(c.Request.Context(),
		middleware.GetTenantID(c), middleware.GetUserID(c), id,
		h.currentTokenID(c),
		middleware.AuthClientIP(c), c.Request.UserAgent())
	switch {
	case errors.Is(err, service.ErrSessionNotFound):
		utils.NotFound(c, "Session not found")
		return
	case err != nil:
		h.logger.Error("failed to terminate a session", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	message := "Session ended. It is refused from its next request onwards."
	if self {
		message = "This session has been ended. The token you are holding will be refused from the next request onwards."
	}

	utils.Success(c, gin.H{
		"id":              id,
		"current_session": self,
		"message":         message,
	})
}

// TerminateAllForUser handles DELETE /sessions/user/:id. Administrator only:
// it ends sessions that are not the caller's.
func (h *SessionHandler) TerminateAllForUser(c *gin.Context) {
	if !h.available(c) {
		return
	}

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	count, err := h.service.TerminateAllForUser(c.Request.Context(),
		middleware.GetTenantID(c), targetID, middleware.GetUserID(c),
		"terminated_by_administrator",
		middleware.AuthClientIP(c), c.Request.UserAgent())
	if err != nil {
		h.logger.Error("failed to terminate the sessions of an account", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, gin.H{
		"user_id":  targetID,
		"sessions": count,
		"message":  "Every session on that account is refused from its next request onwards.",
	})
}

// Reauthenticate handles POST /sessions/current/reauthenticate.
func (h *SessionHandler) Reauthenticate(c *gin.Context) {
	if !h.available(c) {
		return
	}

	var req models.ReauthenticateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "A password is required")
		return
	}

	err := h.service.Reauthenticate(c.Request.Context(),
		middleware.GetTenantID(c), middleware.GetUserID(c),
		h.currentTokenID(c), req.Password,
		middleware.AuthClientIP(c), c.Request.UserAgent())
	switch {
	case errors.Is(err, service.ErrPasswordRejected):
		// Counted by the credential guard in front of this route, which is
		// what stops this becoming a password oracle behind a stolen token.
		c.JSON(http.StatusUnauthorized, utils.APIResponse{
			Success: false,
			Error: &utils.APIError{
				Code:    "INVALID_CREDENTIALS",
				Message: "That password was not accepted",
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	case errors.Is(err, service.ErrSessionNotFound):
		utils.NotFound(c, "This request does not belong to a live session")
		return
	case err != nil:
		h.logger.Error("failed to re-bind a session", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, gin.H{
		"message": "This session is now bound to the address you are using.",
	})
}
