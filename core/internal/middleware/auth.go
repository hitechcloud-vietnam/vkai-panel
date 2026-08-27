package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

func AuthRequired(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "Authorization header required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			utils.Unauthorized(c, "Invalid authorization header format")
			c.Abort()
			return
		}

		// Only an access token is accepted here: a refresh token presented as a
		// bearer credential is rejected both by its "typ" claim and by being
		// signed with a different key.
		claims, err := jwtManager.ValidateAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			// The specific reason is not echoed back to an unauthenticated
			// caller.
			utils.Unauthorized(c, "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID.String())
		c.Set("tenant_id", claims.TenantID.String())
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("role_ids", claims.RoleIDs)
		c.Set("permissions", claims.Permissions)
		c.Set("claims", claims)

		c.Next()
	}
}

// GetUserID returns the authenticated user's id, or the zero UUID when the
// request was not authenticated. It never panics on a missing or mistyped
// context value.
func GetUserID(c *gin.Context) uuid.UUID {
	value, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil
	}
	str, ok := value.(string)
	if !ok {
		return uuid.Nil
	}
	id, err := uuid.Parse(str)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// GetTenantID returns the authenticated caller's tenant, or the zero UUID.
// Every tenant-scoped query uses this, so returning uuid.Nil on a missing value
// fails closed: it matches no row.
func GetTenantID(c *gin.Context) uuid.UUID {
	value, exists := c.Get("tenant_id")
	if !exists {
		return uuid.Nil
	}
	str, ok := value.(string)
	if !ok {
		return uuid.Nil
	}
	id, err := uuid.Parse(str)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// GetClaims returns the validated token claims, or nil when absent.
func GetClaims(c *gin.Context) *auth.TokenClaims {
	return claimsFrom(c)
}
