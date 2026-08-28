// Package server is the agent's inbound control surface.
//
// Everything it serves is behind mutual TLS. There is no unauthenticated
// endpoint at all - not even a health check - because an unauthenticated health
// check on a customer's server is a way to fingerprint that the panel is there.
// An orchestrator that needs liveness can ask systemd.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/ops"
)

// maxBodyBytes caps an operation's argument object. Nothing legitimate is
// larger, and an unbounded read is a way to exhaust a small VPS.
const maxBodyBytes = 1 << 20

// Options configures the listener.
type Options struct {
	Addr      string
	TLSConfig *tls.Config
	Registry  *ops.Registry
	Logger    *log.Logger
	// PeerName names the verified caller in the log. It is a function so the
	// log line reports whichever certificate was actually presented.
	PeerName func(*http.Request) string
}

// Server is the agent's HTTPS control listener.
type Server struct {
	opts Options
	http *http.Server
}

func New(opts Options) (*Server, error) {
	if opts.TLSConfig == nil {
		return nil, errors.New("agent server: refusing to listen without a TLS configuration")
	}
	if opts.Registry == nil {
		return nil, errors.New("agent server: refusing to listen without an operation registry")
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	s := &Server{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/ops/{name}", s.handleOperation)
	mux.HandleFunc("GET /v1/operations", s.handleList)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("/", s.handleNotFound)

	s.http = &http.Server{
		Addr:      opts.Addr,
		Handler:   mux,
		TLSConfig: opts.TLSConfig,
		// A control channel has no reason to hold a connection open for long.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          opts.Logger,
	}
	return s, nil
}

// Serve listens until ctx is cancelled. The TLS configuration carries the
// certificate, so nothing is read from disk here.
func (s *Server) Serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("agent server: cannot listen on %s: %w", s.http.Addr, err)
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutdownCtx)
	}()
	s.opts.Logger.Printf("control channel listening on %s (mutual TLS, %d operations)",
		s.http.Addr, len(s.opts.Registry.Names()))

	err = s.http.ServeTLS(listener, "", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeResponse(w, http.StatusBadRequest, ops.Response{OK: false, Error: "cannot read the request body"})
		return
	}

	start := time.Now()
	result, err := s.opts.Registry.Dispatch(r.Context(), name, body)
	elapsed := time.Since(start)

	switch {
	case errors.Is(err, ops.ErrUnknownOperation):
		s.opts.Logger.Printf("operation refused: %s from %s: unknown", name, s.peer(r))
		writeResponse(w, http.StatusNotFound, ops.Response{OK: false, Error: err.Error()})
	case errors.Is(err, ops.ErrInvalidArgument):
		s.opts.Logger.Printf("operation refused: %s from %s: %v", name, s.peer(r), err)
		writeResponse(w, http.StatusBadRequest, ops.Response{OK: false, Error: err.Error()})
	case err != nil:
		s.opts.Logger.Printf("operation failed: %s from %s after %s: %v", name, s.peer(r), elapsed, err)
		writeResponse(w, http.StatusInternalServerError, ops.Response{OK: false, Error: err.Error()})
	default:
		s.opts.Logger.Printf("operation ok: %s from %s in %s", name, s.peer(r), elapsed)
		writeResponse(w, http.StatusOK, ops.Response{OK: true, Result: result})
	}
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	writeResponse(w, http.StatusOK, ops.Response{OK: true, Result: s.opts.Registry.Names()})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeResponse(w, http.StatusOK, ops.Response{OK: true, Result: map[string]string{"status": "healthy"}})
}

// handleNotFound answers anything unrecognised with the same neutral body,
// including the /execute path this agent used to serve.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSuffix(r.URL.Path, "/") == "/execute" {
		s.opts.Logger.Printf("SECURITY: a caller asked for the removed /execute endpoint from %s", s.peer(r))
	}
	writeResponse(w, http.StatusNotFound, ops.Response{OK: false, Error: "no such endpoint"})
}

func (s *Server) peer(r *http.Request) string {
	if s.opts.PeerName != nil {
		if name := s.opts.PeerName(r); name != "" {
			return name
		}
	}
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		return r.TLS.PeerCertificates[0].Subject.CommonName
	}
	return "unknown"
}

func writeResponse(w http.ResponseWriter, status int, body ops.Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
