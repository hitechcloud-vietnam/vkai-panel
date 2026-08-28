package middleware

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// AllowedOrigins returns the browser origins permitted to call the API. It is
// read from VKAI_CORS_ALLOWED_ORIGINS (comma separated). There is deliberately
// no wildcard fallback.
func AllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("VKAI_CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(o), "/"))
		if o == "" || o == "*" {
			// A wildcard is never honoured: it would defeat the whole check.
			continue
		}
		if u, err := url.Parse(o); err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		origins = append(origins, o)
	}
	return origins
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.String("request_id", c.GetString("request_id")),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if userID, exists := c.Get("user_id"); exists {
			fields = append(fields, zap.String("user_id", userID.(string)))
		}

		if tenantID, exists := c.Get("tenant_id"); exists {
			fields = append(fields, zap.String("tenant_id", tenantID.(string)))
		}

		switch {
		case status >= 500:
			logger.Error("request completed", fields...)
		case status >= 400:
			logger.Warn("request completed", fields...)
		default:
			logger.Info("request completed", fields...)
		}
	}
}

// CORS reflects only origins on the configured allowlist. "*" is never sent:
// with credentials, or after a future switch to cookie auth, a wildcard would
// hand every website on the internet an authenticated channel into the panel.
func CORS(allowedOrigins ...string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		allowedOrigins = AllowedOrigins()
	}

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSuffix(strings.TrimSpace(o), "/")
		if o != "" && o != "*" {
			allowed[o] = true
		}
	}

	return func(c *gin.Context) {
		origin := strings.TrimSuffix(c.GetHeader("Origin"), "/")

		c.Header("Vary", "Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-Tenant-ID")
			c.Header("Access-Control-Expose-Headers", "X-Request-ID")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			if origin != "" && !allowed[origin] {
				c.AbortWithStatus(403)
				return
			}
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("request_id", c.GetString("request_id")),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(500, gin.H{
					"success":    false,
					"error":      gin.H{"code": "INTERNAL_ERROR", "message": "An internal error occurred"},
					"request_id": c.GetString("request_id"),
				})
			}
		}()
		c.Next()
	}
}

// rateLimiter is a fixed-window counter keyed by client IP and route. It is
// in-process; a multi-instance deployment should front it with a shared limiter,
// but an in-process limiter is still what stops a single host being brute
// forced at line rate.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	limit   int
	window  time.Duration
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		windows: make(map[string]*rateWindow),
		limit:   limit,
		window:  window,
	}
}

// allow records a hit and reports whether it is within the limit.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Drop expired windows so the map cannot grow without bound.
	if len(rl.windows) > 10000 {
		for k, w := range rl.windows {
			if now.After(w.resetAt) {
				delete(rl.windows, k)
			}
		}
	}

	w, ok := rl.windows[key]
	if !ok || now.After(w.resetAt) {
		rl.windows[key] = &rateWindow{count: 1, resetAt: now.Add(rl.window)}
		return true
	}

	w.count++
	return w.count <= rl.limit
}

// RateLimit applies a global per-IP limit to every request.
func RateLimit() gin.HandlerFunc {
	return RateLimitWith(600, time.Minute)
}

// RateLimitWith builds a limiter with an explicit budget. Use a tight budget on
// authentication routes and a loose one everywhere else.
func RateLimitWith(limit int, window time.Duration) gin.HandlerFunc {
	rl := newRateLimiter(limit, window)

	return func(c *gin.Context) {
		// AuthClientIP, not c.ClientIP: SetTrustedProxies is never called on
		// this engine, so gin honours whatever X-Forwarded-For the caller
		// sends. A limiter keyed on a value the caller chooses counts to one
		// forever.
		key := AuthClientIP(c) + "|" + c.FullPath()
		if !rl.allow(key) {
			utils.RateLimit(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// AuthRateLimit is the original strict budget for login and refresh: five
// attempts per address per fifteen minutes, counted in this process.
//
// Nothing installs it. internal/handler/router.go used to put it on the /auth
// group; it was removed when ProtectCredentialEndpoints was mounted on the
// engine, because two limiters on one path do not add up to a policy - the
// effective limit becomes whichever is tighter by accident, and here that was
// always this one, so the layered limiter's behaviour would never have been
// reached. The credential endpoints are defended by
// ProtectCredentialEndpoints and by that alone.
//
// It is kept only so an operator running a deployment without the counter
// store can put a crude ceiling on a route by hand. It has the three problems
// a single counter always has:
//
//   - it is per process, so a two-instance deployment allows ten attempts, not
//     five, and neither instance can see the other's;
//   - it is a hard cutoff, which hands an attacker a denial of service against
//     any address they can make requests from;
//   - it counts requests rather than failures, so a busy legitimate user is
//     charged for their successes.
//
// New credential endpoints should not reach for it: they are covered by the
// route table in credential_routes.go, and a new path under a family already
// listed there is guarded the day it is written.
func AuthRateLimit() gin.HandlerFunc {
	return RateLimitWith(5, 15*time.Minute)
}

// SecurityHeaders sets the baseline response headers for the panel. HSTS is
// only emitted over a connection that is actually TLS, so a development
// deployment on plain HTTP is not permanently pinned in the operator's browser.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		// The API returns JSON, so it needs no script or style sources at all.
		c.Header("Content-Security-Policy",
			"default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")

		if isTLSRequest(c) {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

func isTLSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
