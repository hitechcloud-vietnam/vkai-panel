// Command vkaid is the VKAI Panel node agent.
//
// It runs as root on a managed server and does two things: it lets the panel
// ask it to perform a named operation over a mutually authenticated TLS
// channel, and it reports its state back to the panel on a timer.
//
// What it no longer does, and why:
//
//   - It no longer holds VKAI_AGENT_TOKEN. That was one static string, the same
//     on every managed server, sent in a header over plain HTTP and compared
//     with "!=". Reading it once - from the environment file, a log line, a
//     process listing or the wire - was enough to run any command as root on
//     every server the panel managed.
//   - It no longer serves /execute. An authenticated arbitrary-command endpoint
//     means panel compromise is root on the whole fleet in one step, with
//     nothing behind it. What it serves instead is a closed set of named
//     operations with typed arguments; see internal/ops.
//
// The agent's identity is now a private key generated on this host that never
// leaves it, and a 24 hour certificate issued by the panel's internal CA, which
// the agent renews at half life and which the panel can revoke immediately.
//
// # Subcommands
//
//	vkaid                 run the agent (the systemd unit does this)
//	vkaid trust-anchor    print the CA this agent trusts, and exit
//	vkaid re-enrol        replace this agent's identity, and its trust anchor,
//	                      with one from a freshly pasted enrolment token
//
// re-enrol is a separate command on purpose. Renewal is unattended and happens
// twice a day; it may never change what this agent trusts, or one intercepted
// renewal would make the interceptor this agent's certificate authority for
// good. Changing the anchor is therefore something an operator types.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/ops"
	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/pki"
	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/server"
)

const (
	// AgentVersion is the agent protocol and build version.
	AgentVersion = "2.0.0"

	// AgentName is the binary and systemd unit name of the node agent.
	AgentName = "vkaid"

	// AgentProduct is what an operator reading the console sees.
	AgentProduct = "VKAI Panel Agent"

	// AgentVendor appears in the startup line so a support ticket carries the
	// product it belongs to.
	AgentVendor = "HiTechCloud (hitechcloud.vn)"

	// defaultStateDir is where the agent's key, certificate and CA live. It is
	// outside the release directory on purpose: an upgrade replaces
	// /vkai-panel/current, and an identity that lived there would be lost.
	defaultStateDir = "/vkai-panel/ssl/agent"

	defaultListenPort = 30111

	// defaultStatusInterval is how often the agent reports in. It is also how
	// quickly it learns about a revoked panel certificate.
	defaultStatusInterval = 30 * time.Second

	// renewalCheckInterval is how often the agent asks whether it is time to
	// rotate. The certificate lives a day and renews at half life, so hourly
	// leaves twelve chances to succeed before anything expires.
	renewalCheckInterval = time.Hour
)

// Config is everything the agent reads from its environment.
type Config struct {
	PanelURL       string
	ListenAddr     string
	StateDir       string
	EnrolmentToken string
	Hostname       string
	StatusInterval time.Duration
	AllowRawExec   bool
	PanelCAFile    string
	PanelInsecure  bool
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)
	logger.Printf("%s (%s) v%s - %s", AgentProduct, AgentName, AgentVersion, AgentVendor)

	command := ""
	if len(os.Args) > 1 {
		command = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}

	var err error
	switch command {
	case "":
		err = run(logger)
	case "trust-anchor":
		err = showTrustAnchor(logger)
	case "re-enrol", "re-enroll":
		err = reEnrol(logger)
	default:
		err = fmt.Errorf("unknown command %q. Usage: %s [trust-anchor|re-enrol]", command, AgentName)
	}
	if err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

// showTrustAnchor prints the fingerprint of the CA this agent accepts. An
// operator compares it with the one the panel shows before believing that a
// change is legitimate.
func showTrustAnchor(logger *log.Logger) error {
	cfg, manager, err := newManager(logger)
	if err != nil {
		return err
	}
	if loadErr := manager.Load(); loadErr != nil {
		return loadErr
	}
	logger.Printf("state directory: %s", cfg.StateDir)
	logger.Printf("agent id:        %s", manager.AgentID())
	logger.Printf("certificate:     serial %s, expires %s", manager.Serial(), manager.NotAfter().Format(time.RFC3339))
	logger.Printf("trust anchor:    %s", manager.TrustAnchorFingerprint())
	return nil
}

// reEnrol is the explicit, operator-initiated action that may replace this
// agent's trust anchor. Nothing else may: see pki.ErrTrustAnchorChanged.
func reEnrol(logger *log.Logger) error {
	cfg, manager, err := newManager(logger)
	if err != nil {
		return err
	}
	if cfg.EnrolmentToken == "" {
		return errors.New("re-enrolment needs a fresh enrolment token. " +
			"Mint one in the panel (Servers -> Add agent) and run this command with " +
			"VKAI_AGENT_ENROLMENT_TOKEN set to it")
	}
	previous := ""
	if loadErr := manager.Load(); loadErr == nil {
		previous = manager.TrustAnchorFingerprint()
		logger.Printf("this agent currently trusts the CA with fingerprint %s", previous)
	} else if !errors.Is(loadErr, pki.ErrNotEnrolled) {
		logger.Printf("the identity on disk could not be loaded (%v); re-enrolling over it", loadErr)
	}
	if err := manager.ReEnrol(cfg.EnrolmentToken, cfg.Hostname, AgentVersion); err != nil {
		return fmt.Errorf("re-enrolment failed, the identity on disk is unchanged: %w", err)
	}
	logger.Printf("re-enrolled: agent_id=%s serial=%s trust anchor %s -> %s",
		manager.AgentID(), manager.Serial(), previous, manager.TrustAnchorFingerprint())
	logger.Printf("restart %s for the new identity to be served", AgentName)
	return nil
}

// newManager reads the configuration and builds the PKI manager, which is the
// first thing all three commands do.
func newManager(logger *log.Logger) (Config, *pki.Manager, error) {
	cfg, err := loadConfig()
	if err != nil {
		return cfg, nil, err
	}
	panelClient, err := panelHTTPClient(cfg)
	if err != nil {
		return cfg, nil, err
	}
	return cfg, pki.New(pki.Options{
		Dir:      cfg.StateDir,
		PanelURL: cfg.PanelURL,
		Client:   panelClient,
		Logger:   logger,
	}), nil
}

func run(logger *log.Logger) error {
	cfg, manager, err := newManager(logger)
	if err != nil {
		return err
	}

	// The old shared secret is called out rather than ignored: an operator who
	// upgraded and left it in place should be told it does nothing now, so it
	// gets removed from the environment file instead of lingering there. The
	// panel says the same thing from its side, counting the servers that still
	// hold one at every start.
	if env("VKAI_AGENT_TOKEN", "AGENT_TOKEN") != "" {
		logger.Printf("WARNING: VKAI_AGENT_TOKEN is set but this agent no longer sends it. " +
			"The panel-to-agent channel is mutual TLS. The panel still accepts that token from " +
			"an OLD agent that has not been enrolled, and refuses it for good once this server " +
			"has enrolled. Remove it from the environment file.")
	}

	if err := ensureIdentity(cfg, manager, logger); err != nil {
		return err
	}
	logger.Printf("identity: agent_id=%s serial=%s certificate expires %s trust anchor %s",
		manager.AgentID(), manager.Serial(), manager.NotAfter().Format(time.RFC3339),
		manager.TrustAnchorFingerprint())

	registry := ops.New(ops.Deps{
		AllowRawExec: cfg.AllowRawExec,
		Logger:       logger,
		ApplyDenyList: func(serials []string) {
			manager.ApplyDenyList(serials)
		},
		Info: func() ops.AgentInfo {
			return ops.AgentInfo{
				Version:        AgentVersion,
				AgentID:        manager.AgentID(),
				Hostname:       cfg.Hostname,
				CertNotAfter:   manager.NotAfter(),
				CertSerial:     manager.Serial(),
				DeniedSerials:  manager.DeniedCount(),
				RawExecEnabled: cfg.AllowRawExec,
			}
		},
	})

	tlsConfig, err := manager.ServerTLSConfig()
	if err != nil {
		return err
	}
	srv, err := server.New(server.Options{
		Addr:      cfg.ListenAddr,
		TLSConfig: tlsConfig,
		Registry:  registry,
		Logger:    logger,
		PeerName:  peerName,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx) }()
	go statusLoop(ctx, cfg, manager, logger)
	go renewalLoop(ctx, cfg, manager, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		logger.Printf("shutting down")
		cancel()
		<-serveErr
		return nil
	case err := <-serveErr:
		return err
	}
}

// ensureIdentity loads the agent's certificate, enrolling first if there is
// none. Enrolment happens once: the token is spent by it and is dead
// afterwards, so a restart with the same token in the environment does not try
// again and does not fail - the identity on disk is used instead.
func ensureIdentity(cfg Config, manager *pki.Manager, logger *log.Logger) error {
	err := manager.Load()
	if err == nil {
		return nil
	}
	if !errors.Is(err, pki.ErrNotEnrolled) {
		return err
	}
	if cfg.EnrolmentToken == "" {
		return errors.New("this agent is not enrolled and no enrolment token was given. " +
			"Create one in the panel (Servers -> Add agent) and start the agent with " +
			"VKAI_AGENT_ENROLMENT_TOKEN set to it")
	}
	logger.Printf("not enrolled yet; exchanging the enrolment token for a certificate")
	if err := manager.Enrol(cfg.EnrolmentToken, cfg.Hostname, AgentVersion); err != nil {
		return fmt.Errorf("enrolment failed: %w", err)
	}
	return nil
}

// statusLoop reports in on a timer. The report is signed with the agent's
// private key; there is no shared secret in it and none in the headers.
func statusLoop(ctx context.Context, cfg Config, manager *pki.Manager, logger *log.Logger) {
	ticker := time.NewTicker(cfg.StatusInterval)
	defer ticker.Stop()

	report := func() {
		payload := map[string]any{
			"agent_version": AgentVersion,
			"hostname":      cfg.Hostname,
			"timestamp":     time.Now().UTC(),
			"system_info":   ops.CollectSystemInfo(ctx, nil),
			"metrics":       ops.CollectMetrics(ctx, nil),
		}
		if err := manager.StatusReport(payload); err != nil {
			logger.Printf("status report failed: %v", err)
		}
	}
	report()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}

// renewalLoop rotates the certificate well before it expires. A failed
// rotation is logged and retried on the next tick: the certificate in hand
// stays valid, and the panel keeps accepting it through the overlap window, so
// a rotation can fail repeatedly without locking this agent out.
func renewalLoop(ctx context.Context, cfg Config, manager *pki.Manager, logger *log.Logger) {
	ticker := time.NewTicker(renewalCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !manager.NeedsRenewal() {
				continue
			}
			if err := manager.Renew(cfg.Hostname); err != nil {
				if errors.Is(err, pki.ErrTrustAnchorChanged) {
					// Not a transient fault and not something a retry fixes:
					// something answered a renewal with a certificate authority
					// that is not the one this agent was enrolled against.
					// Nothing was written; the identity in hand still works.
					logger.Printf("SECURITY: a renewal tried to change this agent's certificate authority and was "+
						"refused. Nothing was written and the current certificate (expires %s) is still in use. "+
						"If an operator rebuilt the panel CA, run `%s re-enrol` with a fresh enrolment token; "+
						"otherwise treat this as an attempt to take the agent over: %v",
						manager.NotAfter().Format(time.RFC3339), AgentName, err)
					continue
				}
				logger.Printf("certificate renewal failed, will retry in %s (current certificate expires %s): %v",
					renewalCheckInterval, manager.NotAfter().Format(time.RFC3339), err)
			}
		}
	}
}

// peerName names the verified caller for the operation log.
func peerName(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	leaf := r.TLS.PeerCertificates[0]
	return fmt.Sprintf("%s serial=%s", leaf.Subject.CommonName, strings.ToLower(leaf.SerialNumber.Text(16)))
}

// panelHTTPClient builds the client used for enrolment, renewal and status.
//
// This is the panel's public HTTPS certificate, which is a separate question
// from the agent CA: the panel serves the browser UI with it. By default the
// system trust store decides. An operator whose panel uses a self-signed
// certificate points VKAI_PANEL_CA_FILE at it. Turning verification off
// entirely is possible and is logged as the risk it is.
func panelHTTPClient(cfg Config) (*http.Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	switch {
	case cfg.PanelInsecure:
		tlsConfig.InsecureSkipVerify = true
	case cfg.PanelCAFile != "":
		pem, err := os.ReadFile(cfg.PanelCAFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read VKAI_PANEL_CA_FILE %s: %w", cfg.PanelCAFile, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("%s holds no PEM certificate", cfg.PanelCAFile)
		}
		tlsConfig.RootCAs = pool
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

// env returns the first non-empty value among the given variables. The VKAI_
// name is listed first and the pre-whitelabel name after it, so an agent that
// was deployed with the old environment file keeps running after an upgrade.
func env(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

func envBool(names ...string) bool {
	switch strings.ToLower(env(names...)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func loadConfig() (Config, error) {
	cfg := Config{
		PanelURL:       strings.TrimRight(env("VKAI_PANEL_URL", "PANEL_URL"), "/"),
		StateDir:       env("VKAI_AGENT_STATE_DIR"),
		EnrolmentToken: env("VKAI_AGENT_ENROLMENT_TOKEN", "VKAI_ENROLMENT_TOKEN"),
		AllowRawExec:   envBool("VKAI_AGENT_ALLOW_RAW_EXEC"),
		PanelCAFile:    env("VKAI_PANEL_CA_FILE"),
		PanelInsecure:  envBool("VKAI_PANEL_TLS_INSECURE"),
		StatusInterval: defaultStatusInterval,
	}
	if cfg.PanelURL == "" {
		return cfg, errors.New("VKAI_PANEL_URL is required: it is the base URL of the VKAI Panel this agent belongs to")
	}
	if cfg.StateDir == "" {
		cfg.StateDir = defaultStateDir
	}
	cfg.StateDir = filepath.Clean(cfg.StateDir)

	port := defaultListenPort
	if raw := env("VKAI_AGENT_PORT", "AGENT_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return cfg, fmt.Errorf("VKAI_AGENT_PORT=%q is not a port number", raw)
		}
		port = parsed
	}
	// The listen address is bindable to one interface, because a control
	// channel does not need to be reachable from the Internet when the panel is
	// on the same private network.
	host := env("VKAI_AGENT_BIND")
	cfg.ListenAddr = fmt.Sprintf("%s:%d", host, port)

	if raw := env("VKAI_AGENT_STATUS_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 5*time.Second {
			return cfg, fmt.Errorf("VKAI_AGENT_STATUS_INTERVAL=%q is not a duration of at least 5s", raw)
		}
		cfg.StatusInterval = parsed
	}

	cfg.Hostname = env("VKAI_AGENT_HOSTNAME")
	if cfg.Hostname == "" {
		name, err := os.Hostname()
		if err != nil {
			return cfg, fmt.Errorf("cannot determine this host's name: %w", err)
		}
		cfg.Hostname = name
	}
	return cfg, nil
}
