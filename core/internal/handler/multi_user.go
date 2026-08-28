package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type MultiUserHandler struct {
	service *service.MultiUserService
	audit   *service.AuditService
	logger  *zap.Logger
}

func NewMultiUserHandler(service *service.MultiUserService, logger *zap.Logger) *MultiUserHandler {
	return &MultiUserHandler{service: service, logger: logger}
}

// SetAudit installs the audit trail. Every change to who may do what goes into
// it: a permission change is the action that makes every other action possible,
// so a trail that records what an account did but not how it came to be allowed
// to answers only half the question.
//
// It is a setter rather than a constructor argument so that adding it did not
// change NewMultiUserHandler's signature, which the API entry point passes
// positionally.
func (h *MultiUserHandler) SetAudit(a *service.AuditService) { h.audit = a }

// ============================================================
// ROLES
// ============================================================

func (h *MultiUserHandler) CreateRole(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	var req models.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role, err := h.service.CreateRole(c.Request.Context(), tenantID, req)
	if err != nil {
		RecordRequestAudit(c, h.audit, audit.ActionRoleCreated, audit.ResourceRole, nil,
			models.JSONMap{"name": req.Name, "error": err.Error()}, audit.StatusFailure)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	RecordRequestAudit(c, h.audit, audit.ActionRoleCreated, audit.ResourceRole, &role.ID,
		models.JSONMap{"name": role.Name, "description": role.Description}, audit.StatusSuccess)
	c.JSON(http.StatusCreated, gin.H{"role": role})
}

func (h *MultiUserHandler) ListRoles(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	roles, err := h.service.ListRoles(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

func (h *MultiUserHandler) GetRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	role, err := h.service.GetRole(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": role})
}

func (h *MultiUserHandler) UpdateRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var req models.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role, err := h.service.UpdateRole(c.Request.Context(), id, req)
	if err != nil {
		RecordRequestAudit(c, h.audit, audit.ActionRoleUpdated, audit.ResourceRole, &id,
			models.JSONMap{"error": err.Error()}, audit.StatusFailure)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	RecordRequestAudit(c, h.audit, audit.ActionRoleUpdated, audit.ResourceRole, &id,
		models.JSONMap{"name": role.Name, "description": role.Description}, audit.StatusSuccess)
	c.JSON(http.StatusOK, gin.H{"role": role})
}

func (h *MultiUserHandler) DeleteRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	if err := h.service.DeleteRole(c.Request.Context(), id); err != nil {
		RecordRequestAudit(c, h.audit, audit.ActionRoleDeleted, audit.ResourceRole, &id,
			models.JSONMap{"error": err.Error()}, audit.StatusFailure)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	RecordRequestAudit(c, h.audit, audit.ActionRoleDeleted, audit.ResourceRole, &id, nil,
		audit.StatusSuccess)
	c.JSON(http.StatusOK, gin.H{"message": "Role deleted"})
}

// ============================================================
// PERMISSIONS
// ============================================================

func (h *MultiUserHandler) ListPermissions(c *gin.Context) {
	perms, err := h.service.ListPermissions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": perms})
}

// ============================================================
// USER-ROLE ASSIGNMENT
// ============================================================

func (h *MultiUserHandler) AssignUserRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req models.AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.AssignUserRole(c.Request.Context(), userID, req.RoleID); err != nil {
		// A refused grant is recorded with the same detail as a successful
		// one. An attempt to widen somebody's privileges that did not work is
		// still an attempt, and a trail that holds only successes is a trail
		// that hides the reconnaissance.
		RecordRequestAudit(c, h.audit, audit.ActionRoleAssigned, audit.ResourceUser, &userID,
			models.JSONMap{
				"role_id":        req.RoleID.String(),
				"target_user_id": userID.String(),
				"error":          err.Error(),
			}, audit.StatusFailure)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	RecordRequestAudit(c, h.audit, audit.ActionRoleAssigned, audit.ResourceUser, &userID,
		models.JSONMap{"role_id": req.RoleID.String(), "target_user_id": userID.String()},
		audit.StatusSuccess)
	c.JSON(http.StatusOK, gin.H{"message": "Role assigned"})
}

func (h *MultiUserHandler) RemoveUserRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	if err := h.service.RemoveUserRole(c.Request.Context(), userID, roleID); err != nil {
		RecordRequestAudit(c, h.audit, audit.ActionRoleRemoved, audit.ResourceUser, &userID,
			models.JSONMap{
				"role_id":        roleID.String(),
				"target_user_id": userID.String(),
				"error":          err.Error(),
			}, audit.StatusFailure)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	RecordRequestAudit(c, h.audit, audit.ActionRoleRemoved, audit.ResourceUser, &userID,
		models.JSONMap{"role_id": roleID.String(), "target_user_id": userID.String()},
		audit.StatusSuccess)
	c.JSON(http.StatusOK, gin.H{"message": "Role removed"})
}

func (h *MultiUserHandler) GetUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	roles, err := h.service.GetUserRoles(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

func (h *MultiUserHandler) GetUserPermissions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	perms, err := h.service.GetUserPermissions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": perms})
}

// ============================================================
// SESSIONS
// ============================================================

func (h *MultiUserHandler) ListActiveSessions(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	sessions, err := h.service.ListActiveSessions(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *MultiUserHandler) DeleteSession(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	if err := h.service.DeleteSession(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Session terminated"})
}

func (h *MultiUserHandler) TerminateUserSessions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	if err := h.service.DeleteUserSessions(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "All sessions terminated"})
}

// ============================================================
// ACTIVITY LOG
// ============================================================

func (h *MultiUserHandler) ListActivities(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	var userID *uuid.UUID
	if uid := c.Query("user_id"); uid != "" {
		parsed, err := uuid.Parse(uid)
		if err == nil {
			userID = &parsed
		}
	}

	activities, err := h.service.ListActivities(c.Request.Context(), tenantID, userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"activities": activities})
}

// ============================================================
// API KEYS
// ============================================================

func (h *MultiUserHandler) CreateAPIKey(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req models.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiKey, rawKey, err := h.service.CreateAPIKey(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"api_key": apiKey, "key": rawKey})
}

func (h *MultiUserHandler) ListAPIKeys(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keys, err := h.service.ListAPIKeys(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": keys})
}

func (h *MultiUserHandler) DeleteAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}

	if err := h.service.DeleteAPIKey(c.Request.Context(), id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API key deleted"})
}

// ============================================================
// STATS
// ============================================================

func (h *MultiUserHandler) GetStats(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	stats, err := h.service.GetStats(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
