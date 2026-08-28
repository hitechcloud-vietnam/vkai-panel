package handler

import (
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// PackageHandler serves hosting packages and quota over HTTP.
type PackageHandler struct {
	service *service.PackageService
	logger  *zap.Logger
}

// NewPackageHandler wraps an already-built service.
func NewPackageHandler(svc *service.PackageService, logger *zap.Logger) *PackageHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PackageHandler{service: svc, logger: logger}
}

// ============================================================================
// MOUNTING
//
// THE ROUTES ARE MOUNTED FROM cmd/api/main.go, IMMEDIATELY AFTER router.Setup().
// Look for "RegisterPackageRoutes" there. That is the single line that makes
// this feature reachable, and deleting it makes every route below disappear -
// which is what the route test in package_routes_test.go exists to catch.
//
// If these routes are ever moved into internal/handler/router.go instead, the
// line to add inside the `protected` group is:
//
//	RegisterPackageRoutes(protected)
//
// and the call in cmd/api/main.go MUST be deleted in the same commit. Leaving
// both in place is safe - the second registration is ignored and logged, not a
// panic - but only one of them should exist.
// ============================================================================

var packageRoutes struct {
	sync.Mutex
	handler *PackageHandler
	mounted bool
	logger  *zap.Logger
}

// UsePackageHandler installs the handler the routes will use. It is called once
// at startup, before RegisterPackageRoutes.
//
// The indirection exists because internal/handler/router.go cannot be extended
// with another constructor parameter from here. The variable is written once
// during start-up and only read afterwards.
func UsePackageHandler(h *PackageHandler, logger *zap.Logger) {
	packageRoutes.Lock()
	defer packageRoutes.Unlock()
	packageRoutes.handler = h
	packageRoutes.logger = logger
}

// RegisterPackageRoutes mounts the hosting package and quota endpoints on an
// already-authenticated router group.
//
// It does NOT add authentication of its own: the group it is given must already
// carry middleware.AuthRequired, exactly like the `protected` group in
// router.go. Administrative routes add middleware.RequireAdmin below.
//
// When no handler has been installed the paths are still mounted and answer 503
// with the reason. They are never left absent: a 404 tells the interface that
// this panel has no hosting packages, which is how an operator concludes their
// customers do not need quota, and that is precisely the failure this whole
// change exists to end.
//
// Registering twice in one process is a logged no-op rather than a panic, so
// moving the call from main.go into router.go cannot take the panel down
// halfway through the move.
func RegisterPackageRoutes(rg *gin.RouterGroup) {
	packageRoutes.Lock()
	h := packageRoutes.handler
	logger := packageRoutes.logger
	if packageRoutes.mounted {
		packageRoutes.Unlock()
		if logger != nil {
			logger.Info("package and quota routes are already mounted in this process; ignoring the second registration")
		}
		return
	}
	packageRoutes.mounted = true
	packageRoutes.Unlock()

	if h == nil || h.service == nil {
		registerPackagesUnavailable(rg)
		return
	}

	// Package administration crosses tenant boundaries: a package is a product,
	// not a customer's own setting.
	packages := rg.Group("/packages", middleware.RequireAdmin())
	{
		packages.GET("", h.ListPackages)
		packages.POST("", h.CreatePackage)
		packages.GET("/:id", h.GetPackage)
		packages.PUT("/:id", h.UpdatePackage)
		packages.DELETE("/:id", h.DeletePackage)
	}

	quotaGroup := rg.Group("/quota")
	{
		// The caller's own account. Behind no permission check: seeing how much
		// of what you bought you have used is not an administrative action, and
		// gating it would leave the lowest-privilege operators unable to answer
		// the most common support question there is.
		quotaGroup.GET("", h.MyStatus)
		quotaGroup.GET("/events", h.MyEvents)

		// Another account. Administrative, and every route reads the account id
		// from the path rather than from the token.
		accounts := quotaGroup.Group("/accounts", middleware.RequireAdmin())
		{
			accounts.GET("/:tenantId", h.AccountStatus)
			accounts.GET("/:tenantId/events", h.AccountEvents)
			accounts.POST("/:tenantId/package", h.AssignPackage)
			accounts.GET("/:tenantId/overrides", h.ListOverrides)
			accounts.PUT("/:tenantId/overrides/:resource", h.SetOverride)
			accounts.DELETE("/:tenantId/overrides/:resource", h.DeleteOverride)
			accounts.PUT("/:tenantId/features/:feature", h.SetFeatureOverride)
			accounts.DELETE("/:tenantId/features/:feature", h.DeleteFeatureOverride)
			accounts.POST("/:tenantId/suspend", h.Suspend)
			accounts.POST("/:tenantId/resume", h.Resume)
			accounts.POST("/:tenantId/recompute", h.Recompute)
		}
	}
}

// PackageRoutesMounted reports whether RegisterPackageRoutes has run in this
// process. The route test asserts it, so a build that forgot to mount them
// fails there rather than in production.
func PackageRoutesMounted() bool {
	packageRoutes.Lock()
	defer packageRoutes.Unlock()
	return packageRoutes.mounted
}

// resetPackageRoutesForTest lets the route test mount onto its own engine. It is
// unexported and exists only for the test in this package.
func resetPackageRoutesForTest() {
	packageRoutes.Lock()
	packageRoutes.mounted = false
	packageRoutes.Unlock()
}

// registerPackagesUnavailable keeps the paths answering when the service could
// not be built. See the comment on RegisterPackageRoutes for why this is a 503
// and not a 404.
func registerPackagesUnavailable(rg *gin.RouterGroup) {
	const msg = "Hosting packages and quota are not available on this panel: " +
		"the quota service could not be built. Check that migrations/pending/packages.sql has been applied."
	rg.Any("/packages", func(c *gin.Context) { utils.ServiceUnavailable(c, msg) })
	rg.Any("/packages/*path", func(c *gin.Context) { utils.ServiceUnavailable(c, msg) })
	rg.Any("/quota", func(c *gin.Context) { utils.ServiceUnavailable(c, msg) })
	rg.Any("/quota/*path", func(c *gin.Context) { utils.ServiceUnavailable(c, msg) })
}

// ============================================================================
// QUOTA ERRORS ON OTHER ENDPOINTS
// ============================================================================

// WriteQuotaError answers a quota refusal with a status and the message that
// names the limit, and reports whether it did.
//
// Every creation handler calls it before its generic error branch, because the
// generic branch is utils.InternalError, which replaces the message with "An
// internal error occurred". A refusal the customer cannot read is the same
// thing as no explanation at all: "you have used 5 of the 5 websites your
// package includes" is a product, "500" is an obstacle.
//
//	409 Conflict           - a limit is reached; the customer can act on it
//	403 Forbidden          - the account is suspended; only an operator can act
//	503 Service Unavailable - the enforcer could not find out, so it refused
func WriteQuotaError(c *gin.Context, err error) bool {
	switch {
	case quota.IsExceeded(err):
		c.JSON(http.StatusConflict, utils.APIResponse{
			Success:   false,
			Error:     &utils.APIError{Code: "QUOTA_EXCEEDED", Message: err.Error()},
			RequestID: utils.GetRequestID(c),
		})
		return true
	case quota.IsSuspended(err):
		c.JSON(http.StatusForbidden, utils.APIResponse{
			Success:   false,
			Error:     &utils.APIError{Code: "ACCOUNT_SUSPENDED", Message: err.Error()},
			RequestID: utils.GetRequestID(c),
		})
		return true
	case quota.IsUnavailable(err):
		utils.ServiceUnavailable(c, err.Error())
		return true
	}
	return false
}

// ============================================================================
// PACKAGE ADMINISTRATION
// ============================================================================

// ListPackages handles GET /packages.
func (h *PackageHandler) ListPackages(c *gin.Context) {
	packages, err := h.service.ListPackages(c.Request.Context())
	if err != nil {
		h.fail(c, "list hosting packages", err)
		return
	}
	utils.Success(c, packages)
}

// GetPackage handles GET /packages/:id.
func (h *PackageHandler) GetPackage(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "package ID")
	if !ok {
		return
	}
	p, err := h.service.GetPackage(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "read hosting package", err)
		return
	}
	utils.Success(c, p)
}

// CreatePackage handles POST /packages.
func (h *PackageHandler) CreatePackage(c *gin.Context) {
	var req service.PackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	p, err := h.service.CreatePackage(c.Request.Context(), &req)
	if err != nil {
		h.fail(c, "create hosting package", err)
		return
	}
	utils.Created(c, p)
}

// UpdatePackage handles PUT /packages/:id.
//
// This REPLACES the package: a limit left out of the body becomes unlimited.
// The alternative - treating an absent field as unchanged - makes it impossible
// to ever set a limit back to unlimited through the API.
func (h *PackageHandler) UpdatePackage(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "package ID")
	if !ok {
		return
	}
	var req service.PackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	p, err := h.service.UpdatePackage(c.Request.Context(), id, &req)
	if err != nil {
		h.fail(c, "update hosting package", err)
		return
	}
	utils.Success(c, p)
}

// DeletePackage handles DELETE /packages/:id.
func (h *PackageHandler) DeletePackage(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "package ID")
	if !ok {
		return
	}
	if err := h.service.DeletePackage(c.Request.Context(), id); err != nil {
		h.fail(c, "delete hosting package", err)
		return
	}
	utils.Message(c, "Hosting package deleted")
}

// ============================================================================
// QUOTA
// ============================================================================

// MyStatus handles GET /quota: the caller's own account.
func (h *PackageHandler) MyStatus(c *gin.Context) {
	h.writeStatus(c, middleware.GetTenantID(c))
}

// MyEvents handles GET /quota/events.
func (h *PackageHandler) MyEvents(c *gin.Context) {
	h.writeEvents(c, middleware.GetTenantID(c))
}

// AccountStatus handles GET /quota/accounts/:tenantId.
func (h *PackageHandler) AccountStatus(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	h.writeStatus(c, tenantID)
}

// AccountEvents handles GET /quota/accounts/:tenantId/events.
func (h *PackageHandler) AccountEvents(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	h.writeEvents(c, tenantID)
}

// AssignPackage handles POST /quota/accounts/:tenantId/package.
func (h *PackageHandler) AssignPackage(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	var body struct {
		PackageID uuid.UUID `json:"package_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	assignedBy := middleware.GetUserID(c)
	var by *uuid.UUID
	if assignedBy != uuid.Nil {
		by = &assignedBy
	}

	if err := h.service.AssignPackage(c.Request.Context(), tenantID, body.PackageID, by); err != nil {
		h.fail(c, "assign hosting package", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// ListOverrides handles GET /quota/accounts/:tenantId/overrides.
func (h *PackageHandler) ListOverrides(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	overrides, err := h.service.ListOverrides(c.Request.Context(), tenantID)
	if err != nil {
		h.fail(c, "list quota overrides", err)
		return
	}
	utils.Success(c, overrides)
}

// SetOverride handles PUT /quota/accounts/:tenantId/overrides/:resource.
func (h *PackageHandler) SetOverride(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	var req service.OverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	var by *uuid.UUID
	if userID != uuid.Nil {
		by = &userID
	}
	if err := h.service.SetOverride(c.Request.Context(), tenantID, c.Param("resource"), &req, by); err != nil {
		h.fail(c, "set quota override", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// DeleteOverride handles DELETE /quota/accounts/:tenantId/overrides/:resource.
func (h *PackageHandler) DeleteOverride(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	if err := h.service.DeleteOverride(c.Request.Context(), tenantID, c.Param("resource")); err != nil {
		h.fail(c, "delete quota override", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// SetFeatureOverride handles PUT /quota/accounts/:tenantId/features/:feature.
func (h *PackageHandler) SetFeatureOverride(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	var req service.FeatureOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	if err := h.service.SetFeatureOverride(c.Request.Context(), tenantID, c.Param("feature"), &req); err != nil {
		h.fail(c, "set feature override", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// DeleteFeatureOverride handles DELETE /quota/accounts/:tenantId/features/:feature.
func (h *PackageHandler) DeleteFeatureOverride(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	if err := h.service.DeleteFeatureOverride(c.Request.Context(), tenantID, c.Param("feature")); err != nil {
		h.fail(c, "delete feature override", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// Suspend handles POST /quota/accounts/:tenantId/suspend.
func (h *PackageHandler) Suspend(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.service.Suspend(c.Request.Context(), tenantID, body.Reason); err != nil {
		h.fail(c, "suspend account", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// Resume handles POST /quota/accounts/:tenantId/resume.
func (h *PackageHandler) Resume(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.service.Resume(c.Request.Context(), tenantID, body.Reason); err != nil {
		h.fail(c, "resume account", err)
		return
	}
	h.writeStatus(c, tenantID)
}

// Recompute handles POST /quota/accounts/:tenantId/recompute.
func (h *PackageHandler) Recompute(c *gin.Context) {
	tenantID, ok := parseUUIDParam(c, "tenantId", "account ID")
	if !ok {
		return
	}
	status, err := h.service.Recompute(c.Request.Context(), tenantID)
	if err != nil {
		h.fail(c, "recompute quota usage", err)
		return
	}
	utils.Success(c, status)
}

func (h *PackageHandler) writeStatus(c *gin.Context, tenantID uuid.UUID) {
	if tenantID == uuid.Nil {
		utils.BadRequest(c, "No account on this request")
		return
	}
	status, err := h.service.Status(c.Request.Context(), tenantID)
	if err != nil {
		h.fail(c, "read quota status", err)
		return
	}
	utils.Success(c, status)
}

func (h *PackageHandler) writeEvents(c *gin.Context, tenantID uuid.UUID) {
	if tenantID == uuid.Nil {
		utils.BadRequest(c, "No account on this request")
		return
	}
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	events, err := h.service.ListEvents(c.Request.Context(), tenantID, limit)
	if err != nil {
		h.fail(c, "list quota events", err)
		return
	}
	utils.Success(c, events)
}

// fail maps a service error onto a status. Validation errors reach the caller
// verbatim, because "grace_percent must be between 0 and 100" is only useful if
// it is read; anything unrecognised is logged and answered generically.
func (h *PackageHandler) fail(c *gin.Context, action string, err error) {
	if WriteQuotaError(c, err) {
		return
	}
	switch {
	case errors.Is(err, quota.ErrPackageNotFound):
		utils.NotFound(c, "Hosting package not found")
	case errors.Is(err, quota.ErrPackageInUse):
		utils.Conflict(c, quota.ErrPackageInUse.Error())
	case errors.Is(err, service.ErrQuotaUnavailable):
		utils.ServiceUnavailable(c, err.Error())
	default:
		h.logger.Error("hosting package request failed",
			zap.String("action", action), zap.Error(err))
		utils.BadRequest(c, err.Error())
	}
}

// parseUUIDParam reads a UUID path parameter and answers 400 when it is not one.
func parseUUIDParam(c *gin.Context, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		utils.BadRequest(c, "Invalid "+label)
		return uuid.Nil, false
	}
	return id, true
}
