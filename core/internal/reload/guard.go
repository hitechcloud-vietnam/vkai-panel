package reload

// The access gate, swapped whole.
//
// middleware.PanelGuard compiles its entrance, its address matchers and its
// cookie key once, at construction, and never mutates them. That immutability
// is a feature, not an obstacle: it means a configuration change is applied by
// building an entirely new guard and publishing it with a single atomic store.
//
// This is what makes requirement "atomic from a reader's point of view" true
// rather than merely intended. A request reads the pointer once, at its start,
// and is then checked against one guard - one entrance, one allow list, one
// pinned domain, all from the same configuration. There is no window in which a
// request meets the new allow list and the old entrance, because there is no
// moment at which such a combination exists.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

// GuardSwitch is the panel's front door: an http.Handler that delegates to the
// current generation of the access gate.
type GuardSwitch struct {
	secret  string
	logger  *zap.Logger
	backend http.Handler

	live atomic.Pointer[guardGeneration]
}

// guardGeneration is one immutable (configuration, guard, wrapped handler).
type guardGeneration struct {
	cfg     *config.PanelAccessConfig
	guard   *middleware.PanelGuard
	handler http.Handler
}

// NewGuardSwitch builds the first generation of the gate around the handler the
// panel serves.
func NewGuardSwitch(cfg *config.PanelAccessConfig, secret string, backend http.Handler, logger *zap.Logger) (*GuardSwitch, error) {
	if backend == nil {
		return nil, fmt.Errorf("reload: the access gate needs a handler to wrap")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &GuardSwitch{secret: secret, logger: logger, backend: backend}

	generation, err := s.build(cfg)
	if err != nil {
		return nil, err
	}
	s.live.Store(generation)

	return s, nil
}

// build compiles one generation. Every way a configuration can be rejected
// happens here, before anything is published.
func (s *GuardSwitch) build(cfg *config.PanelAccessConfig) (*guardGeneration, error) {
	guard, err := middleware.NewPanelGuardFromConfig(cfg, s.secret, s.logger)
	if err != nil {
		return nil, err
	}
	return &guardGeneration{
		cfg:     Clone(cfg),
		guard:   guard,
		handler: guard.Wrap(s.backend),
	}, nil
}

// ServeHTTP dispatches through the current generation. The pointer is read
// exactly once per request, which is what makes a reload invisible to a request
// that is already in flight.
func (s *GuardSwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	generation := s.live.Load()
	if generation == nil {
		// Unreachable in a built process: the constructor stores a generation
		// before the switch is handed to anything. Answering the neutral 404 is
		// still the right failure, because it discloses nothing.
		middleware.WriteNeutralNotFound(w)
		return
	}
	generation.handler.ServeHTTP(w, r)
}

// Entrance is the entrance the live gate is enforcing.
func (s *GuardSwitch) Entrance() string {
	if generation := s.live.Load(); generation != nil {
		return generation.guard.Entrance()
	}
	return ""
}

// Name implements Applier.
func (s *GuardSwitch) Name() string { return "access gate" }

// Prepare compiles the new gate without publishing it.
func (s *GuardSwitch) Prepare(next *config.PanelAccessConfig) (Staged, error) {
	current := s.live.Load()
	if current != nil && !guardAffected(current.cfg, next) {
		return nil, nil
	}

	generation, err := s.build(next)
	if err != nil {
		return nil, err
	}

	return &guardStaged{sw: s, previous: current, next: generation}, nil
}

// guardAffected reports whether any setting the gate reads has moved. Rebuilding
// a guard is cheap, but a guard rebuilt for an unrelated change would reset
// nothing and log nothing, and a reload report that lists it would be noise.
func guardAffected(current, next *config.PanelAccessConfig) bool {
	return current.Enabled != next.Enabled ||
		current.EntranceEnabled != next.EntranceEnabled ||
		current.Entrance != next.Entrance ||
		current.Domain != next.Domain ||
		current.SessionTTLSeconds != next.SessionTTLSeconds ||
		current.TLS.Enabled != next.TLS.Enabled ||
		strings.Join(current.AllowedIPs, ",") != strings.Join(next.AllowedIPs, ",") ||
		strings.Join(current.TrustedProxies, ",") != strings.Join(next.TrustedProxies, ",")
}

type guardStaged struct {
	sw        *GuardSwitch
	previous  *guardGeneration
	next      *guardGeneration
	committed bool
}

func (g *guardStaged) Commit() {
	g.sw.live.Store(g.next)
	g.committed = true
}

func (g *guardStaged) Rollback() {
	if !g.committed {
		return
	}
	g.sw.live.Store(g.previous)
	g.committed = false
}

func (g *guardStaged) Retire() {}

func (g *guardStaged) Describe() string { return "access gate rebuilt" }

// ---------------------------------------------------------------------------
// The lockout check
// ---------------------------------------------------------------------------

// Verify replays the caller's own request through the new gate.
//
// This is not an approximation of the check a real request gets: it is the same
// code, the exported middleware.PanelGuard.Wrap, driven with a request carrying
// the caller's address, the caller's Host header and the new entrance. If the
// gate would answer that request with the neutral 404, the operator is locked
// out, and the change is undone before they discover it.
func (s *GuardSwitch) Verify(_ context.Context, next *config.PanelAccessConfig, req Request) error {
	if !next.Enabled {
		// With the gate switched off there is nothing that can refuse a request,
		// so there is nothing to prove.
		return nil
	}

	generation := s.live.Load()
	if generation == nil {
		return errNoListener
	}

	clientIP, host, described := probeIdentity(next, req)

	request, err := syntheticRequest(next, clientIP, host)
	if err != nil {
		return err
	}

	admitted := false
	marker := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { admitted = true })
	recorder := &discardWriter{header: http.Header{}}
	generation.guard.Wrap(marker).ServeHTTP(recorder, request)

	if !admitted {
		return fmt.Errorf(
			"the new access rules refuse a request from %s for host %q, which is %s: the panel would be unreachable from there",
			clientIP, host, described)
	}

	return nil
}

// probeIdentity is the point of view the check is made from.
//
// With a caller it is that caller: the operator who is holding this request
// open is the person who must still be able to reach the panel afterwards.
// Without one - a reload from a file or from SIGHUP, where nobody is waiting -
// it is the panel's own advertised host and the first address its allow list
// admits, which proves the configuration is not self-contradictory even though
// it cannot prove anybody in particular can still get in.
func probeIdentity(next *config.PanelAccessConfig, req Request) (clientIP, host, described string) {
	if req.Caller != nil && strings.TrimSpace(req.Caller.ClientIP) != "" {
		host = strings.TrimSpace(req.Caller.Host)
		if host == "" {
			host = next.AccessHost()
		}
		return req.Caller.ClientIP, host, "where the change was requested from"
	}

	clientIP = "127.0.0.1"
	if len(next.AllowedIPs) > 0 {
		if network, err := config.ParseIPMatcher(next.AllowedIPs[0]); err == nil {
			clientIP = network.IP.String()
		}
	}
	host = next.AccessHost()
	if next.Domain != "" {
		host = next.Domain
	}

	return clientIP, host, "the first address this configuration admits"
}

// syntheticRequest builds the request the gate is asked about: a GET for the
// panel's own settings endpoint, through the entrance the new configuration
// defines.
func syntheticRequest(next *config.PanelAccessConfig, clientIP, host string) (*http.Request, error) {
	path := "/api/v1/panel/settings"
	if next.EntranceEnabled && next.Entrance != "" {
		path = next.Entrance + path
	}

	request, err := http.NewRequest(http.MethodGet, "http://"+hostForURL(host)+path, nil)
	if err != nil {
		return nil, fmt.Errorf("reload: cannot build the reachability check request: %w", err)
	}
	request.Host = host
	request.RemoteAddr = net.JoinHostPort(clientIP, "0")

	return request, nil
}

// hostForURL brackets a bare IPv6 literal so url.Parse accepts it.
func hostForURL(host string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return "localhost"
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return "[" + host + "]"
	}
	return host
}

// discardWriter is the response the check throws away. The gate writes a
// neutral 404 into it when it refuses, and the marker handler is what tells us
// it did not.
type discardWriter struct {
	header http.Header
	status int
}

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *discardWriter) WriteHeader(status int)      { w.status = status }
