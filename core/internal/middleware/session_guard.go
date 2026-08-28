package middleware

// Enforcing session binding on every authenticated request.
//
// This is deliberately an http.Handler wrapper rather than a gin middleware,
// and there are two reasons.
//
//  1. Coverage. A gin middleware only guards the routes registered after it,
//     on the group it was attached to. Whether a route is covered then depends
//     on where somebody put a line in the router - which is exactly the class
//     of mistake this panel has already made four times, each time with the
//     feature's own tests passing. Wrapping the engine means there is no
//     ordering in which a request reaches a handler without passing here.
//
//  2. It is where the panel already puts controls of this kind: the panel
//     access gate wraps the engine the same way, in cmd/api/main.go.
//
// The wrapper acts on exactly one kind of request: one carrying an
// Authorization: Bearer header that validates as an access token. Anything
// else - the login form, the health probes, a request presenting an API key,
// a bearer token that does not validate - passes straight through to whatever
// would have handled it, which then makes its own decision. This gate can
// refuse a request; it can never let one in.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Error codes the client sees. They are distinct because the two situations
// need different things from the person: one has to sign in again, the other
// only has to prove their password.
const (
	// SessionEndedCode means the session behind this token is over - it was
	// ended by its owner or an administrator, or the binding policy refused
	// it. The client must sign in again.
	SessionEndedCode = "SESSION_ENDED"
	// SessionReauthCode means the session moved to a different network and is
	// still allowed to read, but must prove the password before it changes
	// anything.
	SessionReauthCode = "SESSION_REAUTH_REQUIRED"
)

// ReauthenticatePath is where a client proves the password to re-bind a moved
// session. It is named here so the error the client receives can point at it.
const ReauthenticatePath = "/api/v1/sessions/current/reauthenticate"

// LogoutPath is the other route a session that has moved must still be able to
// reach. It is a literal rather than a reference to the router because this
// package is below it.
const LogoutPath = "/api/v1/auth/logout"

// reauthExempt is the short list of state-changing routes a session that has
// moved network may still call.
//
// Both are ways OUT of the situation, and a control whose escape hatch is
// itself blocked is a trap:
//
//   - Proving the password is a POST. Without this exemption a session told to
//     re-authenticate would be refused on the exact endpoint that clears the
//     flag, and the only remaining escape would be signing out - which is the
//     outcome the whole policy is designed to avoid.
//   - Signing out is a POST. An operator must always be able to end their own
//     session, and most of all from somewhere they did not expect to be.
//
// Neither grants anything: one checks a password and is behind the credential
// guard, the other only retires the caller's own token. Nothing else is
// exempt - in particular, ending an arbitrary session by id is not, because a
// moved session can reach that after proving its password.
var reauthExempt = map[string]bool{
	ReauthenticatePath: true,
	LogoutPath:         true,
}

// SessionGuardOptions is what the guard needs.
type SessionGuardOptions struct {
	// Evaluator decides. service.SessionService implements it.
	Evaluator auth.SessionEvaluator
	// JWT validates the bearer token so the guard knows which session the
	// request belongs to.
	JWT *auth.JWTManager
	// Logger is optional.
	Logger *zap.Logger
}

// BindSessions wraps a handler with session binding.
//
// A nil evaluator or a nil JWT manager returns the handler unchanged and says
// so at error level. It is a real state - a panel whose session store could not
// be opened - and the one thing it must not be is silent, because the
// difference between "binding is enforced" and "binding is not enforced" is
// invisible from the outside.
func BindSessions(next http.Handler, opts SessionGuardOptions) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	if next == nil {
		return nil
	}
	if opts.Evaluator == nil || opts.JWT == nil {
		logger.Error("session binding is NOT installed: sessions cannot be ended before their token expires, " +
			"and a stolen token is usable from any address until it does")
		return next
	}

	logger.Info("session binding installed on every authenticated request")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims, err := opts.JWT.ValidateAccessToken(token)
		if err != nil || claims == nil {
			// Not an access token, or not a valid one. The route's own
			// authentication will deal with it; refusing here would turn every
			// unauthenticated route that happens to carry a stale header into
			// an error.
			next.ServeHTTP(w, r)
			return
		}

		verdict, err := opts.Evaluator.EvaluateSession(r.Context(), auth.SessionRequest{
			TokenID:   claims.ID,
			UserID:    claims.UserID,
			TenantID:  claims.TenantID,
			ExpiresAt: expiryOf(claims),
			IP:        DefaultClientIPResolver().Resolve(r),
			UserAgent: r.UserAgent(),
			Method:    r.Method,
			Path:      r.URL.Path,
		})
		if err != nil {
			// The evaluator could not decide. It has already logged why. The
			// request proceeds: a session store that is unreachable must not
			// take the whole panel down with it.
			next.ServeHTTP(w, r)
			return
		}

		if verdict.Allow {
			next.ServeHTTP(w, r)
			return
		}

		if verdict.ReauthRequired {
			if reauthExempt[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			writeSessionError(w, r, http.StatusForbidden, SessionReauthCode,
				"This session has moved to a different network. Confirm your password to continue.",
				"POST your password to "+ReauthenticatePath+" to re-bind this session.")
			return
		}

		writeSessionError(w, r, http.StatusUnauthorized, SessionEndedCode,
			"This session is no longer valid. Sign in again.", verdict.Reason)
	})
}

// CurrentTokenID returns the jti of the access token a request was
// authenticated with, which is what identifies its session. It is empty for a
// request that was not authenticated with a JWT.
func CurrentTokenID(claims *auth.TokenClaims) string {
	if claims == nil {
		return ""
	}
	return claims.ID
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(strings.TrimSpace(scheme), "bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}

func expiryOf(claims *auth.TokenClaims) time.Time {
	if claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time
	}
	return time.Time{}
}

// writeSessionError answers in the same shape as every other error this API
// produces, so a client does not need a special case for this gate.
func writeSessionError(w http.ResponseWriter, r *http.Request, status int, code, message, details string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(utils.APIResponse{
		Success: false,
		Error: &utils.APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		RequestID: requestIDOf(r),
	})
}

func requestIDOf(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-ID")); id != "" {
		return id
	}
	return ""
}
