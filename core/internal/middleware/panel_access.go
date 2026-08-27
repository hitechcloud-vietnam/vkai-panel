package middleware

// Panel access gate.
//
// The panel listens on its own port, but a port number is not a secret: it is
// the first thing a scanner finds. This guard is what turns that port into an
// actual entrance:
//
//   - the request must arrive on the pinned host name, when one is configured;
//   - the client address must be on the allow list, when one is configured;
//   - the request must either come through the secret entrance path or carry a
//     valid entrance cookie issued by this process.
//
// Anything that fails answers with a neutral 404 and nothing else. No redirect,
// no "wrong entrance" hint, no panel branding, no WWW-Authenticate: a probe must
// not be able to tell a wrong path on a panel from a host that runs no panel at
// all.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// bypassPaths are answered without any entrance check. They must stay
// unauthenticated for container orchestrators and load balancers, and they
// disclose nothing beyond up/down.
var bypassPaths = map[string]bool{
	"/api/v1/health": true,
	"/health":        true,
	"/healthz":       true,
	"/ready":         true,
	"/readyz":        true,
	"/live":          true,
	"/livez":         true,
}

// acmeChallengePrefix is answered without an entrance check.
//
// The panel does not normally see this path at all: an HTTP-01 challenge is
// fetched on port 80, which belongs to the customer websites, and the panel's
// TLS manager answers it from the catch-all webroot or from a listener it binds
// for the length of one challenge. It is bypassed here for the setups where a
// reverse proxy forwards /.well-known to the panel port anyway - in which case
// the alternative is an issuance that fails with a neutral 404 and no clue why.
//
// Nothing is disclosed by allowing it through: no route is registered under
// this prefix, so the request reaches the same 404 the guard would have
// produced, and the guard is not the thing that would have served a token.
const acmeChallengePrefix = "/.well-known/acme-challenge/"

// PanelGuardOptions configures a guard. It is deliberately decoupled from
// config.PanelAccessConfig so tests can build one directly.
type PanelGuardOptions struct {
	Enabled         bool
	EntranceEnabled bool
	Entrance        string
	AllowedIPs      []string
	TrustedProxies  []string
	Domain          string
	SessionTTL      time.Duration
	CookieName      string
	CookieSecure    bool
	Secret          string
	Logger          *zap.Logger
}

// PanelGuard enforces the panel access rules. The zero value is not usable;
// build one with NewPanelGuard.
type PanelGuard struct {
	opts       PanelGuardOptions
	entrance   string
	allowed    []*net.IPNet
	proxies    []*net.IPNet
	domain     string
	secret     []byte
	cookieName string
	ttl        time.Duration
}

// NewPanelGuardFromConfig builds the guard used by the API server.
//
// secret seeds the entrance cookie signature. Passing the panel's JWT secret
// keeps the cookie tied to the installation's existing key material; when it is
// empty a random per-process key is generated instead, which simply means every
// restart asks operators to walk through the entrance again.
func NewPanelGuardFromConfig(cfg *config.PanelAccessConfig, secret string, logger *zap.Logger) (*PanelGuard, error) {
	if cfg == nil {
		return nil, fmt.Errorf("panel access: thieu cau hinh")
	}

	return NewPanelGuard(PanelGuardOptions{
		Enabled:         cfg.Enabled,
		EntranceEnabled: cfg.EntranceEnabled,
		Entrance:        cfg.Entrance,
		AllowedIPs:      cfg.AllowedIPs,
		TrustedProxies:  cfg.TrustedProxies,
		Domain:          cfg.Domain,
		SessionTTL:      cfg.SessionTTL(),
		CookieName:      config.PanelEntranceCookie,
		CookieSecure:    cfg.TLS.Enabled,
		Secret:          secret,
		Logger:          logger,
	})
}

// NewPanelGuard validates the options and compiles the address matchers once,
// so the request path does no parsing at all.
func NewPanelGuard(opts PanelGuardOptions) (*PanelGuard, error) {
	g := &PanelGuard{
		opts:       opts,
		entrance:   config.NormalizeEntrance(opts.Entrance),
		domain:     strings.ToLower(strings.TrimSpace(opts.Domain)),
		cookieName: strings.TrimSpace(opts.CookieName),
		ttl:        opts.SessionTTL,
	}

	if g.cookieName == "" {
		g.cookieName = config.PanelEntranceCookie
	}
	if g.ttl <= 0 {
		g.ttl = config.DefaultPanelSessionTTL
	}

	if opts.Enabled && opts.EntranceEnabled {
		if err := config.ValidateEntrance(g.entrance); err != nil {
			return nil, err
		}
	}

	for _, raw := range opts.AllowedIPs {
		network, err := config.ParseIPMatcher(raw)
		if err != nil {
			return nil, fmt.Errorf("panel access: danh sach IP cho phep khong hop le: %w", err)
		}
		g.allowed = append(g.allowed, network)
	}
	for _, raw := range opts.TrustedProxies {
		network, err := config.ParseIPMatcher(raw)
		if err != nil {
			return nil, fmt.Errorf("panel access: danh sach proxy tin cay khong hop le: %w", err)
		}
		g.proxies = append(g.proxies, network)
	}

	secret := strings.TrimSpace(opts.Secret)
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("panel access: khong sinh duoc khoa cookie: %w", err)
		}
		secret = hex.EncodeToString(buf)
	}
	// The cookie key is derived, never the raw JWT secret, so a leaked cookie
	// can never be replayed as signing material anywhere else.
	sum := sha256.Sum256([]byte("vkai-panel-entrance|" + secret))
	g.secret = sum[:]

	return g, nil
}

// Entrance is the canonical entrance path, empty when the entrance is off.
func (g *PanelGuard) Entrance() string {
	if !g.opts.EntranceEnabled {
		return ""
	}
	return g.entrance
}

// Wrap installs the guard in front of any http.Handler. This is how the API
// server applies it: the check then covers every route, including the ones
// registered before the guard existed and anything mounted later.
func (g *PanelGuard) Wrap(next http.Handler) http.Handler {
	if next == nil {
		panic("panel access: Wrap called with a nil handler")
	}
	if !g.opts.Enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.check(w, r) {
			writeNeutralNotFound(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler is the same check as a gin middleware, for registration inside the
// router instead of around it. Only one of Wrap/Handler should be installed.
func (g *PanelGuard) Handler() gin.HandlerFunc {
	if !g.opts.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if !g.check(c.Writer, c.Request) {
			// Abort with a bare 404 body: no JSON envelope, because the JSON
			// envelope itself is a fingerprint of this panel.
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.Header("X-Content-Type-Options", "nosniff")
			c.String(http.StatusNotFound, "404 page not found")
			c.Abort()
			return
		}
		c.Next()
	}
}

// check applies every rule. It may rewrite r.URL.Path (stripping the entrance
// prefix) and may set the entrance cookie, so it takes the ResponseWriter.
func (g *PanelGuard) check(w http.ResponseWriter, r *http.Request) bool {
	path := normalizePath(r.URL.Path)

	// Health probes are answered before any other rule so an orchestrator never
	// needs to know the entrance.
	if bypassPaths[path] {
		return true
	}
	if isACMEChallenge(path) {
		return true
	}

	client := g.clientIP(r)

	if !g.hostAllowed(r) {
		g.deny(r, client, "host khong khop PANEL_DOMAIN")
		return false
	}

	if !g.ipAllowed(client) {
		g.deny(r, client, "IP khong nam trong PANEL_ALLOWED_IPS")
		return false
	}

	if !g.opts.EntranceEnabled || g.entrance == "" {
		return true
	}

	// Correct entrance: issue the session cookie and let the rest of the path
	// through as if the prefix had never been there, so /vkai_x/api/v1/... and
	// /api/v1/... reach the same route.
	if rest, ok := stripEntrance(path, g.entrance); ok {
		g.setEntranceCookie(w, r)
		r.URL.Path = rest
		return true
	}

	if g.validCookie(r) {
		return true
	}

	g.deny(r, client, "sai loi vao an toan")
	return false
}

// hostAllowed pins the panel to one host name when PANEL_DOMAIN is set.
func (g *PanelGuard) hostAllowed(r *http.Request) bool {
	if g.domain == "" {
		return true
	}

	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))

	return host == g.domain
}

// ipAllowed matches the resolved client address against the allow list. An
// empty list allows everyone, which is the documented default.
func (g *PanelGuard) ipAllowed(client string) bool {
	if len(g.allowed) == 0 {
		return true
	}

	ip := net.ParseIP(client)
	if ip == nil {
		// An address we cannot parse is never matched against an allow list.
		return false
	}
	for _, network := range g.allowed {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP resolves the address an access decision is made on.
//
// X-Forwarded-For is only consulted when the immediate peer is a configured
// trusted proxy; otherwise any client could hand itself an allow-listed address
// simply by sending the header. Within a trusted chain the right-most address
// that is not itself a trusted proxy is the real client.
func (g *PanelGuard) clientIP(r *http.Request) string {
	peer := remoteIP(r)

	if len(g.proxies) == 0 || !g.isTrustedProxy(peer) {
		return peer
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if candidate == "" {
				continue
			}
			if host, _, err := net.SplitHostPort(candidate); err == nil {
				candidate = host
			}
			candidate = strings.Trim(candidate, "[]")
			if net.ParseIP(candidate) == nil {
				continue
			}
			if g.isTrustedProxy(candidate) {
				continue
			}
			return candidate
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if net.ParseIP(realIP) != nil {
			return realIP
		}
	}

	return peer
}

func (g *PanelGuard) isTrustedProxy(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, network := range g.proxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// setEntranceCookie records that this browser walked through the entrance, so
// XHR calls to /api/v1/... that carry no path prefix keep working.
func (g *PanelGuard) setEntranceCookie(w http.ResponseWriter, r *http.Request) {
	expires := time.Now().Add(g.ttl)
	value := g.signSession(expires)

	http.SetCookie(w, &http.Cookie{
		Name:     g.cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(g.ttl / time.Second),
		HttpOnly: true,
		Secure:   g.opts.CookieSecure || isTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// signSession is "<unix expiry>.<hmac>". The MAC covers the expiry and the
// entrance itself, so rotating the entrance invalidates every live session.
func (g *PanelGuard) signSession(expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	return exp + "." + g.mac(exp)
}

func (g *PanelGuard) mac(exp string) string {
	m := hmac.New(sha256.New, g.secret)
	m.Write([]byte(exp))
	m.Write([]byte("|"))
	m.Write([]byte(g.entrance))
	return hex.EncodeToString(m.Sum(nil))
}

// validCookie verifies signature first and expiry second, both without leaking
// timing information about the expected MAC.
func (g *PanelGuard) validCookie(r *http.Request) bool {
	cookie, err := r.Cookie(g.cookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	exp, sig, found := strings.Cut(cookie.Value, ".")
	if !found {
		return false
	}

	if !hmac.Equal([]byte(sig), []byte(g.mac(exp))) {
		return false
	}

	seconds, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return false
	}

	return time.Now().Before(time.Unix(seconds, 0))
}

// deny logs the rejected attempt. The response itself says nothing, so the log
// is the only place the reason is recorded.
func (g *PanelGuard) deny(r *http.Request, client, reason string) {
	if g.opts.Logger == nil {
		return
	}
	g.opts.Logger.Warn("panel access denied",
		zap.String("reason", reason),
		zap.String("client_ip", client),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("host", r.Host),
		zap.String("user_agent", r.UserAgent()),
	)
}

// writeNeutralNotFound is byte-for-byte what net/http returns for an unknown
// route, so a probe cannot distinguish a guarded panel from an empty port.
func writeNeutralNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("404 page not found\n"))
}

// isACMEChallenge reports whether a path is an ACME HTTP-01 challenge fetch.
// normalizePath has already run path.Clean, which strips the trailing slash off
// the bare prefix, so both forms are matched.
func isACMEChallenge(p string) bool {
	trimmed := strings.TrimSuffix(acmeChallengePrefix, "/")
	return p == trimmed || strings.HasPrefix(p, acmeChallengePrefix)
}

// stripEntrance reports whether path is inside the entrance and returns what is
// left of it. "/vkai_x" and "/vkai_x/" both become "/".
func stripEntrance(path, entrance string) (string, bool) {
	if path == entrance {
		return "/", true
	}
	if strings.HasPrefix(path, entrance+"/") {
		rest := strings.TrimPrefix(path, entrance)
		if rest == "" {
			return "/", true
		}
		return rest, true
	}
	return "", false
}

// normalizePath resolves the request path before it is compared with the
// entrance, so neither "/vkai_x/../admin" nor "//vkai_x" nor a trailing slash
// can change which side of the comparison a request lands on. Go has already
// percent-decoded r.URL.Path by this point.
func normalizePath(raw string) string {
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return path.Clean(raw)
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.Trim(r.RemoteAddr, "[]")
	}
	return strings.Trim(host, "[]")
}

func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
