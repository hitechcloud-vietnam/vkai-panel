package tlsmanager

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebrootIsPreferredWhenTheCatchAllRootExists(t *testing.T) {
	siteRoot := t.TempDir()

	solver := NewHTTP01Solver(HTTP01Options{SiteRoot: siteRoot, ListenAddr: "127.0.0.1:0"})
	t.Cleanup(solver.Close)

	if err := solver.Present("tokenA", "tokenA.keyauth"); err != nil {
		t.Fatalf("Present: %v", err)
	}

	// Nothing may be bound: port 80 belongs to the customer websites, and the
	// whole point of the webroot strategy is not to touch it.
	if addr := solver.listenerAddr(); addr != "" {
		t.Fatalf("a listener was bound at %s even though the webroot was usable", addr)
	}

	path := filepath.Join(siteRoot, ".well-known", "acme-challenge", "tokenA")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the token was not published at %s: %v", path, err)
	}
	if string(body) != "tokenA.keyauth" {
		t.Fatalf("token file contains %q, want the key authorization", body)
	}

	if err := solver.CleanUp("tokenA"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the token file survived CleanUp")
	}
}

func TestTemporaryListenerAnswersWhenThereIsNoWebroot(t *testing.T) {
	// A path that exists but is not a directory: the catch-all document root is
	// unusable, so the solver must fall back to binding a port itself.
	notADir := filepath.Join(t.TempDir(), "default")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	solver := NewHTTP01Solver(HTTP01Options{SiteRoot: notADir, ListenAddr: "127.0.0.1:0"})
	t.Cleanup(solver.Close)

	if err := solver.Present("tokenB", "tokenB.keyauth"); err != nil {
		t.Fatalf("Present: %v", err)
	}

	addr := solver.listenerAddr()
	if addr == "" {
		t.Fatal("no listener was bound, so the challenge could never be answered")
	}

	body, status := fetch(t, "http://"+addr+challengePrefix+"tokenB")
	if status != http.StatusOK || body != "tokenB.keyauth" {
		t.Fatalf("challenge fetch = %d %q, want 200 and the key authorization", status, body)
	}

	// An unknown token is a plain 404: the listener is not a file server.
	if _, status := fetch(t, "http://"+addr+challengePrefix+"unknown"); status != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404", status)
	}

	if err := solver.CleanUp("tokenB"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if got := solver.listenerAddr(); got != "" {
		t.Fatalf("the listener is still bound at %s after the last challenge was cleaned up", got)
	}
}

func TestBusyPortProducesAnActionableError(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "default")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stand in for the nginx that owns port 80 on a real host.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	solver := NewHTTP01Solver(HTTP01Options{SiteRoot: notADir, ListenAddr: busy.Addr().String()})
	t.Cleanup(solver.Close)

	err = solver.Present("tokenC", "tokenC.keyauth")
	if err == nil {
		t.Fatal("Present succeeded even though the port was taken: the failure would surface as an unexplained ACME timeout")
	}

	msg := err.Error()
	for _, want := range []string{busy.Addr().String(), notADir, "VKAI_PANEL_TLS_MODE"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("the error does not mention %q, so it does not say what to do:\n%s", want, msg)
		}
	}
}

func TestTokensThatWouldEscapeTheChallengeDirectoryAreRejected(t *testing.T) {
	siteRoot := t.TempDir()
	solver := NewHTTP01Solver(HTTP01Options{SiteRoot: siteRoot, ListenAddr: "127.0.0.1:0"})
	t.Cleanup(solver.Close)

	for _, token := range []string{"", "../../etc/passwd", "a/b", "tok en", "tok\x00en", strings.Repeat("x", 256)} {
		if err := solver.Present(token, "keyauth"); err == nil {
			t.Fatalf("Present(%q) was accepted", token)
		}
	}

	// Nothing at all may have been written outside the challenge directory.
	if entries, err := os.ReadDir(siteRoot); err == nil && len(entries) != 0 {
		t.Fatalf("rejected tokens still created %v", entries)
	}
}

func fetch(t *testing.T, url string) (string, int) {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body), resp.StatusCode
}
