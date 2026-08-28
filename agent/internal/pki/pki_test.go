package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================
// A CERTIFICATE AUTHORITY, STANDING IN FOR THE PANEL'S
// ============================================================

type testCA struct {
	key     *ecdsa.PrivateKey
	cert    *x509.Certificate
	certPEM []byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cannot create a CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("cannot parse the CA certificate: %v", err)
	}
	return &testCA{
		key:     key,
		cert:    cert,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

func (ca *testCA) fingerprint() string {
	sum := sha256.Sum256(ca.cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

func (ca *testCA) sign(t *testing.T, pub crypto.PublicKey, commonName, role string) (*x509.Certificate, []byte) {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 96))
	if err != nil {
		t.Fatalf("cannot generate a serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, OrganizationalUnit: []string{role}},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, pub, ca.key)
	if err != nil {
		t.Fatalf("cannot sign a certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("cannot parse a signed certificate: %v", err)
	}
	return cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// clientPair issues a complete key and certificate, as the panel would hold.
func (ca *testCA) clientPair(t *testing.T, commonName, role string) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a key: %v", err)
	}
	cert, certPEM := ca.sign(t, &key.PublicKey, commonName, role)
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("cannot marshal a key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("cannot build a pair: %v", err)
	}
	pair.Leaf = cert
	return &pair
}

// ============================================================
// A PANEL, STANDING IN FOR THE REAL ONE
// ============================================================

// fakePanel serves the three endpoints the agent calls. The CA it answers with
// is a parameter, so a test can hand back the wrong one on purpose.
type fakePanel struct {
	ca         *testCA
	answerWith []byte
	denied     []string
	issued     int
}

func (p *fakePanel) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agent-pki/enrol", func(w http.ResponseWriter, r *http.Request) {
		p.issue(t, w, r)
	})
	mux.HandleFunc("/api/v1/agent-pki/renew", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderSignature) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.issue(t, w, r)
	})
	mux.HandleFunc("/api/v1/agent-pki/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderSignature) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeData(w, map[string]any{"denied_serials": p.denied})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (p *fakePanel) issue(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	body, _ := io.ReadAll(r.Body)
	var req struct {
		CSRPEM string `json:"csr_pem"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	block, _ := pem.Decode([]byte(req.CSRPEM))
	if block == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	p.issued++
	cert, certPEM := p.ca.sign(t, csr.PublicKey, "agent-test", RoleAgent)
	answer := p.answerWith
	if answer == nil {
		answer = p.ca.certPEM
	}
	writeData(w, map[string]any{
		"agent_id":        "agent-test",
		"serial":          strings.ToLower(cert.SerialNumber.Text(16)),
		"certificate_pem": string(certPEM),
		"ca_pem":          string(answer),
		"not_after":       cert.NotAfter,
		"renew_after":     cert.NotAfter.Add(-12 * time.Hour),
	})
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func mintToken(ca *testCA) string {
	return strings.Join([]string{"vkai-enrol", "v1", "someid", "somesecret", ca.fingerprint()}, ".")
}

func newManager(t *testing.T, panelURL string) *Manager {
	t.Helper()
	return New(Options{
		Dir:      filepath.Join(t.TempDir(), "pki"),
		PanelURL: panelURL,
		Logger:   log.New(io.Discard, "", 0),
	})
}

// ============================================================
// ENROLMENT
// ============================================================

func TestEnrolStoresAUsableIdentityPrivately(t *testing.T) {
	ca := newTestCA(t)
	panel := &fakePanel{ca: ca}
	srv := panel.start(t)
	m := newManager(t, srv.URL)

	if err := m.Enrol(mintToken(ca), "node-1", "test"); err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	if m.AgentID() != "agent-test" || m.Serial() == "" {
		t.Fatalf("enrolment left no identity: id=%q serial=%q", m.AgentID(), m.Serial())
	}
	for _, name := range []string{keyFile, certFile, caFile, stateFile} {
		info, err := os.Stat(filepath.Join(m.dir, name))
		if err != nil {
			t.Fatalf("%s was not written: %v", name, err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s has mode %#o, want 0600", name, mode)
		}
	}

	// A restart must find the same identity without enrolling again: the token
	// is spent and there is nothing left to enrol with.
	again := New(Options{Dir: m.dir, PanelURL: srv.URL, Logger: log.New(io.Discard, "", 0)})
	if err := again.Load(); err != nil {
		t.Fatalf("a restarted agent could not load its identity: %v", err)
	}
	if again.Serial() != m.Serial() {
		t.Fatal("a restarted agent loaded a different identity")
	}
	if panel.issued != 1 {
		t.Fatalf("the panel issued %d certificates, want 1", panel.issued)
	}
}

func TestEnrolRefusesACAThatDoesNotMatchTheToken(t *testing.T) {
	realCA := newTestCA(t)
	otherCA := newTestCA(t)
	// The panel - or something pretending to be it - answers with a CA other
	// than the one the operator's token names.
	panel := &fakePanel{ca: otherCA, answerWith: otherCA.certPEM}
	srv := panel.start(t)
	m := newManager(t, srv.URL)

	err := m.Enrol(mintToken(realCA), "node-1", "test")
	if err == nil {
		t.Fatal("the agent accepted a CA that the enrolment token did not name")
	}
	if !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("the failure was reported as %v, which does not name the cause", err)
	}
	if _, statErr := os.Stat(filepath.Join(m.dir, keyFile)); statErr == nil {
		t.Fatal("a refused enrolment still wrote a key")
	}
}

func TestEnrolRefusesAMalformedToken(t *testing.T) {
	ca := newTestCA(t)
	srv := (&fakePanel{ca: ca}).start(t)
	m := newManager(t, srv.URL)
	for _, token := range []string{"", "nonsense", "vkai-enrol.v2.a.b.c", "vkai-enrol.v1.a.b"} {
		if err := m.Enrol(token, "node-1", "test"); err == nil {
			t.Fatalf("the agent accepted the token %q", token)
		}
	}
}

func TestLoadReportsThatTheAgentIsNotEnrolled(t *testing.T) {
	m := newManager(t, "http://127.0.0.1:1")
	if err := m.Load(); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("Load returned %v, want ErrNotEnrolled", err)
	}
}

// ============================================================
// ROTATION
// ============================================================

func TestRenewReplacesTheIdentityAndSurvivesARestart(t *testing.T) {
	ca := newTestCA(t)
	srv := (&fakePanel{ca: ca}).start(t)
	m := newManager(t, srv.URL)

	if err := m.Enrol(mintToken(ca), "node-1", "test"); err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	first := m.Serial()
	if err := m.Renew("node-1"); err != nil {
		t.Fatalf("renewal failed: %v", err)
	}
	if m.Serial() == first {
		t.Fatal("renewal did not change the certificate in use")
	}
	again := New(Options{Dir: m.dir, PanelURL: srv.URL, Logger: log.New(io.Discard, "", 0)})
	if err := again.Load(); err != nil {
		t.Fatalf("the rotated identity does not load: %v", err)
	}
	if again.Serial() != m.Serial() {
		t.Fatal("a restart after rotation loaded the wrong certificate")
	}
}

func TestRenewBeforeEnrolmentIsRefused(t *testing.T) {
	m := newManager(t, "http://127.0.0.1:1")
	if err := m.Renew("node-1"); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("Renew returned %v, want ErrNotEnrolled", err)
	}
}

// ============================================================
// SERVING THE PANEL
// ============================================================

// startAgentServer runs the agent's own listener with its real TLS
// configuration.
func startAgentServer(t *testing.T, m *Manager) *httptest.Server {
	t.Helper()
	cfg, err := m.ServerTLSConfig()
	if err != nil {
		t.Fatalf("cannot build the agent TLS configuration: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.TLS.PeerCertificates[0].Subject.CommonName)
	}))
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func callAgent(srv *httptest.Server, pair *tls.Certificate) (string, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*pair},
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Get(srv.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func enrolledManager(t *testing.T, ca *testCA) *Manager {
	t.Helper()
	srv := (&fakePanel{ca: ca}).start(t)
	m := newManager(t, srv.URL)
	if err := m.Enrol(mintToken(ca), "node-1", "test"); err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	return m
}

func TestAgentAcceptsItsPanelAndNobodyElse(t *testing.T) {
	ca := newTestCA(t)
	strangerCA := newTestCA(t)
	m := enrolledManager(t, ca)
	srv := startAgentServer(t, m)

	if name, err := callAgent(srv, ca.clientPair(t, "panel", RolePanel)); err != nil || name != "panel" {
		t.Fatalf("the agent refused its own panel: %v (%q)", err, name)
	}
	if _, err := callAgent(srv, strangerCA.clientPair(t, "panel", RolePanel)); err == nil {
		t.Fatal("the agent accepted a client certificate from another CA")
	}
	if _, err := callAgent(srv, ca.clientPair(t, "agent-other", RoleAgent)); err == nil {
		t.Fatal("the agent accepted another agent's certificate as if it were the panel")
	}
}

func TestDenyListStopsARevokedPanelCertificate(t *testing.T) {
	ca := newTestCA(t)
	m := enrolledManager(t, ca)
	srv := startAgentServer(t, m)

	panelPair := ca.clientPair(t, "panel", RolePanel)
	if _, err := callAgent(srv, panelPair); err != nil {
		t.Fatalf("the agent refused its panel before revocation: %v", err)
	}
	m.ApplyDenyList([]string{strings.ToLower(panelPair.Leaf.SerialNumber.Text(16))})
	if _, err := callAgent(srv, panelPair); err == nil {
		t.Fatal("the agent still accepted a certificate on its deny list")
	}
	if m.DeniedCount() != 1 {
		t.Fatalf("the deny list holds %d entries, want 1", m.DeniedCount())
	}
}

func TestStatusReportCarriesASignatureAndTakesTheDenyListBack(t *testing.T) {
	ca := newTestCA(t)
	panel := &fakePanel{ca: ca, denied: []string{"deadbeef"}}
	srv := panel.start(t)
	m := newManager(t, srv.URL)
	if err := m.Enrol(mintToken(ca), "node-1", "test"); err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	if err := m.StatusReport(map[string]string{"hello": "panel"}); err != nil {
		t.Fatalf("the status report failed: %v", err)
	}
	if m.DeniedCount() != 1 {
		t.Fatalf("the deny list from the reply was not applied: %d entries", m.DeniedCount())
	}
}

func TestSignedHeadersAreVerifiableWithTheIssuedPublicKey(t *testing.T) {
	ca := newTestCA(t)
	m := enrolledManager(t, ca)

	body := []byte(`{"cpu":42}`)
	headers, err := m.SignedHeaders(body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(headers[HeaderSignature])
	if err != nil {
		t.Fatalf("the signature is not base64: %v", err)
	}
	message := signingMessage(headers[HeaderAgentID], headers[HeaderSerial],
		headers[HeaderTimestamp], headers[HeaderNonce], body)
	digest := sha256.Sum256(message)

	pub, ok := m.cert.Leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("the issued certificate does not carry an ECDSA key")
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		t.Fatal("the signature does not verify against the certificate the panel issued")
	}
	// A different body must not verify against the same signature.
	other := sha256.Sum256(signingMessage(headers[HeaderAgentID], headers[HeaderSerial],
		headers[HeaderTimestamp], headers[HeaderNonce], []byte(`{"cpu":0}`)))
	if ecdsa.VerifyASN1(pub, other[:], sig) {
		t.Fatal("the signature verifies over a body it did not cover")
	}
}
