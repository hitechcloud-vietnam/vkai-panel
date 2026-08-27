package utils

import (
	"github.com/gin-gonic/gin"
)

// Context keys
const (
	ContextKeyRequestID = "request_id"
	ContextKeyUserID    = "user_id"
	ContextKeyTenantID  = "tenant_id"
	ContextKeyUserRole  = "user_role"
	ContextKeyUserName  = "user_name"
)

// GetRequestIDFromContext gets request ID from context
func GetRequestIDFromContext(c *gin.Context) string {
	if rid, exists := c.Get(ContextKeyRequestID); exists {
		return rid.(string)
	}
	return ""
}

// GetUserIDFromContext gets user ID from context
func GetUserIDFromContext(c *gin.Context) string {
	if uid, exists := c.Get(ContextKeyUserID); exists {
		return uid.(string)
	}
	return ""
}

// GetTenantIDFromContext gets tenant ID from context
func GetTenantIDFromContext(c *gin.Context) string {
	if tid, exists := c.Get(ContextKeyTenantID); exists {
		return tid.(string)
	}
	return ""
}

// GetUserRoleFromContext gets user role from context
func GetUserRoleFromContext(c *gin.Context) string {
	if role, exists := c.Get(ContextKeyUserRole); exists {
		return role.(string)
	}
	return ""
}

// GetUserNameFromContext gets user name from context
func GetUserNameFromContext(c *gin.Context) string {
	if name, exists := c.Get(ContextKeyUserName); exists {
		return name.(string)
	}
	return ""
}

// SetUserIDInContext sets user ID in context
func SetUserIDInContext(c *gin.Context, userID string) {
	c.Set(ContextKeyUserID, userID)
}

// SetTenantIDInContext sets tenant ID in context
func SetTenantIDInContext(c *gin.Context, tenantID string) {
	c.Set(ContextKeyTenantID, tenantID)
}

// SetUserRoleInContext sets user role in context
func SetUserRoleInContext(c *gin.Context, role string) {
	c.Set(ContextKeyUserRole, role)
}

// SetUserNameInContext sets user name in context
func SetUserNameInContext(c *gin.Context, name string) {
	c.Set(ContextKeyUserName, name)
}

// IsAdmin checks if user is admin
func IsAdmin(c *gin.Context) bool {
	role := GetUserRoleFromContext(c)
	return role == "admin" || role == "super_admin"
}

// IsSuperAdmin checks if user is super admin
func IsSuperAdmin(c *gin.Context) bool {
	role := GetUserRoleFromContext(c)
	return role == "super_admin"
}

// HasRole checks if user has a specific role
func HasRole(c *gin.Context, role string) bool {
	return GetUserRoleFromContext(c) == role
}

// HasAnyRole checks if user has any of the specified roles
func HasAnyRole(c *gin.Context, roles ...string) bool {
	userRole := GetUserRoleFromContext(c)
	for _, role := range roles {
		if userRole == role {
			return true
		}
	}
	return false
}
