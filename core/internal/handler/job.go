package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"go.uber.org/zap"
)

// JobHandler handles job HTTP requests
type JobHandler struct {
	service *service.JobService
	logger  *zap.Logger
}

// NewJobHandler creates a new job handler
func NewJobHandler(service *service.JobService, logger *zap.Logger) *JobHandler {
	return &JobHandler{
		service: service,
		logger:  logger,
	}
}

// GetJob handles GET /jobs/:id
func (h *JobHandler) GetJob(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	record, err := h.service.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	c.JSON(http.StatusOK, record)
}

// ListJobs handles GET /jobs
func (h *JobHandler) ListJobs(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	filter := &job.JobFilter{
		TaskType: c.Query("task_type"),
		Status:   c.Query("status"),
		Queue:    c.Query("queue"),
	}

	if serverIDStr := c.Query("server_id"); serverIDStr != "" {
		serverID, err := uuid.Parse(serverIDStr)
		if err == nil {
			filter.ServerID = &serverID
		}
	}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err == nil {
			filter.UserID = &userID
		}
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	filter.Page = page
	filter.PageSize = pageSize

	records, total, err := h.service.ListJobs(c.Request.Context(), tenantID, filter)
	if err != nil {
		h.logger.Error("Failed to list jobs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list jobs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetJobStats handles GET /jobs/stats
func (h *JobHandler) GetJobStats(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	stats, err := h.service.GetJobStats(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get job stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get job stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetQueueStats handles GET /jobs/queue-stats
func (h *JobHandler) GetQueueStats(c *gin.Context) {
	stats, err := h.service.GetQueueStats()
	if err != nil {
		h.logger.Error("Failed to get queue stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get queue stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// EnqueueBackup handles POST /jobs/backup
func (h *JobHandler) EnqueueBackup(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var payload job.BackupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	record, err := h.service.EnqueueBackup(c.Request.Context(), tenantID, &payload)
	if err != nil {
		h.logger.Error("Failed to enqueue backup job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue backup job"})
		return
	}

	c.JSON(http.StatusCreated, record)
}

// EnqueueRestore handles POST /jobs/restore
func (h *JobHandler) EnqueueRestore(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var payload job.RestorePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	record, err := h.service.EnqueueRestore(c.Request.Context(), tenantID, &payload)
	if err != nil {
		h.logger.Error("Failed to enqueue restore job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue restore job"})
		return
	}

	c.JSON(http.StatusCreated, record)
}

// EnqueueDeploy handles POST /jobs/deploy
func (h *JobHandler) EnqueueDeploy(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var payload job.DeployPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	record, err := h.service.EnqueueDeploy(c.Request.Context(), tenantID, &payload)
	if err != nil {
		h.logger.Error("Failed to enqueue deploy job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue deploy job"})
		return
	}

	c.JSON(http.StatusCreated, record)
}

// EnqueueSSL handles POST /jobs/ssl
func (h *JobHandler) EnqueueSSL(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var payload job.SSLEventPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	record, err := h.service.EnqueueSSL(c.Request.Context(), tenantID, &payload)
	if err != nil {
		h.logger.Error("Failed to enqueue SSL job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue SSL job"})
		return
	}

	c.JSON(http.StatusCreated, record)
}

// EnqueueCleanup handles POST /jobs/cleanup
func (h *JobHandler) EnqueueCleanup(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var payload job.CleanupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	record, err := h.service.EnqueueCleanup(c.Request.Context(), tenantID, &payload)
	if err != nil {
		h.logger.Error("Failed to enqueue cleanup job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue cleanup job"})
		return
	}

	c.JSON(http.StatusCreated, record)
}

// CancelJob handles POST /jobs/:id/cancel
func (h *JobHandler) CancelJob(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if err := h.service.CancelJob(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to cancel job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job cancelled"})
}

// DeleteJob handles DELETE /jobs/:id
func (h *JobHandler) DeleteJob(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if err := h.service.DeleteJob(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to delete job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job deleted"})
}

// RetryJob handles POST /jobs/:id/retry
func (h *JobHandler) RetryJob(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if err := h.service.RetryJob(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to retry job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retry job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job queued for retry"})
}

// CleanupOldJobs handles POST /jobs/cleanup
func (h *JobHandler) CleanupOldJobs(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	retentionDays, _ := strconv.Atoi(c.DefaultQuery("retention_days", "30"))

	count, err := h.service.CleanupOldJobs(c.Request.Context(), tenantID, retentionDays)
	if err != nil {
		h.logger.Error("Failed to cleanup old jobs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup old jobs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Old jobs cleaned up",
		"count":   count,
	})
}
