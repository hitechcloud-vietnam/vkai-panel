package tlsmanager

// HTTP-01 challenge solving for the panel's own certificate.
//
// An "ip" identifier can only be validated over HTTP-01 (port 80) or
// TLS-ALPN-01 (port 443). There is no DNS-01 for an IP address, so there is no
// way to prove control of this machine without briefly owning one of the two
// ports the customer websites live on. The panel therefore never takes either
// port for itself; it borrows port 80 for the length of one challenge, and
// prefers not to borrow it at all:
//
//	1. Webroot. If the catch-all vhost's document root exists, the token is
//	   dropped into it and the web server already listening on port 80 answers
//	   the challenge. Nothing is bound, nothing is interrupted, and a server
//	   hosting live sites never stops answering them.
//	2. Temporary listener. Only when there is no such document root: bind :80,
//	   answer the one path, shut down. If port 80 is already taken by something
//	   that is not serving the webroot, that is a configuration the operator has
//	   to resolve, and the error says how.

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// challengePrefix is the fixed URL prefix ACME fetches tokens from.
const challengePrefix = "/.well-known/acme-challenge/"

// DefaultHTTP01Addr is the address the temporary listener binds. Port 80 is not
// negotiable: the CA fetches the challenge over plain HTTP on port 80 and
// follows no redirect to another port.
const DefaultHTTP01Addr = ":80"

// HTTP01Options configures a solver. Every field has a working default, so the
// zero value is what production uses and the fields exist for tests.
type HTTP01Options struct {
	// SiteRoot is the document root of the catch-all vhost, the one a request
	// for a bare IP address lands in. Defaults to config.DefaultSite().
	SiteRoot string

	// ListenAddr is where the fallback listener binds. Defaults to :80.
	ListenAddr string

	Logger *zap.Logger
}

// HTTP01Solver answers HTTP-01 challenges. One solver may serve several
// challenges at once; the temporary listener is started on the first challenge
// that needs it and stopped when the last one is cleaned up.
type HTTP01Solver struct {
	siteRoot     string
	challengeDir string
	listenAddr   string
	log          *zap.Logger

	mu     sync.Mutex
	tokens map[string]string // token -> key authorization
	files  map[string]string // token -> file written under the webroot
	srv    *http.Server
	ln     net.Listener
}

// NewHTTP01Solver builds a solver. It touches no filesystem and binds no port
// until a challenge actually arrives.
func NewHTTP01Solver(opts HTTP01Options) *HTTP01Solver {
	siteRoot := strings.TrimSpace(opts.SiteRoot)
	if siteRoot == "" {
		// config.DefaultSite() is <web root>/../default: the catch-all vhost.
		// It is resolved through the path helpers rather than written out, so a
		// relocated installation (VKAI_PANEL_ROOT) still writes the token where
		// its own web server will look for it.
		siteRoot = config.DefaultSite()
	}
	listenAddr := strings.TrimSpace(opts.ListenAddr)
	if listenAddr == "" {
		listenAddr = DefaultHTTP01Addr
	}
	log := opts.Logger
	if log == nil {
		log = zap.NewNop()
	}

	return &HTTP01Solver{
		siteRoot:     siteRoot,
		challengeDir: filepath.Join(siteRoot, ".well-known", "acme-challenge"),
		listenAddr:   listenAddr,
		log:          log,
		tokens:       make(map[string]string),
		files:        make(map[string]string),
	}
}

// ChallengeDir is where the webroot strategy writes tokens.
func (s *HTTP01Solver) ChallengeDir() string { return s.challengeDir }

// listenerAddr is the address the fallback listener actually bound, or "" when
// no listener is running. Tests bind port 0 and read the port back from here.
func (s *HTTP01Solver) listenerAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Present publishes one key authorization, webroot first.
func (s *HTTP01Solver) Present(token, keyAuth string) error {
	if err := validateToken(token); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[token] = keyAuth

	path, err := s.writeWebroot(token, keyAuth)
	if err == nil {
		s.files[token] = path
		s.log.Info("panel TLS: HTTP-01 token published through the webroot",
			zap.String("path", path))
		return nil
	}
	s.log.Debug("panel TLS: webroot is not usable for HTTP-01, falling back to a temporary listener",
		zap.String("site_root", s.siteRoot), zap.Error(err))

	if err := s.startListenerLocked(); err != nil {
		delete(s.tokens, token)
		return err
	}

	s.log.Info("panel TLS: HTTP-01 token published from a temporary listener",
		zap.String("addr", s.listenAddr))
	return nil
}

// CleanUp withdraws one key authorization and releases port 80 as soon as the
// last challenge is done. Leaving the listener up for even a few extra seconds
// means the next customer request to port 80 hits the panel instead of nginx.
func (s *HTTP01Solver) CleanUp(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, token)
	if path, ok := s.files[token]; ok {
		delete(s.files, token)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			// A stale token file is a leftover, not a failure: it authorises
			// nothing on its own and the next issuance overwrites it.
			s.log.Warn("panel TLS: could not remove the HTTP-01 token file",
				zap.String("path", path), zap.Error(err))
		}
	}

	if len(s.tokens) == 0 {
		s.stopListenerLocked()
	}
	return nil
}

// Close releases everything the solver still holds. Safe to call twice.
func (s *HTTP01Solver) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, path := range s.files {
		delete(s.files, token)
		_ = os.Remove(path)
	}
	s.tokens = make(map[string]string)
	s.stopListenerLocked()
}

// writeWebroot drops the token into the catch-all vhost's document root.
//
// The document root itself must already exist: creating it here would produce a
// directory no web server is configured to serve, so the challenge would fail
// anyway and the operator would be left with an unexplained empty tree. Only
// the .well-known subtree below an existing root is created.
func (s *HTTP01Solver) writeWebroot(token, keyAuth string) (string, error) {
	info, err := os.Stat(s.siteRoot)
	if err != nil {
		return "", fmt.Errorf("catch-all document root %s is not available: %w", s.siteRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("catch-all document root %s is not a directory", s.siteRoot)
	}

	// MkdirAll and WriteFile are the writability test: an access() check would
	// answer a different question (the effective uid may differ from the file
	// owner, and the mount may be read-only) and would still have to be
	// followed by the write.
	if err := os.MkdirAll(s.challengeDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", s.challengeDir, err)
	}

	path := filepath.Join(s.challengeDir, token)
	// World readable on purpose: the web server answering port 80 usually runs
	// as a different user, and the key authorization is public by design.
	if err := os.WriteFile(path, []byte(keyAuth), 0o644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	return path, nil
}

// startListenerLocked binds the fallback listener. The caller holds s.mu.
func (s *HTTP01Solver) startListenerLocked() error {
	if s.srv != nil {
		return nil
	}

	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("panel TLS: cannot bind %s to answer the ACME HTTP-01 challenge: %w. "+
			"Something else is already listening there (find it with: ss -lptn 'sport = :80'). "+
			"Pick one: create the catch-all document root %s and make it writable, so the web server on port 80 serves the challenge itself; "+
			"or stop the process holding port 80 for the length of one issuance; "+
			"or set VKAI_PANEL_TLS_MODE=%s to keep the locally generated certificate",
			s.listenAddr, err, s.siteRoot, config.TLSModeSelfSigned)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(challengePrefix, s.serveChallenge)

	srv := &http.Server{
		Handler: mux,
		// This listener is exposed to the whole internet for the duration of
		// one challenge, so it gets the timeouts of a public server, not the
		// defaults of a throwaway one.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
	}

	s.ln, s.srv = ln, srv
	go func() {
		// Serve returns as soon as the listener closes; that is the normal exit.
		_ = srv.Serve(ln)
	}()

	return nil
}

// stopListenerLocked shuts the fallback listener down. The caller holds s.mu.
func (s *HTTP01Solver) stopListenerLocked() {
	if s.srv == nil {
		return
	}
	_ = s.srv.Close()
	s.srv, s.ln = nil, nil
}

// serveChallenge answers exactly one path shape and nothing else. It is not a
// file server: it never touches the disk, so no path it is handed can escape
// anywhere, and an unknown token is a plain 404.
func (s *HTTP01Solver) serveChallenge(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, challengePrefix)

	s.mu.Lock()
	keyAuth, ok := s.tokens[token]
	s.mu.Unlock()

	if !ok || keyAuth == "" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(keyAuth))
}

// validateToken keeps a hostile token out of the filesystem path built from it.
// ACME tokens are base64url, so this rejects nothing a real CA sends.
func validateToken(token string) error {
	if token == "" {
		return fmt.Errorf("panel TLS: empty HTTP-01 token")
	}
	if len(token) > 255 {
		return fmt.Errorf("panel TLS: HTTP-01 token is longer than 255 characters")
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("panel TLS: HTTP-01 token %q contains a character outside base64url", token)
		}
	}
	return nil
}
