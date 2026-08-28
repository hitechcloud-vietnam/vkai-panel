// Package pki holds the agent's half of the mutual TLS channel with the panel.
//
// The agent owns a private key that is generated on the managed server and
// never leaves it. What crosses the wire during enrolment is a certificate
// signing request - a public key and a signature proving the agent holds the
// matching private key - and what comes back is a short-lived certificate
// signed by the panel's internal CA.
//
// The agent trusts exactly one certificate authority: the one whose public key
// fingerprint was baked into the one-time enrolment token the operator pasted
// into the installer. It does not trust the system root store for the control
// channel, it does not trust a host name, and it does not hold a shared secret.
// The static VKAI_AGENT_TOKEN this replaced was a single string that granted
// root on every managed server to anyone who read it once.
//
// # The trust anchor is pinned
//
// The CA certificate is written once, at enrolment, and checked against the
// fingerprint inside the operator's token. Nothing else may replace it. In
// particular a renewal may not: renewals are unattended and frequent, so a CA
// that could arrive in one would mean that a single intercepted renewal makes
// the interceptor this agent's permanent certificate authority - it would issue
// every certificate afterwards, and every one of them would verify. Changing
// the anchor takes an explicit operator-initiated re-enrolment (Manager.ReEnrol,
// `vkaid re-enrol`), where the new fingerprint again arrives on paper rather
// than off the wire.
//
// This file deliberately mirrors core/internal/agentpki. The agent is a
// separate Go module with no dependencies, so the wire contract is duplicated
// rather than imported; any change to the signing message, the header names or
// the enrolment payloads has to be made in both places.
package pki

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Roles are carried in the certificate subject's OrganizationalUnit. An agent
// certificate cannot be replayed at another agent as if it were the panel,
// because the role differs even though the CA is the same.
const (
	RoleAgent = "vkai-agent"
	RolePanel = "vkai-panel"
)

// Headers that authenticate an agent request to the panel. They must match
// core/internal/agentpki/signed_request.go exactly.
const (
	HeaderAgentID   = "X-VKAI-Agent-Id"
	HeaderSerial    = "X-VKAI-Agent-Serial"
	HeaderTimestamp = "X-VKAI-Timestamp"
	HeaderNonce     = "X-VKAI-Nonce"
	HeaderSignature = "X-VKAI-Signature"
)

// Panel endpoints.
const (
	pathEnrol  = "/api/v1/agent-pki/enrol"
	pathRenew  = "/api/v1/agent-pki/renew"
	pathStatus = "/api/v1/agent-pki/status"
)

// File names inside the state directory. Everything here is 0600 in a 0700
// directory: the private key is the whole of the agent's identity.
const (
	keyFile   = "agent.key"
	certFile  = "agent.crt"
	caFile    = "ca.crt"
	stateFile = "state.json"

	// The identity that was in use before the last rotation. It is kept
	// because the panel goes on accepting it for an overlap window, so an
	// agent whose new key and certificate did not both survive a crash, a
	// snapshot revert or a full disk has something that still works. Without
	// it, a rotation is a moment where the agent can lock itself out of its own
	// panel.
	prevKeyFile  = "agent.key.prev"
	prevCertFile = "agent.crt.prev"
)

const (
	tokenPrefix  = "vkai-enrol"
	tokenVersion = "v1"
)

// ErrNotEnrolled means there is no usable identity on disk yet.
var ErrNotEnrolled = errors.New("agent pki: this agent has not been enrolled")

// ErrTrustAnchorChanged is returned when something asks this agent to trust a
// certificate authority other than the one it was enrolled against, outside an
// explicit operator-initiated re-enrolment.
//
// This is the difference between a channel that can be taken over and one that
// cannot. A renewal is unattended, it happens twice a day, and it is answered
// by whatever is at the panel URL. If a renewal could replace ca.crt, then one
// intercepted renewal would make the interceptor this agent's permanent trust
// anchor: it would issue every certificate from then on, and the agent would
// never notice, because everything would verify. So the anchor is pinned at
// enrolment and can be changed by exactly one thing: an operator pasting a new
// enrolment token, which carries the fingerprint of the CA they meant.
var ErrTrustAnchorChanged = errors.New("agent pki: the panel presented a different certificate authority")

// state is the small amount of metadata kept next to the key material.
type state struct {
	AgentID    string    `json:"agent_id"`
	Serial     string    `json:"serial"`
	NotAfter   time.Time `json:"not_after"`
	RenewAfter time.Time `json:"renew_after"`
	PanelURL   string    `json:"panel_url"`
	Denied     []string  `json:"denied_serials"`
}

// Manager owns the agent's key, certificate and trust anchor.
type Manager struct {
	dir      string
	panelURL string
	client   *http.Client
	logger   *log.Logger
	now      func() time.Time

	mu         sync.RWMutex
	key        crypto.Signer
	cert       *tls.Certificate
	caPool     *x509.CertPool
	caPEM      []byte
	agentID    string
	serial     string
	notAfter   time.Time
	renewAfter time.Time
	denied     map[string]bool
}

// Options configures a Manager.
type Options struct {
	Dir      string
	PanelURL string
	// Client is used for the calls to the panel. It carries the transport
	// policy for the panel's own HTTPS certificate, which is a separate
	// question from this CA.
	Client *http.Client
	Logger *log.Logger
	Now    func() time.Time
}

func New(opts Options) *Manager {
	m := &Manager{
		dir:      opts.Dir,
		panelURL: strings.TrimRight(opts.PanelURL, "/"),
		client:   opts.Client,
		logger:   opts.Logger,
		now:      opts.Now,
		denied:   make(map[string]bool),
	}
	if m.client == nil {
		m.client = &http.Client{Timeout: 30 * time.Second}
	}
	if m.logger == nil {
		m.logger = log.Default()
	}
	if m.now == nil {
		m.now = time.Now
	}
	return m
}

// AgentID, Serial and NotAfter report the current identity.
func (m *Manager) AgentID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentID
}

func (m *Manager) Serial() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serial
}

func (m *Manager) NotAfter() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.notAfter
}

// ============================================================
// LOADING WHAT IS ALREADY ON DISK
// ============================================================

// Load reads an existing identity. It returns ErrNotEnrolled when there is
// nothing to load, which is the installer's signal to enrol.
//
// If the current pair is unusable - a rotation that was interrupted between the
// two writes, a restored snapshot, a truncated file - the identity kept from
// before the last rotation is loaded instead. The panel accepts it for the
// overlap window, so the agent comes back up and renews, rather than sitting
// there enrolled and unable to prove it.
func (m *Manager) Load() error {
	st, err := m.readState()
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(filepath.Join(m.dir, caFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotEnrolled
		}
		return err
	}
	keyPEM, certPEM, readErr := readPair(m.dir, keyFile, certFile)
	if readErr == nil {
		readErr = m.install(keyPEM, certPEM, caPEM, st)
		if readErr == nil {
			return nil
		}
	}
	if errors.Is(readErr, os.ErrNotExist) {
		// Nothing current at all. There may still be a previous identity, from
		// a rotation that wrote the new files away and then lost them.
		readErr = ErrNotEnrolled
	}

	prevKeyPEM, prevCertPEM, prevErr := readPair(m.dir, prevKeyFile, prevCertFile)
	if prevErr != nil {
		return readErr
	}
	if err := m.install(prevKeyPEM, prevCertPEM, caPEM, st); err != nil {
		return readErr
	}
	m.logger.Printf("WARNING: the current certificate is unusable (%v); "+
		"falling back to the one held before the last rotation, serial %s, which the panel accepts "+
		"for its overlap window. A renewal will replace it.", readErr, m.Serial())
	// Renew at once rather than at half life: what is in hand is on borrowed
	// time by definition.
	m.mu.Lock()
	m.renewAfter = m.now().Add(-time.Second)
	m.mu.Unlock()
	return nil
}

// readPair reads a key and certificate together, so a half-written rotation is
// one error rather than two states.
func readPair(dir, keyName, certName string) ([]byte, []byte, error) {
	keyPEM, err := os.ReadFile(filepath.Join(dir, keyName))
	if err != nil {
		return nil, nil, err
	}
	certPEM, err := os.ReadFile(filepath.Join(dir, certName))
	if err != nil {
		return nil, nil, err
	}
	return keyPEM, certPEM, nil
}

// verifyMaterial checks a key, certificate and CA as a set: the key matches the
// certificate, the certificate chains to that CA, and it is inside its validity
// window. It mutates nothing, so it can be called before a write as well as
// after a read.
func verifyMaterial(keyPEM, certPEM, caPEM []byte, now time.Time) (*tls.Certificate, *x509.Certificate, *x509.CertPool, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("agent pki: the key and certificate do not match: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, nil, err
	}
	pair.Leaf = leaf
	if _, ok := pair.PrivateKey.(crypto.Signer); !ok {
		return nil, nil, nil, errors.New("agent pki: the private key cannot sign")
	}
	caCert, err := parseCACert(caPEM)
	if err != nil {
		return nil, nil, nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	// A certificate that does not chain to the stored CA is not usable, and
	// carrying on with it would produce handshake failures that look like
	// network faults. Failing here says what is actually wrong.
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:       pool,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("agent pki: the certificate is not usable: %w", err)
	}
	return &pair, leaf, pool, nil
}

func (m *Manager) install(keyPEM, certPEM, caPEM []byte, st *state) error {
	pair, leaf, pool, err := verifyMaterial(keyPEM, certPEM, caPEM, m.now())
	if err != nil {
		return err
	}
	signer := pair.PrivateKey.(crypto.Signer)

	denied := make(map[string]bool, len(st.Denied))
	for _, serial := range st.Denied {
		denied[serial] = true
	}

	m.mu.Lock()
	m.key = signer
	m.cert = pair
	m.caPool = pool
	m.caPEM = append([]byte(nil), caPEM...)
	m.agentID = leaf.Subject.CommonName
	m.serial = serialString(leaf)
	m.notAfter = leaf.NotAfter
	m.renewAfter = st.RenewAfter
	if m.renewAfter.IsZero() {
		// Half life, if the panel did not say. Never later than expiry.
		m.renewAfter = leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) / 2)
	}
	m.denied = denied
	m.mu.Unlock()
	return nil
}

func (m *Manager) readState() (*state, error) {
	data, err := os.ReadFile(filepath.Join(m.dir, stateFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &state{}, nil
		}
		return nil, err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("agent pki: %s is not readable state: %w", stateFile, err)
	}
	return &st, nil
}

func (m *Manager) writeState() error {
	m.mu.RLock()
	st := state{
		AgentID:    m.agentID,
		Serial:     m.serial,
		NotAfter:   m.notAfter,
		RenewAfter: m.renewAfter,
		PanelURL:   m.panelURL,
	}
	for serial := range m.denied {
		st.Denied = append(st.Denied, serial)
	}
	m.mu.RUnlock()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeSecret(filepath.Join(m.dir, stateFile), data)
}

// ============================================================
// ENROLMENT
// ============================================================

type enrolRequest struct {
	Token        string `json:"token"`
	CSRPEM       string `json:"csr_pem"`
	Hostname     string `json:"hostname"`
	AgentVersion string `json:"agent_version"`
}

type renewRequest struct {
	CSRPEM string `json:"csr_pem"`
}

type issuedResponse struct {
	AgentID    string    `json:"agent_id"`
	Serial     string    `json:"serial"`
	CertPEM    string    `json:"certificate_pem"`
	CAPEM      string    `json:"ca_pem"`
	NotAfter   time.Time `json:"not_after"`
	RenewAfter time.Time `json:"renew_after"`
}

// Enrol trades the one-time token for a certificate. It is called once, by the
// installer; afterwards the token is dead and Renew takes over.
//
// If this agent already trusts a certificate authority, Enrol refuses a token
// that names a different one. Changing the trust anchor is the one thing that
// cannot be allowed to happen quietly, so it needs ReEnrol, which an operator
// asks for by name.
func (m *Manager) Enrol(token, hostname, agentVersion string) error {
	return m.enrol(token, hostname, agentVersion, false)
}

// ReEnrol is the explicit, operator-initiated re-enrolment. It is the only path
// that may replace this agent's trust anchor, and it exists for exactly two
// situations: the panel's CA was rebuilt, or the CA key leaked and every
// certificate it ever signed is worthless.
//
// The operator mints a fresh enrolment token in the new panel and pastes it
// here. The token carries the new CA's public key fingerprint, so the anchor
// still arrives through the operator rather than off the wire - trust on paste,
// not trust on first sight.
func (m *Manager) ReEnrol(token, hostname, agentVersion string) error {
	return m.enrol(token, hostname, agentVersion, true)
}

// TrustAnchorFingerprint is the SHA-256 of the public key of the CA this agent
// trusts, lowercase hex, or "" when it is not enrolled. It is what an operator
// compares against the panel before deciding that a change is legitimate.
func (m *Manager) TrustAnchorFingerprint() string {
	m.mu.RLock()
	caPEM := m.caPEM
	m.mu.RUnlock()
	if len(caPEM) == 0 {
		// Not loaded, or loaded from a broken identity. The anchor on disk is
		// still the anchor: an enrolment that would replace it has to see it,
		// whether or not the certificate beside it could be used.
		onDisk, err := os.ReadFile(filepath.Join(m.dir, caFile))
		if err != nil {
			return ""
		}
		caPEM = onDisk
	}
	cert, err := parseCACert(caPEM)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) enrol(token, hostname, agentVersion string, allowAnchorChange bool) error {
	// Only the fingerprint is needed here: the whole token string is what the
	// panel checks, and the fingerprint is what this side checks the answer
	// against.
	caFingerprint, err := tokenCAFingerprint(token)
	if err != nil {
		return err
	}
	// An anchor is already installed and the token names a different CA. That
	// is either a panel rebuild or an attempt to hand this root-privileged
	// agent to somebody else; the two are indistinguishable from here, so it
	// takes a decision an operator made deliberately.
	if current := m.TrustAnchorFingerprint(); current != "" &&
		!strings.EqualFold(current, caFingerprint) && !allowAnchorChange {
		return fmt.Errorf("%w: this agent trusts the CA with fingerprint %s and the token names %s. "+
			"Run `%s re-enrol` if an operator really is moving this agent to a new certificate authority",
			ErrTrustAnchorChanged, current, caFingerprint, "vkaid")
	}

	key, csrPEM, err := generateKeyAndCSR(hostname)
	if err != nil {
		return err
	}

	body, err := json.Marshal(enrolRequest{
		Token:        strings.TrimSpace(token),
		CSRPEM:       string(csrPEM),
		Hostname:     hostname,
		AgentVersion: agentVersion,
	})
	if err != nil {
		return err
	}

	var issued issuedResponse
	if err := m.postJSON(pathEnrol, body, nil, &issued); err != nil {
		return err
	}

	// The operator is the trusted channel. The CA certificate the panel just
	// sent is accepted only if its public key matches the fingerprint that was
	// inside the token, so a machine in the middle cannot swap in its own CA.
	if err := checkCAFingerprint([]byte(issued.CAPEM), caFingerprint); err != nil {
		return err
	}

	keyPEM, err := marshalKey(key)
	if err != nil {
		return err
	}
	previousAnchor := m.TrustAnchorFingerprint()
	if err := m.persist(keyPEM, []byte(issued.CertPEM), []byte(issued.CAPEM), issued.RenewAfter); err != nil {
		return err
	}
	if previousAnchor != "" && !strings.EqualFold(previousAnchor, caFingerprint) {
		m.logger.Printf("WARNING: the trust anchor was replaced by an operator-initiated re-enrolment: %s -> %s",
			previousAnchor, caFingerprint)
	}
	m.logger.Printf("enrolled with the panel: agent_id=%s serial=%s expires=%s ca_fingerprint=%s",
		issued.AgentID, issued.Serial, issued.NotAfter.Format(time.RFC3339), caFingerprint)
	return nil
}

// Renew asks for the next certificate, proving possession of the current key.
//
// The certificate in use is not thrown away until the new one is on disk and
// verified, and the panel keeps accepting the old one for an overlap window, so
// a renewal that fails halfway - a dropped connection, a panel restart, a power
// cut between the two writes - leaves a working agent behind.
func (m *Manager) Renew(hostname string) error {
	m.mu.RLock()
	enrolled := m.cert != nil
	m.mu.RUnlock()
	if !enrolled {
		return ErrNotEnrolled
	}

	key, csrPEM, err := generateKeyAndCSR(hostname)
	if err != nil {
		return err
	}
	body, err := json.Marshal(renewRequest{CSRPEM: string(csrPEM)})
	if err != nil {
		return err
	}
	headers, err := m.SignedHeaders(body)
	if err != nil {
		return err
	}

	var issued issuedResponse
	if err := m.postJSON(pathRenew, body, headers, &issued); err != nil {
		return err
	}

	// A renewal may not change what this agent trusts. The reply's ca_pem is
	// compared against the anchor on disk and then thrown away: what gets
	// written is the anchor this agent already had. A panel that has genuinely
	// rebuilt its CA cannot renew this agent - it has to re-enrol it, which is
	// an operator pasting a token, which is the point.
	m.mu.RLock()
	anchorPEM := append([]byte(nil), m.caPEM...)
	m.mu.RUnlock()
	if err := m.checkAnchorUnchanged([]byte(issued.CAPEM), anchorPEM); err != nil {
		m.logger.Printf("SECURITY: refusing this renewal: %v", err)
		return err
	}

	keyPEM, err := marshalKey(key)
	if err != nil {
		return err
	}
	previous := m.Serial()
	if err := m.persist(keyPEM, []byte(issued.CertPEM), anchorPEM, issued.RenewAfter); err != nil {
		return err
	}
	m.logger.Printf("certificate rotated: serial %s -> %s, expires %s",
		previous, issued.Serial, issued.NotAfter.Format(time.RFC3339))
	return nil
}

// checkAnchorUnchanged compares a CA offered in a renewal reply with the one
// this agent is pinned to. An empty offer is fine - it means the panel sent no
// CA and there is nothing to disagree about.
func (m *Manager) checkAnchorUnchanged(offered, pinned []byte) error {
	if len(bytes.TrimSpace(offered)) == 0 {
		return nil
	}
	if len(pinned) == 0 {
		return ErrNotEnrolled
	}
	offeredCert, err := parseCACert(offered)
	if err != nil {
		return fmt.Errorf("%w: the renewal carried a CA this agent cannot parse: %v", ErrTrustAnchorChanged, err)
	}
	pinnedCert, err := parseCACert(pinned)
	if err != nil {
		return err
	}
	offeredSum := sha256.Sum256(offeredCert.RawSubjectPublicKeyInfo)
	pinnedSum := sha256.Sum256(pinnedCert.RawSubjectPublicKeyInfo)
	// Constant time out of habit rather than necessity: both values are public.
	if subtle.ConstantTimeCompare(offeredSum[:], pinnedSum[:]) != 1 {
		return fmt.Errorf("%w: the renewal offered the CA with fingerprint %s, this agent trusts %s. "+
			"Nothing was written. If an operator really did rebuild the panel CA, re-enrol this agent with a "+
			"fresh enrolment token; if not, this is somebody trying to become this agent's certificate authority",
			ErrTrustAnchorChanged,
			hex.EncodeToString(offeredSum[:]), hex.EncodeToString(pinnedSum[:]))
	}
	return nil
}

// NeedsRenewal reports whether the renewal point has passed.
func (m *Manager) NeedsRenewal() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cert == nil {
		return false
	}
	return !m.now().Before(m.renewAfter)
}

// persist writes the new material and only then swaps it in.
//
// The order matters, and it is chosen so that no failure leaves this agent
// unable to talk to its panel:
//
//  1. The new key, certificate and CA are checked together before anything is
//     written. Material that does not verify is refused while the identity in
//     use is still untouched.
//  2. The identity currently on disk is copied aside as the previous one. The
//     panel goes on accepting it for its overlap window, so it is a working
//     fallback, not a museum piece.
//  3. Only then are the new files written, each through a temporary file and a
//     rename.
//
// A crash at any point leaves either the old identity or the new one, and Load
// prefers the new and falls back to the old.
func (m *Manager) persist(keyPEM, certPEM, caPEM []byte, renewAfter time.Time) error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return fmt.Errorf("agent pki: cannot create %s: %w", m.dir, err)
	}
	if _, _, _, err := verifyMaterial(keyPEM, certPEM, caPEM, m.now()); err != nil {
		return fmt.Errorf("agent pki: refusing to install this identity, the one in use is untouched: %w", err)
	}
	if err := m.keepPrevious(); err != nil {
		// Not fatal: it costs the fallback, not the rotation.
		m.logger.Printf("warning: cannot keep the previous identity as a fallback: %v", err)
	}
	if err := writeSecret(filepath.Join(m.dir, keyFile), keyPEM); err != nil {
		return err
	}
	if err := writeSecret(filepath.Join(m.dir, certFile), certPEM); err != nil {
		return err
	}
	if err := writeSecret(filepath.Join(m.dir, caFile), caPEM); err != nil {
		return err
	}
	st, err := m.readState()
	if err != nil {
		return err
	}
	st.RenewAfter = renewAfter
	if err := m.install(keyPEM, certPEM, caPEM, st); err != nil {
		return err
	}
	return m.writeState()
}

// keepPrevious copies the identity that is on disk now to the .prev files. It
// does nothing when there is no complete identity to keep, which is the case on
// first enrolment.
func (m *Manager) keepPrevious() error {
	keyPEM, certPEM, err := readPair(m.dir, keyFile, certFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := writeSecret(filepath.Join(m.dir, prevKeyFile), keyPEM); err != nil {
		return err
	}
	return writeSecret(filepath.Join(m.dir, prevCertFile), certPEM)
}

// ============================================================
// TALKING TO THE PANEL
// ============================================================

// SignedHeaders authenticates one request to the panel with the agent's private
// key. The signed message must match agentpki.SigningMessage byte for byte.
func (m *Manager) SignedHeaders(body []byte) (map[string]string, error) {
	m.mu.RLock()
	key, agentID, serial := m.key, m.agentID, m.serial
	m.mu.RUnlock()
	if key == nil {
		return nil, ErrNotEnrolled
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := m.now().UTC().Format(time.RFC3339Nano)

	digest := sha256.Sum256(signingMessage(agentID, serial, timestamp, nonce, body))
	sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		HeaderAgentID:   agentID,
		HeaderSerial:    serial,
		HeaderTimestamp: timestamp,
		HeaderNonce:     nonce,
		HeaderSignature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// StatusReport is the periodic "I am alive and this is what I see" call. It
// replaces the old heartbeat, which authenticated itself by putting the shared
// secret in a header and in the body.
func (m *Manager) StatusReport(payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers, err := m.SignedHeaders(body)
	if err != nil {
		return err
	}
	var reply struct {
		DeniedSerials []string `json:"denied_serials"`
	}
	if err := m.postJSON(pathStatus, body, headers, &reply); err != nil {
		return err
	}
	// The panel pushes its deny list on every status call, so a revoked panel
	// certificate stops being accepted here within one status interval.
	m.ApplyDenyList(reply.DeniedSerials)
	return nil
}

func (m *Manager) postJSON(path string, body []byte, headers map[string]string, out any) error {
	req, err := http.NewRequest(http.MethodPost, m.panelURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("agent pki: cannot reach the panel at %s: %w", m.panelURL+path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("agent pki: the panel refused %s with status %d: %s",
			path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	// The panel wraps every reply in {"success":..., "data":...}; unwrap when
	// that envelope is present and fall back to the bare object when it is not.
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}
	return json.Unmarshal(data, out)
}

// ============================================================
// SERVING THE PANEL
// ============================================================

// ServerTLSConfig is what the agent listens with. It presents the certificate
// the panel issued and demands one back, and it accepts only a certificate
// issued by the panel CA that carries the panel role and is not on the deny
// list. No host name is checked in either direction.
func (m *Manager) ServerTLSConfig() (*tls.Config, error) {
	m.mu.RLock()
	cert := m.cert
	m.mu.RUnlock()
	if cert == nil {
		return nil, ErrNotEnrolled
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.RequireAnyClientCert,
		// Fetched per handshake so a rotation is picked up without rebuilding
		// the listener and without dropping a connection.
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			m.mu.RLock()
			defer m.mu.RUnlock()
			if m.cert == nil {
				return nil, ErrNotEnrolled
			}
			return m.cert, nil
		},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			_, err := m.VerifyPanelPeer(rawCerts)
			return err
		},
	}, nil
}

// VerifyPanelPeer accepts only the panel: issued by the CA this agent was
// enrolled against, carrying the panel role, and not revoked.
func (m *Manager) VerifyPanelPeer(rawCerts [][]byte) (*x509.Certificate, error) {
	m.mu.RLock()
	pool, denied := m.caPool, m.denied
	m.mu.RUnlock()
	if pool == nil {
		return nil, ErrNotEnrolled
	}
	if len(rawCerts) == 0 {
		return nil, errors.New("agent pki: the caller presented no certificate")
	}
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return nil, fmt.Errorf("agent pki: cannot parse the caller certificate: %w", err)
	}
	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		cert, parseErr := x509.ParseCertificate(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("agent pki: cannot parse an intermediate certificate: %w", parseErr)
		}
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		CurrentTime:   m.now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return nil, fmt.Errorf("agent pki: the caller certificate was not issued by the panel CA: %w", err)
	}
	if !hasRole(leaf, RolePanel) {
		return nil, errors.New("agent pki: the caller certificate does not carry the panel role")
	}
	if denied[serialString(leaf)] {
		return nil, errors.New("agent pki: the caller certificate is revoked")
	}
	return leaf, nil
}

// ApplyDenyList replaces the local deny list with the one the panel pushed.
//
// Its limit, stated plainly: an agent the panel cannot reach keeps the list it
// last received. A panel certificate revoked while this agent is unreachable is
// still accepted here until it expires, which is why the certificates are short.
func (m *Manager) ApplyDenyList(serials []string) {
	denied := make(map[string]bool, len(serials))
	for _, serial := range serials {
		serial = strings.ToLower(strings.TrimSpace(serial))
		if serial != "" {
			denied[serial] = true
		}
	}
	m.mu.Lock()
	changed := len(denied) != len(m.denied)
	if !changed {
		for serial := range denied {
			if !m.denied[serial] {
				changed = true
				break
			}
		}
	}
	m.denied = denied
	m.mu.Unlock()
	if changed {
		m.logger.Printf("deny list updated: %d revoked serial(s)", len(denied))
		if err := m.writeState(); err != nil {
			m.logger.Printf("warning: cannot persist the deny list: %v", err)
		}
	}
}

// DeniedCount reports the size of the local deny list.
func (m *Manager) DeniedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.denied)
}

// ============================================================
// HELPERS
// ============================================================

func signingMessage(agentID, serial, timestamp, nonce string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		agentID, serial, timestamp, nonce, hex.EncodeToString(sum[:]),
	}, "\n"))
}

// tokenCAFingerprint validates the shape of a pasted enrolment token and
// returns the CA fingerprint it carries.
func tokenCAFingerprint(raw string) (string, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 5 || parts[0] != tokenPrefix || parts[1] != tokenVersion {
		return "", errors.New("agent pki: the enrolment token is not in the expected form")
	}
	for _, part := range parts[2:] {
		if part == "" {
			return "", errors.New("agent pki: the enrolment token is incomplete")
		}
	}
	return parts[4], nil
}

func checkCAFingerprint(caPEM []byte, want string) error {
	cert, err := parseCACert(caPEM)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	got := hex.EncodeToString(sum[:])
	// Constant time, because this is a secret-shaped comparison even though the
	// value itself is public.
	if subtle.ConstantTimeCompare([]byte(got), []byte(strings.ToLower(want))) != 1 {
		return fmt.Errorf("agent pki: the panel returned a CA whose fingerprint is %s, not the %s in the enrolment token", got, want)
	}
	return nil
}

func parseCACert(caPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("agent pki: the CA material is not a PEM CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent pki: cannot parse the CA certificate: %w", err)
	}
	if !cert.IsCA {
		return nil, errors.New("agent pki: the CA certificate is not a CA certificate")
	}
	return cert, nil
}

func generateKeyAndCSR(hostname string) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("agent pki: cannot generate a key: %w", err)
	}
	tmpl := &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: hostname},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, nil, fmt.Errorf("agent pki: cannot create a certificate request: %w", err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func marshalKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

func hasRole(cert *x509.Certificate, role string) bool {
	for _, ou := range cert.Subject.OrganizationalUnit {
		if ou == role {
			return true
		}
	}
	return false
}

func serialString(cert *x509.Certificate) string {
	if cert.SerialNumber == nil {
		return ""
	}
	return strings.ToLower(cert.SerialNumber.Text(16))
}

// writeSecret writes 0600 through a temporary file and a rename, so an
// interrupted write cannot destroy the identity that is already there.
func writeSecret(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("agent pki: cannot write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("agent pki: cannot write %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}
