package acme

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ChallengePath is the URL prefix an HTTP-01 challenge response is served under.
const ChallengePath = "/.well-known/acme-challenge/"

// HTTP01Server is a ChallengeSolver that binds a port itself and answers
// validation requests from memory.
//
// Use it when nothing else holds port 80, typically during a fresh install
// before the web server is configured. On a machine already serving customer
// websites the port is taken, and WebrootSolver is the right choice there.
type HTTP01Server struct {
	mu       sync.RWMutex
	tokens   map[string]string
	listener net.Listener
	server   *http.Server
	done     chan struct{}
}

// NewHTTP01Server starts a listener on addr, usually ":80", and serves challenge
// responses until Close is called. It returns as soon as the port is bound, so a
// successful return means validation requests can already be answered.
func NewHTTP01Server(addr string) (*HTTP01Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("acme: listen on %s for http-01: %w", addr, err)
	}
	s := &HTTP01Server{
		tokens:   make(map[string]string),
		listener: listener,
		done:     make(chan struct{}),
	}
	s.server = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		defer close(s.done)
		// http.ErrServerClosed is the normal outcome of Close.
		_ = s.server.Serve(listener)
	}()
	return s, nil
}

// Addr reports the address the solver is listening on, which is useful when addr
// used port 0.
func (s *HTTP01Server) Addr() net.Addr { return s.listener.Addr() }

// ServeHTTP answers challenge requests and nothing else.
func (s *HTTP01Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, ChallengePath) {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, ChallengePath)

	s.mu.RLock()
	keyAuth, ok := s.tokens[token]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write([]byte(keyAuth))
}

// Present registers a key authorization for a token.
func (s *HTTP01Server) Present(token, keyAuth string) error {
	if token == "" {
		return errors.New("acme: cannot present an empty challenge token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = keyAuth
	return nil
}

// CleanUp forgets a token. Removing one that was never present is not an error,
// because CleanUp runs on failure paths too.
func (s *HTTP01Server) CleanUp(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

// Close shuts the listener down and waits for the serving goroutine to finish.
func (s *HTTP01Server) Close() error {
	err := s.server.Close()
	<-s.done
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("acme: close http-01 server: %w", err)
	}
	return nil
}

// WebrootSolver is a ChallengeSolver that writes the challenge response into a
// directory already served on port 80 by another web server.
//
// This is the solver to use on a machine hosting customer websites: the panel
// never takes the port, it only drops a file where the running web server will
// find it.
type WebrootSolver struct {
	// Root is the document root whose /.well-known/acme-challenge/ subdirectory
	// receives the token files.
	Root string

	mu      sync.Mutex
	created []string
}

// NewWebrootSolver returns a solver writing under root.
func NewWebrootSolver(root string) *WebrootSolver {
	return &WebrootSolver{Root: root}
}

// challengeDir is the directory token files are written into.
func (s *WebrootSolver) challengeDir() string {
	return filepath.Join(s.Root, ".well-known", "acme-challenge")
}

// tokenPath returns the file path for a token, rejecting anything that is not a
// plain file name so a hostile token can never escape the webroot.
func (s *WebrootSolver) tokenPath(token string) (string, error) {
	if token == "" || token != filepath.Base(token) || token == "." || token == ".." || strings.ContainsRune(token, os.PathSeparator) {
		return "", fmt.Errorf("acme: refusing to use %q as a challenge file name", token)
	}
	return filepath.Join(s.challengeDir(), token), nil
}

// Present writes the key authorization where the web server will serve it. The
// file is world readable because the web server process is usually a different
// user, and the value is public by design.
func (s *WebrootSolver) Present(token, keyAuth string) error {
	if s.Root == "" {
		return errors.New("acme: WebrootSolver.Root is empty")
	}
	path, err := s.tokenPath(token)
	if err != nil {
		return err
	}
	dir := s.challengeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("acme: create challenge directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(keyAuth), 0o644); err != nil {
		return fmt.Errorf("acme: write challenge file %s: %w", path, err)
	}
	s.mu.Lock()
	s.created = append(s.created, path)
	s.mu.Unlock()
	return nil
}

// CleanUp removes the token file. A file that is already gone is not an error.
func (s *WebrootSolver) CleanUp(token string) error {
	path, err := s.tokenPath(token)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("acme: remove challenge file %s: %w", path, err)
	}
	s.mu.Lock()
	for i, created := range s.created {
		if created == path {
			s.created = append(s.created[:i], s.created[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return nil
}
