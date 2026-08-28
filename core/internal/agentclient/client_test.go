package agentclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
)

// A stand-in for the agent: it answers the operation envelope and records what
// it was asked for, so the test can assert the panel never sends anything but a
// named operation.
type stubAgent struct {
	lastOperation string
	lastArgs      map[string]any
}

func (s *stubAgent) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastOperation = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&s.lastArgs)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"name": "nginx", "status": "active"},
		})
	})
}

func newAuthority(t *testing.T) *agentpki.Authority {
	t.Helper()
	a, err := agentpki.New(agentpki.Options{Dir: t.TempDir(), Store: agentpki.NewMemoryStore()})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	return a
}

// enrolAgent runs a real enrolment and returns the resulting TLS material.
func enrolAgent(t *testing.T, a *agentpki.Authority) (string, *tls.Certificate) {
	t.Helper()
	invite, err := a.MintEnrolment(context.Background(), "", "node-1", "operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a key: %v", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "node-1"},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}, key)
	if err != nil {
		t.Fatalf("cannot create a certificate request: %v", err)
	}
	issued, err := a.Enrol(context.Background(), agentpki.EnrolRequest{
		Token:  invite.Token,
		CSRPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})),
	})
	if err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("cannot marshal the key: %v", err)
	}
	pair, err := tls.X509KeyPair(issued.CertPEM,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatalf("cannot build the pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("cannot parse the issued certificate: %v", err)
	}
	pair.Leaf = leaf
	return issued.AgentID, &pair
}

func startStubAgent(t *testing.T, a *agentpki.Authority, pair *tls.Certificate, stub *stubAgent) *httptest.Server {
	t.Helper()
	pool := x509.NewCertPool()
	block, _ := pem.Decode(a.CACertPEM())
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("cannot parse the CA certificate: %v", err)
	}
	pool.AddCert(caCert)

	srv := httptest.NewUnstartedServer(stub.handler())
	srv.TLS = agentpki.ServerTLSConfig(pair, agentpki.PanelVerifier{Pool: pool})
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestNamedOperationTravelsOverMutualTLS(t *testing.T) {
	authority := newAuthority(t)
	agentID, pair := enrolAgent(t, authority)
	stub := &stubAgent{}
	srv := startStubAgent(t, authority, pair, stub)

	client := New(authority, nil)
	state, err := client.ServiceControl(context.Background(),
		Target{AgentID: agentID, Address: srv.URL}, "nginx", "restart")
	if err != nil {
		t.Fatalf("service.control failed: %v", err)
	}
	if state.Status != "active" {
		t.Fatalf("the agent reported %q", state.Status)
	}
	if stub.lastOperation != "/v1/ops/service.control" {
		t.Fatalf("the panel asked for %q, want a named operation", stub.lastOperation)
	}
	if stub.lastArgs["name"] != "nginx" || stub.lastArgs["action"] != "restart" {
		t.Fatalf("the arguments arrived as %v", stub.lastArgs)
	}
}

func TestARevokedAgentCannotBeCalled(t *testing.T) {
	authority := newAuthority(t)
	agentID, pair := enrolAgent(t, authority)
	srv := startStubAgent(t, authority, pair, &stubAgent{})

	client := New(authority, nil)
	target := Target{AgentID: agentID, Address: srv.URL}
	if _, err := client.AgentInfo(context.Background(), target); err != nil {
		t.Fatalf("the agent was unreachable before revocation: %v", err)
	}
	if err := authority.Revoke(context.Background(), agentID, "test"); err != nil {
		t.Fatalf("revocation failed: %v", err)
	}
	// The pooled connection must not outlive the decision.
	client.Forget(agentID)
	if _, err := client.AgentInfo(context.Background(), target); err == nil {
		t.Fatal("the panel still reached a revoked agent")
	}
}

func TestAnAgentServedByAnotherCAIsRefused(t *testing.T) {
	authority := newAuthority(t)
	other := newAuthority(t)
	agentID, _ := enrolAgent(t, authority)
	_, strangerPair := enrolAgent(t, other)

	srv := startStubAgent(t, authority, strangerPair, &stubAgent{})
	client := New(authority, nil)
	if _, err := client.AgentInfo(context.Background(), Target{AgentID: agentID, Address: srv.URL}); err == nil {
		t.Fatal("the panel accepted an agent certificate from another CA")
	}
}

func TestAnOperationNameCannotCarryAPath(t *testing.T) {
	authority := newAuthority(t)
	client := New(authority, nil)
	err := client.Call(context.Background(),
		Target{AgentID: "agent-1", Address: "127.0.0.1:30111"}, "../../execute", nil, nil)
	if err == nil {
		t.Fatal("an operation name containing a path was accepted")
	}
}
