package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/twofactor"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// TwoFactorHandler exposes the second factor life cycle over HTTP.
//
// Every route here acts on the authenticated caller and on nobody else: the
// user id comes from the validated token, never from the request body. An
// endpoint that took a user id from the body would let any authenticated user
// disable an administrator's second factor.
type TwoFactorHandler struct {
	service *twofactor.Service
	logger  *zap.Logger
}

// NewTwoFactorHandler wraps an already-built service.
func NewTwoFactorHandler(service *twofactor.Service, logger *zap.Logger) *TwoFactorHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TwoFactorHandler{service: service, logger: logger}
}

// NewTwoFactorHandlerFromDB builds the whole stack - Postgres store, secret
// box, in-process rate limiter - from the panel database and the master key in
// VKAI_SECRET_KEY. It exists so wiring in cmd/api/main.go is one line.
//
// Pass the panel audit service as `audit` (it satisfies twofactor.AuditLogger
// as it stands) and the shared rate limiter as `limiter` once that lands; a nil
// limiter falls back to an in-process one, never to no limit.
func NewTwoFactorHandlerFromDB(
	db *sqlx.DB,
	audit twofactor.AuditLogger,
	limiter twofactor.Limiter,
	issuer string,
	logger *zap.Logger,
) (*TwoFactorHandler, error) {
	master, err := twofactor.MasterKeyFromEnv()
	if err != nil {
		return nil, err
	}
	box, err := twofactor.NewSecretBox(master)
	if err != nil {
		return nil, err
	}

	service := twofactor.NewService(
		twofactor.NewPostgresStore(db),
		box,
		audit,
		limiter,
		logger,
		twofactor.Options{Issuer: issuer},
	)
	return NewTwoFactorHandler(service, logger), nil
}

// RegisterTwoFactorRoutes mounts the two-factor routes on an authenticated
// group. It is the single entry point so router.go needs one line.
//
// These routes are deliberately NOT behind a permission check: managing your
// own second factor is not an administrative action, and gating it behind a
// permission would leave low-privilege accounts unable to protect themselves.
func RegisterTwoFactorRoutes(rg *gin.RouterGroup, h *TwoFactorHandler) {
	if h == nil {
		return
	}

	group := rg.Group("/two-factor")
	{
		group.GET("/status", h.Status)
		group.POST("/enroll", h.StartEnrolment)
		group.POST("/enroll/verify", h.ConfirmEnrolment)
		group.POST("/verify", h.Verify)
		group.POST("/recovery-codes", h.RegenerateRecoveryCodes)
		group.POST("/disable", h.Disable)
	}
}

// Status handles GET /two-factor/status.
func (h *TwoFactorHandler) Status(c *gin.Context) {
	status, err := h.service.Status(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		h.fail(c, "read two-factor status", err)
		return
	}
	utils.Success(c, status)
}

// StartEnrolment handles POST /two-factor/enroll. It returns the secret and the
// otpauth URI; it does not enable anything.
func (h *TwoFactorHandler) StartEnrolment(c *gin.Context) {
	var body struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "Current password is required to start enrolment")
		return
	}

	start, err := h.service.StartEnrolment(c.Request.Context(), h.request(c, body.Password, ""))
	if err != nil {
		h.fail(c, "start two-factor enrolment", err)
		return
	}
	utils.Success(c, start)
}

// ConfirmEnrolment handles POST /two-factor/enroll/verify. This is the only
// call that turns two-factor on, and only on a proven code.
func (h *TwoFactorHandler) ConfirmEnrolment(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "A code from your authenticator app is required")
		return
	}

	codes, err := h.service.ConfirmEnrolment(c.Request.Context(), h.request(c, "", body.Code))
	if err != nil {
		h.fail(c, "confirm two-factor enrolment", err)
		return
	}

	// The recovery codes cross the wire exactly once, here. The panel keeps
	// only their hashes and cannot show them again.
	utils.Success(c, codes)
}

// Verify handles POST /two-factor/verify: one TOTP code or one recovery code
// for the authenticated caller.
func (h *TwoFactorHandler) Verify(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "A verification code is required")
		return
	}

	result, err := h.service.Verify(c.Request.Context(), h.request(c, "", body.Code))
	if err != nil {
		h.fail(c, "verify two-factor code", err)
		return
	}
	utils.Success(c, result)
}

// RegenerateRecoveryCodes handles POST /two-factor/recovery-codes. A fresh set
// is a fresh set of bypasses, so it costs the password and a current code.
func (h *TwoFactorHandler) RegenerateRecoveryCodes(c *gin.Context) {
	var body struct {
		Password string `json:"password" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "Current password and a current code are required")
		return
	}

	codes, err := h.service.RegenerateRecoveryCodes(c.Request.Context(), h.request(c, body.Password, body.Code))
	if err != nil {
		h.fail(c, "regenerate recovery codes", err)
		return
	}
	utils.Success(c, codes)
}

// Disable handles POST /two-factor/disable. Password and a current code, and
// the result is written to the audit log by the service.
func (h *TwoFactorHandler) Disable(c *gin.Context) {
	var body struct {
		Password string `json:"password" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "Current password and a current code are required")
		return
	}

	if err := h.service.Disable(c.Request.Context(), h.request(c, body.Password, body.Code)); err != nil {
		h.fail(c, "disable two-factor", err)
		return
	}
	utils.Message(c, "Two-factor authentication has been turned off")
}

// request builds the service request from the validated token and the
// transport, so the caller can never act on another account.
func (h *TwoFactorHandler) request(c *gin.Context, password, code string) twofactor.Request {
	return twofactor.Request{
		UserID:    middleware.GetUserID(c),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Password:  password,
		Code:      code,
	}
}

// fail maps a service error to a status code. Wrong password, wrong code and
// spent recovery code all arrive as ErrVerificationFailed and all answer 401
// with the same message: telling the caller which half was wrong halves the
// work of guessing.
func (h *TwoFactorHandler) fail(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, twofactor.ErrVerificationFailed):
		utils.Unauthorized(c, "Verification failed")
	case errors.Is(err, twofactor.ErrCodeReplayed):
		utils.Unauthorized(c, "That code has already been used. Wait for your app to show the next one.")
	case errors.Is(err, twofactor.ErrLockedOut):
		utils.Error(c, http.StatusTooManyRequests, "Too many failed attempts. Try again later.")
	case errors.Is(err, twofactor.ErrRateLimited):
		utils.RateLimit(c)
	case errors.Is(err, twofactor.ErrAlreadyEnabled):
		utils.Conflict(c, "Two-factor authentication is already enabled. Turn it off first to enrol a new device.")
	case errors.Is(err, twofactor.ErrNoPendingEnrolment):
		utils.Conflict(c, "There is no pending enrolment. Start again to get a new secret.")
	case errors.Is(err, twofactor.ErrNotEnrolled):
		utils.Conflict(c, "Two-factor authentication is not enabled for this account")
	case errors.Is(err, twofactor.ErrNoAccount):
		utils.NotFound(c, "Account not found")
	case errors.Is(err, twofactor.ErrSecretUnreadable):
		// The stored secret cannot be decrypted: the encryption key changed, or
		// the row was edited. Say so plainly - the user must re-enrol.
		h.logger.Error("stored two-factor secret could not be decrypted",
			zap.String("operation", operation))
		utils.Error(c, http.StatusConflict,
			"The stored two-factor secret cannot be read on this server. It must be reset by an administrator.")
	default:
		h.logger.Error("two-factor operation failed",
			zap.String("operation", operation), zap.Error(err))
		utils.InternalError(c, "Two-factor operation failed")
	}
}
