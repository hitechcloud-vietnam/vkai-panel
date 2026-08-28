package uiproxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

// stubAPI stands in for the gin engine: it answers with the path it was given,
// so a test can tell which side of the front door served a request.
func stubAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api:" + r.URL.Path))
	})
}

// stubUI stands in for the Next.js service.
func stubUI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ui:" + r.URL.Path + ":host=" + r.Host))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newFrontDoor(t *testing.T, upstream string) http.Handler {
	t.Helper()
	h, err := New(Options{Upstream: upstream, API: stubAPI()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func get(h http.Handler, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = "203.0.113.9:51000"
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPIPathsStayWithTheAPI(t *testing.T) {
	ui := stubUI(t)
	h := newFrontDoor(t, ui.URL)

	for _, target := range []string{
		"/api/v1/servers",
		"/api",
		"/health",
		"/ready",
		"/live",
		"/ws/terminal",
		"/x/../api/v1/servers",
	} {
		rec := get(h, target, nil)
		if !strings.HasPrefix(rec.Body.String(), "api:") {
			t.Fatalf("%s was served by %q, want the API", target, rec.Body.String())
		}
	}
}

func TestEverythingElseGoesToTheInterface(t *testing.T) {
	ui := stubUI(t)
	h := newFrontDoor(t, ui.URL)

	for _, target := range []string{"/", "/login", "/_next/static/chunk.js", "/favicon.ico", "/apikeys"} {
		rec := get(h, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", target, rec.Code)
		}
		if !strings.HasPrefix(rec.Body.String(), "ui:") {
			t.Fatalf("%s was served by %q, want the interface", target, rec.Body.String())
		}
	}
}

// The Host the browser used has to survive the hop, or Next.js redirects the
// administrator to the loopback port nginx never published.
func TestBrowserHostIsPreserved(t *testing.T) {
	ui := stubUI(t)
	h := newFrontDoor(t, ui.URL)

	rec := get(h, "/", func(r *http.Request) { r.Host = "panel.example.vn:8888" })
	if got := rec.Body.String(); !strings.HasSuffix(got, ":host=panel.example.vn:8888") {
		t.Fatalf("body = %q, want the browser Host forwarded", got)
	}
}

func TestUnreachableInterfaceIsHonestlyReported(t *testing.T) {
	ui := stubUI(t)
	upstream := ui.URL
	ui.Close() // nothing is listening any more

	h := newFrontDoor(t, upstream)
	rec := get(h, "/", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestEmptyUpstreamLeavesTheAPIAlone(t *testing.T) {
	h := newFrontDoor(t, "")

	rec := get(h, "/login", nil)
	if got := rec.Body.String(); got != "api:/login" {
		t.Fatalf("body = %q: with no interface attached every path is the API's", got)
	}
}

func TestBadUpstreamIsRejectedAtStartup(t *testing.T) {
	for _, raw := range []string{"127.0.0.1:3000", "ftp://127.0.0.1:3000", "http://"} {
		if _, err := New(Options{Upstream: raw, API: stubAPI()}); err == nil {
			t.Fatalf("New(%q) succeeded, want an error at startup rather than a 502 on every page", raw)
		}
	}
}

func TestAPIHandlerIsRequired(t *testing.T) {
	if _, err := New(Options{Upstream: "http://127.0.0.1:3000"}); err == nil {
		t.Fatal("New without an API handler succeeded, want an error")
	}
}

// TestTheEntranceGuardsTheInterface is the regression test for the defect this
// package exists to fix: the whole panel - pages, login form and API - behind
// one gate, reached through one door.
//
// It asserts, end to end, exactly what a probe sees on the panel port.
func TestTheEntranceGuardsTheInterface(t *testing.T) {
	const entrance = "/vkai_a8d888ab"

	ui := stubUI(t)
	front := newFrontDoor(t, ui.URL)

	guard, err := middleware.NewPanelGuard(middleware.PanelGuardOptions{
		Enabled:         true,
		EntranceEnabled: true,
		Entrance:        entrance,
		Secret:          "test-secret-value-for-panel-entrance-cookie",
	})
	if err != nil {
		t.Fatalf("NewPanelGuard: %v", err)
	}
	panel := guard.Wrap(front)

	// A client that has not passed the entrance sees a bare port.
	for _, target := range []string{"/", "/login", "/_next/static/chunk.js", "/api/v1/servers", "/vkai_wrong/"} {
		rec := get(panel, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", target, rec.Code)
		}
		if body := rec.Body.String(); body != "404 page not found\n" {
			t.Fatalf("%s: body = %q, want the neutral 404 body", target, body)
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Fatalf("%s: a rejected request must not redirect (Location: %s)", target, loc)
		}
	}

	// The entrance reaches the interface and issues the cookie.
	rec := get(panel, entrance+"/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("entrance: status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); !strings.HasPrefix(got, "ui:/:") {
		t.Fatalf("entrance served %q, want the interface at /", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "vkai_entrance" {
		t.Fatalf("cookies = %+v, want one vkai_entrance cookie", cookies)
	}
	withCookie := func(r *http.Request) { r.AddCookie(cookies[0]) }

	// With the cookie the panel loads at the root and its API calls work.
	rec = get(panel, "/", withCookie)
	if got := rec.Body.String(); rec.Code != http.StatusOK || !strings.HasPrefix(got, "ui:/:") {
		t.Fatalf("/ with cookie: status %d body %q, want the interface", rec.Code, got)
	}
	rec = get(panel, "/_next/static/chunk.js", withCookie)
	if got := rec.Body.String(); rec.Code != http.StatusOK || !strings.HasPrefix(got, "ui:") {
		t.Fatalf("static asset with cookie: status %d body %q", rec.Code, got)
	}
	rec = get(panel, "/api/v1/servers", withCookie)
	if got := rec.Body.String(); rec.Code != http.StatusOK || got != "api:/api/v1/servers" {
		t.Fatalf("API with cookie: status %d body %q", rec.Code, got)
	}

	// Probes that must answer without the cookie.
	for _, target := range []string{"/health", "/api/v1/health"} {
		rec := get(panel, target, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "api:"+target {
			t.Fatalf("%s: status %d body %q, want the API to answer without the cookie",
				target, rec.Code, rec.Body.String())
		}
	}

	// The ACME challenge prefix is let through by the gate but must not reach
	// the interface: a Next.js 404 page here would tell an unauthenticated
	// prober that a panel is on this port. The answer is byte-for-byte the one
	// the gate gives a request it rejected.
	for _, target := range []string{"/.well-known/acme-challenge", "/.well-known/acme-challenge/token"} {
		rec = get(panel, target, nil)
		if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
			t.Fatalf("%s: status %d body %q, want the same neutral 404 a rejected request gets",
				target, rec.Code, rec.Body.String())
		}
	}
}

// The interface must never see the ACME prefix, whichever way it is spelled.
func TestACMEChallengeNeverReachesTheInterface(t *testing.T) {
	ui := stubUI(t)
	h := newFrontDoor(t, ui.URL)

	for _, target := range []string{
		"/.well-known/acme-challenge/token",
		"/.well-known/acme-challenge/",
		"/x/../.well-known/acme-challenge/token",
	} {
		rec := get(h, target, nil)
		if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
			t.Fatalf("%s: status %d body %q, want the neutral 404", target, rec.Code, rec.Body.String())
		}
	}
}
