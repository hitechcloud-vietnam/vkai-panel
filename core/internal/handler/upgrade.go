package handler

// HTTP surface for the panel's own upgrade.
//
// Five routes, and the shape of each is decided by one fact: the panel restarts
// itself in the middle of this work. So POST /upgrade/start returns as soon as
// the job is accepted and never waits for it, GET /upgrade/progress/:id answers
// from state that survives the restart, and GET /version is the endpoint a
// client falls back to when the other four stop answering - it is the only one
// that can prove the upgrade landed.
//
// Everything except /version is administrator-only, and every action - a forced
// check, a start, a refused second start - is written to the audit log by the
// service. The two polled reads are not: an audit log with one entry every two
// seconds for the length of an upgrade is an audit log nobody can read.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// UpgradeHandler serves /api/v1/version and /api/v1/upgrade/*.
type UpgradeHandler struct {
	service *service.UpgradeService
	logger  *zap.Logger
}

// NewUpgradeHandler wires the handler to the service.
func NewUpgradeHandler(svc *service.UpgradeService, logger *zap.Logger) *UpgradeHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UpgradeHandler{service: svc, logger: logger}
}

// upgradeStartRequest is the body of the start endpoint. The version is
// optional: an empty body means "install whatever the last check found".
type upgradeStartRequest struct {
	Version string `json:"version"`
}

// Version reports what this binary is, and nothing else.
//
// GET /api/v1/version
//
// This route is deliberately unauthenticated. A client watching an upgrade has
// no other way to tell "the API is still restarting" from "the API came back on
// the new version", and it has to be able to ask that question while its own
// session may be being re-established. The cost is that the build is
// fingerprintable by anyone who can already reach the panel's entrance, which
// is why the answer is three fields and stops there: no host details, no
// configuration, no counts, no job state.
func (h *UpgradeHandler) Version(c *gin.Context) {
	build := service.ResolveBuildInfo()
	if h != nil && h.service != nil {
		build = h.service.Build()
	}
	utils.Success(c, build)
}

// Status returns the last known release status without contacting the release
// source, together with the current job if there is one.
//
// GET /api/v1/upgrade/status
func (h *UpgradeHandler) Status(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	utils.Success(c, h.service.Status())
}

// Check contacts the release source and replaces the cached status.
//
// POST /api/v1/upgrade/check
func (h *UpgradeHandler) Check(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	status, err := h.service.Check(c.Request.Context(), h.caller(c))
	if err != nil {
		h.respondError(c, err)
		return
	}

	utils.Success(c, status)
}

// Start begins an upgrade and returns a job id immediately.
//
// POST /api/v1/upgrade/start
func (h *UpgradeHandler) Start(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	// An empty body is a valid request, not a malformed one: it means the
	// latest version.
	var req upgradeStartRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, "The request body is not valid JSON.")
			return
		}
	}

	result, err := h.service.Start(c.Request.Context(), h.caller(c), req.Version)
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, utils.APIResponse{
		Success:   true,
		Data:      result,
		RequestID: utils.GetRequestID(c),
	})
}

// Progress reports where one job has got to.
//
// GET /api/v1/upgrade/progress/:id
func (h *UpgradeHandler) Progress(c *gin.Context) {
	if !h.ready(c) {
		return
	}

	job, err := h.service.Progress(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondError(c, err)
		return
	}

	utils.Success(c, job)
}

// caller captures who is asking and from where, for the audit trail.
func (h *UpgradeHandler) caller(c *gin.Context) service.UpgradeCaller {
	return service.UpgradeCaller{
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		UserID:    middleware.GetUserID(c),
		TenantID:  middleware.GetTenantID(c),
	}
}

// ready guards against a build in which the upgrade service could not be
// constructed, so the route answers honestly instead of panicking.
func (h *UpgradeHandler) ready(c *gin.Context) bool {
	if h == nil || h.service == nil {
		utils.ServiceUnavailable(c, "Upgrades are not available on this instance.")
		return false
	}
	return true
}

// respondError maps the service's errors onto status codes. Each one is a
// distinct thing a client has to act on differently, so none of them collapse
// into a generic 500.
func (h *UpgradeHandler) respondError(c *gin.Context, err error) {
	var validation *service.UpgradeValidationError
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

	switch {
	case errors.Is(err, service.ErrUpgradeUnavailable):
		utils.ServiceUnavailable(c, "This panel has no upgrade source configured, so it cannot upgrade itself.")
	case errors.Is(err, service.ErrUpgradeInProgress):
		utils.Conflict(c, "An upgrade is already running. Wait for it to finish before starting another.")
	case errors.Is(err, service.ErrUpgradeNoTarget):
		utils.BadRequest(c, "There is no newer version to install. Run a check first, or name a version explicitly.")
	case errors.Is(err, service.ErrUpgradeJobNotFound):
		utils.NotFound(c, "No upgrade job with that id. It may belong to an older install of the panel.")
	default:
		h.logger.Error("upgrade request failed", zap.Error(err))
		utils.InternalError(c, err)
	}
}
