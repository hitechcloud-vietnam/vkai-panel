package agentpki

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Roles are written into the certificate subject's OrganizationalUnit. They are
// what stops an agent certificate from being replayed at another agent as if it
// were the panel: both chain to the same CA, only the role differs.
const (
	RoleAgent = "vkai-agent"
	RolePanel = "vkai-panel"
)

// PanelAgentID is the identity of the panel's own client certificate inside the
// store, so that certificate can be rotated and revoked by the same code paths
// as an agent's.
const PanelAgentID = "panel"

const (
	// DefaultCertTTL is how long an issued leaf certificate lives. Short
	// enough that a stolen key is a day's problem rather than a year's, long
	// enough that a panel that is down for a morning does not strand its fleet.
	DefaultCertTTL = 24 * time.Hour

	// DefaultRenewBefore is how long before expiry a holder should renew. At
	// half the lifetime an agent gets twelve further chances to renew, one an
	// hour, before anything expires.
	DefaultRenewBefore = 12 * time.Hour

	// DefaultOverlap is how long a superseded certificate stays acceptable
	// after its replacement was issued. This is what makes a rotation safe: the
	// agent that never received its new certificate is still talking.
	DefaultOverlap = 24 * time.Hour

	// DefaultEnrolmentTTL is how long a one-time enrolment token stays usable.
	// It is a value an operator pastes into an installer within minutes.
	DefaultEnrolmentTTL = 30 * time.Minute

	// caLifetime is the CA's own validity. The CA is not what rotates here;
	// the leaves are. Replacing it means re-enrolling every agent.
	caLifetime = 10 * 365 * 24 * time.Hour

	// clockSkew is the backdating applied to NotBefore, so an agent whose clock
	// is a couple of minutes behind the panel does not reject a fresh
	// certificate as not yet valid.
	clockSkew = 5 * time.Minute

	caCommonName = "VKAI Panel Agent CA"
	organization = "HiTechCloud"

	caCertFile     = "ca.crt"
	caKeyFile      = "ca.key"
	panelCertFile  = "panel-client.crt"
	panelKeyFile   = "panel-client.key"
	storeStateFile = "state.json"

	// minRSABits is the smallest RSA key accepted in a certificate request.
	minRSABits = 2048
)

// Options configures an Authority. Every duration has a default; Dir and Store
// are resolved from the panel layout when they are left empty.
type Options struct {
	// Dir is where the CA material lives. Defaults to <SSLRoot>/agent-pki.
	Dir string

	// Store holds enrolment tokens, issued certificates and the deny list.
	// Defaults to a FileStore inside Dir.
	Store Store

	CertTTL      time.Duration
	RenewBefore  time.Duration
	Overlap      time.Duration
	EnrolmentTTL time.Duration

	// Now exists so the tests can move time. Production leaves it nil.
	Now func() time.Time

	Logger *zap.Logger
}

// Authority is the panel's certificate authority for the agent channel. It is
// safe for concurrent use.
type Authority struct {
	dir          string
	store        Store
	certTTL      time.Duration
	renewBefore  time.Duration
	overlap      time.Duration
	enrolmentTTL time.Duration
	now          func() time.Time
	logger       *zap.Logger

	replay *replayGuard

	// rotateMu serialises rotation of the panel's own certificate. Without it
	// two concurrent handshakes could both decide to rotate and issue two
	// certificates where one was needed.
	rotateMu sync.Mutex

	mu        sync.RWMutex
	caCert    *x509.Certificate
	caKey     *ecdsa.PrivateKey
	caPEM     []byte
	pool      *x509.CertPool
	panelPair *tls.Certificate
}

// New opens the CA at opts.Dir, creating it on first run. It also makes sure the
// panel holds a usable client certificate of its own.
func New(opts Options) (*Authority, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		dir = DefaultDir()
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	a := &Authority{
		dir:          dir,
		store:        opts.Store,
		certTTL:      orDuration(opts.CertTTL, DefaultCertTTL),
		renewBefore:  orDuration(opts.RenewBefore, DefaultRenewBefore),
		overlap:      orDuration(opts.Overlap, DefaultOverlap),
		enrolmentTTL: orDuration(opts.EnrolmentTTL, DefaultEnrolmentTTL),
		now:          opts.Now,
		logger:       logger,
		replay:       newReplayGuard(),
	}
	if a.now == nil {
		a.now = time.Now
	}
	// The directory holds a private key, so it is not group readable.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("agentpki: cannot create %s: %w", dir, err)
	}
	if a.store == nil {
		store, err := NewFileStore(filepath.Join(dir, storeStateFile))
		if err != nil {
			return nil, err
		}
		a.store = store
	}
	if err := a.loadOrCreateCA(); err != nil {
		return nil, err
	}
	if _, err := a.PanelClientCertificate(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

// DefaultDir is where the CA lives: a subdirectory of the panel SSL directory,
// separate from the panel's own web certificate and from the customer site
// certificates, so no renewal of either can ever overwrite the CA key.
func DefaultDir() string { return filepath.Join(sslRoot(), "agent-pki") }

// Store exposes the backing store, for handlers that list or revoke.
func (a *Authority) Store() Store { return a.store }

// Now is the authority's clock.
func (a *Authority) Now() time.Time { return a.now() }

// CertTTL, RenewBefore and Overlap report the configured lifetimes, so a
// response to an agent can tell it when to come back.
func (a *Authority) CertTTL() time.Duration     { return a.certTTL }
func (a *Authority) RenewBefore() time.Duration { return a.renewBefore }
func (a *Authority) Overlap() time.Duration     { return a.overlap }

// CACertPEM is the CA certificate an agent needs in order to verify the panel.
// It is a public key: handing it out discloses nothing.
func (a *Authority) CACertPEM() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]byte(nil), a.caPEM...)
}

// CAFingerprint is the SHA-256 of the CA's SubjectPublicKeyInfo, lowercase hex.
// It is what an enrolment token carries, so the agent can check that the CA
// certificate it is handed is the one the operator meant to give it.
func (a *Authority) CAFingerprint() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return spkiFingerprint(a.caCert)
}

// CAPool is the trust anchor used for both verification directions.
func (a *Authority) CAPool() *x509.CertPool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pool
}

// loadOrCreateCA reads the CA from disk, generating it if this is the first run.
func (a *Authority) loadOrCreateCA() error {
	certPath := filepath.Join(a.dir, caCertFile)
	keyPath := filepath.Join(a.dir, caKeyFile)

	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		cert, key, err := parseCAMaterial(certPEM, keyPEM)
		if err != nil {
			return err
		}
		a.installCA(cert, key, certPEM)
		return nil
	}
	if certErr != nil && !errors.Is(certErr, os.ErrNotExist) {
		return fmt.Errorf("agentpki: cannot read %s: %w", certPath, certErr)
	}
	if keyErr != nil && !errors.Is(keyErr, os.ErrNotExist) {
		return fmt.Errorf("agentpki: cannot read %s: %w", keyPath, keyErr)
	}
	// A certificate without its key, or the other way round, is a half-created
	// CA. Refusing is better than silently issuing from a new one, which would
	// lock out every agent already enrolled against the old one.
	if certErr == nil || keyErr == nil {
		return fmt.Errorf("agentpki: %s holds only one half of the CA; move it aside and re-enrol the agents", a.dir)
	}
	return a.createCA(certPath, keyPath)
}

func (a *Authority) createCA(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("agentpki: cannot generate the CA key: %w", err)
	}
	serial, err := newSerial()
	if err != nil {
		return err
	}
	now := a.now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCommonName,
			Organization: []string{organization},
		},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(caLifetime),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("agentpki: cannot create the CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Both files are 0600. The certificate does not need to be secret, but a
	// uniform mode is one less thing to get wrong, and nothing outside the
	// panel process reads these paths.
	if err := writeSecret(certPath, certPEM); err != nil {
		return err
	}
	if err := writeSecret(keyPath, keyPEM); err != nil {
		return err
	}
	a.installCA(cert, key, certPEM)
	a.logger.Info("Agent PKI: internal CA created",
		zap.String("dir", a.dir),
		zap.String("ca_fingerprint", spkiFingerprint(cert)),
		zap.Time("not_after", cert.NotAfter))
	return nil
}

func (a *Authority) installCA(cert *x509.Certificate, key *ecdsa.PrivateKey, certPEM []byte) {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	a.mu.Lock()
	a.caCert = cert
	a.caKey = key
	a.caPEM = certPEM
	a.pool = pool
	a.mu.Unlock()
}

// ============================================================
// ISSUANCE
// ============================================================

// Issued is one freshly signed certificate, as returned to whoever asked.
type Issued struct {
	AgentID     string
	Serial      string
	Fingerprint string
	CertPEM     []byte
	CAPEM       []byte
	NotBefore   time.Time
	NotAfter    time.Time
	RenewAfter  time.Time
}

// sign issues a leaf for the given role from a certificate request. The private
// key stays with whoever made the request: only a public key ever crosses the
// wire.
func (a *Authority) sign(subjectID, role string, pub crypto.PublicKey) (*x509.Certificate, []byte, error) {
	if err := checkPublicKey(pub); err != nil {
		return nil, nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}
	now := a.now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         subjectID,
			Organization:       []string{organization},
			OrganizationalUnit: []string{role},
		},
		NotBefore: now.Add(-clockSkew),
		NotAfter:  now.Add(a.certTTL),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		// Both usages on both roles: an agent certificate is a server
		// certificate when the panel dials in and a client certificate when the
		// agent calls back to renew.
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		// A name is present so that tooling has something to print, but nothing
		// in this package verifies it: see VerifyAgentPeer.
		DNSNames: []string{subjectID + ".agent.vkai.internal"},
	}

	a.mu.RLock()
	caCert, caKey := a.caCert, a.caKey
	a.mu.RUnlock()

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, pub, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("agentpki: cannot sign the certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// issueFromCSR validates a certificate request and turns it into a record.
func (a *Authority) issueFromCSR(subjectID, role string, csrPEM []byte) (*Issued, CertRecord, error) {
	csr, err := parseCSR(csrPEM)
	if err != nil {
		return nil, CertRecord{}, err
	}
	cert, certPEM, err := a.sign(subjectID, role, csr.PublicKey)
	if err != nil {
		return nil, CertRecord{}, err
	}
	rec, err := recordFor(cert, a.now())
	if err != nil {
		return nil, CertRecord{}, err
	}
	return &Issued{
		AgentID:     subjectID,
		Serial:      rec.Serial,
		Fingerprint: rec.Fingerprint,
		CertPEM:     certPEM,
		CAPEM:       a.CACertPEM(),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		RenewAfter:  cert.NotAfter.Add(-a.renewBefore),
	}, rec, nil
}

// ============================================================
// THE PANEL'S OWN CLIENT CERTIFICATE
// ============================================================

// PanelClientCertificate returns the certificate the panel presents when it
// dials an agent, issuing or rotating it when needed. Rotation writes the new
// pair to disk and marks the old serial superseded rather than revoked, so an
// agent that has not yet heard about the change keeps accepting the old one for
// the overlap window.
func (a *Authority) PanelClientCertificate(ctx context.Context) (*tls.Certificate, error) {
	if pair := a.currentPanelPair(); pair != nil {
		return pair, nil
	}

	a.rotateMu.Lock()
	defer a.rotateMu.Unlock()
	// Re-check: another goroutine may have rotated while this one waited.
	if pair := a.currentPanelPair(); pair != nil {
		return pair, nil
	}

	a.mu.RLock()
	pair := a.panelPair
	a.mu.RUnlock()

	certPath := filepath.Join(a.dir, panelCertFile)
	keyPath := filepath.Join(a.dir, panelKeyFile)

	// A pair on disk that is still well inside its life is adopted rather than
	// replaced, so a panel restart does not churn a certificate per boot.
	if pair == nil {
		if loaded, err := loadPair(certPath, keyPath); err == nil && loaded.Leaf != nil {
			if a.verifyOwnLeaf(loaded.Leaf) == nil && a.now().Before(loaded.Leaf.NotAfter.Add(-a.renewBefore)) {
				a.mu.Lock()
				a.panelPair = loaded
				a.mu.Unlock()
				return loaded, nil
			}
		}
	}
	return a.rotatePanelClient(ctx, certPath, keyPath)
}

// currentPanelPair returns the loaded panel certificate while it is still
// comfortably inside its life, and nil when it is time to rotate.
func (a *Authority) currentPanelPair() *tls.Certificate {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.panelPair == nil || a.panelPair.Leaf == nil {
		return nil
	}
	if a.now().Before(a.panelPair.Leaf.NotAfter.Add(-a.renewBefore)) {
		return a.panelPair
	}
	return nil
}

func (a *Authority) rotatePanelClient(ctx context.Context, certPath, keyPath string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("agentpki: cannot generate the panel client key: %w", err)
	}
	cert, certPEM, err := a.sign(PanelAgentID, RolePanel, &key.PublicKey)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := writeSecret(certPath, certPEM); err != nil {
		return nil, err
	}
	if err := writeSecret(keyPath, keyPEM); err != nil {
		return nil, err
	}

	rec, err := recordFor(cert, a.now())
	if err != nil {
		return nil, err
	}
	if err := a.recordRotation(ctx, PanelAgentID, "", "", RolePanel, rec); err != nil {
		return nil, err
	}

	pair := &tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}
	a.mu.Lock()
	a.panelPair = pair
	a.mu.Unlock()

	a.logger.Info("Agent PKI: panel client certificate issued",
		zap.String("serial", rec.Serial),
		zap.Time("not_after", rec.NotAfter))
	return pair, nil
}

// verifyOwnLeaf checks that a leaf found on disk was signed by the CA that is
// loaded now. After a CA has been replaced, the old panel certificate on disk
// is worthless and must not be adopted.
func (a *Authority) verifyOwnLeaf(leaf *x509.Certificate) error {
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:       a.CAPool(),
		CurrentTime: a.now(),
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err
}

// ============================================================
// RECORD KEEPING
// ============================================================

// recordRotation stores a newly issued certificate as the current one and moves
// the certificate it replaces into the previous slot, stamped with the moment
// it was superseded. That stamp is what the overlap window is measured from.
func (a *Authority) recordRotation(ctx context.Context, agentID, hostname, serverID, role string, rec CertRecord) error {
	now := a.now()
	existing, err := a.store.GetAgent(ctx, agentID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if existing == nil {
		return a.store.PutAgent(ctx, &AgentRecord{
			AgentID:    agentID,
			Hostname:   hostname,
			ServerID:   serverID,
			Role:       role,
			Current:    rec,
			EnrolledAt: now,
			RenewedAt:  now,
		})
	}
	previous := existing.Current
	previous.SupersededAt = &now
	existing.Previous = &previous
	existing.Current = rec
	existing.RenewedAt = now
	if role == RolePanel {
		// The panel reissuing its own certificate clears the revoked mark: the
		// serial that was revoked stays on the deny list, and the new one was
		// never on it. An agent record is not treated this way - Renew refuses
		// a revoked agent outright, so it never reaches here.
		existing.Revoked = false
		existing.RevokedAt = nil
	}
	if hostname != "" {
		existing.Hostname = hostname
	}
	if serverID != "" {
		existing.ServerID = serverID
	}
	return a.store.PutAgent(ctx, existing)
}

// ============================================================
// REVOCATION
// ============================================================

// Revoke puts every serial an agent holds on the deny list and marks the record
// revoked. It takes effect on the next handshake, panel side, without waiting
// for anything to expire.
func (a *Authority) Revoke(ctx context.Context, agentID, reason string) error {
	rec, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	now := a.now()
	serials := []string{rec.Current.Serial}
	if rec.Previous != nil {
		serials = append(serials, rec.Previous.Serial)
	}
	for _, serial := range serials {
		if serial == "" {
			continue
		}
		if err := a.store.Revoke(ctx, RevokedCert{
			Serial:    serial,
			AgentID:   agentID,
			Reason:    reason,
			RevokedAt: now,
		}); err != nil {
			return err
		}
	}
	rec.Revoked = true
	rec.RevokedAt = &now
	if err := a.store.PutAgent(ctx, rec); err != nil {
		return err
	}
	a.logger.Warn("Agent PKI: certificate revoked",
		zap.String("agent_id", agentID),
		zap.Strings("serials", serials),
		zap.String("reason", reason))
	return nil
}

// DeniedSerials is the deny list as a plain slice, for pushing to agents.
func (a *Authority) DeniedSerials(ctx context.Context) ([]string, error) {
	entries, err := a.store.DenyList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Serial)
	}
	return out, nil
}

// ============================================================
// SMALL HELPERS
// ============================================================

func orDuration(v, fallback time.Duration) time.Duration {
	if v <= 0 {
		return fallback
	}
	return v
}

func newSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("agentpki: cannot generate a serial: %w", err)
	}
	return serial, nil
}

// SerialString is the canonical text form of a certificate serial: lowercase
// hex, no separators. Everything that compares serials compares this.
func SerialString(serial *big.Int) string {
	if serial == nil {
		return ""
	}
	return strings.ToLower(serial.Text(16))
}

// Fingerprint is the SHA-256 of the certificate DER, lowercase hex.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func spkiFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

func recordFor(cert *x509.Certificate, now time.Time) (CertRecord, error) {
	pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return CertRecord{}, err
	}
	return CertRecord{
		Serial:       SerialString(cert.SerialNumber),
		Fingerprint:  Fingerprint(cert),
		PublicKeyDER: pubDER,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		IssuedAt:     now,
	}, nil
}

// checkPublicKey rejects keys nobody should be issuing against in 2026.
func checkPublicKey(pub crypto.PublicKey) error {
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		switch key.Curve {
		case elliptic.P256(), elliptic.P384(), elliptic.P521():
			return nil
		default:
			return errors.New("agentpki: unsupported elliptic curve in the certificate request")
		}
	case *rsa.PublicKey:
		if key.N.BitLen() < minRSABits {
			return fmt.Errorf("agentpki: RSA key of %d bits is too small, %d is the minimum", key.N.BitLen(), minRSABits)
		}
		return nil
	default:
		return errors.New("agentpki: unsupported public key type in the certificate request")
	}
}

func parseCSR(csrPEM []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("agentpki: the certificate request is not a PEM CERTIFICATE REQUEST block")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agentpki: cannot parse the certificate request: %w", err)
	}
	// The signature proves the requester holds the private key for the public
	// key it is asking us to certify.
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("agentpki: the certificate request is not correctly signed: %w", err)
	}
	return csr, nil
}

func parseCAMaterial(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, nil, errors.New("agentpki: the CA certificate file is not a PEM CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("agentpki: cannot parse the CA certificate: %w", err)
	}
	if !cert.IsCA {
		return nil, nil, errors.New("agentpki: the CA certificate is not a CA certificate")
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("agentpki: the CA key file is not PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("agentpki: cannot parse the CA key: %w", err)
	}
	if !key.PublicKey.Equal(cert.PublicKey) {
		return nil, nil, errors.New("agentpki: the CA key does not belong to the CA certificate")
	}
	return cert, key, nil
}

func loadPair(certPath, keyPath string) (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	pair.Leaf = leaf
	return &pair, nil
}

// writeSecret writes 0600 through a temporary file and a rename, so a reader
// never sees a half-written key and an interrupted write never destroys the one
// that was there.
func writeSecret(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("agentpki: cannot write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("agentpki: cannot write %s: %w", path, err)
	}
	// A file that already existed keeps its old mode through a rename onto it
	// only if the mode was set on the temporary file, which it was; this is
	// belt and braces for an inherited file that was created too permissively.
	return os.Chmod(path, 0o600)
}
