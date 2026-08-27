package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Authorization is deny-by-default: a route guarded by one of the helpers below
// is only reachable by a caller whose token carries the matching permission (or
// an administrative role). Permissions and roles are put into the token at
// login time by AuthService.loadAuthorization.

// adminRoles bypass individual permission checks. The names cover both the
// migrations that seed "super_admin" and the ones that seed "Super Admin".
var adminRoles = map[string]bool{
	"super_admin":    true,
	"super admin":    true,
	"superadmin":     true,
	"admin":          true,
	"platform_admin": true,
}

// EnforceRBAC reports whether authorization checks are active. It is on unless
// an operator explicitly disables it, which exists only as a rollout escape
// hatch for an installation whose role assignments have not been migrated yet.
func EnforceRBAC() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("VKAI_RBAC_ENFORCE"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// claimsFrom returns the validated claims attached by AuthRequired.
func claimsFrom(c *gin.Context) *auth.TokenClaims {
	value, exists := c.Get("claims")
	if !exists {
		return nil
	}
	claims, ok := value.(*auth.TokenClaims)
	if !ok {
		return nil
	}
	return claims
}

// HasRole reports whether the caller holds one of the named roles.
func HasRole(claims *auth.TokenClaims, roles ...string) bool {
	if claims == nil {
		return false
	}
	want := make(map[string]bool, len(roles))
	for _, r := range roles {
		want[strings.ToLower(r)] = true
	}
	for _, held := range claims.RoleIDs {
		if want[strings.ToLower(held)] {
			return true
		}
	}
	return false
}

// IsAdmin reports whether the caller holds an administrative role.
func IsAdmin(claims *auth.TokenClaims) bool {
	if claims == nil {
		return false
	}
	for _, held := range claims.RoleIDs {
		if adminRoles[strings.ToLower(held)] {
			return true
		}
	}
	return false
}

// HasPermission reports whether the caller holds "resource.action". A caller
// holding "resource.*" or an admin role satisfies any action on that resource.
func HasPermission(claims *auth.TokenClaims, resource, action string) bool {
	if claims == nil {
		return false
	}
	if IsAdmin(claims) {
		return true
	}

	want := strings.ToLower(resource + "." + action)
	wildcard := strings.ToLower(resource + ".*")

	for _, held := range claims.Permissions {
		held = strings.ToLower(strings.TrimSpace(held))
		if held == want || held == wildcard || held == "*" {
			return true
		}
	}
	return false
}

// actionForMethod maps an HTTP method onto the permission action it needs, so a
// single group guard covers both reads and writes correctly.
func actionForMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return "read"
	default:
		return "write"
	}
}

// RequirePermission guards a route group. GET/HEAD need "<resource>.read";
// every mutating method needs "<resource>.write".
func RequirePermission(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := claimsFrom(c)
		if claims == nil {
			utils.Unauthorized(c, "Authentication required")
			c.Abort()
			return
		}

		if !EnforceRBAC() {
			c.Next()
			return
		}

		if !HasPermission(claims, resource, actionForMethod(c.Request.Method)) {
			utils.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireExactPermission guards a route with one fixed permission regardless of
// the HTTP method. Used where a POST is really a read, or where an endpoint is
// dangerous enough to warrant its own permission.
func RequireExactPermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := claimsFrom(c)
		if claims == nil {
			utils.Unauthorized(c, "Authentication required")
			c.Abort()
			return
		}

		if !EnforceRBAC() {
			c.Next()
			return
		}

		if !HasPermission(claims, resource, action) {
			utils.Forbidden(c, "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin restricts a route group to administrative roles. Used for the
// endpoints that cross tenant boundaries or reconfigure the host.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := claimsFrom(c)
		if claims == nil {
			utils.Unauthorized(c, "Authentication required")
			c.Abort()
			return
		}

		if !EnforceRBAC() {
			c.Next()
			return
		}

		if !IsAdmin(claims) {
			utils.Forbidden(c, "Administrator role required")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireRole restricts a route group to a named set of roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := claimsFrom(c)
		if claims == nil {
			utils.Unauthorized(c, "Authentication required")
			c.Abort()
			return
		}

		if !EnforceRBAC() {
			c.Next()
			return
		}

		if !IsAdmin(claims) && !HasRole(claims, roles...) {
			utils.Forbidden(c, "Insufficient role")
			c.Abort()
			return
		}

		c.Next()
	}
}
