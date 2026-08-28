package handler

// The HTTP surface for multi-version PHP and the WordPress toolkit, and the
// ONE registration function that mounts all of it.
//
// ============================================================================
// ROUTER.GO NEEDS EXACTLY ONE LINE. NOTHING ELSE. NOTHING IN cmd/api/main.go.
//
// Inside the `protected` block of internal/handler/router.go, anywhere:
//
//     RegisterPHPWordPressRuntimeRoutes(protected, r.phpHandler, r.wordpressHandler)
//
// Without that line every route below returns 404 and this feature does not
// exist, however green its tests are. This project has already shipped
// two-factor authentication, mutual TLS, a rate limiter and an ACME client that
// were all written, tested, merged - and mounted nowhere. The tests in
// php_wordpress_runtime_test.go assert that this function registers what it
// claims to, and that the routes it adds do not collide with the PHP and
// WordPress blocks router.go already has; they cannot assert that the line is
// present, because they cannot add it.
// ============================================================================
//
// The handlers below are methods on the EXISTING PHPHandler and
// WordPressHandler, which router.go already holds as r.phpHandler and
// r.wordpressHandler. That is deliberate: a new handler type would need a new
// NewRouter parameter, and a new NewRouter parameter would need cmd/api/main.go
// changed, and every additional file that has to be edited is another place the
// wiring can be left out.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/phpfpm"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/wpcli"
)

// RegisterPHPWordPressRuntimeRoutes mounts every route added by task F2.
//
// It is purely additive: it registers nothing router.go already registers, so
// the one line above cannot make gin panic on a duplicate path at start-up.
// That property is asserted in TestRuntimeRoutesDoNotCollideWithTheRouter,
// which builds the real engine through NewRouter and then calls this function
// on it - because "it should be fine" is what was believed about the four
// features that turned out to be unreachable.
//
// The route table it produces does NOT depend on whether the handlers are wired.
// Registration takes a method value, never a call, so a nil handler yields
// exactly the same paths as a live one, and each handler answers 503 with the
// reason when its service is missing. That is deliberate on both counts:
//
//   - it is the property router_test.go relies on to assert the real route
//     table with nil handlers, and a registration function that mounted fewer
//     routes when a handler was nil would make that whole file blind;
//   - a missing route is indistinguishable from a feature that was never
//     written, which is how an operator concludes the panel cannot do this at
//     all. A 503 with a reason is a configuration problem somebody can fix.
func RegisterPHPWordPressRuntimeRoutes(rg *gin.RouterGroup, php *PHPHandler, wordpress *WordPressHandler) {
	RegisterPHPRuntimeRoutes(rg, php)
	RegisterWordPressRuntimeRoutes(rg, wordpress)
}

// RegisterPHPRuntimeRoutes mounts the multi-version PHP routes under /php.
//
// The paths deliberately avoid /php/versions/<something-static>, because
// router.go already has /php/versions/:id and a static sibling of a parameter
// is the kind of thing that works until the day gin changes how it resolves
// them. /php/install and /php/sites/... share no segment with anything that is
// already there.
func RegisterPHPRuntimeRoutes(rg *gin.RouterGroup, h *PHPHandler) {
	group := rg.Group("/php", middleware.RequirePermission("php"))

	// What this host can really do: the distribution, whether several PHP
	// versions can live side by side here, and the whole nine-family matrix.
	group.GET("/system", h.PHPSystemReport)

	// Installing and removing a version, for real.
	group.POST("/install", h.InstallPHPVersionOnHost)
	group.POST("/uninstall", h.UninstallPHPVersionFromHost)

	// Per-site pool settings that reach the pool file.
	group.GET("/pools/:id/settings", h.GetPoolSettings)
	group.PUT("/pools/:id/settings", h.ApplyPoolSettings)

	// A site's PHP version.
	group.GET("/sites/:website_id/version", h.GetSitePHPVersion)
	group.PUT("/sites/:website_id/version", h.SetSitePHPVersion)
}

// RegisterWordPressRuntimeRoutes mounts the WordPress toolkit under /wordpress.
func RegisterWordPressRuntimeRoutes(rg *gin.RouterGroup, h *WordPressHandler) {
	group := rg.Group("/wordpress", middleware.RequirePermission("wordpress"))

	// A real installation: download, configure, salt, install, own.
	group.POST("/:id/install", h.InstallWordPress)

	// Who this site's commands run as. Never root.
	group.GET("/:id/runtime", h.GetSiteRuntime)
	group.PUT("/:id/runtime", h.SetSiteRuntime)

	// The live view, from WP-CLI, rather than the panel's record of what
	// somebody once typed into a form.
	group.GET("/:id/plugins/live", h.ListLivePlugins)
	group.POST("/:id/plugins/update", h.UpdateLivePlugins)
	group.GET("/:id/themes/live", h.ListLiveThemes)
	group.POST("/:id/themes/update", h.UpdateLiveThemes)

	group.GET("/:id/core/version", h.GetCoreVersion)
	group.POST("/:id/core/update", h.UpdateCore)

	// Migration and account recovery.
	group.POST("/:id/search-replace", h.SearchReplace)
	group.POST("/:id/users/password", h.ResetUserPassword)

	// Staging, and the push that requires an explicit database decision.
	group.GET("/:id/staging", h.GetStaging)
	group.POST("/:id/staging", h.CreateStaging)
	group.POST("/:id/staging/push", h.PushStaging)
	group.DELETE("/:id/staging", h.DeleteStaging)
}

// wired reports whether the PHP handler can serve. A method value on a nil
// receiver is legal in Go and is what keeps the route table constant; this is
// the check that stops it becoming a nil dereference at request time.
func (h *PHPHandler) wired(c *gin.Context) bool {
	if h == nil || h.phpService == nil {
		utils.ServiceUnavailable(c, "PHP host management is not available on this panel: the "+
			"PHP service was not wired when the router was built")
		return false
	}
	return true
}

// wired is the same check for the WordPress handler.
func (h *WordPressHandler) wired(c *gin.Context) bool {
	if h == nil || h.service == nil {
		utils.ServiceUnavailable(c, "The WordPress toolkit is not available on this panel: the "+
			"WordPress service was not wired when the router was built")
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Error mapping
//
// The status code carries the meaning, so an operator can tell "this host
// cannot do that" from "you asked for something impossible" from "something
// broke". Everything that lands on 500 is a genuine surprise.
// ---------------------------------------------------------------------------

func runtimeError(c *gin.Context, logger *zap.Logger, what string, err error) {
	var unsupported *phpfpm.ErrMultiVersionUnsupported
	var wouldBeRoot *wpcli.ErrWouldRunAsRoot
	var badArg *wpcli.ErrMetacharacter

	switch {
	case errors.As(err, &unsupported):
		// A clean refusal, not a failure: this distribution is supported by
		// the panel, just not for side-by-side PHP.
		utils.Error(c, http.StatusNotImplemented, err.Error())

	case errors.As(err, &wouldBeRoot):
		utils.Error(c, http.StatusForbidden, err.Error())

	case errors.As(err, &badArg):
		utils.BadRequest(c, err.Error())

	case errors.Is(err, wpcli.ErrDatabaseChoiceRequired):
		utils.BadRequest(c, err.Error())

	case errors.Is(err, service.ErrPHPRuntimeUnavailable),
		errors.Is(err, service.ErrWordPressRuntimeUnavailable),
		errors.Is(err, wpcli.ErrNotInstalled):
		utils.ServiceUnavailable(c, err.Error())

	case errors.Is(err, repository.ErrNoRuntime),
		errors.Is(err, repository.ErrNoStaging),
		errors.Is(err, repository.ErrNoSettings):
		utils.NotFound(c, err.Error())

	default:
		if logger != nil {
			logger.Error(what, zap.Error(err))
		}
		utils.InternalError(c, err)
	}
}

func tenantOf(c *gin.Context) (string, bool) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		// The WordPress routes carry the tenant as a uuid.UUID rather than a
		// string; both shapes are accepted so this file works behind either
		// middleware.
		if value, exists := c.Get("tenant_id"); exists {
			if id, ok := value.(uuid.UUID); ok {
				return id.String(), true
			}
		}
		utils.Unauthorized(c, "Tenant ID not found")
		return "", false
	}
	return tenantID, true
}

func tenantUUIDOf(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get("tenant_id")
	if exists {
		if id, ok := value.(uuid.UUID); ok {
			return id, true
		}
		if s, ok := value.(string); ok && s != "" {
			if id, err := uuid.Parse(s); err == nil {
				return id, true
			}
		}
	}
	utils.Unauthorized(c, "Tenant ID not found")
	return uuid.Nil, false
}

// ---------------------------------------------------------------------------
// PHP handlers
// ---------------------------------------------------------------------------

// PHPSystemReport handles GET /php/system.
func (h *PHPHandler) PHPSystemReport(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	report, err := h.phpService.SystemReport(c.Request.Context())
	if err != nil {
		runtimeError(c, h.logger, "PHP system report failed", err)
		return
	}
	utils.Success(c, report)
}

// InstallPHPVersionOnHost handles POST /php/install.
func (h *PHPHandler) InstallPHPVersionOnHost(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	var req service.InstallPHPVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	php, err := h.phpService.InstallVersion(c.Request.Context(), &req, tenantID)
	if err != nil {
		runtimeError(c, h.logger, "PHP installation failed", err)
		return
	}
	utils.Created(c, php)
}

// UninstallPHPVersionFromHost handles POST /php/uninstall.
func (h *PHPHandler) UninstallPHPVersionFromHost(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	var req struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.phpService.UninstallVersion(c.Request.Context(), req.ID, tenantID); err != nil {
		runtimeError(c, h.logger, "PHP removal failed", err)
		return
	}
	utils.Message(c, "PHP version removed from this host")
}

// GetPoolSettings handles GET /php/pools/:id/settings.
func (h *PHPHandler) GetPoolSettings(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	settings, err := h.phpService.GetPoolSettings(c.Request.Context(), c.Param("id"), tenantID)
	if err != nil {
		runtimeError(c, h.logger, "reading PHP pool settings failed", err)
		return
	}
	utils.Success(c, gin.H{"settings": settings, "applied": settings.Applied()})
}

// ApplyPoolSettings handles PUT /php/pools/:id/settings: rewrite the pool file,
// validate it, reload FPM, and roll back if the reload fails.
func (h *PHPHandler) ApplyPoolSettings(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	var req service.PoolSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	result, err := h.phpService.ApplyPoolSettings(c.Request.Context(), c.Param("id"), tenantID, &req)
	if err != nil {
		// A validation failure is the caller's fault and must not read as a
		// server error: they typed 256MB where 256M was expected.
		if isPoolValidationError(err) {
			utils.BadRequest(c, err.Error())
			return
		}
		runtimeError(c, h.logger, "applying PHP pool settings failed", err)
		return
	}
	utils.Success(c, result)
}

// isPoolValidationError distinguishes "the settings you sent cannot be
// rendered" from "the host would not take them".
func isPoolValidationError(err error) bool {
	text := err.Error()
	for _, marker := range []string{"invalid ", "must be between", "must not exceed",
		"must not be below", "may not run as root", "may not run in the root group",
		"is outside the supported range", "must be at least"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// GetSitePHPVersion handles GET /php/sites/:website_id/version.
func (h *PHPHandler) GetSitePHPVersion(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	result, err := h.phpService.GetSiteVersion(c.Request.Context(), c.Param("website_id"), tenantID)
	if err != nil {
		runtimeError(c, h.logger, "reading a site's PHP version failed", err)
		return
	}
	utils.Success(c, result)
}

// SetSitePHPVersion handles PUT /php/sites/:website_id/version. This is the
// feature customers migrate for.
func (h *PHPHandler) SetSitePHPVersion(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	var req service.SwitchVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	tenantID, ok := tenantOf(c)
	if !ok {
		return
	}
	result, err := h.phpService.SetSiteVersion(c.Request.Context(), c.Param("website_id"), tenantID, &req)
	if err != nil {
		if isPoolValidationError(err) {
			utils.BadRequest(c, err.Error())
			return
		}
		runtimeError(c, h.logger, "switching a site's PHP version failed", err)
		return
	}
	utils.Success(c, result)
}

// ---------------------------------------------------------------------------
// WordPress handlers
// ---------------------------------------------------------------------------

// siteAndTenant parses the two values every WordPress runtime route needs.
func siteAndTenant(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	siteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid site id")
		return uuid.Nil, uuid.Nil, false
	}
	tenantID, ok := tenantUUIDOf(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return siteID, tenantID, true
}

// InstallWordPress handles POST /wordpress/:id/install.
func (h *WordPressHandler) InstallWordPress(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.InstallSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	result, err := h.service.InstallSite(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "WordPress installation failed", err)
		return
	}
	utils.Created(c, result)
}

// GetSiteRuntime handles GET /wordpress/:id/runtime: the answer to "which user
// does this site's WP-CLI run as".
func (h *WordPressHandler) GetSiteRuntime(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	runtime, err := h.service.GetRuntime(c.Request.Context(), tenantID, siteID)
	if err != nil {
		runtimeError(c, nil, "reading a WordPress site's runtime identity failed", err)
		return
	}
	utils.Success(c, runtime.View())
}

// SetSiteRuntime handles PUT /wordpress/:id/runtime.
func (h *WordPressHandler) SetSiteRuntime(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.SetRuntimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	runtime, err := h.service.SetRuntime(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "setting a WordPress site's runtime identity failed", err)
		return
	}
	utils.Success(c, runtime.View())
}

// ListLivePlugins handles GET /wordpress/:id/plugins/live.
func (h *WordPressHandler) ListLivePlugins(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	plugins, ranAs, err := h.service.LivePlugins(c.Request.Context(), tenantID, siteID)
	if err != nil {
		runtimeError(c, nil, "listing WordPress plugins failed", err)
		return
	}
	utils.Success(c, gin.H{"plugins": plugins, "ran_as": ranAs})
}

// UpdateLivePlugins handles POST /wordpress/:id/plugins/update.
func (h *WordPressHandler) UpdateLivePlugins(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.UpdateRequest
	_ = c.ShouldBindJSON(&req) // an empty body means "update everything"
	result, err := h.service.UpdatePluginsLive(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "updating WordPress plugins failed", err)
		return
	}
	utils.Success(c, result)
}

// ListLiveThemes handles GET /wordpress/:id/themes/live.
func (h *WordPressHandler) ListLiveThemes(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	themes, ranAs, err := h.service.LiveThemes(c.Request.Context(), tenantID, siteID)
	if err != nil {
		runtimeError(c, nil, "listing WordPress themes failed", err)
		return
	}
	utils.Success(c, gin.H{"themes": themes, "ran_as": ranAs})
}

// UpdateLiveThemes handles POST /wordpress/:id/themes/update.
func (h *WordPressHandler) UpdateLiveThemes(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.UpdateRequest
	_ = c.ShouldBindJSON(&req)
	result, err := h.service.UpdateThemesLive(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "updating WordPress themes failed", err)
		return
	}
	utils.Success(c, result)
}

// GetCoreVersion handles GET /wordpress/:id/core/version.
func (h *WordPressHandler) GetCoreVersion(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	runtime, err := h.service.GetRuntime(c.Request.Context(), tenantID, siteID)
	if err != nil {
		runtimeError(c, nil, "reading the WordPress core version failed", err)
		return
	}
	utils.Success(c, runtime.View())
}

// UpdateCore handles POST /wordpress/:id/core/update.
func (h *WordPressHandler) UpdateCore(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.CoreUpdateRequest
	_ = c.ShouldBindJSON(&req)
	result, err := h.service.UpdateCoreLive(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "updating WordPress core failed", err)
		return
	}
	utils.Success(c, result)
}

// SearchReplace handles POST /wordpress/:id/search-replace.
//
// dry_run defaults to true when the field is absent: this rewrites every row of
// a customer's database, and the answer you get by leaving a field out has to
// be the safe one.
func (h *WordPressHandler) SearchReplace(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.SearchReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	report, ranAs, err := h.service.SearchReplaceLive(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "WordPress search-replace failed", err)
		return
	}
	utils.Success(c, gin.H{"report": report, "ran_as": ranAs})
}

// ResetUserPassword handles POST /wordpress/:id/users/password.
func (h *WordPressHandler) ResetUserPassword(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	result, err := h.service.ResetUserPasswordLive(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "resetting a WordPress password failed", err)
		return
	}
	utils.Success(c, result)
}

// GetStaging handles GET /wordpress/:id/staging.
func (h *WordPressHandler) GetStaging(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	view, err := h.service.GetStaging(c.Request.Context(), tenantID, siteID)
	if err != nil {
		runtimeError(c, nil, "reading a staging environment failed", err)
		return
	}
	utils.Success(c, view)
}

// CreateStaging handles POST /wordpress/:id/staging.
func (h *WordPressHandler) CreateStaging(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.CreateStagingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	view, err := h.service.CreateStaging(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "cloning a site to staging failed", err)
		return
	}
	utils.Created(c, view)
}

// PushStaging handles POST /wordpress/:id/staging/push.
//
// A body with no "database" field is refused with 400 and the list of choices.
// That refusal is the point of the endpoint: pushing a staging database over
// production is how a customer loses a week of orders, so the panel will not
// choose for them.
func (h *WordPressHandler) PushStaging(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	var req service.PushStagingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	view, err := h.service.PushStaging(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		runtimeError(c, nil, "pushing staging to production failed", err)
		return
	}
	utils.Success(c, view)
}

// DeleteStaging handles DELETE /wordpress/:id/staging.
func (h *WordPressHandler) DeleteStaging(c *gin.Context) {
	if !h.wired(c) {
		return
	}
	siteID, tenantID, ok := siteAndTenant(c)
	if !ok {
		return
	}
	if err := h.service.DeleteStaging(c.Request.Context(), tenantID, siteID); err != nil {
		runtimeError(c, nil, "removing a staging environment failed", err)
		return
	}
	utils.Message(c, "staging environment record removed; its files and database were left in place")
}
