package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type HealthHandler struct {
	logger *zap.Logger
}

func NewHealthHandler(logger *zap.Logger) *HealthHandler {
	return &HealthHandler{logger: logger}
}

func (h *HealthHandler) Health(c *gin.Context) {
	utils.Success(c, gin.H{
		"status":  "healthy",
		"service": "vkai-panel",
		"version": "1.0.0",
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	// TODO: Check database and Redis connectivity
	utils.Success(c, gin.H{
		"status": "ready",
	})
}
