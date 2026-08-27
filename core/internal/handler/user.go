package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type UserHandler struct {
	userService *service.UserService
	logger      *zap.Logger
}

func NewUserHandler(userService *service.UserService, logger *zap.Logger) *UserHandler {
	return &UserHandler{userService: userService, logger: logger}
}

func (h *UserHandler) Create(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	user, err := h.userService.Create(c.Request.Context(), req,
		middleware.GetTenantID(c), middleware.IsAdmin(middleware.GetClaims(c)))
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, user)
}

func (h *UserHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "User not found")
		return
	}

	utils.Success(c, user)
}

func (h *UserHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		params.Page = 1
		params.PerPage = 20
	}
	params.Normalize()

	users, total, err := h.userService.ListByTenant(c.Request.Context(), tenantID, params.Page, params.PerPage)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Paginated(c, users, total, params.Page, params.PerPage)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), middleware.GetTenantID(c), id)
	if err != nil {
		utils.NotFound(c, "User not found")
		return
	}

	var req struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Status    string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.FirstName != "" {
		user.FirstName = req.FirstName
	}
	if req.LastName != "" {
		user.LastName = req.LastName
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Status != "" {
		user.Status = req.Status
	}

	if err := h.userService.Update(c.Request.Context(), user); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	if err := h.userService.Delete(c.Request.Context(), middleware.GetTenantID(c), id); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	// A password may only be changed by its owner or by an administrator, so
	// this endpoint cannot be used to brute force a peer's current password.
	if id != middleware.GetUserID(c) && !middleware.IsAdmin(middleware.GetClaims(c)) {
		utils.Forbidden(c, "You may only change your own password")
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.userService.ChangePassword(c.Request.Context(), middleware.GetTenantID(c), id, req.OldPassword, req.NewPassword); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Password changed successfully"})
}
