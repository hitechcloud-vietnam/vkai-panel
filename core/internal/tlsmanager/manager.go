package tlsmanager

// The panel's TLS certificate lifecycle.
//
// The panel answers HTTPS on its own port from its first request, in every
// mode. That single requirement shapes everything here:
//
//   - the live certificate sits in an atomic pointer behind
//     tls.Config.GetCertificate, so a renewal swaps it in place. No listener is
//     rebuilt, no connection is dropped and no restart is needed - which
//     matters far more with Let's Encrypt's six-day short-lived profile (the
//     only one that issues for a bare IP address) than it ever did at ninety
//     days;
//   - startup never waits on a certificate authority. A locally generated pair
//     is loaded first and the ACME order runs behind the already-listening
//     server, so an unreachable CA delays a trusted certificate, never the
//     panel;
//   - every failure keeps the certificate that is already on the wire. The
//     only thing that can stop the panel starting is being unable to produce
//     any certificate at all, which means the filesystem is broken.
//
// What this package deliberately does not know: anything about ACME beyond the
// three declarations in client.go. internal/acme owns the protocol.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Certificate sources, as reported to the operator. These describe what is on
// the wire right now, which is not always what was configured: an operator who
// asked for Let's Encrypt and got the fallback needs to be told so.
const (
	SourceSelfSigned            = "self-signed"
	SourceSelfSignedFallback    = "self-signed (fallback)"
	SourceCustomFile            = "custom file"
	SourceLetsEncryptStaging    = "letsencrypt (staging)"
	SourceLetsEncryptProduction = "letsencrypt (production)"
)

const (
	// DefaultCheckInterval is how often the renewal check runs. Hourly is far
	// more often than a ninety day certificate needs and about right for a six
	// day one, and the check itself is a comparison against a timestamp.
	DefaultCheckInterval = time.Hour

	// DefaultIssueTimeout bounds one issuance attempt end to end. A stuck order
	// must not hold the renewal loop past the next tick.
	DefaultIssueTimeout = 5 * time.Minute

	// minRenewLead / maxRenewLead bound the renewal window computed from the
	// certificate's own lifetime. A third of the lifetime leaves two full
	// retry windows before expiry at any duration: thirty days on a ninety day
	// certificate, two days on a six day one.
	minRenewLead = time.Hour
	maxRenewLead = 30 * 24 * time.Hour
)

// Options configures a Manager. Only Config is required.
type Options struct {
	// Config is the live panel access configuration. The manager reads it
	// throughout its life and writes to it only before Start returns; see
	// persistLocked for why.
	Config *config.PanelAccessConfig

	Logger *zap.Logger

	// Client and Solver override the ACME client and the challenge solver.
	// Both are nil in production, where the client comes from newACMEClient and
	// a fresh HTTP-01 solver is built per attempt so port 80 is held for no
	// longer than one order.
	Client Client
	Solver Solver

	CheckInterval time.Duration
	IssueTimeout  time.Duration

	// Now is the clock, injectable so renewal windows are testable.
	Now func() time.Time
}

// Manager owns the certificate the panel serves and keeps it valid.
type Manager struct {
	cfg *config.PanelAccessConfig
	log *zap.Logger

	client Client
	solver Solver

	checkInterval time.Duration
	issueTimeout  time.Duration
	now           func() time.Time

	// current is read on every TLS handshake and written on every renewal,
	// which is exactly what an atomic pointer is for: handshakes never block
	// behind an issuance, and an issuance never has to wait for idle sockets.
	current    atomic.Pointer[tls.Certificate]
	source     atomic.Pointer[string]
	selfSigned atomic.Bool
	ident      atomic.Pointer[config.PanelIdentifier]
	identErr   atomic.Pointer[string]
	lastErr    atomic.Pointer[string]

	// mu serialises issuance, disk writes and state-file updates against each
	// other. It is never held during a handshake.
	mu            sync.Mutex
	customModTime time.Time

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New validates the options and resolves the ACME identifier, so a caller can
// log the startup decision before anything is issued. It performs no I/O
// beyond reading the machine's own interface addresses.
func New(opts Options) (*Manager, error) {
	if opts.Config == nil {
		return nil, errors.New("tlsmanager: panel access config is required")
	}

	m := &Manager{
		cfg:           opts.Config,
		log:           opts.Logger,
		client:        opts.Client,
		solver:        opts.Solver,
		checkInterval: opts.CheckInterval,
		issueTimeout:  opts.IssueTimeout,
		now:           opts.Now,
		stop:          make(chan struct{}),
	}
	if m.log == nil {
		m.log = zap.NewNop()
	}
	if m.checkInterval <= 0 {
		m.checkInterval = DefaultCheckInterval
	}
	if m.issueTimeout <= 0 {
		m.issueTimeout = DefaultIssueTimeout
	}
	if m.now == nil {
		m.now = time.Now
	}

	// Every mode needs somewhere to read the certificate from, including the
	// self-signed fallback that a failing custom or ACME mode lands on.
	if strings.TrimSpace(m.cfg.TLS.CertFile) == "" {
		m.cfg.TLS.CertFile = filepath.Join(config.PanelSSLDir(), "panel.crt")
	}
	if strings.TrimSpace(m.cfg.TLS.KeyFile) == "" {
		m.cfg.TLS.KeyFile = filepath.Join(config.PanelSSLDir(), "panel.key")
	}

	m.storeString(&m.source, m.cfg.TLSMode())
	if m.cfg.TLSMode() == config.TLSModeLetsEncrypt {
		m.resolveIdentifier()
	}

	return m, nil
}

// Start makes a certificate available and launches the renewal loop. It returns
// only when the panel can actually serve TLS, and it returns an error only when
// no certificate could be produced by any means.
func (m *Manager) Start(ctx context.Context) error {
	if !m.cfg.TLS.Enabled {
		return nil
	}

	if err := m.bootstrap(); err != nil {
		return err
	}

	if m.cfg.TLSMode() == config.TLSModeLetsEncrypt && m.selfSigned.Load() {
		m.log.Info("panel TLS: serving the locally generated certificate while the ACME order runs in the background",
			zap.String("identifier", m.identifierString()),
			zap.String("profile", m.Profile()))
	}

	m.wg.Add(1)
	go m.loop(ctx)

	return nil
}

// Stop ends the renewal loop and waits for an in-flight issuance to finish, so
// no goroutine outlives the process shutdown holding port 80.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
	m.wg.Wait()
}

// TLSConfig is what http.Server serves with. The certificate is resolved per
// handshake, which is what makes an in-place renewal invisible.
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: m.GetCertificate,
	}
}

// GetCertificate implements tls.Config.GetCertificate.
func (m *Manager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if cert := m.current.Load(); cert != nil {
		return cert, nil
	}
	return nil, errors.New("tlsmanager: no certificate has been loaded yet")
}

// Certificate is the pair currently being served, or nil before Start.
func (m *Manager) Certificate() *tls.Certificate { return m.current.Load() }

// Source names what produced the certificate on the wire.
func (m *Manager) Source() string { return m.loadString(&m.source) }

// Profile is the ACME profile an order would carry. Empty outside ACME mode.
func (m *Manager) Profile() string {
	if m.cfg.TLSMode() != config.TLSModeLetsEncrypt {
		return ""
	}
	id := m.ident.Load()
	if id == nil {
		return m.cfg.ACMEProfileFor("")
	}
	return m.cfg.ACMEProfileFor(id.Type)
}

// Info is the startup and status summary: the mode that was configured, the
// identifier and profile that were derived from it, and what is actually being
// served.
type Info struct {
	Mode           string
	Source         string
	IdentifierType string
	Identifier     string
	Profile        string
	Staging        bool
	NotAfter       time.Time
	LastError      string
}

// Info snapshots the current decision for logging and for the UI.
func (m *Manager) Info() Info {
	info := Info{
		Mode:      m.cfg.TLSMode(),
		Source:    m.Source(),
		Profile:   m.Profile(),
		Staging:   m.cfg.TLS.ACME.UseStaging,
		LastError: m.loadString(&m.lastErr),
	}
	if id := m.ident.Load(); id != nil {
		info.IdentifierType, info.Identifier = id.Type, id.Value
	}
	if info.LastError == "" {
		info.LastError = m.loadString(&m.identErr)
	}
	if cert := m.current.Load(); cert != nil && cert.Leaf != nil {
		info.NotAfter = cert.Leaf.NotAfter
	}
	return info
}

// Refresh runs one check: renew, reissue or reload, whichever the mode calls
// for. The renewal loop calls it hourly; it is exported so an operator-facing
// "renew now" action and the tests can drive the same code path.
func (m *Manager) Refresh(ctx context.Context) {
	switch m.cfg.TLSMode() {
	case config.TLSModeLetsEncrypt:
		m.refreshACME(ctx)
	case config.TLSModeSelfSigned:
		m.refreshSelfSigned()
	default:
		m.refreshCustom()
	}
}

// loop is the renewal ticker. The first pass runs immediately: Start returns
// without waiting for a CA, so this is where a missing or due certificate is
// actually ordered.
func (m *Manager) loop(ctx context.Context) {
	defer m.wg.Done()

	m.Refresh(ctx)

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-ticker.C:
			m.Refresh(ctx)
		}
	}
}

// bootstrap loads whatever certificate already exists, generating a self-signed
// pair when there is none. It runs on the caller's goroutine before the renewal
// loop starts, which is why it may write to the shared config.
func (m *Manager) bootstrap() error {
	certFile, keyFile, err := m.cfg.EnsureTLSMaterial()
	if err == nil {
		if err = m.adopt(certFile, keyFile); err == nil {
			m.cfg.CertSource = m.Source()
			m.log.Info("panel TLS: certificate loaded",
				zap.String("mode", m.cfg.TLSMode()),
				zap.String("source", m.Source()),
				zap.String("cert_file", certFile))
			return nil
		}
	}

	// Getting here means the configured certificate is missing or corrupt: a
	// custom pair that was never installed, or a file truncated by a full disk.
	// The panel still has to come up, and it still has to come up on HTTPS -
	// falling back to plain HTTP would put the login form in cleartext, which
	// is a worse outcome than a browser warning.
	m.log.Error("panel TLS: the configured certificate is unusable, falling back to a locally generated one",
		zap.String("mode", m.cfg.TLSMode()), zap.Error(err))

	cert, certFile, keyFile, genErr := m.generateSelfSigned()
	if genErr != nil {
		return fmt.Errorf("tlsmanager: no certificate could be produced: %w", genErr)
	}
	m.cfg.TLS.CertFile, m.cfg.TLS.KeyFile = certFile, keyFile
	m.install(cert, SourceSelfSignedFallback)
	m.storeString(&m.lastErr, err.Error())
	m.cfg.CertSource = m.Source()

	return nil
}

// refreshACME renews when the certificate is due, and issues when the panel is
// still on the self-signed bootstrap.
func (m *Manager) refreshACME(ctx context.Context) {
	cert := m.current.Load()
	if cert != nil && !m.selfSigned.Load() && !m.due(cert.Leaf) {
		return
	}
	m.issue(ctx)
}

// refreshSelfSigned reissues the local certificate before it expires and when
// the set of hosts the panel answers on has changed - a machine that changed
// address otherwise keeps presenting a certificate for the old one, and every
// visit becomes a warning operators learn to click through.
func (m *Manager) refreshSelfSigned() {
	cert := m.current.Load()
	if cert != nil && !m.due(cert.Leaf) && m.certStillMatches() {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// EnsureTLSMaterial only regenerates a pair that has already expired or has
	// stopped covering the hosts. Removing it first is what turns "due for
	// renewal" into "regenerate now", well before the expiry that would
	// otherwise be the trigger.
	if cert != nil && m.due(cert.Leaf) {
		_ = os.Remove(m.cfg.TLS.CertFile)
		_ = os.Remove(m.cfg.TLS.KeyFile)
	}

	fresh, certFile, keyFile, err := m.generateSelfSigned()
	if err != nil {
		m.log.Error("panel TLS: could not reissue the self-signed certificate, keeping the current one",
			zap.Error(err))
		return
	}
	// A reissue must not quietly upgrade the story the banner tells. When the
	// operator asked for "custom" or "letsencrypt" and the panel is only serving
	// a generated pair because that failed, it is still serving a fallback -
	// reporting it as a plain self-signed certificate would read as a choice
	// rather than as the symptom of a certificate that could not be loaded or
	// issued, and the operator would stop looking for the cause.
	source := SourceSelfSigned
	if m.cfg.TLSMode() != config.TLSModeSelfSigned {
		source = SourceSelfSignedFallback
	}
	m.install(fresh, source)
	m.log.Info("panel TLS: self-signed certificate reissued",
		zap.String("cert_file", certFile), zap.String("key_file", keyFile),
		zap.String("source", source))
}

// refreshCustom picks up a certificate an operator (or their own automation)
// replaced on disk, without asking them to restart the panel to apply it.
func (m *Manager) refreshCustom() {
	certFile, keyFile := m.cfg.TLS.CertFile, m.cfg.TLS.KeyFile

	info, err := os.Stat(certFile)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !info.ModTime().After(m.customModTime) {
		return
	}
	if err := m.adopt(certFile, keyFile); err != nil {
		m.log.Error("panel TLS: the custom certificate changed on disk but does not load, keeping the current one",
			zap.String("cert_file", certFile), zap.Error(err))
		return
	}
	m.log.Info("panel TLS: reloaded the custom certificate after it changed on disk",
		zap.String("cert_file", certFile))
}

// issue runs one full ACME order. Every exit path other than success leaves the
// certificate that is already being served untouched.
func (m *Manager) issue(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, err := m.resolveIdentifier()
	if err != nil {
		// Not a retryable failure: a private, loopback, link-local or CGNAT
		// address cannot be validated no matter how often it is attempted, and
		// each attempt spends a rate limit that is shared with every future
		// order from this account.
		m.log.Warn("panel TLS: not attempting ACME issuance",
			zap.Error(err), zap.String("serving", m.Source()))
		m.failLocked(err)
		return
	}
	profile := m.cfg.ACMEProfileFor(id.Type)

	client := m.client
	if client == nil {
		client, err = newACMEClient(strings.TrimSpace(m.cfg.TLS.ACME.Email), m.cfg.TLS.ACME.UseStaging)
		if err != nil {
			m.log.Error("panel TLS: no ACME client available, keeping the current certificate",
				zap.Error(err), zap.String("serving", m.Source()))
			m.failLocked(err)
			return
		}
	}

	solver := m.solver
	if solver == nil {
		// A fresh solver per attempt: it may bind port 80, and port 80 belongs
		// to the customer websites for every second it is not needed here.
		own := NewHTTP01Solver(HTTP01Options{Logger: m.log})
		defer own.Close()
		solver = own
	}

	m.log.Info("panel TLS: requesting a certificate",
		zap.String("identifier", id.String()),
		zap.String("profile", profile),
		zap.Bool("staging", m.cfg.TLS.ACME.UseStaging))

	issueCtx, cancel := context.WithTimeout(ctx, m.issueTimeout)
	defer cancel()

	chainPEM, keyPEM, err := client.Obtain(issueCtx,
		[]Identifier{{Type: id.Type, Value: id.Value}}, profile, solver)
	if err != nil {
		m.log.Error("panel TLS: certificate issuance failed, keeping the certificate already on the wire",
			zap.String("identifier", id.String()),
			zap.String("profile", profile),
			zap.String("serving", m.Source()),
			zap.Error(err))
		m.failLocked(err)
		return
	}

	pair, err := parsePair(chainPEM, keyPEM)
	if err != nil {
		m.log.Error("panel TLS: the issued certificate does not load, keeping the current one",
			zap.Error(err))
		m.failLocked(err)
		return
	}

	// Install before persisting. A disk that refuses the write is a problem for
	// the next restart; refusing to use a valid certificate over it would be a
	// problem right now.
	m.install(pair, m.acmeSource())
	if err := writePair(m.cfg.TLS.CertFile, m.cfg.TLS.KeyFile, chainPEM, keyPEM); err != nil {
		m.log.Error("panel TLS: serving the new certificate but could not store it; it will have to be reissued after a restart",
			zap.String("cert_file", m.cfg.TLS.CertFile), zap.Error(err))
	}

	m.storeString(&m.lastErr, "")
	m.persistLocked(m.Source(), m.now(), nil)

	m.log.Info("panel TLS: certificate issued",
		zap.String("identifier", id.String()),
		zap.String("profile", profile),
		zap.String("source", m.Source()),
		zap.Time("not_after", pair.Leaf.NotAfter),
		zap.Time("renew_after", renewalTime(pair.Leaf)))
}

// failLocked records an issuance failure. The caller holds m.mu.
func (m *Manager) failLocked(err error) {
	m.storeString(&m.lastErr, err.Error())
	m.persistLocked(m.Source(), time.Time{}, err)
}

// persistLocked writes the display state to the state file.
//
// It writes through a copy of the configuration rather than mutating the live
// one. The live config is read without a lock by everything that was handed a
// pointer to it at startup - the banner, the access guard - and a background
// renewal writing into it would be a data race for the sake of a field nobody
// reads twice. The state file is the durable copy, and that is what the CLI and
// the next boot read.
func (m *Manager) persistLocked(source string, issuedAt time.Time, issueErr error) {
	if m.cfg.StateFile == "" {
		return
	}
	saved := m.configCopy()
	saved.CertSource = source
	saved.RecordACMEResult(issuedAt, issueErr)
	if err := saved.Save(); err != nil {
		// A read-only /etc must not turn into a failed renewal.
		m.log.Debug("panel TLS: could not update the state file", zap.Error(err))
	}
}

// configCopy is a detached copy safe to mutate off the startup goroutine.
func (m *Manager) configCopy() config.PanelAccessConfig {
	clone := *m.cfg
	// Both slices are append targets in config; sharing a backing array with
	// the live config would let a copy write into it.
	clone.Generated = nil
	clone.EnvOverrides = nil
	return clone
}

// generateSelfSigned produces a local pair on disk and loads it. It goes
// through a copy of the config with the mode forced, so the same well-tested
// generator serves both the self-signed mode and the fallback out of the other
// two.
func (m *Manager) generateSelfSigned() (*tls.Certificate, string, string, error) {
	clone := m.configCopy()
	clone.TLS.Enabled = true
	clone.TLS.Mode = config.TLSModeSelfSigned
	clone.TLS.SelfSigned = true

	certFile, keyFile, err := clone.EnsureTLSMaterial()
	if err != nil {
		return nil, "", "", err
	}
	pair, err := loadPair(certFile, keyFile)
	if err != nil {
		return nil, "", "", err
	}
	return pair, certFile, keyFile, nil
}

// certStillMatches reports whether the served self-signed certificate still
// covers every host the panel answers on.
func (m *Manager) certStillMatches() bool {
	cert := m.current.Load()
	if cert == nil || cert.Leaf == nil {
		return false
	}
	host := m.cfg.AccessHost()
	if host == "" {
		return true
	}
	return cert.Leaf.VerifyHostname(strings.Trim(host, "[]")) == nil
}

// adopt loads a pair from disk and installs it.
func (m *Manager) adopt(certFile, keyFile string) error {
	pair, err := loadPair(certFile, keyFile)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(certFile); statErr == nil {
		m.customModTime = info.ModTime()
	}
	m.install(pair, m.classify(pair.Leaf))
	return nil
}

// install swaps the served certificate. This is the only writer of m.current.
func (m *Manager) install(cert *tls.Certificate, source string) {
	m.current.Store(cert)
	m.storeString(&m.source, source)
	m.selfSigned.Store(isSelfSigned(cert.Leaf))
}

// classify names the source of a certificate found on disk. A self-signed leaf
// is recognised from the certificate itself rather than from the configuration,
// because the configured mode says what was asked for, not what is there: in
// letsencrypt mode the file on disk is the bootstrap certificate until the
// first order completes.
func (m *Manager) classify(leaf *x509.Certificate) string {
	if isSelfSigned(leaf) {
		return SourceSelfSigned
	}
	if m.cfg.TLSMode() == config.TLSModeLetsEncrypt {
		return m.acmeSource()
	}
	return SourceCustomFile
}

func (m *Manager) acmeSource() string {
	if m.cfg.TLS.ACME.UseStaging {
		return SourceLetsEncryptStaging
	}
	return SourceLetsEncryptProduction
}

// resolveIdentifier derives and caches the ACME identifier, so the startup log
// can report it before anything is ordered.
func (m *Manager) resolveIdentifier() (config.PanelIdentifier, error) {
	id, err := m.cfg.ACMEIdentifier()
	if err != nil {
		m.ident.Store(nil)
		m.storeString(&m.identErr, err.Error())
		return config.PanelIdentifier{}, err
	}
	m.ident.Store(&id)
	m.storeString(&m.identErr, "")
	return id, nil
}

func (m *Manager) identifierString() string {
	if id := m.ident.Load(); id != nil {
		return id.String()
	}
	return ""
}

// due reports whether the renewal window has opened.
func (m *Manager) due(leaf *x509.Certificate) bool {
	if leaf == nil {
		return true
	}
	return !m.now().Before(renewalTime(leaf))
}

func (m *Manager) storeString(p *atomic.Pointer[string], value string) {
	v := value
	p.Store(&v)
}

func (m *Manager) loadString(p *atomic.Pointer[string]) string {
	if v := p.Load(); v != nil {
		return *v
	}
	return ""
}

// renewalTime is when renewal should start, derived from the certificate's own
// lifetime rather than from a constant.
//
// A fixed "renew 30 days before expiry" is wrong in both directions here: it
// never fires on a six-day short-lived certificate, which is the only kind
// Let's Encrypt issues for an IP address, and it fires immediately on a
// one-week custom certificate. A third of the lifetime always leaves room for
// two more attempts before expiry.
func renewalTime(leaf *x509.Certificate) time.Time {
	lifetime := leaf.NotAfter.Sub(leaf.NotBefore)
	lead := lifetime / 3
	if lead < minRenewLead {
		lead = minRenewLead
	}
	if lead > maxRenewLead {
		lead = maxRenewLead
	}
	return leaf.NotAfter.Add(-lead)
}

// isSelfSigned reports whether a certificate signed itself, which is how the
// locally generated bootstrap pair is told apart from a CA-issued one without
// trusting what the configuration claims.
func isSelfSigned(leaf *x509.Certificate) bool {
	if leaf == nil {
		return false
	}
	if !bytes.Equal(leaf.RawIssuer, leaf.RawSubject) {
		return false
	}
	return leaf.CheckSignatureFrom(leaf) == nil
}

// loadPair reads a certificate and key from disk with the leaf parsed, which
// the renewal check and the source classification both need.
func loadPair(certFile, keyFile string) (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return withLeaf(&pair)
}

func parsePair(chainPEM, keyPEM []byte) (*tls.Certificate, error) {
	pair, err := tls.X509KeyPair(chainPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return withLeaf(&pair)
}

func withLeaf(pair *tls.Certificate) (*tls.Certificate, error) {
	if len(pair.Certificate) == 0 {
		return nil, errors.New("tlsmanager: certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	pair.Leaf = leaf
	return pair, nil
}

// writePair stores an issued certificate. Both files are replaced through a
// temporary file and a rename, so a crash mid-write cannot leave a certificate
// that no longer matches its key - which would lock the panel out of TLS on the
// next restart, exactly when nobody is watching.
func writePair(certFile, keyFile string, chainPEM, keyPEM []byte) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o750); err != nil {
		return err
	}
	// The key is never group or world readable; the chain is public.
	if err := writeFileAtomic(keyFile, keyPEM, 0o600); err != nil {
		return err
	}
	return writeFileAtomic(certFile, chainPEM, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
