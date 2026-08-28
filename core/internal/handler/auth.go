package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/twofactor"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type AuthHandler struct {
	authService *service.AuthService
	logger      *zap.Logger
}

func NewAuthHandler(authService *service.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{authService: authService, logger: logger}
}

// TwoFactorServiceOf exposes the two-factor service a handler was built with,
// so the API entry point can hand the same instance to the auth service:
//
//	authService.SetTwoFactor(handler.TwoFactorServiceOf(twoFactorHandler))
//
// It exists so that the settings routes and the login gate share one service -
// one rate limiter, one lockout counter, one audit trail - rather than two that
// disagree about an account's state.
func TwoFactorServiceOf(h *TwoFactorHandler) *twofactor.Service {
	if h == nil {
		return nil
	}
	return h.service
}

// Login is the password step. It answers with either a session or a two-factor
// challenge, never both. See service.AuthService.Login.
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
		h.failSignIn(c, err)
		return
	}

	utils.Success(c, resp)
}

// LoginTwoFactor is the second step: POST /api/v1/auth/two-factor.
//
// It takes the challenge from the password step plus one code - from an
// authenticator app or one recovery code - and returns the real token pair. It
// is unauthenticated by design: the challenge is the credential, and it is
// single use, short lived and bound to one account.
func (h *AuthHandler) LoginTwoFactor(c *gin.Context) {
	var req models.TwoFactorLoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "A challenge token and a verification code are required")
		return
	}

	resp, err := h.authService.CompleteTwoFactorLogin(c.Request.Context(), service.TwoFactorExchange{
		ChallengeToken: req.ChallengeToken,
		Code:           req.Code,
		IP:             c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
	})
	if err != nil {
		h.failSignIn(c, err)
		return
	}

	utils.Success(c, resp)
}

// failSignIn maps a sign-in failure to a status code. Every wrong-credential
// answer carries the same information: the attempt failed. Which half was
// wrong - username, password, or code - is never disclosed.
func (h *AuthHandler) failSignIn(c *gin.Context, err error) {
	var retry *service.TwoFactorRetryError
	if errors.As(err, &retry) {
		// The code was wrong and the challenge window has time left. The
		// challenge that came in was spent by the attempt, so a replacement
		// goes back with the failure - carrying the original deadline - and the
		// client asks for the code again rather than the password.
		c.JSON(http.StatusUnauthorized, utils.APIResponse{
			Success: false,
			Error: &utils.APIError{
				Code:    "TWO_FACTOR_VERIFICATION_FAILED",
				Message: "That code was not accepted. Check your authenticator app and try again.",
			},
			Data: gin.H{
				"two_factor_required":  true,
				"challenge_token":      retry.ChallengeToken,
				"challenge_expires_in": retry.ChallengeExpiresIn,
			},
			RequestID: utils.GetRequestID(c),
		})
		return
	}

	switch {
	case errors.Is(err, service.ErrTooManyAttempts):
		utils.Error(c, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, service.ErrTwoFactorUnavailable):
		// The account owes a second factor that this process cannot check.
		// That is a deployment fault, not a credential fault, and it must read
		// as one rather than as a wrong password.
		h.logger.Error("sign-in refused: two-factor gate unavailable")
		utils.ServiceUnavailable(c, err.Error())
	case errors.Is(err, service.ErrChallengeInvalid):
		c.JSON(http.StatusUnauthorized, utils.APIResponse{
			Success: false,
			Error: &utils.APIError{
				Code:    "TWO_FACTOR_CHALLENGE_INVALID",
				Message: err.Error(),
			},
			RequestID: utils.GetRequestID(c),
		})
	default:
		utils.Unauthorized(c, err.Error())
	}
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
	// Revoke the access token this request was authenticated with, plus the
	// refresh token if the client hands it back, so neither survives logout.
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&req)

	h.authService.Logout(middleware.GetClaims(c), req.RefreshToken)

	utils.Success(c, gin.H{"message": "Logged out successfully"})
}
