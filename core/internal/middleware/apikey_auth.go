package middleware

// API key authentication.
//
// The panel can mint API keys (handler/apikey.go, service.APIKeyService) but
// nothing in the router ever authenticates with one: every route is behind the
// JWT middleware. So this is the missing half - and it is written here, behind
// the credential guard, so that the moment an operator wires an API key onto a
// route it is brute-force protected from the first request rather than from
// the first incident.
//
// An API key is a bearer secret with a long life and no second factor, which
// makes it the most attractive credential the panel issues. It is guarded on
// the same three dimensions as the login form, with the key's fingerprint
// standing in for the account name so the key itself never reaches Redis or a
// log file.

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// APIKeyHeader is the header the panel's own clients use.
const APIKeyHeader = "X-API-Key"

// APIKeyPrincipal is what a valid key resolves to. It is a plain struct rather
// than the database model so this package does not depend on the data layer.
type APIKeyPrincipal struct {
	KeyID       uuid.UUID
	UserID      uuid.UUID
	TenantID    uuid.UUID
	Permissions []string
	RoleIDs     []string
}

// APIKeyValidator verifies a raw key. It must return an error for every
// rejection reason - unknown, expired, revoked - without distinguishing them,
// for the same reason the login form does not.
type APIKeyValidator func(ctx context.Context, rawKey string) (*APIKeyPrincipal, error)

// APIKeyAuth authenticates a request by API key.
//
// It does not rate limit by itself. Compose it with the guard, which is what
// counts the failures:
//
//	group.Use(middleware.ProtectCredentials(middleware.ScopeAPIKey, logger))
//	group.Use(middleware.APIKeyAuth(validate))
func APIKeyAuth(validate APIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := extractAPIKey(c)
		if rawKey == "" {
			// No credential was offered at all. That is not a guess, so it is
			// not counted as one: otherwise a misconfigured client with no key
			// would exhaust the budget for the address it sits behind.
			MarkAuthOutcome(c, "")
			utils.Unauthorized(c, "Invalid credentials")
			c.Abort()
			return
		}

		if validate == nil {
			MarkAuthOutcome(c, "")
			utils.ServiceUnavailable(c, "API key authentication is not configured")
			c.Abort()
			return
		}

		principal, err := validate(c.Request.Context(), rawKey)
		if err != nil || principal == nil {
			// One answer for every rejection reason. The guard in front of
			// this handler equalises the timing.
			utils.Unauthorized(c, "Invalid credentials")
			c.Abort()
			return
		}

		c.Set("user_id", principal.UserID.String())
		c.Set("tenant_id", principal.TenantID.String())
		c.Set("api_key_id", principal.KeyID.String())
		c.Set("auth_method", "api_key")
		c.Set("role_ids", principal.RoleIDs)
		c.Set("permissions", principal.Permissions)

		c.Next()
	}
}

// extractAPIKey takes the key from the dedicated header, or from an
// Authorization header using either the ApiKey scheme or Bearer.
func extractAPIKey(c *gin.Context) string {
	if key := strings.TrimSpace(c.GetHeader(APIKeyHeader)); key != "" {
		return key
	}

	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	switch strings.ToLower(parts[0]) {
	case "apikey", "api-key", "bearer":
		return strings.TrimSpace(parts[1])
	default:
		return ""
	}
}

// LogAPIKeyRejection records a rejection that happened outside the guard, for
// a caller that wants the event in the authentication log without wiring the
// full middleware.
func LogAPIKeyRejection(c *gin.Context, logger *zap.Logger, reason string) {
	DefaultAuthLogger(logger).Log(AuthEvent{
		Outcome:   AuthOutcomeFailure,
		Reason:    reason,
		IP:        AuthClientIP(c),
		Account:   CredentialFingerprint(extractAPIKey(c)),
		Scope:     ScopeAPIKey,
		Path:      c.FullPath(),
		RequestID: c.GetString("request_id"),
	})
}
