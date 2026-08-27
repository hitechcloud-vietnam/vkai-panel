package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type NodeAppHandler struct {
	nodeAppService *service.NodeAppService
	logger         *zap.Logger
}

func NewNodeAppHandler(nodeAppService *service.NodeAppService, logger *zap.Logger) *NodeAppHandler {
	return &NodeAppHandler{
		nodeAppService: nodeAppService,
		logger:         logger,
	}
}

// Create creates a new Node.js application
func (h *NodeAppHandler) Create(c *gin.Context) {
	var req models.CreateNodeAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{
			"server_id": "Server ID is required",
			"name":      "Name is required",
			"path":      "Path is required",
			"port":      "Port is required",
		})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	app, err := h.nodeAppService.Create(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create Node.js app")
		return
	}

	utils.Success(c, http.StatusCreated, app)
}

// Get gets a Node.js application by ID
func (h *NodeAppHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	app, err := h.nodeAppService.Get(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get Node.js app", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "Node.js app not found")
		return
	}

	utils.Success(c, http.StatusOK, app)
}

// List lists all Node.js applications for a tenant
func (h *NodeAppHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	apps, total, err := h.nodeAppService.List(c.Request.Context(), tenantID, page, perPage)
	if err != nil {
		h.logger.Error("Failed to list Node.js apps", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list Node.js apps")
		return
	}

	utils.SuccessWithPagination(c, apps, total, page, perPage)
}

// Update updates a Node.js application
func (h *NodeAppHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	var req models.UpdateNodeAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	app, err := h.nodeAppService.Update(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update Node.js app")
		return
	}

	utils.Success(c, http.StatusOK, app)
}

// Delete deletes a Node.js application
func (h *NodeAppHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.Delete(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete Node.js app")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Node.js app deleted successfully"})
}

// Start starts a Node.js application
func (h *NodeAppHandler) Start(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.Start(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to start Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to start Node.js app")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Node.js app started successfully"})
}

// Stop stops a Node.js application
func (h *NodeAppHandler) Stop(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.Stop(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to stop Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to stop Node.js app")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Node.js app stopped successfully"})
}

// Restart restarts a Node.js application
func (h *NodeAppHandler) Restart(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.Restart(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to restart Node.js app", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to restart Node.js app")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Node.js app restarted successfully"})
}

// GetStatus gets the status of a Node.js application
func (h *NodeAppHandler) GetStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	status, err := h.nodeAppService.GetStatus(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get Node.js app status", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to get Node.js app status")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"status": status})
}

// GetLogs gets the logs of a Node.js application
func (h *NodeAppHandler) GetLogs(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if lines < 1 {
		lines = 100
	}

	logs, err := h.nodeAppService.GetLogs(c.Request.Context(), id, tenantID, lines)
	if err != nil {
		h.logger.Error("Failed to get Node.js app logs", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to get Node.js app logs")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"logs": logs})
}

// CreateDependency creates a new dependency for a Node.js app
func (h *NodeAppHandler) CreateDependency(c *gin.Context) {
	appID := c.Param("id")
	if appID == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	var req models.CreateNodeAppDependencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{
			"name":    "Name is required",
			"version": "Version is required",
		})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	dep, err := h.nodeAppService.CreateDependency(c.Request.Context(), appID, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to create dependency", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create dependency")
		return
	}

	utils.Success(c, http.StatusCreated, dep)
}

// ListDependencies lists all dependencies for a Node.js app
func (h *NodeAppHandler) ListDependencies(c *gin.Context) {
	appID := c.Param("id")
	if appID == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	deps, err := h.nodeAppService.ListDependencies(c.Request.Context(), appID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list dependencies", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list dependencies")
		return
	}

	utils.Success(c, http.StatusOK, deps)
}

// UpdateDependency updates a dependency
func (h *NodeAppHandler) UpdateDependency(c *gin.Context) {
	id := c.Param("depId")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Dependency ID is required")
		return
	}

	var req models.UpdateNodeAppDependencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{"version": "Version is required"})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	dep, err := h.nodeAppService.UpdateDependency(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update dependency", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update dependency")
		return
	}

	utils.Success(c, http.StatusOK, dep)
}

// DeleteDependency deletes a dependency
func (h *NodeAppHandler) DeleteDependency(c *gin.Context) {
	id := c.Param("depId")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Dependency ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.DeleteDependency(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete dependency", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete dependency")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Dependency deleted successfully"})
}

// CreateEnvironment creates a new environment variable for a Node.js app
func (h *NodeAppHandler) CreateEnvironment(c *gin.Context) {
	appID := c.Param("id")
	if appID == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	var req models.CreateNodeAppEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{
			"key":   "Key is required",
			"value": "Value is required",
		})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	env, err := h.nodeAppService.CreateEnvironment(c.Request.Context(), appID, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to create environment variable", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create environment variable")
		return
	}

	utils.Success(c, http.StatusCreated, env)
}

// ListEnvironments lists all environment variables for a Node.js app
func (h *NodeAppHandler) ListEnvironments(c *gin.Context) {
	appID := c.Param("id")
	if appID == "" {
		utils.Error(c, http.StatusBadRequest, "App ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	envs, err := h.nodeAppService.ListEnvironments(c.Request.Context(), appID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list environment variables", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list environment variables")
		return
	}

	utils.Success(c, http.StatusOK, envs)
}

// UpdateEnvironment updates an environment variable
func (h *NodeAppHandler) UpdateEnvironment(c *gin.Context) {
	id := c.Param("envId")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Environment ID is required")
		return
	}

	var req models.UpdateNodeAppEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{"value": "Value is required"})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	env, err := h.nodeAppService.UpdateEnvironment(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update environment variable", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update environment variable")
		return
	}

	utils.Success(c, http.StatusOK, env)
}

// DeleteEnvironment deletes an environment variable
func (h *NodeAppHandler) DeleteEnvironment(c *gin.Context) {
	id := c.Param("envId")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Environment ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.nodeAppService.DeleteEnvironment(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete environment variable", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete environment variable")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "Environment variable deleted successfully"})
}
