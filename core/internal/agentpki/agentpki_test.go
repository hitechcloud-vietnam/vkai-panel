package agentpki

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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a movable time source, so rotation, overlap and expiry can be tested
// without any test sleeping.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newAuthority(t *testing.T, clk *clock) *Authority {
	t.Helper()
	a, err := New(Options{
		Dir:     t.TempDir(),
		Store:   NewMemoryStore(),
		CertTTL: 72 * time.Hour,
		// A short renewal margin keeps the freshly issued panel certificate out
		// of its own renewal window for the whole test.
		RenewBefore:  time.Hour,
		Overlap:      2 * time.Hour,
		EnrolmentTTL: 30 * time.Minute,
		Now:          clk.Now,
	})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	return a
}

// agentIdentity is one enrolled agent, as it would exist on a managed server.
type agentIdentity struct {
	id      string
	key     *ecdsa.PrivateKey
	pair    *tls.Certificate
	issued  *Issued
	certPEM []byte
}

func newCSR(t *testing.T, commonName string) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a key: %v", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: commonName},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}, key)
	if err != nil {
		t.Fatalf("cannot create a certificate request: %v", err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func pairFrom(t *testing.T, key *ecdsa.PrivateKey, certPEM []byte) *tls.Certificate {
	t.Helper()
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("cannot marshal a key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("cannot load the issued pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("cannot parse the issued certificate: %v", err)
	}
	pair.Leaf = leaf
	return &pair
}

// enrol runs the whole enrolment: mint a token, generate a key and a request on
// the "agent", trade the token for a certificate.
func enrol(t *testing.T, a *Authority, hostname string) *agentIdentity {
	t.Helper()
	invite, err := a.MintEnrolment(context.Background(), "", hostname, "test-operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	key, csrPEM := newCSR(t, hostname)
	issued, err := a.Enrol(context.Background(), EnrolRequest{
		Token:        invite.Token,
		CSRPEM:       string(csrPEM),
		Hostname:     hostname,
		AgentVersion: "test",
	})
	if err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	return &agentIdentity{
		id:      issued.AgentID,
		key:     key,
		pair:    pairFrom(t, key, issued.CertPEM),
		issued:  issued,
		certPEM: issued.CertPEM,
	}
}

// startAgent serves one endpoint over the agent's half of the channel: present
// the agent certificate, demand the panel's, verify it against the panel CA.
func startAgent(t *testing.T, a *Authority, id *agentIdentity, clk *clock, denied map[string]bool) *httptest.Server {
	t.Helper()
	pool := x509.NewCertPool()
	block, _ := pem.Decode(a.CACertPEM())
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("cannot parse the CA certificate: %v", err)
	}
	pool.AddCert(caCert)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok:"+r.TLS.PeerCertificates[0].Subject.CommonName)
	}))
	srv.TLS = ServerTLSConfig(id.pair, PanelVerifier{Pool: pool, Denied: denied, Now: clk.Now})
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// dial makes one request from the panel to an agent on a connection of its own,
// so nothing is answered out of a pool that predates the decision under test.
func dial(a *Authority, srv *httptest.Server, agentID string) (string, error) {
	transport := &http.Transport{
		TLSClientConfig:   a.ClientTLSConfig(agentID),
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

// ============================================================
// ISSUANCE AND MUTUAL VERIFICATION
// ============================================================

func TestPanelAndAgentVerifyEachOther(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")
	srv := startAgent(t, a, id, clk, nil)

	body, err := dial(a, srv, id.id)
	if err != nil {
		t.Fatalf("the panel could not talk to the agent: %v", err)
	}
	// The agent saw the panel's certificate, and the panel accepted the agent's:
	// both directions were verified inside one handshake.
	if body != "ok:"+PanelAgentID {
		t.Fatalf("the agent saw %q, want the panel's client certificate", body)
	}
}

func TestPanelRefusesAnAgentItDidNotIssue(t *testing.T) {
	clk := newClock()
	panelA := newAuthority(t, clk)
	// A second panel, with its own CA. Its agent is perfectly valid - to it.
	panelB := newAuthority(t, clk)

	known := enrol(t, panelA, "node-1.example.vn")
	stranger := enrol(t, panelB, "node-1.example.vn")

	// The stranger serves, presenting a certificate from the wrong CA, while
	// claiming the identity of an agent panel A does know.
	strangerServing := &agentIdentity{id: known.id, pair: stranger.pair}
	srv := startAgent(t, panelA, strangerServing, clk, nil)

	if _, err := dial(panelA, srv, known.id); err == nil {
		t.Fatal("the panel accepted an agent certificate from another CA")
	}
}

func TestAgentRefusesAPanelItWasNotEnrolledWith(t *testing.T) {
	clk := newClock()
	panelA := newAuthority(t, clk)
	panelB := newAuthority(t, clk)

	id := enrol(t, panelA, "node-1.example.vn")
	srv := startAgent(t, panelA, id, clk, nil)

	// Panel B holds a valid client certificate - from its own CA - and knows the
	// agent's address. That must not be enough.
	if _, err := dial(panelB, srv, id.id); err == nil {
		t.Fatal("the agent accepted a client certificate from another CA")
	}
}

func TestPanelPinsTheCertificateNotTheName(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	first := enrol(t, a, "node-1.example.vn")
	second := enrol(t, a, "node-2.example.vn")

	// The second agent's certificate is genuine, issued by this very CA. It is
	// still not the certificate the panel expects from the first agent.
	impostor := &agentIdentity{id: first.id, pair: second.pair}
	srv := startAgent(t, a, impostor, clk, nil)

	if _, err := dial(a, srv, first.id); err == nil {
		t.Fatal("the panel accepted one agent's certificate in place of another's")
	}
}

func TestAgentRefusesAnAgentCertificateActingAsThePanel(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	server := enrol(t, a, "node-1.example.vn")
	other := enrol(t, a, "node-2.example.vn")
	srv := startAgent(t, a, server, clk, nil)

	// A compromised agent has a certificate from the right CA. It must not be
	// able to turn round and drive its neighbours: the role differs.
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*other.pair},
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("the agent accepted another agent's certificate as if it were the panel")
	}
}

// ============================================================
// ENROLMENT TOKENS
// ============================================================

func TestEnrolmentTokenWorksOnceOnly(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)

	invite, err := a.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	_, csrPEM := newCSR(t, "node-1.example.vn")
	if _, err := a.Enrol(context.Background(), EnrolRequest{Token: invite.Token, CSRPEM: string(csrPEM)}); err != nil {
		t.Fatalf("the first enrolment failed: %v", err)
	}

	// A second attempt with the same token - a replayed installer command, a
	// leaked terminal history - gets nothing.
	_, csrPEM2 := newCSR(t, "node-2.example.vn")
	_, err = a.Enrol(context.Background(), EnrolRequest{Token: invite.Token, CSRPEM: string(csrPEM2)})
	if !errors.Is(err, ErrTokenUsed) {
		t.Fatalf("the second enrolment returned %v, want ErrTokenUsed", err)
	}
}

func TestEnrolmentTokenExpires(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)

	invite, err := a.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", 10*time.Minute)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	clk.advance(11 * time.Minute)

	_, csrPEM := newCSR(t, "node-1.example.vn")
	_, err = a.Enrol(context.Background(), EnrolRequest{Token: invite.Token, CSRPEM: string(csrPEM)})
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("enrolment with an expired token returned %v, want ErrTokenExpired", err)
	}
}

func TestEnrolmentRefusesAWrongSecretWithoutSpendingTheToken(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)

	invite, err := a.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	parsed, err := ParseEnrolmentToken(invite.Token)
	if err != nil {
		t.Fatalf("cannot parse the token: %v", err)
	}
	// Right id, wrong secret: a guess against a token whose id was observed.
	forged := strings.Join([]string{"vkai-enrol", "v1", parsed.ID, "not-the-secret", parsed.CAFingerprint}, ".")

	_, csrPEM := newCSR(t, "node-1.example.vn")
	if _, err := a.Enrol(context.Background(), EnrolRequest{Token: forged, CSRPEM: string(csrPEM)}); !errors.Is(err, ErrBadToken) {
		t.Fatalf("a forged token returned %v, want ErrBadToken", err)
	}
	// The real token must still work: a wrong guess must not burn it.
	if _, err := a.Enrol(context.Background(), EnrolRequest{Token: invite.Token, CSRPEM: string(csrPEM)}); err != nil {
		t.Fatalf("the genuine token stopped working after a failed guess: %v", err)
	}
}

func TestEnrolmentTokenCarriesTheCAFingerprint(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	invite, err := a.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	parsed, err := ParseEnrolmentToken(invite.Token)
	if err != nil {
		t.Fatalf("cannot parse the token: %v", err)
	}
	if parsed.CAFingerprint != a.CAFingerprint() {
		t.Fatal("the token does not carry this CA's fingerprint, so an agent cannot check what it is given")
	}
}

// ============================================================
// ROTATION AND THE OVERLAP WINDOW
// ============================================================

func TestRotationKeepsTheOldCertificateWorkingForTheOverlapWindow(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	old := enrol(t, a, "node-1.example.vn")

	oldServer := startAgent(t, a, old, clk, nil)
	if _, err := dial(a, oldServer, old.id); err != nil {
		t.Fatalf("the freshly enrolled agent was refused: %v", err)
	}

	// Rotate. The agent has a new certificate now, but imagine it never
	// received the answer: it is still serving the old one.
	clk.advance(30 * time.Minute)
	key, csrPEM := newCSR(t, "node-1.example.vn")
	issued, err := a.Renew(context.Background(), old.id, csrPEM)
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if issued.Serial == old.issued.Serial {
		t.Fatal("rotation returned the same serial")
	}
	fresh := &agentIdentity{id: issued.AgentID, key: key, pair: pairFrom(t, key, issued.CertPEM), issued: issued}

	if _, err := dial(a, oldServer, old.id); err != nil {
		t.Fatalf("rotation locked out an agent still using its previous certificate: %v", err)
	}
	newServer := startAgent(t, a, fresh, clk, nil)
	if _, err := dial(a, newServer, fresh.id); err != nil {
		t.Fatalf("the newly issued certificate was refused: %v", err)
	}

	// Past the overlap window the previous certificate stops being accepted.
	clk.advance(2*time.Hour + time.Minute)
	if _, err := dial(a, oldServer, old.id); err == nil {
		t.Fatal("the previous certificate was still accepted after the overlap window closed")
	}
	if _, err := dial(a, newServer, fresh.id); err != nil {
		t.Fatalf("the current certificate stopped working: %v", err)
	}
}

func TestRotationTwiceRetiresTheOldestCertificate(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	first := enrol(t, a, "node-1.example.vn")
	firstServer := startAgent(t, a, first, clk, nil)

	for i := 0; i < 2; i++ {
		clk.advance(time.Minute)
		_, csrPEM := newCSR(t, "node-1.example.vn")
		if _, err := a.Renew(context.Background(), first.id, csrPEM); err != nil {
			t.Fatalf("rotation %d failed: %v", i, err)
		}
	}
	// Only one previous certificate is kept. The one before that is finished,
	// overlap window or not.
	if _, err := dial(a, firstServer, first.id); err == nil {
		t.Fatal("a certificate two rotations old was still accepted")
	}
}

// ============================================================
// REVOCATION
// ============================================================

func TestRevocationTakesEffectOnTheNextHandshake(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")
	srv := startAgent(t, a, id, clk, nil)

	if _, err := dial(a, srv, id.id); err != nil {
		t.Fatalf("the agent was refused before revocation: %v", err)
	}
	if err := a.Revoke(context.Background(), id.id, "key suspected leaked"); err != nil {
		t.Fatalf("revocation failed: %v", err)
	}
	// No clock advance: revocation is not something that happens at expiry.
	if _, err := dial(a, srv, id.id); err == nil {
		t.Fatal("a revoked agent was still accepted")
	}
}

func TestRevocationAlsoStopsSignedRequests(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")

	body := []byte(`{"hello":"panel"}`)
	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, body); err != nil {
		t.Fatalf("a genuine signed request was refused: %v", err)
	}
	if err := a.Revoke(context.Background(), id.id, "decommissioned"); err != nil {
		t.Fatalf("revocation failed: %v", err)
	}
	headers2, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers2, body); !errors.Is(err, ErrRevoked) {
		t.Fatalf("a revoked agent's signed request returned %v, want ErrRevoked", err)
	}
}

func TestRevokedPanelCertificateIsRefusedByTheAgent(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")

	panelPair, err := a.PanelClientCertificate(context.Background())
	if err != nil {
		t.Fatalf("cannot load the panel certificate: %v", err)
	}
	denied := map[string]bool{SerialString(panelPair.Leaf.SerialNumber): true}
	srv := startAgent(t, a, id, clk, denied)

	if _, err := dial(a, srv, id.id); err == nil {
		t.Fatal("the agent accepted a panel certificate that is on its deny list")
	}
}

// ============================================================
// SIGNED REQUESTS
// ============================================================

func TestSignedRequestRejectsATamperedBody(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")

	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), []byte(`{"cpu":10}`))
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, []byte(`{"cpu":99}`)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a tampered body returned %v, want ErrBadSignature", err)
	}
}

func TestSignedRequestRejectsAReplay(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")

	body := []byte(`{"cpu":10}`)
	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, body); err != nil {
		t.Fatalf("the first request was refused: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a replayed request returned %v, want a refusal", err)
	}
}

func TestSignedRequestRejectsAStaleTimestamp(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1.example.vn")

	body := []byte(`{}`)
	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now().Add(-time.Hour), body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a stale request returned %v, want a refusal", err)
	}
}

func TestSignedRequestRejectsAnUnknownAgent(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	other := newAuthority(t, clk)
	id := enrol(t, other, "node-1.example.vn")

	body := []byte(`{}`)
	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign: %v", err)
	}
	if _, err := a.VerifySignedRequest(context.Background(), headers, body); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("a stranger's signed request returned %v, want ErrUnknownAgent", err)
	}
}

// ============================================================
// CA MATERIAL ON DISK
// ============================================================

func TestCAIsReusedAcrossRestartsAndKeptPrivate(t *testing.T) {
	clk := newClock()
	dir := t.TempDir()
	store := NewMemoryStore()

	first, err := New(Options{Dir: dir, Store: store, Now: clk.Now})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	second, err := New(Options{Dir: dir, Store: store, Now: clk.Now})
	if err != nil {
		t.Fatalf("cannot reopen the authority: %v", err)
	}
	if first.CAFingerprint() != second.CAFingerprint() {
		t.Fatal("reopening the authority produced a different CA, which would lock out every enrolled agent")
	}
	for _, name := range []string{caKeyFile, caCertFile, panelKeyFile, panelCertFile} {
		info, statErr := osStat(t, dir, name)
		if statErr != nil {
			t.Fatalf("%s was not written: %v", name, statErr)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s has mode %#o, want 0600", name, mode)
		}
	}
}

func TestIssuedCertificatesAreShortLived(t *testing.T) {
	clk := newClock()
	a, err := New(Options{Dir: t.TempDir(), Store: NewMemoryStore(), Now: clk.Now})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	id := enrol(t, a, "node-1.example.vn")
	life := id.issued.NotAfter.Sub(clk.Now())
	if life > DefaultCertTTL+time.Minute {
		t.Fatalf("an issued certificate lives %s, want no more than %s", life, DefaultCertTTL)
	}
	if !id.issued.RenewAfter.Before(id.issued.NotAfter) {
		t.Fatal("the renewal point is not before expiry, so an agent would never rotate in time")
	}
}

// osStat is a small helper so the permission assertions read as one line each.
func osStat(t *testing.T, dir, name string) (os.FileInfo, error) {
	t.Helper()
	return os.Stat(filepath.Join(dir, name))
}
