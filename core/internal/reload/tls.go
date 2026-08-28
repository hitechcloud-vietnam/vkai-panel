package reload

// The panel's certificate, changed while it is being served.
//
// internal/tlsmanager already solves the hard half of this: it holds the live
// certificate in an atomic pointer behind tls.Config.GetCertificate, so a
// renewal is a pointer swap and no listener is ever rebuilt. What it does not
// do - deliberately, because it is built once at start-up - is change its mind
// about which mode it is in. A manager built for "letsencrypt" keeps an ACME
// renewal loop running for the life of the process, and that loop rewrites the
// certificate files on a timer.
//
// So a mode change is a manager change. This switch keeps exactly one manager
// alive, hands the listener a tls.Config that resolves through whichever
// manager is current, and replaces the manager when the mode, the paths or the
// ACME settings move. Two consequences matter and are the reason this exists:
//
//	letsencrypt -> custom  the renewal loop is stopped. Without this, a
//	                       certificate an operator pasted would be quietly
//	                       overwritten by a renewal weeks later - a defect
//	                       nobody notices until they wonder why the certificate
//	                       changed back on its own;
//	custom -> letsencrypt  the renewal loop is started again, so the panel
//	                       resumes renewing instead of drifting to an expiry
//	                       that locks its operator out of the tool they would
//	                       fix it with.

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/tlsmanager"
)

// TLSSwitch owns the certificate manager generation the panel serves from.
type TLSSwitch struct {
	ctx    context.Context
	logger *zap.Logger

	live atomic.Pointer[tlsGeneration]
}

// tlsGeneration is one manager and the configuration it was built for.
type tlsGeneration struct {
	cfg *config.PanelAccessConfig
	mgr *tlsmanager.Manager
}

// NewTLSSwitch builds and starts the first certificate manager.
//
// ctx bounds every manager this switch will ever create: cancelling it ends the
// renewal loops at process shutdown.
func NewTLSSwitch(ctx context.Context, cfg *config.PanelAccessConfig, logger *zap.Logger) (*TLSSwitch, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &TLSSwitch{ctx: ctx, logger: logger}

	generation, err := s.build(cfg)
	if err != nil {
		return nil, err
	}
	s.live.Store(generation)

	// The manager writes the certificate paths and the resolved source back into
	// the configuration it was given during Start. Copy them onto the caller's
	// configuration so the banner and the settings endpoint report what is being
	// served rather than what was asked for.
	cfg.TLS.CertFile = generation.cfg.TLS.CertFile
	cfg.TLS.KeyFile = generation.cfg.TLS.KeyFile
	cfg.CertSource = generation.cfg.CertSource

	return s, nil
}

// build creates a manager for a configuration and starts it. On return the
// manager is either serving a certificate or the error explains why no
// certificate could be produced at all.
func (s *TLSSwitch) build(cfg *config.PanelAccessConfig) (*tlsGeneration, error) {
	// The manager mutates the configuration it is handed. It gets a clone, so
	// the live snapshot every reader holds stays immutable.
	owned := Clone(cfg)

	mgr, err := tlsmanager.New(tlsmanager.Options{Config: owned, Logger: s.logger})
	if err != nil {
		return nil, err
	}
	if err := mgr.Start(s.ctx); err != nil {
		return nil, err
	}

	generation := &tlsGeneration{cfg: owned, mgr: mgr}

	if err := verifyServedCertificate(owned, mgr); err != nil {
		mgr.Stop()
		return nil, err
	}

	return generation, nil
}

// verifyServedCertificate is the proof that a pasted certificate is actually on
// the wire.
//
// The manager falls back to a generated self-signed pair whenever the
// configured certificate cannot be loaded, which is right for a panel that must
// answer HTTPS from its first request and wrong as a silent answer to "save
// this certificate". In custom mode the file on disk is the operator's explicit
// instruction, so the certificate being served is compared against it and a
// mismatch is an error the caller can roll back on.
func verifyServedCertificate(cfg *config.PanelAccessConfig, mgr *tlsmanager.Manager) error {
	if !cfg.TLS.Enabled || cfg.TLSMode() != config.TLSModeCustom {
		return nil
	}

	served := mgr.Certificate()
	if served == nil || served.Leaf == nil {
		return fmt.Errorf("the certificate in %s was not loaded and the panel would be serving something else", cfg.TLS.CertFile)
	}

	expected, err := config.InspectCertificateFile(cfg.TLS.CertFile)
	if err != nil {
		return fmt.Errorf("cannot read back the certificate written to %s: %w", cfg.TLS.CertFile, err)
	}

	if actual := fingerprintOf(served.Leaf.Raw); actual != expected.Fingerprint {
		return fmt.Errorf(
			"the panel is serving a different certificate (%s) from the one that was saved (%s), which means the saved pair could not be loaded",
			actual, expected.Fingerprint)
	}

	return nil
}

// TLSConfig is the configuration the listener holds for the whole life of the
// process. It never has to be rebuilt: GetCertificate resolves through whatever
// manager is current at the moment of the handshake.
func (s *TLSSwitch) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: s.GetCertificate,
	}
}

// GetCertificate implements tls.Config.GetCertificate.
func (s *TLSSwitch) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	generation := s.live.Load()
	if generation == nil {
		return nil, fmt.Errorf("reload: the panel has no certificate manager")
	}
	return generation.mgr.GetCertificate(hello)
}

// Info summarises what is being served, for the startup log and the UI.
func (s *TLSSwitch) Info() tlsmanager.Info {
	if generation := s.live.Load(); generation != nil {
		return generation.mgr.Info()
	}
	return tlsmanager.Info{}
}

// Source names what produced the certificate currently on the wire.
func (s *TLSSwitch) Source() string {
	if generation := s.live.Load(); generation != nil {
		return generation.mgr.Source()
	}
	return ""
}

// Reload rebuilds the certificate manager from the configuration it is already
// running, so a certificate that was replaced on disk is served immediately.
//
// It is the path a reissue takes: the files change, no setting does, and the
// diff-driven reload would rightly find nothing to do. The previous manager is
// kept running until the new one has produced a certificate, so a reissue that
// fails leaves the panel serving the certificate it already had.
func (s *TLSSwitch) Reload() error {
	current := s.live.Load()
	if current == nil {
		return fmt.Errorf("reload: the panel has no certificate manager")
	}
	if !current.cfg.TLS.Enabled {
		return nil
	}

	generation, err := s.build(current.cfg)
	if err != nil {
		return err
	}

	s.live.Store(generation)
	go current.mgr.Stop()

	s.logger.Info("panel certificate manager rebuilt; the next handshake uses the certificate now on disk",
		zap.String("mode", generation.cfg.TLSMode()),
		zap.String("source", generation.mgr.Source()))

	return nil
}

// Stop ends the live renewal loop, at process shutdown.
func (s *TLSSwitch) Stop() {
	if generation := s.live.Load(); generation != nil {
		generation.mgr.Stop()
	}
}

// Name implements Applier.
func (s *TLSSwitch) Name() string { return "TLS certificate" }

// Prepare builds and starts the new manager without publishing it. A
// certificate that cannot be loaded fails here, while the old manager is still
// serving the certificate that works.
func (s *TLSSwitch) Prepare(next *config.PanelAccessConfig) (Staged, error) {
	current := s.live.Load()
	if current != nil && !tlsAffected(current.cfg, next) {
		return nil, nil
	}

	if !next.TLS.Enabled {
		// Nothing to build: the listener for the new configuration is plain
		// HTTP. The old manager is retired so its renewal loop stops.
		return &tlsStaged{sw: s, previous: current, next: nil}, nil
	}

	generation, err := s.build(next)
	if err != nil {
		return nil, err
	}

	return &tlsStaged{sw: s, previous: current, next: generation}, nil
}

// tlsAffected reports whether anything the certificate depends on has moved.
// The domain and the bind address are in the list because both change the names
// a self-signed certificate has to carry and the identifier an ACME order is
// placed for.
func tlsAffected(current, next *config.PanelAccessConfig) bool {
	return current.TLS.Enabled != next.TLS.Enabled ||
		current.TLSMode() != next.TLSMode() ||
		strings.TrimSpace(current.TLS.CertFile) != strings.TrimSpace(next.TLS.CertFile) ||
		strings.TrimSpace(current.TLS.KeyFile) != strings.TrimSpace(next.TLS.KeyFile) ||
		current.TLS.ACME.Email != next.TLS.ACME.Email ||
		current.TLS.ACME.UseStaging != next.TLS.ACME.UseStaging ||
		current.TLS.ACME.Profile != next.TLS.ACME.Profile ||
		current.Domain != next.Domain ||
		current.Bind != next.Bind
}

type tlsStaged struct {
	sw        *TLSSwitch
	previous  *tlsGeneration
	next      *tlsGeneration
	committed bool
}

func (t *tlsStaged) Commit() {
	t.sw.live.Store(t.next)
	t.committed = true
}

// Rollback restores the previous manager and stops the one that was built for
// the change. The previous manager was never stopped, so the certificate it
// holds is still valid and still served.
func (t *tlsStaged) Rollback() {
	if t.committed {
		t.sw.live.Store(t.previous)
		t.committed = false
	}
	if t.next != nil {
		t.next.mgr.Stop()
	}
}

// Retire stops the renewal loop of the generation that has been replaced. This
// is the line that stops an ACME renewal from overwriting a pasted certificate.
func (t *tlsStaged) Retire() {
	if t.previous == nil {
		return
	}
	previous := t.previous
	go previous.mgr.Stop()
}

func (t *tlsStaged) Describe() string {
	from := "off"
	if t.previous != nil && t.previous.cfg.TLS.Enabled {
		from = t.previous.cfg.TLSMode()
	}
	to := "off"
	if t.next != nil && t.next.cfg.TLS.Enabled {
		to = t.next.cfg.TLSMode()
	}
	return "TLS mode " + from + " -> " + to
}

func fingerprintOf(der []byte) string {
	return config.FingerprintDER(der)
}
