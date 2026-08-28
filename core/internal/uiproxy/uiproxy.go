// Package uiproxy puts the Next.js user interface behind the panel access gate
// instead of beside it.
//
// # Why this exists
//
// The panel is two processes: this Go API and a Next.js server, both listening
// on loopback, with nginx on the panel port in front of them. When nginx sent
// "/" to Next.js directly and only "/api/" to this process, the security
// entrance guarded the API and nothing else: the whole interface, the login
// form included, was served to anyone who found the port. The entrance was
// decorative, and the URL the installer prints returned 404 because Next.js
// knows nothing about it.
//
// So the panel now has ONE front door. nginx has a single upstream - this
// server - and this package forwards whatever the API does not own to Next.js.
// The access gate (internal/middleware.PanelGuard) wraps the handler built
// here, so every UI request passes the same host pin, the same IP allow list
// and the same entrance/cookie check as an API call, and there is no ordering
// in which a request can reach Next.js without passing the gate first.
//
// # What that buys
//
//   - one guard and one cookie, so the rule cannot drift between two places;
//   - nginx never has to learn the entrance, so rotating it is an .env edit and
//     a restart of this process - no nginx render, no UI rebuild;
//   - a wrong entrance and a host with no panel on it produce the same neutral
//     404 from the same code, rather than a Go 404 on one path and an nginx
//     error page on another.
//
// The cost is one loopback hop for UI responses. For a control panel served to
// a handful of administrators that is not a price worth optimising away.
package uiproxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

// Options configures the front door.
type Options struct {
	// Upstream is the base URL of the Next.js server, for example
	// "http://127.0.0.1:3000". An empty value means no interface is attached
	// to this process: every UI path is then answered by API, which is the
	// right behaviour for an API-only deployment and for the tests.
	Upstream string

	// API handles everything this process serves itself: /api/..., the health
	// probes, and anything else registered on the router.
	API http.Handler

	// Logger records upstream failures. Optional.
	Logger *zap.Logger
}

// dialTimeout bounds the connection to a process on loopback. A local socket
// either accepts at once or is not there.
const dialTimeout = 5 * time.Second

// acmeChallengePrefix is the fixed URL prefix ACME fetches tokens from. It
// mirrors the prefix the access gate lets through without the entrance.
const acmeChallengePrefix = "/.well-known/acme-challenge/"

// isACMEChallenge reports whether a path is an ACME HTTP-01 challenge fetch,
// matching both the bare prefix and anything below it.
func isACMEChallenge(p string) bool {
	p = normalize(p)
	return p == strings.TrimSuffix(acmeChallengePrefix, "/") || strings.HasPrefix(p, acmeChallengePrefix)
}

// responseHeaderTimeout bounds how long Next.js may take to produce response
// headers. Server-side rendering of a panel page is well under a second; a
// minute is generous enough that a slow first compile is not mistaken for a
// dead upstream.
const responseHeaderTimeout = 60 * time.Second

// New builds the handler that serves the whole panel: the API from opts.API and
// everything else from the Next.js upstream.
//
// The returned handler performs no access control of its own - it is meant to
// be wrapped by the panel access gate - and it must not be installed inside the
// gin engine: the API's response headers (a Content-Security-Policy of
// default-src 'none', among others) are written for JSON replies and would
// break the interface.
func New(opts Options) (http.Handler, error) {
	if opts.API == nil {
		return nil, errors.New("uiproxy: the API handler is required")
	}

	// A nil proxy means no interface is attached to this process, which is a
	// valid API-only deployment: every path is then the API's.
	var proxy http.Handler

	if upstream := strings.TrimSpace(opts.Upstream); upstream != "" {
		var err error
		if proxy, err = newReverseProxy(upstream, opts.Logger); err != nil {
			return nil, err
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isACMEChallenge(r.URL.Path) {
			// The access gate lets this prefix through without the entrance.
			// Nothing on the panel port serves a challenge token - the panel's
			// own certificate is validated on port 80 by internal/tlsmanager -
			// so answer it exactly as the gate answers a request it rejected.
			// Forwarding it instead would hand an unauthenticated prober either
			// the panel's own 404 page (from Next.js) or the API's response
			// headers, and either one tells them a panel is here.
			middleware.WriteNeutralNotFound(w)
			return
		}
		if proxy == nil || servedByAPI(r.URL.Path) {
			opts.API.ServeHTTP(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}

// newReverseProxy builds the hop to the Next.js service.
func newReverseProxy(upstream string, logger *zap.Logger) (http.Handler, error) {
	target, err := parseUpstream(upstream)
	if err != nil {
		return nil, err
	}

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// The inbound path has already had the entrance prefix stripped by
			// the access gate, so Next.js is asked for "/" whether the browser
			// typed "/" or "/<entrance>/". That is what keeps the interface
			// free of any knowledge of the entrance, and what makes rotating
			// the entrance a configuration change rather than a rebuild.
			pr.SetURL(target)

			// Keep the Host the browser used. Next.js echoes it into
			// redirects, and a redirect to 127.0.0.1:3000 would send the
			// administrator to a port that is not published.
			pr.Out.Host = pr.In.Host

			pr.SetXForwarded()

			// SetXForwarded reports the scheme of the hop it can see, and the
			// hop from nginx to this process is plaintext loopback. Preserve
			// what nginx said the browser actually used, so the interface does
			// not think an HTTPS panel is running on HTTP.
			if proto := pr.In.Header.Get("X-Forwarded-Proto"); proto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", proto)
			}
		},

		// Flush as soon as bytes are available. The App Router streams its HTML,
		// and buffering that until the copy buffer fills delays the first paint
		// for no benefit on a loopback hop.
		FlushInterval: -1,

		Transport: &http.Transport{
			// Never consult HTTP_PROXY for a hop to loopback.
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: time.Second,
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// The caller has already passed the access gate, so there is
			// nothing to hide from them: say the interface is down rather than
			// pretending the path does not exist, which would send an
			// administrator hunting for the wrong fault.
			if logger != nil {
				logger.Warn("panel UI upstream unreachable",
					zap.String("upstream", target.String()),
					zap.String("path", r.URL.Path),
					zap.Error(err),
				)
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("The panel interface is not responding.\n"))
		},
	}, nil
}

// parseUpstream validates the Next.js base URL. A typo here would otherwise
// surface as a 502 on every page of the panel.
func parseUpstream(raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("uiproxy: %q is not a valid upstream URL: %w", raw, err)
	}
	switch target.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("uiproxy: upstream %q must be an http:// or https:// URL", raw)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("uiproxy: upstream %q has no host", raw)
	}
	return target, nil
}

// servedByAPI reports whether a request path belongs to this process rather
// than to the interface. It is the whole routing rule of the front door, in one
// place, so that adding a route to the API cannot silently start forwarding it
// to Next.js.
//
// The list is deliberately short: the API owns /api/..., the WebSocket prefix
// and the unauthenticated probe paths. Everything else - pages, /_next/...,
// favicons, anything the interface adds later - is the interface's.
func servedByAPI(p string) bool {
	// Compare on the cleaned path so "/x/../api/v1/users" cannot be routed to
	// the interface and then reinterpreted as an API call further down.
	p = normalize(p)

	switch p {
	case "/health", "/healthz", "/ready", "/readyz", "/live", "/livez":
		return true
	}

	// "/ws" is not registered today (the terminal and log streams live under
	// /api/v1/ws), but the prefix is reserved for this process so a root-level
	// socket can be added without touching the front door.
	for _, prefix := range []string{"/api", "/ws"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}

	return false
}

func normalize(raw string) string {
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return path.Clean(raw)
}
