package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type AuthHandler struct {
	authService *service.AuthService
	logger      *zap.Logger
}

func NewAuthHandler(authService *service.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{authService: authService, logger: logger}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), models.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	}, c.ClientIP())
	if err != nil {
		utils.Unauthorized(c, err.Error())
		return
	}

	utils.Success(c, resp)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		utils.Unauthorized(c, err.Error())
		return
	}

	utils.Success(c, tokenPair)
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)

	user, err := h.authService.GetCurrentUser(c.Request.Context(), userID)
	if err != nil {
		utils.NotFound(c, "User not found")
		return
	}

	utils.Success(c, user)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	// TODO: Add token to blacklist in Redis
	utils.Success(c, gin.H{"message": "Logged out successfully"})
}
