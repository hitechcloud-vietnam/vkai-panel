package handler

// The offsite backup endpoints: destinations, encrypted archives to them,
// one-action restore with a dry run, and the restorability check.
//
// ============================================================================
// THIS FILE ADDS NO ROUTES ON ITS OWN. ONE LINE IS NEEDED IN router.go:
//
//	RegisterBackupOffsiteRoutes(protected, r.backupHandler)
//
// It goes immediately after the existing `backups := protected.Group(...)`
// block in Router.Setup. Without that line every handler below is dead code -
// which is exactly the failure this project spent a day undoing, four times
// over, so it is written here at the top of the file rather than in a commit
// message nobody will read.
// ============================================================================
//
// The routes deliberately live in a second group under the same /backups
// prefix rather than replacing the eight lines already in router.go. The two
// groups do not collide: every path added here is under a different static
// segment (destinations, artifacts, verifications, restores, operations,
// health) or a longer path under an existing one (jobs/:id/offsite), and
// backup_offsite_routes_test.go builds an engine with BOTH registered to prove
// gin accepts them together.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/backup"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// RegisterBackupOffsiteRoutes mounts the offsite backup endpoints onto the
// authenticated group. It is the single entry point, so router.go needs one
// line and cannot mount half of the feature.
//
// The whole group is gated on the "backup" permission, the same resource the
// existing /backups group uses, so no role has to be edited to reach it. The
// two actions this feature adds - backup:restore and backup:verify - are
// seeded by migrations/pending/backup.sql for the administrator roles; they
// exist so a future policy can distinguish "may read backups" from "may
// restore over a live site", and are not yet enforced per route because
// RequirePermission takes a resource, not an action.
func RegisterBackupOffsiteRoutes(rg *gin.RouterGroup, h *BackupHandler) {
	if h == nil {
		return
	}

	group := rg.Group("/backups", middleware.RequirePermission("backup"))
	{
		// Where backups go.
		group.POST("/destinations", h.CreateDestination)
		group.GET("/destinations", h.ListDestinations)
		group.GET("/destinations/:id", h.GetDestination)
		group.DELETE("/destinations/:id", h.DeleteDestination)
		group.POST("/destinations/:id/probe", h.ProbeDestination)

		// Pointing a job at one, and running it.
		group.PUT("/jobs/:id/offsite", h.ConfigureJob)
		group.GET("/jobs/:id/offsite", h.GetJobSettings)
		group.POST("/jobs/:id/offsite/run", h.RunOffsiteBackup)
		group.POST("/jobs/:id/offsite/retention", h.ApplyRetention)

		// The archives that exist, and the evidence that they restore.
		group.GET("/artifacts", h.ListArtifacts)
		group.GET("/artifacts/:id", h.GetArtifact)
		group.POST("/artifacts/:id/verify", h.VerifyArtifact)
		group.GET("/artifacts/:id/verifications", h.ListArtifactVerifications)
		group.GET("/verifications", h.ListVerifications)
		group.POST("/verifications/run-due", h.RunDueVerifications)
		group.GET("/health", h.BackupHealth)

		// Restore, and the dry run that comes before it.
		group.POST("/restores", h.Restore)
		group.GET("/restores", h.ListRestores)
		group.GET("/restores/:id", h.GetRestore)

		// Progress and cancellation for the long-running ones.
		group.GET("/operations", h.ListOperations)
		group.GET("/operations/:id", h.GetOperation)
		group.POST("/operations/:id/cancel", h.CancelOperation)
	}
}

// pathUUID parses the :id parameter, answering 400 rather than 500 when it is
// not a UUID.
func pathUUID(c *gin.Context, what string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid "+what)
		return uuid.Nil, false
	}
	return id, true
}

func queryLimit(c *gin.Context, fallback int) int {
	raw := c.Query("limit")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// ------------------------------------------------------------
// Destinations
// ------------------------------------------------------------

func (h *BackupHandler) CreateDestination(c *gin.Context) {
	var req service.CreateDestinationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	dest, err := h.backupService.CreateDestination(c.Request.Context(), middleware.GetTenantID(c), &req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Created(c, dest)
}

func (h *BackupHandler) ListDestinations(c *gin.Context) {
	destinations, err := h.backupService.ListDestinations(c.Request.Context(), middleware.GetTenantID(c))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, destinations)
}

func (h *BackupHandler) GetDestination(c *gin.Context) {
	id, ok := pathUUID(c, "destination ID")
	if !ok {
		return
	}
	dest, err := h.backupService.GetDestination(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Backup destination not found")
		return
	}
	utils.Success(c, dest)
}

func (h *BackupHandler) DeleteDestination(c *gin.Context) {
	id, ok := pathUUID(c, "destination ID")
	if !ok {
		return
	}
	if err := h.backupService.DeleteDestination(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		// A destination still referenced by a job or an artifact is refused by
		// the database, which is a conflict and not a server fault.
		utils.Conflict(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"message": "Backup destination deleted"})
}

// ProbeDestination writes an object, reads it back and deletes it. It is the
// only honest answer to "is this destination configured correctly".
func (h *BackupHandler) ProbeDestination(c *gin.Context) {
	id, ok := pathUUID(c, "destination ID")
	if !ok {
		return
	}
	if err := h.backupService.ProbeDestination(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"data":    gin.H{"ok": false, "error": err.Error()},
		})
		return
	}
	utils.Success(c, gin.H{"ok": true})
}

// ------------------------------------------------------------
// Job settings
// ------------------------------------------------------------

func (h *BackupHandler) ConfigureJob(c *gin.Context) {
	id, ok := pathUUID(c, "job ID")
	if !ok {
		return
	}
	var req service.ConfigureJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	settings, err := h.backupService.ConfigureJob(c.Request.Context(), middleware.GetTenantID(c), id, &req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, settings)
}

func (h *BackupHandler) GetJobSettings(c *gin.Context) {
	id, ok := pathUUID(c, "job ID")
	if !ok {
		return
	}
	settings, err := h.backupService.GetJobSettings(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}
	utils.Success(c, settings)
}

// RunOffsiteBackup starts a backup and answers immediately with the operation.
// The client polls /backups/operations/:id for progress and can cancel there.
func (h *BackupHandler) RunOffsiteBackup(c *gin.Context) {
	id, ok := pathUUID(c, "job ID")
	if !ok {
		return
	}
	op, err := h.backupService.RunOffsiteBackup(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": op})
}

func (h *BackupHandler) ApplyRetention(c *gin.Context) {
	id, ok := pathUUID(c, "job ID")
	if !ok {
		return
	}
	decisions, err := h.backupService.ApplyRetention(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"decisions": decisions})
}

// ------------------------------------------------------------
// Artifacts and verification
// ------------------------------------------------------------

func (h *BackupHandler) ListArtifacts(c *gin.Context) {
	artifacts, err := h.backupService.ListArtifacts(c.Request.Context(), middleware.GetTenantID(c), queryLimit(c, 100))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, artifacts)
}

func (h *BackupHandler) GetArtifact(c *gin.Context) {
	id, ok := pathUUID(c, "artifact ID")
	if !ok {
		return
	}
	artifact, err := h.backupService.GetArtifact(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Backup artifact not found")
		return
	}
	utils.Success(c, artifact)
}

// VerifyArtifact restores the archive into scratch space and checks it.
//
// A verification that FAILS is answered 200 with a failed record, not 500. The
// call did what it was asked to do; the answer is bad news, and returning it as
// a server error would tempt a client into treating it as something to retry.
func (h *BackupHandler) VerifyArtifact(c *gin.Context) {
	id, ok := pathUUID(c, "artifact ID")
	if !ok {
		return
	}
	record, err := h.backupService.VerifyArtifact(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, record)
}

func (h *BackupHandler) ListArtifactVerifications(c *gin.Context) {
	id, ok := pathUUID(c, "artifact ID")
	if !ok {
		return
	}
	records, err := h.backupService.ListVerificationsForArtifact(
		c.Request.Context(), middleware.GetTenantID(c), id, queryLimit(c, 50))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, records)
}

func (h *BackupHandler) ListVerifications(c *gin.Context) {
	records, err := h.backupService.ListVerifications(c.Request.Context(), middleware.GetTenantID(c), queryLimit(c, 100))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, records)
}

// RunDueVerifications is what a scheduled task calls. It is also exposed so an
// operator can ask "prove the backups restore" and get an answer now.
func (h *BackupHandler) RunDueVerifications(c *gin.Context) {
	records, err := h.backupService.VerifyDue(c.Request.Context(), queryLimit(c, 10))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"verifications": records, "count": len(records)})
}

// BackupHealth is the one number that matters: how much of what is stored has
// been proved to restore.
func (h *BackupHandler) BackupHealth(c *gin.Context) {
	health, err := h.backupService.BackupHealth(c.Request.Context(), middleware.GetTenantID(c))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, health)
}

// ------------------------------------------------------------
// Restore
// ------------------------------------------------------------

// restoreRequestBody is the wire shape. dry_run is a pointer so that the
// server can tell "the client said false" from "the client did not say", and
// default the second to a dry run. Restoring over a live document root must be
// something somebody typed, not something they omitted.
type restoreRequestBody struct {
	ArtifactID     string     `json:"artifact_id" binding:"required"`
	TargetPath     string     `json:"target_path" binding:"omitempty,max=512"`
	DryRun         *bool      `json:"dry_run"`
	AllowOverwrite bool       `json:"allow_overwrite"`
	TargetServerID *uuid.UUID `json:"target_server_id"`
}

func (h *BackupHandler) Restore(c *gin.Context) {
	var body restoreRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	artifactID, err := uuid.Parse(body.ArtifactID)
	if err != nil {
		utils.BadRequest(c, "Invalid artifact ID")
		return
	}

	dryRun := true
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}

	req := &service.RestoreRequest{
		ArtifactID:     artifactID,
		TargetPath:     body.TargetPath,
		DryRun:         dryRun,
		AllowOverwrite: body.AllowOverwrite,
		TargetServerID: body.TargetServerID,
	}

	record, op, err := h.backupService.Restore(c.Request.Context(), middleware.GetTenantID(c), req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	payload := gin.H{"restore": record, "dry_run": dryRun}
	if op != nil {
		payload["operation"] = op
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": payload})
		return
	}
	utils.Success(c, payload)
}

func (h *BackupHandler) ListRestores(c *gin.Context) {
	records, err := h.backupService.ListRestores(c.Request.Context(), middleware.GetTenantID(c), queryLimit(c, 100))
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, records)
}

func (h *BackupHandler) GetRestore(c *gin.Context) {
	id, ok := pathUUID(c, "restore ID")
	if !ok {
		return
	}
	record, err := h.backupService.GetRestore(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "Restore not found")
		return
	}
	utils.Success(c, record)
}

// ------------------------------------------------------------
// Operations
// ------------------------------------------------------------

func (h *BackupHandler) ListOperations(c *gin.Context) {
	utils.Success(c, h.backupService.ListOperations(middleware.GetTenantID(c)))
}

func (h *BackupHandler) GetOperation(c *gin.Context) {
	id, ok := pathUUID(c, "operation ID")
	if !ok {
		return
	}
	op, err := h.backupService.GetOperation(middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}
	utils.Success(c, op)
}

func (h *BackupHandler) CancelOperation(c *gin.Context) {
	id, ok := pathUUID(c, "operation ID")
	if !ok {
		return
	}
	if err := h.backupService.CancelOperation(middleware.GetTenantID(c), id); err != nil {
		utils.Conflict(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"message": "Cancellation requested", "phase": backup.PhaseCancelled})
}
