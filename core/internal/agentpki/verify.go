package agentpki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"
)

// ============================================================
// CHAIN VERIFICATION
// ============================================================

// verifyChain is the part both directions share: the peer's leaf must chain to
// this CA, be inside its validity window, be usable for the role it is being
// used in, and carry the expected role marker.
//
// What it deliberately does NOT do is check the host name. Customer servers
// move between addresses, get rebuilt behind a new NAT, and change their
// reverse DNS; a channel that breaks when an IP changes is a channel operators
// will disable. Identity here is the certificate itself, and the caller pins it
// by serial and fingerprint against what the panel issued.
func verifyChain(pool *x509.CertPool, role string, usage x509.ExtKeyUsage, now time.Time, rawCerts [][]byte) (*x509.Certificate, error) {
	if len(rawCerts) == 0 {
		return nil, errors.New("agentpki: the peer presented no certificate")
	}
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return nil, fmt.Errorf("agentpki: cannot parse the peer certificate: %w", err)
	}
	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		cert, parseErr := x509.ParseCertificate(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("agentpki: cannot parse an intermediate certificate: %w", parseErr)
		}
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{usage},
	}); err != nil {
		return nil, fmt.Errorf("agentpki: the peer certificate was not issued by this CA: %w", err)
	}
	if !hasRole(leaf, role) {
		return nil, ErrBadRole
	}
	return leaf, nil
}

func hasRole(cert *x509.Certificate, role string) bool {
	for _, ou := range cert.Subject.OrganizationalUnit {
		if ou == role {
			return true
		}
	}
	return false
}

// ============================================================
// PANEL SIDE: IS THIS AN AGENT I ISSUED?
// ============================================================

// VerifyAgentPeer decides whether a certificate presented by an agent is one
// this panel issued and still accepts. expectAgentID pins the connection to one
// agent; passing "" accepts any agent this panel knows, which is what the
// enrolment-independent endpoints use.
func (a *Authority) VerifyAgentPeer(ctx context.Context, rawCerts [][]byte, expectAgentID string) (*AgentRecord, error) {
	now := a.now()
	leaf, err := verifyChain(a.CAPool(), RoleAgent, x509.ExtKeyUsageServerAuth, now, rawCerts)
	if err != nil {
		return nil, err
	}
	agentID := leaf.Subject.CommonName
	if expectAgentID != "" && agentID != expectAgentID {
		return nil, fmt.Errorf("agentpki: expected agent %q but the peer is %q", expectAgentID, agentID)
	}
	return a.checkIssued(ctx, agentID, SerialString(leaf.SerialNumber), Fingerprint(leaf), now)
}

// checkIssued is the deny list and the pinning, in that order: a revoked
// certificate is refused before anything else is considered.
func (a *Authority) checkIssued(ctx context.Context, agentID, serial, fingerprint string, now time.Time) (*AgentRecord, error) {
	rec, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnknownAgent
		}
		return nil, err
	}
	if rec.Revoked {
		return nil, ErrRevoked
	}
	// Checked on every handshake, not cached: revoking has to bite on the next
	// connection, not at the next restart.
	revoked, err := a.store.IsRevoked(ctx, serial)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrRevoked
	}
	if _, err := a.acceptSerial(rec, serial, fingerprint, now); err != nil {
		return nil, err
	}
	return rec, nil
}

// acceptSerial implements the overlap window. The current certificate is always
// accepted. The one it replaced is accepted too, until the overlap window
// closes or it expires, whichever comes first - so a rotation whose answer the
// agent never received does not lock the agent out.
func (a *Authority) acceptSerial(rec *AgentRecord, serial, fingerprint string, now time.Time) (*CertRecord, error) {
	if rec.Current.Serial == serial {
		if fingerprint != "" && rec.Current.Fingerprint != fingerprint {
			return nil, ErrWrongCertificate
		}
		return &rec.Current, nil
	}
	prev := rec.Previous
	if prev == nil || prev.Serial != serial {
		return nil, ErrWrongCertificate
	}
	if fingerprint != "" && prev.Fingerprint != fingerprint {
		return nil, ErrWrongCertificate
	}
	if prev.SupersededAt == nil {
		return nil, ErrWrongCertificate
	}
	if now.After(prev.SupersededAt.Add(a.overlap)) {
		return nil, fmt.Errorf("%w: the overlap window for this certificate has closed", ErrWrongCertificate)
	}
	if now.After(prev.NotAfter) {
		return nil, fmt.Errorf("%w: this certificate has expired", ErrWrongCertificate)
	}
	return prev, nil
}

// ClientTLSConfig is the configuration the panel dials an agent with. The panel
// certificate is fetched per handshake, so a rotation is picked up without
// rebuilding anything, and the agent is verified by VerifyAgentPeer rather than
// by name.
func (a *Authority) ClientTLSConfig(agentID string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		// The standard verifier is switched off because it would insist on the
		// host name matching. VerifyPeerCertificate below is strictly stronger:
		// it demands the exact certificate this panel issued to this agent.
		InsecureSkipVerify: true,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return a.PanelClientCertificate(context.Background())
		},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			_, err := a.VerifyAgentPeer(context.Background(), rawCerts, agentID)
			return err
		},
	}
}

// ============================================================
// AGENT SIDE: IS THIS MY PANEL?
// ============================================================

// PanelVerifier is what an agent needs in order to decide whether an inbound
// client certificate is its panel. It lives here as the reference
// implementation - the agent is a separate Go module and carries its own copy -
// and it is what the tests in this package exercise on the server side.
type PanelVerifier struct {
	// Pool is the panel CA, and nothing else. A certificate from any other CA,
	// public or private, fails here.
	Pool *x509.CertPool

	// Denied is the serial deny list the panel last pushed. An agent that has
	// not been reached since a revocation still holds the previous list; the
	// certificate lifetime is what bounds that gap.
	Denied map[string]bool

	// Now exists for the tests.
	Now func() time.Time
}

// VerifyPanelPeer accepts only a certificate issued by the panel CA carrying
// the panel role and not on the deny list.
func (v PanelVerifier) VerifyPanelPeer(rawCerts [][]byte) (*x509.Certificate, error) {
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	leaf, err := verifyChain(v.Pool, RolePanel, x509.ExtKeyUsageClientAuth, now(), rawCerts)
	if err != nil {
		return nil, err
	}
	if v.Denied[SerialString(leaf.SerialNumber)] {
		return nil, ErrRevoked
	}
	return leaf, nil
}

// ServerTLSConfig is the configuration an agent listens with: present the
// certificate the panel issued it, demand a client certificate, and accept only
// the panel's.
func ServerTLSConfig(cert *tls.Certificate, verifier PanelVerifier) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{*cert},
		// RequireAnyClientCert rather than RequireAndVerifyClientCert: the
		// verification that matters is done in VerifyPeerCertificate, which
		// checks the role and the deny list as well as the chain. Letting the
		// standard verifier run first would only duplicate the chain check.
		ClientAuth: tls.RequireAnyClientCert,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			_, err := verifier.VerifyPanelPeer(rawCerts)
			return err
		},
	}
}
