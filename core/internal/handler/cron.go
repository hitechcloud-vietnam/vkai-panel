package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type CronHandler struct {
	cronService *service.CronService
}

func NewCronHandler(cronService *service.CronService) *CronHandler {
	return &CronHandler{cronService: cronService}
}

func (h *CronHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateCronJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	job, err := h.cronService.Create(c.Request.Context(), &req, tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Created(c, job)
}

func (h *CronHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid cron job ID")
		return
	}

	job, err := h.cronService.GetByID(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Cron job not found")
		return
	}

	utils.Success(c, job)
}

func (h *CronHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	jobs, err := h.cronService.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, jobs)
}

func (h *CronHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid cron job ID")
		return
	}

	job, err := h.cronService.GetByID(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Cron job not found")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Command  string `json:"command"`
		Schedule string `json:"schedule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Name != "" {
		job.Name = req.Name
	}
	if req.Command != "" {
		job.Command = req.Command
	}
	if req.Schedule != "" {
		job.Schedule = req.Schedule
	}

	if err := h.cronService.Update(c.Request.Context(), job); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, job)
}

func (h *CronHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid cron job ID")
		return
	}

	if err := h.cronService.Delete(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Cron job deleted"})
}

func (h *CronHandler) ToggleStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid cron job ID")
		return
	}

	job, err := h.cronService.ToggleStatus(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, job)
}

func (h *CronHandler) RunNow(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid cron job ID")
		return
	}

	if err := h.cronService.RunNow(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Cron job executed"})
}
