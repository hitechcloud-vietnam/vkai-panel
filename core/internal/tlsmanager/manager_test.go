package tlsmanager

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// newTestConfig builds a panel access config that writes everything into a
// temporary directory, so no test touches the real installation root.
func newTestConfig(t *testing.T, dir string) *config.PanelAccessConfig {
	t.Helper()

	cfg := config.DefaultPanelAccess()
	cfg.StateFile = filepath.Join(dir, "panel_access.json")
	cfg.Entrance = "/vkai_a1b2c3d4"
	cfg.TLS.CertFile = filepath.Join(dir, "panel.crt")
	cfg.TLS.KeyFile = filepath.Join(dir, "panel.key")
	return cfg
}

// issuePair mints a leaf signed by a throwaway CA, which is what tells the
// manager it is looking at a CA-issued certificate rather than the self-signed
// bootstrap one.
func issuePair(t *testing.T, host string, notBefore, notAfter time.Time) (chainPEM, keyPEM []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "VKAI Test CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if ip := net.ParseIP(host); ip != nil {
		leafTemplate.IPAddresses = []net.IP{ip}
	} else {
		leafTemplate.DNSNames = []string{host}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}

	chainPEM = append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})...,
	)
	der, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	return chainPEM, keyPEM
}

// randomSerial keeps every minted certificate distinguishable, which is how the
// renewal test proves the listener really swapped certificates.
func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	return serial
}

// fakeClient records what it was asked for and returns a canned answer.
type fakeClient struct {
	mu          sync.Mutex
	chain       []byte
	key         []byte
	err         error
	calls       int
	identifiers []Identifier
	profile     string
}

func (f *fakeClient) Obtain(_ context.Context, ids []Identifier, profile string, solver Solver) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.identifiers = ids
	f.profile = profile

	// Exercise the solver contract the way a real order would.
	if solver != nil {
		if err := solver.Present("token-abc", "token-abc.keyauth"); err != nil {
			return nil, nil, err
		}
		defer func() { _ = solver.CleanUp("token-abc") }()
	}

	if f.err != nil {
		return nil, nil, f.err
	}
	return f.chain, f.key, nil
}

func (f *fakeClient) snapshot() (int, []Identifier, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.identifiers, f.profile
}

type noopSolver struct{}

func (noopSolver) Present(string, string) error { return nil }
func (noopSolver) CleanUp(string) error         { return nil }

// waitFor polls a condition, because issuance runs behind the listener on
// purpose: Start must not wait for a CA.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSelfSignedModeServesImmediately(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)

	m, err := New(Options{Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(m.Stop)

	cert, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "localhost"})
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert.Leaf == nil {
		t.Fatal("the served certificate has no parsed leaf, so renewal could never be scheduled")
	}
	if got := m.Source(); got != SourceSelfSigned {
		t.Fatalf("Source() = %q, want %q", got, SourceSelfSigned)
	}
	if cfg.CertSource != SourceSelfSigned {
		t.Fatalf("CertSource = %q, the banner would not name the certificate source", cfg.CertSource)
	}
}

func TestLetsEncryptIssuanceReplacesTheBootstrapCertificate(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TLS.Mode = config.TLSModeLetsEncrypt
	cfg.Domain = "panel.example.com"

	chainPEM, keyPEM := issuePair(t, "panel.example.com", time.Now().Add(-time.Hour), time.Now().Add(80*24*time.Hour))
	client := &fakeClient{chain: chainPEM, key: keyPEM}

	m, err := New(Options{Config: cfg, Client: client, Solver: noopSolver{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(m.Stop)

	waitFor(t, "the issued certificate to be installed", func() bool {
		return m.Source() == SourceLetsEncryptStaging
	})

	calls, ids, profile := client.snapshot()
	if calls != 1 {
		t.Fatalf("Obtain called %d times, want exactly one order", calls)
	}
	if len(ids) != 1 || ids[0].Type != config.ACMEIdentifierDNS || ids[0].Value != "panel.example.com" {
		t.Fatalf("ordered identifiers = %+v, want one dns identifier for the pinned domain", ids)
	}
	if profile != config.ACMEProfileTLSServer {
		t.Fatalf("profile = %q, want %q for a dns identifier", profile, config.ACMEProfileTLSServer)
	}

	cert := m.Certificate()
	if cert == nil || cert.Leaf.VerifyHostname("panel.example.com") != nil {
		t.Fatal("the served certificate is not the issued one")
	}

	// The certificate is installed before it is written, on purpose: a disk that
	// refuses the write must not stop a valid certificate reaching the wire. So
	// the store lands just after the swap.
	waitFor(t, "the issued chain to be stored", func() bool {
		stored, readErr := os.ReadFile(cfg.TLS.CertFile)
		return readErr == nil && string(stored) == string(chainPEM)
	})
	info, err := os.Stat(cfg.TLS.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("issued key mode = %o, want 0600", perm)
	}
}

func TestIssuanceFailureKeepsServingAndNeverBlocksStartup(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TLS.Mode = config.TLSModeLetsEncrypt
	cfg.Domain = "panel.example.com"

	client := &fakeClient{err: errors.New("dial acme-v02.api.letsencrypt.org: connection refused")}

	m, err := New(Options{Config: cfg, Client: client, Solver: noopSolver{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start must not fail because the CA is unreachable: %v", err)
	}
	t.Cleanup(m.Stop)

	if m.Certificate() == nil {
		t.Fatal("no certificate is being served, so the panel could not answer HTTPS at all")
	}
	// The message already said "fallback" while the comparison asked for the
	// plain constant. In letsencrypt mode a self-signed certificate is what the
	// panel fell back to when the order could not complete, so the message was
	// right and the constant was wrong.
	if got := m.Source(); got != SourceSelfSignedFallback {
		t.Fatalf("Source() = %q, want %q so the banner admits the fallback",
			got, SourceSelfSignedFallback)
	}

	waitFor(t, "the failure to be recorded", func() bool {
		return m.Info().LastError != ""
	})
}

func TestStubClientLeavesThePanelOnSelfSigned(t *testing.T) {
	// No Client is injected, so the manager falls back to newACMEClient, which
	// is the placeholder until internal/acme lands. The panel must still run.
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TLS.Mode = config.TLSModeLetsEncrypt
	cfg.Domain = "panel.example.com"

	m, err := New(Options{Config: cfg, Solver: noopSolver{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(m.Stop)

	if m.Certificate() == nil {
		t.Fatal("no certificate is being served")
	}
	waitFor(t, "the unavailable client to be reported", func() bool {
		return m.Info().LastError != ""
	})
}

func TestCustomModeWithMissingFilesStillStarts(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TLS.Mode = config.TLSModeCustom
	cfg.TLS.SelfSigned = false
	cfg.TLS.CertFile = filepath.Join(dir, "absent.crt")
	cfg.TLS.KeyFile = filepath.Join(dir, "absent.key")

	m, err := New(Options{Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start must fall back rather than fail: %v", err)
	}
	t.Cleanup(m.Stop)

	if m.Certificate() == nil {
		t.Fatal("no certificate is being served")
	}
	if got := m.Source(); got != SourceSelfSignedFallback {
		t.Fatalf("Source() = %q, want %q so the banner admits the fallback", got, SourceSelfSignedFallback)
	}
}

func TestRenewalSwapsTheCertificateOnALiveListener(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)
	cfg.TLS.Mode = config.TLSModeLetsEncrypt
	cfg.Domain = "panel.example.com"

	first, firstKey := issuePair(t, "panel.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	client := &fakeClient{chain: first, key: firstKey}

	m, err := New(Options{Config: cfg, Client: client, Solver: noopSolver{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(m.Stop)

	waitFor(t, "the first certificate", func() bool { return m.Source() == SourceLetsEncryptStaging })

	ln, err := tls.Listen("tcp", "127.0.0.1:0", m.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}()
		}
	}()

	serial := func() *big.Int {
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // the test verifies which certificate is served, not the chain
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber
	}

	before := serial()

	// A renewal, on the same manager and the same listener.
	second, secondKey := issuePair(t, "panel.example.com", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))
	client.mu.Lock()
	client.chain, client.key = second, secondKey
	client.mu.Unlock()

	// The current certificate expires within the hour, so it is due.
	m.Refresh(context.Background())

	after := serial()
	if before.Cmp(after) == 0 {
		t.Fatal("the listener still serves the old certificate: renewal would need a restart")
	}
	if calls, _, _ := client.snapshot(); calls != 2 {
		t.Fatalf("Obtain called %d times, want the initial order plus one renewal", calls)
	}
}

func TestRenewalWindowScalesWithTheCertificateLifetime(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		lifetime time.Duration
		wantLead time.Duration
	}{
		// The only profile Let's Encrypt issues for an IP address.
		{"shortlived six days", 6 * 24 * time.Hour, 48 * time.Hour},
		// A classic ninety day certificate: clamped to the thirty day lead.
		{"classic ninety days", 90 * 24 * time.Hour, 30 * 24 * time.Hour},
		// A two year self-signed certificate: clamped to the same maximum.
		{"self-signed two years", 730 * 24 * time.Hour, 30 * 24 * time.Hour},
		// Anything very short still leaves at least an hour to retry.
		{"one hour", time.Hour, time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			leaf := &x509.Certificate{NotBefore: now, NotAfter: now.Add(tc.lifetime)}
			want := leaf.NotAfter.Add(-tc.wantLead)
			if got := renewalTime(leaf); !got.Equal(want) {
				t.Fatalf("renewalTime = %s, want %s", got, want)
			}
		})
	}
}

func TestSelfSignedCertificateIsRecognisedAsSuch(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(t, dir)

	m, err := New(Options{Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(m.Stop)

	if !isSelfSigned(m.Certificate().Leaf) {
		t.Fatal("the generated bootstrap certificate is not recognised as self-signed, so ACME mode would never replace it")
	}

	chainPEM, keyPEM := issuePair(t, "panel.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	pair, err := parsePair(chainPEM, keyPEM)
	if err != nil {
		t.Fatalf("parsePair: %v", err)
	}
	if isSelfSigned(pair.Leaf) {
		t.Fatal("a CA-issued certificate is reported as self-signed, so it would be reissued on every check")
	}
}
