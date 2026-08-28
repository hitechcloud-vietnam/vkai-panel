package middleware

// Scope enforcement for API keys.
//
// An endpoint declares what it needs:
//
//	integration.GET("/websites", middleware.RequireScope("website"), h.List)
//	integration.DELETE("/databases/:id", middleware.RequireScope("database"), h.Delete)
//
// RequireScope derives the action from the HTTP method - GET, HEAD and OPTIONS
// are reads, everything else is a write - which is the same mapping
// RequirePermission uses, so a route that is a read for RBAC is a read for
// scopes. Where that mapping is wrong (a POST that is really a search, an
// endpoint dangerous enough to deserve its own answer) RequireScopeAction
// states the action outright.
//
// Two checks run here, not one:
//
//  1. The KEY's scopes must cover the demand. This is the new half.
//  2. The OWNER's RBAC permissions must cover it as well. A key is issued by a
//     person and acts as that person; it must not become a way to exceed them.
//     If the account loses a permission, every key it issued loses it in the
//     same instant, without anyone having to remember to re-scope the keys.
//
// Both are deny-by-default. A request with no API key context does not reach a
// scoped route at all, and a scope set that is empty or does not parse
// authorises nothing.

import (
	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Context keys set by APIKeyAuth and read here.
const (
	// APIKeyScopesKey holds the []string scopes of the authenticated key.
	APIKeyScopesKey = "api_key_scopes"
	// APIKeyIDKey holds the key's identifier.
	APIKeyIDKey = "api_key_id"
	// AuthMethodKey says how the request was authenticated: "api_key" for a
	// key, absent for a session token.
	AuthMethodKey = "auth_method"
)

// RequireScope guards a route with "<module>:<read|write>", chosen by method.
func RequireScope(module string) gin.HandlerFunc {
	return requireScope(module, "")
}

// RequireScopeAction guards a route with one fixed action regardless of method.
func RequireScopeAction(module, action string) gin.HandlerFunc {
	return requireScope(module, action)
}

func requireScope(module, fixedAction string) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := fixedAction
		if action == "" {
			action = auth.ActionForMethod(c.Request.Method)
		}

		raw, present := APIKeyScopes(c)
		if !present {
			// The route is scope-guarded and the request did not arrive with a
			// key. That is a wiring mistake or a caller using the wrong
			// credential; either way the request does not proceed.
			utils.Unauthorized(c, "This endpoint requires an API key")
			c.Abort()
			return
		}

		scopes, dropped := auth.ParseScopeSetLenient(raw)
		required := auth.Scope{Module: module, Action: action}

		if !scopes.Allows(GetTenantID(c), auth.Access{
			Tenant: GetTenantID(c),
			Module: module,
			Action: action,
		}) {
			details := "this key holds: " + joinOrNone(scopes.Strings())
			if len(dropped) > 0 {
				details += "; ignored (unparseable): " + joinOrNone(dropped)
			}
			c.JSON(403, utils.APIResponse{
				Success: false,
				Error: &utils.APIError{
					Code:    "SCOPE_REQUIRED",
					Message: "This API key is not scoped for " + required.String(),
					Details: details,
				},
				RequestID: utils.GetRequestID(c),
			})
			c.Abort()
			return
		}

		// The key is scoped for it. The account behind the key must be
		// entitled to it too.
		if EnforceRBAC() && !HasPermission(ownerClaims(c), module, action) {
			c.JSON(403, utils.APIResponse{
				Success: false,
				Error: &utils.APIError{
					Code: "FORBIDDEN",
					Message: "The account this API key belongs to does not hold " +
						auth.PermissionForScope(module, action),
				},
				RequestID: utils.GetRequestID(c),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// APIKeyScopes returns the scopes of the API key this request was
// authenticated with, and whether the request was authenticated with one at
// all. The second return value matters: no key is not the same as a key with
// no scopes, and both are refused, but only one of them is a caller mistake.
func APIKeyScopes(c *gin.Context) ([]string, bool) {
	value, exists := c.Get(APIKeyScopesKey)
	if !exists {
		return nil, false
	}
	scopes, ok := value.([]string)
	if !ok {
		return nil, false
	}
	return scopes, true
}

// ownerClaims rebuilds just enough of a token's authorisation for the RBAC
// helpers, from what APIKeyAuth put in the context.
//
// It is built here and not stored in the context on purpose: a synthetic
// claims value sitting under the "claims" key would be indistinguishable from
// a real session token to every other piece of code that reads it.
func ownerClaims(c *gin.Context) *auth.TokenClaims {
	claims := &auth.TokenClaims{}
	if roles, ok := c.Get("role_ids"); ok {
		if list, ok := roles.([]string); ok {
			claims.RoleIDs = list
		}
	}
	if perms, ok := c.Get("permissions"); ok {
		if list, ok := perms.([]string); ok {
			claims.Permissions = list
		}
	}
	return claims
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(nothing)"
	}
	out := values[0]
	for _, v := range values[1:] {
		out += ", " + v
	}
	return out
}
