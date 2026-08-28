package middleware

// One line of wiring for every credential-accepting endpoint.
//
// The alternative - attaching a guard to each route by hand - fails in the way
// that matters most: the route somebody adds next month is unguarded, and
// nothing reports it. So the router installs one engine-level middleware that
// looks at the resolved route and applies the right guard, and a new endpoint
// under a known path is protected the day it is written.
//
// In internal/handler/router.go, alongside the other engine middleware:
//
//	r.engine.Use(middleware.ProtectCredentialEndpoints(r.logger))
//
// Anything that is not a credential endpoint passes through without touching
// Redis, so this costs a map lookup on the ordinary request.

import (
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CredentialRoute binds a route to a credential scope. Prefix matching exists
// so that a family of routes - every path under /auth/2fa, say - is covered by
// one entry, including the members of that family that do not exist yet.
type CredentialRoute struct {
	// Path is matched against the resolved route (gin's FullPath).
	Path string
	// Prefix makes the match a prefix match rather than an exact one.
	Prefix bool
	// Scope is the credential kind.
	Scope string
}

// defaultCredentialRoutes is every path in this panel that accepts a secret
// from an unauthenticated or partly authenticated caller.
//
// Some of these routes do not exist yet: two-factor verification and password
// reset are on the roadmap, and the agent endpoints are still stubs. They are
// listed anyway. An entry for a route that does not exist costs nothing; a
// missing entry for a route that does is an unguarded credential endpoint.
var defaultCredentialRoutes = []CredentialRoute{
	// The login form.
	{Path: "/api/v1/auth/login", Scope: ScopeLogin},

	// Token refresh. A refresh token is a credential with a week of life: an
	// attacker who guesses one gets a session without ever seeing a password.
	{Path: "/api/v1/auth/refresh", Scope: ScopeRefresh},

	// Two-factor verification. A six-digit code has a million possibilities,
	// which is nothing without a limiter - and the account is already half
	// authenticated by the time the code is asked for, so this is the last
	// gate before an attacker who already has the password.
	{Path: "/api/v1/auth/2fa", Prefix: true, Scope: ScopeTwoFactor},
	{Path: "/api/v1/auth/two-factor", Prefix: true, Scope: ScopeTwoFactor},
	{Path: "/api/v1/auth/mfa", Prefix: true, Scope: ScopeTwoFactor},
	{Path: "/api/v1/auth/otp", Prefix: true, Scope: ScopeTwoFactor},
	{Path: "/api/v1/auth/backup-code", Prefix: true, Scope: ScopeTwoFactor},

	// Password reset, both halves: asking for a link (which must not disclose
	// which addresses have accounts) and redeeming the token in it (which is a
	// guessable secret that changes the password).
	{Path: "/api/v1/auth/forgot-password", Prefix: true, Scope: ScopePasswordReset},
	{Path: "/api/v1/auth/reset-password", Prefix: true, Scope: ScopePasswordReset},
	{Path: "/api/v1/auth/password-reset", Prefix: true, Scope: ScopePasswordReset},

	// Agent enrolment and heartbeat. The agent runs as root on a managed
	// server; a guessed enrolment token is a foothold on the customer's
	// machine, not just on the panel.
	{Path: "/api/v1/agent/register", Prefix: true, Scope: ScopeAgentEnrol},
	{Path: "/api/v1/agent/enroll", Prefix: true, Scope: ScopeAgentEnrol},
	{Path: "/api/v1/agent/heartbeat", Prefix: true, Scope: ScopeAgentEnrol},
	{Path: "/api/v1/nodes/register", Prefix: true, Scope: ScopeAgentEnrol},
}

// resolveCredentialScope returns the scope guarding a resolved route path.
// Exact matches win over prefix matches, and the first matching prefix in the
// table wins, so a longer, more specific entry must be listed before a shorter
// one that would also match it.
func resolveCredentialScope(routes []CredentialRoute, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	for _, route := range routes {
		if !route.Prefix && route.Path == path {
			return route.Scope, true
		}
	}
	for _, route := range routes {
		if route.Prefix && strings.HasPrefix(path, route.Path) {
			return route.Scope, true
		}
	}
	return "", false
}

// ProtectCredentialEndpoints guards every credential-accepting route in the
// panel. Extra routes may be added by a caller that registers its own.
func ProtectCredentialEndpoints(logger *zap.Logger, extra ...CredentialRoute) gin.HandlerFunc {
	routes := make([]CredentialRoute, 0, len(defaultCredentialRoutes)+len(extra))
	for _, route := range append(append([]CredentialRoute(nil), extra...), defaultCredentialRoutes...) {
		if strings.TrimSpace(route.Path) == "" || strings.TrimSpace(route.Scope) == "" {
			continue
		}
		routes = append(routes, route)
	}

	limiter := CredentialLimiter(logger)
	log := DefaultAuthLogger(logger)

	// One guard per scope, built once.
	guards := make(map[string]gin.HandlerFunc)
	guardFor := func(scope string) gin.HandlerFunc {
		if existing, ok := guards[scope]; ok {
			return existing
		}
		handler := CredentialGuard(CredentialGuardOptions{
			Scope:   scope,
			Account: DefaultAccountFor(scope),
			Guard:   limiter,
			Log:     log,
			Logger:  logger,
		})
		guards[scope] = handler
		return handler
	}

	for _, route := range routes {
		guardFor(route.Scope)
	}
	guardFor(ScopeAPIKey)

	return func(c *gin.Context) {
		if scope, ok := resolveCredentialScope(routes, c.FullPath()); ok {
			guards[scope](c)
			return
		}

		// Any route may be reached with an API key rather than a session
		// token, so the key is guarded wherever it is presented. A Bearer
		// token is left alone: that is the JWT the login flow issued, and it
		// was already counted when it was obtained.
		if carriesAPIKey(c) {
			guards[ScopeAPIKey](c)
			return
		}

		c.Next()
	}
}

func carriesAPIKey(c *gin.Context) bool {
	if strings.TrimSpace(c.GetHeader(APIKeyHeader)) != "" {
		return true
	}
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return false
	}
	scheme, _, found := strings.Cut(header, " ")
	if !found {
		return false
	}
	switch strings.ToLower(scheme) {
	case "apikey", "api-key":
		return true
	default:
		return false
	}
}
