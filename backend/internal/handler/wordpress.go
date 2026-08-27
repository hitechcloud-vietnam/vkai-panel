package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type WordPressHandler struct {
	service *service.WordPressService
}

func NewWordPressHandler(service *service.WordPressService) *WordPressHandler {
	return &WordPressHandler{service: service}
}

func (h *WordPressHandler) Create(c *gin.Context) {
	var req models.CreateWordPressSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	site, err := h.service.Create(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, site)
}

func (h *WordPressHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	site, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, site)
}

func (h *WordPressHandler) List(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	sites, total, err := h.service.List(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sites":  sites,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *WordPressHandler) ListByServer(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("server_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	sites, err := h.service.ListByServer(c.Request.Context(), tenantID, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sites": sites})
}

func (h *WordPressHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	var req models.UpdateWordPressSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	site, err := h.service.Update(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, site)
}

func (h *WordPressHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "wordpress site deleted"})
}

// Plugin handlers
func (h *WordPressHandler) InstallPlugin(c *gin.Context) {
	siteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	var req models.InstallPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	plugin, err := h.service.InstallPlugin(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, plugin)
}

func (h *WordPressHandler) ListPlugins(c *gin.Context) {
	siteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	plugins, err := h.service.ListPlugins(c.Request.Context(), tenantID, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"plugins": plugins})
}

func (h *WordPressHandler) UpdatePlugin(c *gin.Context) {
	pluginID, err := uuid.Parse(c.Param("pluginId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin id"})
		return
	}

	var req models.InstallPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	plugin, err := h.service.UpdatePlugin(c.Request.Context(), tenantID, pluginID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plugin)
}

func (h *WordPressHandler) DeletePlugin(c *gin.Context) {
	pluginID, err := uuid.Parse(c.Param("pluginId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeletePlugin(c.Request.Context(), tenantID, pluginID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "plugin deleted"})
}

// Theme handlers
func (h *WordPressHandler) InstallTheme(c *gin.Context) {
	siteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	var req models.InstallThemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	theme, err := h.service.InstallTheme(c.Request.Context(), tenantID, siteID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, theme)
}

func (h *WordPressHandler) ListThemes(c *gin.Context) {
	siteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	themes, err := h.service.ListThemes(c.Request.Context(), tenantID, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"themes": themes})
}

func (h *WordPressHandler) UpdateTheme(c *gin.Context) {
	themeID, err := uuid.Parse(c.Param("themeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid theme id"})
		return
	}

	var req models.InstallThemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	theme, err := h.service.UpdateTheme(c.Request.Context(), tenantID, themeID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, theme)
}

func (h *WordPressHandler) DeleteTheme(c *gin.Context) {
	themeID, err := uuid.Parse(c.Param("themeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid theme id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteTheme(c.Request.Context(), tenantID, themeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "theme deleted"})
}
