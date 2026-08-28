package reload

// Moving the panel to a different port without dropping the process.
//
// The order of operations here is the whole design, and it is not negotiable:
//
//	1. bind the new address. This is where "port already in use" is discovered,
//	   and discovering it here costs nothing - the old listener has not been
//	   touched;
//	2. start serving on it;
//	3. prove it answers;
//	4. only then stop accepting on the old address, and let the requests already
//	   in flight there finish.
//
// The step that must never move is 4. A panel that closed its only listener,
// failed to bind the new one and has no way back is a machine its operator
// administers over the network and can no longer reach. Every failure path
// below therefore ends in "the old listener is still accepting", including the
// paths that are only reachable if this file is wrong.

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// DefaultDrainTimeout is how long a retired listener is given to finish the
// requests already running on it. It is generous: the alternative to waiting is
// cutting off an operator's own upload or backup restore mid-flight.
const DefaultDrainTimeout = 30 * time.Second

// RebinderOptions configures the listener manager.
type RebinderOptions struct {
	// Handler is what every generation serves. It is the same handler across
	// generations - the access gate swaps underneath it - so a rebind does not
	// disturb the gate and a gate change does not disturb the listener.
	Handler http.Handler

	// TLSConfig is the stable TLS configuration: its GetCertificate resolves
	// the current certificate on every handshake, so a certificate change never
	// requires a new listener.
	TLSConfig *tls.Config

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// DrainTimeout bounds how long a retired listener may take to finish.
	DrainTimeout time.Duration

	// Address resolves the address to bind for a configuration. It is injected
	// because the panel binds its own port when the gate is enabled and the
	// legacy server.host/server.port pair when it is not.
	Address func(*config.PanelAccessConfig) string

	Logger *zap.Logger
}

// Rebinder owns the listening sockets.
type Rebinder struct {
	opts   RebinderOptions
	logger *zap.Logger

	mu      sync.Mutex
	current *generation

	// fatal carries the death of the generation that is currently serving. A
	// listener that stops accepting for a reason other than being retired means
	// the panel is off the network, which the process must not survive quietly.
	fatal chan error

	// useTLS is read on every accepted connection, so switching the panel
	// between HTTP and HTTPS is a single atomic store rather than a rebind.
	useTLS atomic.Bool
}

// errNoTLSConfig is refused rather than answered in the clear.
var errNoTLSConfig = errors.New("reload: TLS is enabled for the panel but no TLS configuration is available")

// generation is one listening socket and the server that accepts on it.
//
// Whether it terminates TLS is deliberately not a property of the generation.
// Turning TLS on or off would otherwise mean rebinding the same address, which
// cannot be done while the old socket still holds it - and closing that socket
// first to make room is exactly the move this package exists to forbid. So TLS
// is decided per accepted connection instead, from a flag the reload flips, and
// a certificate change never touches the listener at all.
type generation struct {
	addr    string
	ln      net.Listener
	srv     *http.Server
	retired chan struct{}
	once    sync.Once
}

// tlsToggleListener decides per connection whether to speak TLS.
//
// This is what net/http's own ServeTLS does with tls.NewListener, except that
// the decision is read at accept time rather than fixed when the listener is
// built. net/http recognises a *tls.Conn and performs the handshake itself, so
// the server needs no other change.
type tlsToggleListener struct {
	net.Listener
	enabled func() bool
	config  func() *tls.Config
}

func (l *tlsToggleListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if !l.enabled() {
		return conn, nil
	}
	cfg := l.config()
	if cfg == nil {
		// Refusing the connection is the honest failure: answering it in the
		// clear would put an operator's session cookie on the wire unencrypted
		// on a panel that was configured for HTTPS.
		_ = conn.Close()
		return nil, errNoTLSConfig
	}
	return tls.Server(conn, cfg), nil
}

// NewRebinder builds the listener manager. It binds nothing yet.
func NewRebinder(opts RebinderOptions) (*Rebinder, error) {
	if opts.Handler == nil {
		return nil, errors.New("reload: the listener needs a handler")
	}
	if opts.Address == nil {
		opts.Address = func(cfg *config.PanelAccessConfig) string { return cfg.ListenAddr() }
	}
	if opts.DrainTimeout <= 0 {
		opts.DrainTimeout = DefaultDrainTimeout
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}

	return &Rebinder{
		opts:   opts,
		logger: opts.Logger,
		fatal:  make(chan error, 4),
	}, nil
}

// Fatal reports a serving generation that died. The API entry point waits on it
// alongside the shutdown signal: a panel whose listener has gone is not a panel
// that should keep running as if nothing happened.
func (r *Rebinder) Fatal() <-chan error { return r.fatal }

// Start binds and serves the first generation.
func (r *Rebinder) Start(cfg *config.PanelAccessConfig) error {
	r.useTLS.Store(cfg.TLS.Enabled)

	gen, err := r.bind(cfg)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.current = gen
	r.mu.Unlock()

	r.serve(gen)
	return nil
}

// Addr is the address currently being served, empty before Start.
func (r *Rebinder) Addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return ""
	}
	return r.current.ln.Addr().String()
}

// UsesTLS reports whether accepted connections are being wrapped in TLS.
func (r *Rebinder) UsesTLS() bool { return r.useTLS.Load() }

// Shutdown drains every generation. It is the process's ordinary exit path.
func (r *Rebinder) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	current := r.current
	r.current = nil
	r.mu.Unlock()

	if current == nil {
		return nil
	}
	current.once.Do(func() { close(current.retired) })
	return current.srv.Shutdown(ctx)
}

// bind opens the socket. Everything that can fail about a new address fails
// here, with the running listener untouched.
func (r *Rebinder) bind(cfg *config.PanelAccessConfig) (*generation, error) {
	addr := r.opts.Address(cfg)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", addr, explainBindError(addr, err))
	}

	if cfg.TLS.Enabled && r.opts.TLSConfig == nil {
		_ = ln.Close()
		return nil, errors.New("TLS is enabled for the panel but no TLS configuration was supplied")
	}

	gen := &generation{
		addr: addr,
		ln: &tlsToggleListener{
			Listener: ln,
			enabled:  r.useTLS.Load,
			config:   func() *tls.Config { return r.opts.TLSConfig },
		},
		retired: make(chan struct{}),
		srv: &http.Server{
			Addr:         addr,
			Handler:      r.opts.Handler,
			ReadTimeout:  r.opts.ReadTimeout,
			WriteTimeout: r.opts.WriteTimeout,
			IdleTimeout:  r.opts.IdleTimeout,
			TLSConfig:    r.opts.TLSConfig,
		},
	}

	return gen, nil
}

// serve starts accepting. The goroutine outlives this call and ends when the
// generation is retired or when accepting fails.
func (r *Rebinder) serve(gen *generation) {
	go func() {
		// Serve, never ServeTLS: the listener wraps each connection itself, so
		// the certificate - and whether there is one at all - is resolved per
		// connection rather than fixed when this goroutine starts.
		err := gen.srv.Serve(gen.ln)

		if errors.Is(err, http.ErrServerClosed) || err == nil {
			return
		}

		select {
		case <-gen.retired:
			// Expected: the socket was closed underneath a retired generation.
			return
		default:
		}

		r.logger.Error("panel listener stopped accepting",
			zap.String("addr", gen.addr), zap.Error(err))
		select {
		case r.fatal <- fmt.Errorf("the panel listener on %s stopped accepting: %w", gen.addr, err):
		default:
		}
	}()
}

// Name implements Applier.
func (r *Rebinder) Name() string { return "listener" }

// Prepare binds the new address. Nothing about the running listener changes:
// on return the process is listening on both, and only the old one is serving.
func (r *Rebinder) Prepare(next *config.PanelAccessConfig) (Staged, error) {
	r.mu.Lock()
	current := r.current
	r.mu.Unlock()

	addr := r.opts.Address(next)
	sameAddress := current != nil && current.addr == addr
	sameTLS := r.useTLS.Load() == next.TLS.Enabled

	if sameAddress && sameTLS {
		return nil, nil
	}

	staged := &listenerStaged{
		rebinder:    r,
		previous:    current,
		previousTLS: r.useTLS.Load(),
		nextTLS:     next.TLS.Enabled,
	}

	if sameAddress {
		// Only the TLS decision moved. There is nothing to bind: the socket
		// stays exactly where it is and the next accepted connection is wrapped
		// - or not - according to the flag this commit flips.
		if next.TLS.Enabled && r.opts.TLSConfig == nil {
			return nil, errors.New("TLS is enabled for the panel but no TLS configuration was supplied")
		}
		return staged, nil
	}

	gen, err := r.bind(next)
	if err != nil {
		return nil, err
	}
	staged.next = gen

	return staged, nil
}

// Verify proves the live generation is accepting and serving this process's
// handler, by making a real request to it over a real socket.
func (r *Rebinder) Verify(ctx context.Context, _ *config.PanelAccessConfig, _ Request) error {
	r.mu.Lock()
	gen := r.current
	r.mu.Unlock()

	if gen == nil {
		return errNoListener
	}

	target := dialTarget(gen.ln.Addr())
	useTLS := r.useTLS.Load()

	return retry(ctx, func() error {
		if err := probeHTTP(target, useTLS); err != nil {
			return fmt.Errorf("the new listener on %s does not answer: %w", gen.addr, err)
		}
		return nil
	})
}

// listenerStaged is a bound-but-not-yet-serving generation, a TLS flip, or both.
type listenerStaged struct {
	rebinder    *Rebinder
	previous    *generation
	next        *generation
	previousTLS bool
	nextTLS     bool
	committed   bool
}

// Commit starts serving on the new socket and makes it the live generation. The
// previous one is still accepting at this point, and stays that way until the
// change has been proven.
func (l *listenerStaged) Commit() {
	l.rebinder.useTLS.Store(l.nextTLS)

	if l.next != nil {
		l.rebinder.serve(l.next)

		l.rebinder.mu.Lock()
		l.rebinder.current = l.next
		l.rebinder.mu.Unlock()
	}

	l.committed = true
}

// Rollback puts the previous generation back and closes the new socket. The
// previous listener was never stopped, so there is nothing to restart: the
// panel keeps answering on the address it was already answering on.
func (l *listenerStaged) Rollback() {
	if l.committed {
		l.rebinder.useTLS.Store(l.previousTLS)
		if l.next != nil {
			l.rebinder.mu.Lock()
			l.rebinder.current = l.previous
			l.rebinder.mu.Unlock()
		}
		l.committed = false
	}

	if l.next == nil {
		return
	}

	l.next.once.Do(func() { close(l.next.retired) })
	// Close, not Shutdown: this generation was never proven and anything that
	// reached it in the last few milliseconds is better off retrying against
	// the address that works.
	_ = l.next.srv.Close()
	_ = l.next.ln.Close()

	l.rebinder.logger.Warn("panel listener rebind undone, still serving the previous address",
		zap.String("attempted", l.next.addr),
		zap.String("serving", previousAddr(l.previous)))
}

// Retire stops the old listener accepting and lets its in-flight requests
// finish. It runs only after the new listener has been proven.
func (l *listenerStaged) Retire() {
	previous := l.previous
	if previous == nil || l.next == nil {
		// Nothing was rebound: the socket did not move, only the decision about
		// what to do with the connections it accepts.
		return
	}

	previous.once.Do(func() { close(previous.retired) })

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), l.rebinder.opts.DrainTimeout)
		defer cancel()

		// Shutdown closes the listener immediately - no new connection is
		// accepted from this instant - and then waits for the requests already
		// running to return.
		if err := previous.srv.Shutdown(ctx); err != nil {
			l.rebinder.logger.Warn("the previous panel listener did not drain in time and was closed",
				zap.String("addr", previous.addr),
				zap.Duration("drain_timeout", l.rebinder.opts.DrainTimeout),
				zap.Error(err))
			_ = previous.srv.Close()
			return
		}

		l.rebinder.logger.Info("previous panel listener drained and closed",
			zap.String("addr", previous.addr))
	}()
}

func (l *listenerStaged) Describe() string {
	if l.next == nil {
		if l.nextTLS {
			return "listener switched to HTTPS in place"
		}
		return "listener switched to plain HTTP in place"
	}
	return fmt.Sprintf("listener moved from %s to %s", previousAddr(l.previous), l.next.addr)
}

func previousAddr(gen *generation) string {
	if gen == nil {
		return "(none)"
	}
	return gen.addr
}

// dialTarget turns a listening address into one that can be dialled from this
// machine. A wildcard bind cannot be connected to as written.
func dialTarget(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	switch strings.Trim(host, "[]") {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

// probeHTTP makes one request against the panel's own listener.
//
// The path is the health endpoint, which the access gate lets through without
// an entrance: this check is about the socket and the handler behind it, not
// about the gate - the gate is checked separately, and checking it here would
// fail an allow list that legitimately excludes this machine's own loopback.
//
// Any complete HTTP response counts. The listener is this process's own socket,
// so an answer of any status proves what is being asked: that it is bound,
// accepting, and running the panel's handler.
func probeHTTP(target string, useTLS bool) error {
	dialer := &net.Dialer{Timeout: 2 * time.Second}

	var (
		conn net.Conn
		err  error
	)
	if useTLS {
		// The certificate is not verified here on purpose. Whether a browser
		// trusts it is a separate question, answered when the certificate is
		// accepted; what this proves is that the TLS listener completes a
		// handshake and answers HTTP.
		conn, err = tls.DialWithDialer(dialer, "tcp", target, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
	} else {
		conn, err = dialer.Dial("tcp", target)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodGet, "http://"+target+"/api/v1/health", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "vkai-panel-reload-probe")
	if err := request.Write(conn); err != nil {
		return err
	}

	response, err := http.ReadResponse(newBufferedReader(conn), request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	return nil
}

// explainBindError turns a syscall error into something an operator can act on.
// "bind: address already in use" is accurate and says nothing about which of
// their services is already there.
func explainBindError(addr string, err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "address already in use"):
		return fmt.Errorf("%w - something else on this machine is already listening on %s. "+
			"The panel is still serving on its previous address; choose a free port or stop the other service first", err, addr)
	case strings.Contains(message, "permission denied"):
		return fmt.Errorf("%w - binding %s needs privileges this process does not have. "+
			"Ports below 1024 require root; the panel is still serving on its previous address", err, addr)
	case strings.Contains(message, "cannot assign requested address"):
		return fmt.Errorf("%w - no interface on this machine has the address %s. "+
			"The panel is still serving on its previous address", err, addr)
	}
	return err
}

// newBufferedReader wraps a connection for http.ReadResponse, which needs a
// buffered reader.
func newBufferedReader(conn net.Conn) *bufio.Reader { return bufio.NewReader(conn) }
