package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

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

func (h *HealthHandler) Health(c *gin.Context) {
	utils.Success(c, gin.H{
		"status":  "healthy",
		"service": "vkai-panel",
		"version": "1.0.0",
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
