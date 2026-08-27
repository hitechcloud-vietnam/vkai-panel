package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type PHPHandler struct {
	phpService *service.PHPService
	logger     *zap.Logger
}

func NewPHPHandler(phpService *service.PHPService, logger *zap.Logger) *PHPHandler {
	return &PHPHandler{
		phpService: phpService,
		logger:     logger,
	}
}

// CreatePHPVersion creates a new PHP version
func (h *PHPHandler) CreatePHPVersion(c *gin.Context) {
	var req models.CreatePHPVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	php, err := h.phpService.CreatePHPVersion(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create PHP version", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, php)
}

// GetPHPVersion gets a PHP version by ID
func (h *PHPHandler) GetPHPVersion(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	php, err := h.phpService.GetPHPVersion(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get PHP version", zap.Error(err))
		utils.NotFound(c, "PHP version not found")
		return
	}

	utils.Success(c, php)
}

// ListPHPVersions lists all PHP versions
func (h *PHPHandler) ListPHPVersions(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	serverID := c.Query("server_id")

	phpVersions, err := h.phpService.ListPHPVersions(c.Request.Context(), tenantID, serverID)
	if err != nil {
		h.logger.Error("Failed to list PHP versions", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, phpVersions)
}

// UpdatePHPVersion updates a PHP version
func (h *PHPHandler) UpdatePHPVersion(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	var req models.UpdatePHPVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	php, err := h.phpService.UpdatePHPVersion(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update PHP version", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, php)
}

// DeletePHPVersion deletes a PHP version
func (h *PHPHandler) DeletePHPVersion(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	if err := h.phpService.DeletePHPVersion(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete PHP version", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

// CreatePHPPool creates a new PHP-FPM pool
func (h *PHPHandler) CreatePHPPool(c *gin.Context) {
	var req models.CreatePHPPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	pool, err := h.phpService.CreatePHPPool(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create PHP-FPM pool", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, pool)
}

// GetPHPPool gets a PHP-FPM pool by ID
func (h *PHPHandler) GetPHPPool(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	pool, err := h.phpService.GetPHPPool(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get PHP-FPM pool", zap.Error(err))
		utils.NotFound(c, "PHP-FPM pool not found")
		return
	}

	utils.Success(c, pool)
}

// ListPHPPools lists all PHP-FPM pools
func (h *PHPHandler) ListPHPPools(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	serverID := c.Query("server_id")
	websiteID := c.Query("website_id")

	pools, err := h.phpService.ListPHPPools(c.Request.Context(), tenantID, serverID, websiteID)
	if err != nil {
		h.logger.Error("Failed to list PHP-FPM pools", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, pools)
}

// UpdatePHPPool updates a PHP-FPM pool
func (h *PHPHandler) UpdatePHPPool(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	var req models.UpdatePHPPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	pool, err := h.phpService.UpdatePHPPool(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update PHP-FPM pool", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, pool)
}

// DeletePHPPool deletes a PHP-FPM pool
func (h *PHPHandler) DeletePHPPool(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	if err := h.phpService.DeletePHPPool(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete PHP-FPM pool", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

// InstallPHPExtension installs a PHP extension
func (h *PHPHandler) InstallPHPExtension(c *gin.Context) {
	var req models.InstallPHPExtensionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	ext, err := h.phpService.InstallPHPExtension(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to install PHP extension", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, ext)
}

// ListPHPExtensions lists all PHP extensions
func (h *PHPHandler) ListPHPExtensions(c *gin.Context) {
	phpVersionID := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	extensions, err := h.phpService.ListPHPExtensions(c.Request.Context(), phpVersionID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list PHP extensions", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, extensions)
}

// UpdatePHPExtension updates a PHP extension
func (h *PHPHandler) UpdatePHPExtension(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	ext, err := h.phpService.UpdatePHPExtension(c.Request.Context(), id, tenantID, req.IsEnabled)
	if err != nil {
		h.logger.Error("Failed to update PHP extension", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, ext)
}

// DeletePHPExtension deletes a PHP extension
func (h *PHPHandler) DeletePHPExtension(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	if err := h.phpService.DeletePHPExtension(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete PHP extension", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

// GetPHPConfig gets PHP configuration
func (h *PHPHandler) GetPHPConfig(c *gin.Context) {
	phpVersionID := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	config, err := h.phpService.GetPHPConfig(c.Request.Context(), phpVersionID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get PHP config", zap.Error(err))
		utils.NotFound(c, "PHP config not found")
		return
	}

	utils.Success(c, config)
}

// UpdatePHPConfig updates PHP configuration
func (h *PHPHandler) UpdatePHPConfig(c *gin.Context) {
	phpVersionID := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	var req models.UpdatePHPConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	config, err := h.phpService.UpdatePHPConfig(c.Request.Context(), phpVersionID, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update PHP config", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, config)
}

// DeletePHPConfig deletes PHP configuration
func (h *PHPHandler) DeletePHPConfig(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Unauthorized(c, "Tenant ID not found")
		return
	}

	if err := h.phpService.DeletePHPConfig(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete PHP config", zap.Error(err))
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}
