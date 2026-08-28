package handler

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type AuditHandler struct {
	service *service.AuditService
	logger  *zap.Logger
}

func NewAuditHandler(service *service.AuditService, logger *zap.Logger) *AuditHandler {
	return &AuditHandler{
		service: service,
		logger:  logger,
	}
}

// Service exposes the audit service so other handlers constructed inside the
// router can record entries without changing the router's constructor
// signature, which the API entry point passes positionally.
func (h *AuditHandler) Service() *service.AuditService {
	if h == nil {
		return nil
	}
	return h.service
}

func (h *AuditHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit log id"})
		return
	}

	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}
	log, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, log)
}

func (h *AuditHandler) Search(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}

	req := &models.AuditLogSearchRequest{
		Action:   c.Query("action"),
		Resource: c.Query("resource"),
		Status:   c.Query("status"),
	}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err == nil {
			req.UserID = &userID
		}
	}

	if resourceIDStr := c.Query("resource_id"); resourceIDStr != "" {
		resourceID, err := uuid.Parse(resourceIDStr)
		if err == nil {
			req.ResourceID = &resourceID
		}
	}

	if startStr := c.Query("start"); startStr != "" {
		start, err := time.Parse(time.RFC3339, startStr)
		if err == nil {
			req.Start = &start
		}
	}

	if endStr := c.Query("end"); endStr != "" {
		end, err := time.Parse(time.RFC3339, endStr)
		if err == nil {
			req.End = &end
		}
	}

	req.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "100"))
	req.Offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.service.Search(c.Request.Context(), tenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  req.Limit,
		"offset": req.Offset,
	})
}

func (h *AuditHandler) GetStats(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	stats, err := h.service.GetStats(c.Request.Context(), tenantID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// RegisterAuditRoutes mounts the tamper-evident chain endpoints.
//
// It is PURELY ADDITIVE. It deliberately does not re-register /audit/search,
// /audit/stats, /audit/:id or /audit/cleanup, which router.go already mounts:
// registering a path twice makes gin panic at start-up, so a registration
// function that overlapped an existing block would turn one line in router.go
// into a panic on every install. Everything new lives under /audit/chain.
//
// router.go needs exactly one line, anywhere inside the protected group, and
// nothing in that file has to be moved or deleted:
//
//	RegisterAuditRoutes(protected, r.auditHandler)
//
// The group carries the same "audit" permission the existing block carries.
// Reading somebody else's trail, and proving it has not been edited, are both
// administrative acts.
//
// /audit/chain/... does not collide with the existing /audit/:id: gin matches
// the static segment first. That is asserted in a test rather than assumed,
// because "the routes were never actually reachable" is a failure this project
// has already paid for once.
func RegisterAuditRoutes(rg *gin.RouterGroup, h *AuditHandler) {
	chain := rg.Group("/audit/chain", middleware.RequirePermission("audit"))

	// A panel whose audit service could not be built serves 503 with the
	// reason, not 404 and not nothing. The same choice registerTwoFactorUnavailable
	// makes, for the same reason: a 404 reads as "this panel has no such
	// feature", which sends an operator looking for a version problem, while a
	// 503 that names the cause is something they can fix. Registering nothing
	// at all would be worse still - it is indistinguishable from the line in
	// router.go having been forgotten, which is the failure this project has
	// already paid for once.
	if h == nil || h.Service() == nil {
		chain.Any("/*path", func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "audit chain verification is unavailable on this panel: " +
					"the audit service could not be built, which usually means the database " +
					"connection or migrations/pending/audit_chain.sql is missing",
			})
		})
		return
	}

	{
		chain.GET("/status", h.ChainStatus)
		chain.GET("/verify", h.VerifyChain)
		chain.POST("/verify", h.VerifyChain)
		chain.GET("/export", h.ExportChain)
		chain.GET("/seals", h.ChainSeals)
		chain.GET("/retention", h.Retention)
		chain.GET("/procedure", h.Procedure)
		chain.GET("/verifier", h.ReferenceVerifier)
	}
}

// ChainStatus handles GET /audit/chain/status: the cheap answer, no scan.
func (h *AuditHandler) ChainStatus(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}

	status, err := h.service.Status(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("audit chain status failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// VerifyChain handles GET|POST /audit/chain/verify.
//
// Query parameters:
//
//	deep=true   re-read audit_logs and recompute every content hash. This is
//	            what catches an EDITED entry, and it costs a join. Default true.
//	full=true   ignore what earlier passes established and walk from the oldest
//	            surviving entry. Default false: resume from the last clean pass.
//	from_seq, to_seq  verify an explicit range.
//
// A break answers 409 Conflict, not 200 with ok:false. A monitoring system that
// only looks at status codes must not read a tampered audit log as healthy.
func (h *AuditHandler) VerifyChain(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}

	req := service.VerifyRequest{
		Deep: c.DefaultQuery("deep", "true") == "true",
		Full: c.Query("full") == "true",
	}
	req.FromSeq, _ = strconv.ParseInt(c.DefaultQuery("from_seq", "0"), 10, 64)
	req.ToSeq, _ = strconv.ParseInt(c.DefaultQuery("to_seq", "0"), 10, 64)

	report, err := h.service.Verify(c.Request.Context(), tenantID, req)
	if err != nil {
		h.logger.Error("audit chain verification failed to run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !report.OK {
		h.logger.Error("AUDIT CHAIN BROKEN",
			zap.String("tenant_id", tenantID.String()),
			zap.Any("break_seq", report.BreakSeq),
			zap.Any("break_reason", report.BreakReason),
			zap.Any("break_at", report.BreakAt))
		c.JSON(http.StatusConflict, report)
		return
	}

	c.JSON(http.StatusOK, report)
}

// ExportChain handles GET /audit/chain/export: a bundle an outside auditor can
// verify with a SHA-256 implementation and nothing else.
func (h *AuditHandler) ExportChain(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}

	fromSeq, _ := strconv.ParseInt(c.DefaultQuery("from_seq", "0"), 10, 64)
	toSeq, _ := strconv.ParseInt(c.DefaultQuery("to_seq", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	note := c.Query("note")

	exp, err := h.service.Export(c.Request.Context(), tenantID, fromSeq, toSeq, limit, note)
	if err != nil {
		// A range that does not verify still comes back, with the reason, and
		// with a status that says it is not a clean bundle. Withholding it
		// would deny an investigator the evidence of the tampering.
		h.logger.Error("audit chain export did not verify", zap.Error(err))
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "bundle": exp})
		return
	}

	filename := fmt.Sprintf("audit-%s-%s.json", tenantID.String()[:8],
		time.Now().UTC().Format("20060102T150405Z"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.JSON(http.StatusOK, exp)
}

// ChainSeals handles GET /audit/chain/seals: every prune and every export.
func (h *AuditHandler) ChainSeals(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	seals, err := h.service.Seals(c.Request.Context(), tenantID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"seals": seals, "count": len(seals)})
}

// Retention handles GET /audit/chain/retention: what a prune would remove, and
// the commands an operator runs to do it. The panel cannot do it itself.
func (h *AuditHandler) Retention(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))

	plan, err := h.service.PlanRetention(c.Request.Context(), tenantID, days, panelDBUser())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

// Procedure handles GET /audit/chain/procedure: the published specification an
// auditor verifies a bundle against, served as text so it can be saved next to
// the bundle.
func (h *AuditHandler) Procedure(c *gin.Context) {
	c.Header("Content-Disposition", `attachment; filename="VERIFYING.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(h.service.ProcedureDoc()))
}

// ReferenceVerifier handles GET /audit/chain/verifier: a standalone Python 3
// implementation of that specification, depending on nothing but the standard
// library and on no part of this panel.
func (h *AuditHandler) ReferenceVerifier(c *gin.Context) {
	c.Header("Content-Disposition", `attachment; filename="verify_audit_export.py"`)
	c.Data(http.StatusOK, "text/x-python; charset=utf-8", []byte(h.service.ReferenceVerifier()))
}

// CleanupOld handles POST /audit/cleanup, which no longer cleans anything up.
//
// The route is kept and answers 409 with the reason and the operator's command,
// rather than being deleted. A client that used to call this needs to be told
// that the answer changed; a 404 would read as "this panel has no retention" and
// send somebody looking for a bug that is not there.
func (h *AuditHandler) CleanupOld(c *gin.Context) {
	tenantID, ok := auditTenant(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))

	plan, err := h.service.PlanRetention(c.Request.Context(), tenantID, days, panelDBUser())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusConflict, gin.H{
		"error": service.ErrRetentionIsManual.Error(),
		"plan":  plan,
	})
}

// ============================================================
// Recording from other handlers
// ============================================================

// RecordRequestAudit writes one audit entry for the request being served.
//
// It exists so that a handler wanting to record a security-relevant action does
// not have to reassemble the actor, the tenant, the address and the user agent
// from the gin context every time - four things that are easy to get subtly
// wrong, and that are worthless in an investigation if they are.
//
// A nil service is a no-op rather than a panic: a handler must keep working on
// a panel where the audit service could not be built, and the absence is
// already shouted about at start-up by whoever failed to wire it.
func RecordRequestAudit(c *gin.Context, svc *service.AuditService, action, resource string, resourceID *uuid.UUID, details models.JSONMap, status string) {
	if svc == nil || c == nil {
		return
	}

	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		// An authenticated route should always carry one. If it does not, the
		// entry still gets written rather than dropped - see
		// audit.DefaultTenantID for why silence is the worse answer.
		tenantID = audit.DefaultTenantID
	}

	var userID *uuid.UUID
	if id := middleware.GetUserID(c); id != uuid.Nil {
		userID = &id
	}

	if details == nil {
		details = models.JSONMap{}
	}
	details["path"] = c.Request.URL.Path
	details["method"] = c.Request.Method

	svc.Record(c.Request.Context(), tenantID, userID, action, resource, resourceID,
		details, c.ClientIP(), c.Request.UserAgent(), status)
}

// panelDBUser is the role name that goes into the operator's prune command.
//
// The names are the ones internal/config binds database.user to, in the same
// order, so the command that comes back names the role this panel is actually
// connected as rather than a guess. "vkai" is the installer's default and the
// config default, and is the right fallback when the process was configured
// from a file rather than the environment.
func panelDBUser() string {
	for _, name := range []string{"VKAI_DATABASE_USER", "VKAI_DB_USER", "DB_USER"} {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return "vkai"
}

// auditTenant reads the caller's tenant, and answers 401 when there is not one.
//
// It replaces `c.MustGet("tenant_id").(uuid.UUID)`, which every handler in this
// file used and which PANICS on every real request: middleware.AuthRequired
// stores tenant_id as a STRING (auth.go:42, `c.Set("tenant_id",
// claims.TenantID.String())`), so the type assertion to uuid.UUID cannot ever
// succeed. The audit endpoints have been answering 500 from gin's recovery
// middleware since they were written.
//
// middleware.GetTenantID is the accessor that reads it correctly, and it fails
// closed - uuid.Nil matches no row - so the check below turns "no tenant" into
// a 401 rather than a query that quietly returns nothing.
func auditTenant(c *gin.Context) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		c.Abort()
		return uuid.Nil, false
	}
	return tenantID, true
}
