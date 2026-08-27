package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type ServiceHandler struct {
	serviceManager *service.ServiceManager
}

func NewServiceHandler(serviceManager *service.ServiceManager) *ServiceHandler {
	return &ServiceHandler{serviceManager: serviceManager}
}

func (h *ServiceHandler) List(c *gin.Context) {
	services, err := h.serviceManager.ListServices(c.Request.Context())
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, services)
}

func (h *ServiceHandler) GetStatus(c *gin.Context) {
	name := c.Param("name")

	info, err := h.serviceManager.GetServiceStatus(c.Request.Context(), name)
	if err != nil {
		utils.NotFound(c, "Service not found")
		return
	}

	utils.Success(c, info)
}

func (h *ServiceHandler) Start(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.StartService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service started"})
}

func (h *ServiceHandler) Stop(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.StopService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service stopped"})
}

func (h *ServiceHandler) Restart(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.RestartService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service restarted"})
}

func (h *ServiceHandler) Enable(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.EnableService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service enabled"})
}

func (h *ServiceHandler) Disable(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.DisableService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service disabled"})
}

func (h *ServiceHandler) Create(c *gin.Context) {
	var req struct {
		Name        string            `json:"name" binding:"required"`
		Description string            `json:"description" binding:"required"`
		ExecStart   string            `json:"exec_start" binding:"required"`
		WorkDir     string            `json:"work_dir"`
		User        string            `json:"user"`
		Env         map[string]string `json:"env"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Services created through the API never run as root.
	if req.User == "" {
		req.User = "www-data"
	}

	if err := h.serviceManager.CreateService(c.Request.Context(), req.Name, req.Description, req.ExecStart, req.WorkDir, req.User, req.Env); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, gin.H{"message": "Service created"})
}

func (h *ServiceHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	if err := h.serviceManager.DeleteService(c.Request.Context(), name); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Service deleted"})
}

func (h *ServiceHandler) GetLogs(c *gin.Context) {
	name := c.Param("name")
	linesStr := c.DefaultQuery("lines", "100")
	lines, _ := strconv.Atoi(linesStr)
	if lines <= 0 {
		lines = 100
	}

	logs, err := h.serviceManager.GetServiceLogs(c.Request.Context(), name, lines)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"logs": logs})
}
