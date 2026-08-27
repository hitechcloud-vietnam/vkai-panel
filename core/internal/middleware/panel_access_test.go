package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testGuard(t *testing.T, opts PanelGuardOptions) http.Handler {
	t.Helper()

	if opts.Secret == "" {
		opts.Secret = "test-secret-value-for-panel-entrance-cookie"
	}
	guard, err := NewPanelGuard(opts)
	if err != nil {
		t.Fatalf("NewPanelGuard: %v", err)
	}

	return guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("panel:" + r.URL.Path))
	}))
}

func do(h http.Handler, method, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "203.0.113.9:51000"
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEntranceRejectsWrongPathWithNeutral404(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	for _, target := range []string{"/", "/admin", "/api/v1/servers", "/vkai_a1b2c3d5", "/vkai_a1b2c3d4x"} {
		rec := do(h, http.MethodGet, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", target, rec.Code)
		}
		if body := rec.Body.String(); body != "404 page not found\n" {
			t.Fatalf("%s: body = %q, want the neutral 404 body", target, body)
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Fatalf("%s: a rejected request must not hint at the entrance (Location: %s)", target, loc)
		}
	}
}

func TestEntranceGrantsAccessAndIssuesCookie(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	rec := do(h, http.MethodGet, "/vkai_a1b2c3d4/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "panel:/" {
		t.Fatalf("body = %q: the entrance prefix must be stripped before routing", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "vkai_entrance" {
		t.Fatalf("cookies = %+v, want one vkai_entrance cookie", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("the entrance cookie must be HttpOnly")
	}

	// A later API call carries no prefix but does carry the cookie.
	rec2 := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) {
		r.AddCookie(cookies[0])
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("status with cookie = %d, want 200", rec2.Code)
	}
}

func TestForgedCookieIsRejected(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	forged := &http.Cookie{
		Name:  "vkai_entrance",
		Value: "99999999999.0000000000000000000000000000000000000000000000000000000000000000",
	}
	rec := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.AddCookie(forged) })
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a forged entrance cookie", rec.Code)
	}
}

func TestExpiredCookieIsRejected(t *testing.T) {
	guard, err := NewPanelGuard(PanelGuardOptions{
		Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4",
		Secret: "test-secret-value-for-panel-entrance-cookie",
	})
	if err != nil {
		t.Fatalf("NewPanelGuard: %v", err)
	}
	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	expired := &http.Cookie{Name: "vkai_entrance", Value: guard.signSession(time.Now().Add(-time.Minute))}
	rec := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.AddCookie(expired) })
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an expired entrance cookie", rec.Code)
	}
}

func TestTraversalCannotReachTheEntrance(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	for _, target := range []string{"/vkai_a1b2c3d4/../api/v1/servers", "//vkai_a1b2c3d4/../../etc"} {
		rec := do(h, http.MethodGet, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", target, rec.Code)
		}
	}
}

func TestHealthBypassesTheEntrance(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	for _, target := range []string{"/api/v1/health", "/health", "/ready", "/live"} {
		rec := do(h, http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (health must stay reachable)", target, rec.Code)
		}
	}
}

func TestIPAllowListBlocksEverythingElse(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{
		Enabled:    true,
		AllowedIPs: []string{"10.0.0.0/8", "198.51.100.7"},
	})

	allowed := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.RemoteAddr = "10.4.5.6:4444" })
	if allowed.Code != http.StatusOK {
		t.Fatalf("allow-listed IP got %d, want 200", allowed.Code)
	}

	blocked := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.RemoteAddr = "203.0.113.9:4444" })
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("non allow-listed IP got %d, want 404", blocked.Code)
	}
}

func TestForwardedForIsIgnoredFromAnUntrustedPeer(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, AllowedIPs: []string{"10.0.0.0/8"}})

	// The peer is not a trusted proxy, so it cannot promote itself by claiming
	// an allow-listed address in X-Forwarded-For.
	rec := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) {
		r.RemoteAddr = "203.0.113.9:4444"
		r.Header.Set("X-Forwarded-For", "10.1.2.3")
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: X-Forwarded-For from an untrusted peer must be ignored", rec.Code)
	}
}

func TestForwardedForIsHonouredFromATrustedProxy(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{
		Enabled:        true,
		AllowedIPs:     []string{"10.0.0.0/8"},
		TrustedProxies: []string{"172.20.0.0/16"},
	})

	rec := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) {
		r.RemoteAddr = "172.20.0.5:4444"
		r.Header.Set("X-Forwarded-For", "10.1.2.3, 172.20.0.5")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a client forwarded by a trusted proxy", rec.Code)
	}

	spoofed := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) {
		r.RemoteAddr = "172.20.0.5:4444"
		r.Header.Set("X-Forwarded-For", "10.1.2.3, 203.0.113.9")
	})
	if spoofed.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: the right-most untrusted hop is the client", spoofed.Code)
	}
}

func TestDomainPinning(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: true, Domain: "panel.example.com"})

	ok := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.Host = "panel.example.com:8888" })
	if ok.Code != http.StatusOK {
		t.Fatalf("matching host got %d, want 200", ok.Code)
	}

	bad := do(h, http.MethodGet, "/api/v1/servers", func(r *http.Request) { r.Host = "203.0.113.9:8888" })
	if bad.Code != http.StatusNotFound {
		t.Fatalf("mismatched host got %d, want 404", bad.Code)
	}
}

func TestDisabledGuardIsTransparent(t *testing.T) {
	h := testGuard(t, PanelGuardOptions{Enabled: false, EntranceEnabled: true, Entrance: "/vkai_a1b2c3d4"})

	rec := do(h, http.MethodGet, "/api/v1/servers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when the gate is disabled", rec.Code)
	}
}
