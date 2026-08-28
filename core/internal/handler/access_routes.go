package handler

// ============================================================================
//
//	THE ONE LINE THAT MOUNTS THIS FEATURE
//
//	In internal/handler/router.go, at the end of (*Router).Setup(), beside
//	RegisterAgentPKIRoutes:
//
//	        r.RegisterAccessRoutes()
//
//	Without that line the scoped API key surface, key rotation, key
//	revocation and the whole session management API are not reachable. They
//	compile, their tests pass, and no request can get to them - which is the
//	exact failure this codebase has had four times over (two-factor, agent
//	mTLS, the credential limiter, panel TLS), every time with green tests.
//
//	The call is idempotent: it looks at the route table before registering
//	anything, so it can be called from cmd/api/main.go as well - which it is
//	today - and adding the line above will not produce a duplicate-route
//	panic. Once the line is in the router, the call in main.go can go.
//
// ============================================================================
//
// Why a separate file with its own registration function rather than lines in
// router.go: router.go is owned elsewhere, and this feature needs three route
// families with three different authentications - a session token, an API key,
// and an administrator - which do not fit in one group.

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

// Route prefixes, named so the tests and the idempotence check refer to the
// same strings the registration uses.
const (
	// IntegrationPrefix is the API-key-authenticated surface. It is a separate
	// prefix rather than the same paths as the session-authenticated API
	// because the two have different authentication and different
	// authorisation, and a route that accepts either credential has to be
	// correct for both.
	IntegrationPrefix = "/integration"
	// SessionsPrefix is the operator's own session management.
	SessionsPrefix = "/sessions"
)

// sessionsProbePath is what the idempotence check looks for.
const sessionsProbePath = "/api/v1" + SessionsPrefix

// accessControl is the wiring the API entry point supplies, for the parts of
// this feature the Router does not already hold.
//
// It is a package-level value set once at start-up, in the same spirit as
// middleware.SetCredentialLimiter: NewRouter is called positionally from
// cmd/api/main.go, so a new parameter would break that call, and this feature
// needs a session handler the Router has no field for.
type accessControlWiring struct {
	sessions *SessionHandler
}

var accessControl accessControlWiring

// SetSessionHandler gives the router the session handler to mount.
//
// Call it before Setup(). Without it the session routes are still registered
// and answer 503 with the reason, because a route that answers 404 reads as
// "this panel has no such feature", which is how an operator concludes their
// account does not need one.
func SetSessionHandler(h *SessionHandler) {
	accessControl.sessions = h
}

// sourceAddressKey carries the resolved client address into the API key
// validator, which is given a context rather than the request.
type sourceAddressKey struct{}

// SourceAddressFrom returns the address a keyed request came from.
func SourceAddressFrom(ctx context.Context) string {
	if value, ok := ctx.Value(sourceAddressKey{}).(string); ok {
		return value
	}
	return ""
}

// RegisterAccessRoutes mounts the scoped API key surface, key rotation and
// revocation, and session management.
//
// It is safe to call more than once: gin panics on a duplicate route, and a
// panel must not fail to start over where a wiring line ended up.
func (r *Router) RegisterAccessRoutes() {
	if r == nil || r.engine == nil {
		return
	}

	logger := r.logger
	if logger == nil {
		logger = zap.NewNop()
	}

	for _, route := range r.engine.Routes() {
		if route.Path == sessionsProbePath {
			logger.Debug("access routes already registered")
			return
		}
	}

	v1 := r.engine.Group("/api/v1")

	sessions := accessControl.sessions
	if sessions == nil {
		// A handler with no service answers 503 on every route, which keeps
		// the route table identical whether or not the entry point wired a
		// session store. A missing route is indistinguishable from a feature
		// that was never written.
		sessions = NewSessionHandler(nil, logger)
	}

	r.registerSessionRoutes(v1, sessions, logger)
	r.registerKeyManagementRoutes(v1)
	r.registerIntegrationRoutes(v1, logger)

	logger.Info("access routes mounted",
		zap.String("integration", "/api/v1"+IntegrationPrefix),
		zap.String("sessions", sessionsProbePath),
		zap.Bool("session_binding_enforced", sessions.Service().Enforcing()),
		zap.Bool("api_keys_available", r.apiKeyHandler.Service().Available()))
}

// registerSessionRoutes mounts the operator's own session management, behind a
// session token.
func (r *Router) registerSessionRoutes(v1 *gin.RouterGroup, sessions *SessionHandler, logger *zap.Logger) {
	group := v1.Group(SessionsPrefix)
	group.Use(middleware.AuthRequired(r.jwtManager))

	// No permission check: these are the caller's own sessions, and an
	// operator with no permissions at all must still be able to see where they
	// are signed in and sign themselves out.
	group.GET("", sessions.List)
	group.DELETE("/:id", sessions.Terminate)

	// Proving the password to re-bind a moved session is a credential
	// endpoint, so it is counted like one. The account is taken from the
	// session token rather than from the body: the caller is already
	// authenticated, and the thing being guessed is their password.
	group.POST("/current/reauthenticate",
		middleware.CredentialGuard(middleware.CredentialGuardOptions{
			Scope:   middleware.ScopeLogin,
			Account: func(c *gin.Context) string { return middleware.GetUserID(c).String() },
			Guard:   middleware.CredentialLimiter(logger),
			Log:     middleware.DefaultAuthLogger(logger),
			Logger:  logger,
		}),
		sessions.Reauthenticate)

	// Ending every session on somebody else's account is an administrator's
	// action.
	group.DELETE("/user/:id", middleware.RequireAdmin(), sessions.TerminateAllForUser)
}

// registerKeyManagementRoutes mounts the API key operations that router.go
// does not already mount. Create, list, get, update and delete are already
// there under /api/v1/api-keys and reach the same service.
func (r *Router) registerKeyManagementRoutes(v1 *gin.RouterGroup) {
	keys := v1.Group("/api-keys")
	keys.Use(middleware.AuthRequired(r.jwtManager), middleware.RequirePermission("user"))
	{
		keys.POST("/:id/rotate", r.apiKeyHandler.Rotate)
		keys.POST("/:id/revoke", r.apiKeyHandler.Revoke)
	}

	// The scope catalogue, so an interface can offer a scope picker rather
	// than a free text box. Its own prefix, because /api-keys/:id is already a
	// parameter route and a sibling literal there is a trap for the next
	// person adding one.
	access := v1.Group("/access")
	access.Use(middleware.AuthRequired(r.jwtManager))
	access.GET("/scopes", r.apiKeyHandler.Scopes)
}

// registerIntegrationRoutes mounts the surface an API key can reach.
//
// Every route here declares the module it belongs to with RequireScope, which
// checks the key's scopes AND the RBAC permissions of the account the key
// belongs to. A route added here without a RequireScope is authenticated but
// unscoped - that is what TestIntegrationRoutesAllDeclareAScope exists to
// catch.
func (r *Router) registerIntegrationRoutes(v1 *gin.RouterGroup, logger *zap.Logger) {
	group := v1.Group(IntegrationPrefix)
	group.Use(withSourceAddress(), middleware.APIKeyAuth(r.apiKeyValidator(logger)))

	// What this key is and what it may do. No module scope: an integration
	// being refused has to be able to find out why without an operator reading
	// the panel's logs.
	group.GET("/whoami", r.apiKeyHandler.WhoAmI)

	websites := group.Group("/websites", middleware.RequireScope("website"))
	{
		websites.GET("", r.websiteHandler.List)
		websites.GET("/:id", r.websiteHandler.Get)
		websites.POST("", r.websiteHandler.Create)
		websites.PUT("/:id", r.websiteHandler.Update)
		websites.DELETE("/:id", r.websiteHandler.Delete)
		websites.GET("/:id/domains", r.websiteHandler.ListDomains)
	}

	databases := group.Group("/databases", middleware.RequireScope("database"))
	{
		databases.GET("", r.databaseHandler.ListDatabases)
		databases.POST("", r.databaseHandler.CreateDatabase)
		databases.DELETE("/:id", r.databaseHandler.DeleteDatabase)
		databases.GET("/servers", r.databaseHandler.ListServers)
	}

	dns := group.Group("/dns", middleware.RequireScope("dns"))
	{
		dns.GET("/zones", r.dnsHandler.ListZones)
		dns.GET("/zones/:id", r.dnsHandler.GetZone)
		dns.POST("/zones", r.dnsHandler.CreateZone)
		dns.DELETE("/zones/:id", r.dnsHandler.DeleteZone)
		dns.GET("/zones/:id/records", r.dnsHandler.ListRecords)
		dns.POST("/zones/:id/records", r.dnsHandler.CreateRecord)
	}

	ssl := group.Group("/ssl", middleware.RequireScope("ssl"))
	{
		ssl.GET("", r.sslHandler.List)
		ssl.GET("/expiring", r.sslHandler.GetExpiringSoon)
		ssl.GET("/:id", r.sslHandler.Get)
	}
}

// apiKeyValidator turns a presented key into a principal for the middleware.
//
// It is a closure over the one API key service this process has, so a key
// authenticated on a request and a key managed through /api/v1/api-keys are
// the same key with the same rules - rather than two implementations that
// agree until one of them is changed.
func (r *Router) apiKeyValidator(logger *zap.Logger) middleware.APIKeyValidator {
	return func(ctx context.Context, rawKey string) (*middleware.APIKeyPrincipal, error) {
		svc := r.apiKeyHandler.Service()
		if svc == nil || !svc.Available() {
			logger.Error("an API key was presented but API key authentication is not available on this panel")
			return nil, errors.New("API key authentication is unavailable")
		}

		principal, err := svc.Authenticate(ctx, rawKey, SourceAddressFrom(ctx))
		if err != nil {
			return nil, err
		}

		return &middleware.APIKeyPrincipal{
			KeyID:       principal.KeyID,
			UserID:      principal.UserID,
			TenantID:    principal.TenantID,
			Permissions: principal.Permissions,
			RoleIDs:     principal.RoleIDs,
			Scopes:      principal.Scopes.Strings(),
		}, nil
	}
}

// withSourceAddress puts the resolved client address into the request context,
// because the API key validator is handed a context and not a request, and a
// key pinned to a network has to know where the request came from.
//
// The address is the one middleware.AuthClientIP resolves - the peer, or a
// forwarded header from a proxy the operator has explicitly named. Never a
// header taken on trust.
func withSourceAddress() gin.HandlerFunc {
	return func(c *gin.Context) {
		address := middleware.AuthClientIP(c)
		if address != "" {
			c.Request = c.Request.WithContext(
				context.WithValue(c.Request.Context(), sourceAddressKey{}, address))
		}
		c.Next()
	}
}
