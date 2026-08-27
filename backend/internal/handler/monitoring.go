package handler

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type MonitoringHandler struct {
	service *service.MonitoringService
	logger  *zap.Logger
}

func NewMonitoringHandler(service *service.MonitoringService, logger *zap.Logger) *MonitoringHandler {
	return &MonitoringHandler{
		service: service,
		logger:  logger,
	}
}

func (h *MonitoringHandler) GetSystemInfo(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	utils.Success(c, gin.H{
		"system": gin.H{
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
			"num_cpu":       runtime.NumCPU(),
			"num_goroutine": runtime.NumGoroutine(),
			"go_version":    runtime.Version(),
		},
		"memory": gin.H{
			"alloc":         m.Alloc,
			"total_alloc":   m.TotalAlloc,
			"sys":           m.Sys,
			"num_gc":        m.NumGC,
			"heap_alloc":    m.HeapAlloc,
			"heap_sys":      m.HeapSys,
			"heap_idle":     m.HeapIdle,
			"heap_inuse":    m.HeapInuse,
			"heap_released": m.HeapReleased,
			"heap_objects":  m.HeapObjects,
			"stack_inuse":   m.StackInuse,
			"stack_sys":     m.StackSys,
		},
		"timestamp": time.Now().UTC(),
	})
}

func (h *MonitoringHandler) GetMetrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	utils.Success(c, gin.H{
		"memory": gin.H{
			"alloc":      m.Alloc,
			"total_alloc": m.TotalAlloc,
			"sys":        m.Sys,
			"num_gc":     m.NumGC,
		},
		"goroutines": runtime.NumGoroutine(),
		"timestamp":  time.Now().UTC(),
	})
}

// Metric handlers
func (h *MonitoringHandler) RecordMetric(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("server_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	var req struct {
		Metric string      `json:"metric" binding:"required"`
		Value  float64     `json:"value" binding:"required"`
		Unit   string      `json:"unit"`
		Tags   models.JSONMap `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.RecordMetric(c.Request.Context(), tenantID, serverID, req.Metric, req.Value, req.Unit, req.Tags); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "metric recorded"})
}

func (h *MonitoringHandler) GetServerMetrics(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("server_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	metric := c.Query("metric")
	if metric == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metric is required"})
		return
	}

	startStr := c.DefaultQuery("start", time.Now().AddDate(0, 0, -1).Format(time.RFC3339))
	endStr := c.DefaultQuery("end", time.Now().Format(time.RFC3339))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time"})
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	metrics, err := h.service.GetMetrics(c.Request.Context(), tenantID, serverID, metric, start, end, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metrics": metrics})
}

func (h *MonitoringHandler) GetLatestMetric(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("server_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	metric := c.Query("metric")
	if metric == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metric is required"})
		return
	}

	m, err := h.service.GetLatestMetric(c.Request.Context(), serverID, metric)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, m)
}

// Alert handlers
func (h *MonitoringHandler) CreateAlert(c *gin.Context) {
	var req models.CreateMonitoringAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	alert, err := h.service.CreateAlert(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, alert)
}

func (h *MonitoringHandler) GetAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	alert, err := h.service.GetAlertByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *MonitoringHandler) ListAlerts(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	alerts, total, err := h.service.ListAlerts(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *MonitoringHandler) UpdateAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}

	var req models.UpdateMonitoringAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	alert, err := h.service.UpdateAlert(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, alert)
}

func (h *MonitoringHandler) DeleteAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteAlert(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert deleted"})
}

func (h *MonitoringHandler) ListAlertLogs(c *gin.Context) {
	alertID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.service.ListAlertLogs(c.Request.Context(), tenantID, alertID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Dashboard handlers
func (h *MonitoringHandler) CreateDashboard(c *gin.Context) {
	var req models.CreateMonitoringDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	dashboard, err := h.service.CreateDashboard(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dashboard)
}

func (h *MonitoringHandler) GetDashboard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dashboard id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	dashboard, err := h.service.GetDashboardByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

func (h *MonitoringHandler) ListDashboards(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	dashboards, err := h.service.ListDashboards(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"dashboards": dashboards})
}

func (h *MonitoringHandler) UpdateDashboard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dashboard id"})
		return
	}

	var req models.UpdateMonitoringDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	dashboard, err := h.service.UpdateDashboard(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

func (h *MonitoringHandler) DeleteDashboard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dashboard id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteDashboard(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dashboard deleted"})
}

func (h *MonitoringHandler) GetMetrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	utils.Success(c, gin.H{
		"metrics": gin.H{
			"goroutines": runtime.NumGoroutine(),
			"memory": gin.H{
				"alloc_mb":     float64(m.Alloc) / 1024 / 1024,
				"total_mb":     float64(m.TotalAlloc) / 1024 / 1024,
				"sys_mb":       float64(m.Sys) / 1024 / 1024,
				"heap_alloc_mb": float64(m.HeapAlloc) / 1024 / 1024,
				"heap_sys_mb":  float64(m.HeapSys) / 1024 / 1024,
			},
			"gc": gin.H{
				"num_gc":         m.NumGC,
				"pause_total_ns": m.PauseTotalNs,
				"last_gc":        time.Unix(0, int64(m.LastGC)).UTC(),
			},
		},
		"timestamp": time.Now().UTC(),
	})
}
