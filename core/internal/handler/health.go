package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/version"
)

const (
	// HealthPath is the CANONICAL health probe path of the panel API.
	//
	// It is what nginx, deploy/install.sh, deploy/scripts/deploy.sh, the
	// documentation and every load balancer call, and it deliberately sits
	// outside /api/v1: an infrastructure probe must not have to move when the
	// API version does.
	HealthPath = "/health"

	// HealthAliasPath is an alias of HealthPath, kept so nothing that already
	// calls it breaks.
	//
	// internal/middleware.PanelGuard has always listed it among the paths it
	// answers without the security entrance, and docs/API.md offers it, but no
	// route was ever registered for it, so it answered 404 on every install.
	HealthAliasPath = "/api/v1/health"
)

// registerHealthRoutes installs the unauthenticated probe routes. Both health
// paths are registered from one place so the canonical path and its alias
// cannot drift apart.
func registerHealthRoutes(router gin.IRoutes, h *HealthHandler) {
	router.GET(HealthPath, h.Health)
	router.GET(HealthAliasPath, h.Health)
	router.GET("/ready", h.Ready)
	router.GET("/live", h.Live)
}

type HealthHandler struct {
	logger *zap.Logger
	db     interface{ Ping() error }
	rdb    interface{ Ping() error }
}

func NewHealthHandler(logger *zap.Logger) *HealthHandler {
	return &HealthHandler{logger: logger}
}

func (h *HealthHandler) SetDatabase(db interface{ Ping() error }) {
	h.db = db
}

func (h *HealthHandler) SetRedis(rdb interface{ Ping() error }) {
	h.rdb = rdb
}

// Health answers the liveness probe every load balancer, the installer and
// deploy/scripts/deploy.sh call. It is reachable without the security entrance
// (see internal/middleware.PanelGuard) and discloses nothing but up/down and
// which release is running.
//
// The version is read from internal/version, which is stamped at link time from
// the repository VERSION file. It used to be the string literal "1.0.0", so the
// installed panel reported a release that had not existed for months and the
// in-panel upgrade check compared against it.
func (h *HealthHandler) Health(c *gin.Context) {
	utils.Success(c, gin.H{
		"status":  "healthy",
		"service": "vkai-panel",
		"version": version.Version,
		"time":    time.Now().UTC(),
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	checks := gin.H{}
	allHealthy := true

	// Check database connectivity
	if h.db != nil {
		if err := h.db.Ping(); err != nil {
			checks["database"] = gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			allHealthy = false
		} else {
			checks["database"] = gin.H{
				"status": "healthy",
			}
		}
	}

	// Check Redis connectivity
	if h.rdb != nil {
		if err := h.rdb.Ping(); err != nil {
			checks["redis"] = gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			allHealthy = false
		} else {
			checks["redis"] = gin.H{
				"status": "healthy",
			}
		}
	}

	if allHealthy {
		utils.Success(c, gin.H{
			"status": "ready",
			"checks": checks,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"checks": checks,
		})
	}
}

func (h *HealthHandler) Live(c *gin.Context) {
	utils.Success(c, gin.H{
		"status": "alive",
	})
}
