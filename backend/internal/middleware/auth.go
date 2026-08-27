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

		claims, err := jwtManager.ValidateToken(parts[1])
		if err != nil {
			utils.Unauthorized(c, err.Error())
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID.String())
		c.Set("tenant_id", claims.TenantID.String())
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("role_ids", claims.RoleIDs)
		c.Set("claims", claims)

		c.Next()
	}
}

func GetUserID(c *gin.Context) uuid.UUID {
	id, _ := c.Get("user_id")
	uid, _ := uuid.Parse(id.(string))
	return uid
}

func GetTenantID(c *gin.Context) uuid.UUID {
	id, _ := c.Get("tenant_id")
	uid, _ := uuid.Parse(id.(string))
	return uid
}

func GetClaims(c *gin.Context) *auth.TokenClaims {
	claims, _ := c.Get("claims")
	return claims.(*auth.TokenClaims)
}
