package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type BackupHandler struct {
	backupService *service.BackupService
}

func NewBackupHandler(backupService *service.BackupService) *BackupHandler {
	return &BackupHandler{backupService: backupService}
}

func (h *BackupHandler) CreateJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateBackupJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	job, err := h.backupService.CreateJob(c.Request.Context(), &req, tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, job)
}

func (h *BackupHandler) GetJob(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid job ID")
		return
	}

	job, err := h.backupService.GetJobByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Backup job not found")
		return
	}

	utils.Success(c, job)
}

func (h *BackupHandler) ListJobs(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	jobs, err := h.backupService.ListJobsByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, jobs)
}

func (h *BackupHandler) UpdateJob(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid job ID")
		return
	}

	job, err := h.backupService.GetJobByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Backup job not found")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Destination string `json:"destination"`
		Schedule    string `json:"schedule"`
		Retention   int    `json:"retention"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Name != "" {
		job.Name = req.Name
	}
	if req.Destination != "" {
		job.Destination = req.Destination
	}
	if req.Schedule != "" {
		job.Schedule = req.Schedule
	}
	if req.Retention > 0 {
		job.Retention = req.Retention
	}

	if err := h.backupService.UpdateJob(c.Request.Context(), job); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, job)
}

func (h *BackupHandler) DeleteJob(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid job ID")
		return
	}

	if err := h.backupService.DeleteJob(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Backup job deleted"})
}

func (h *BackupHandler) RunBackup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid job ID")
		return
	}

	record, err := h.backupService.RunBackup(c.Request.Context(), id)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, record)
}

func (h *BackupHandler) ListRecords(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	records, err := h.backupService.ListRecordsByTenant(c.Request.Context(), tenantID, 50)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, records)
}

func (h *BackupHandler) DeleteRecord(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid record ID")
		return
	}

	if err := h.backupService.DeleteRecord(c.Request.Context(), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Backup record deleted"})
}
