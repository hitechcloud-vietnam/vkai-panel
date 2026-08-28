package handler

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
)

// These tests drive the agent-facing half of the PKI routes over real HTTP, in
// the order an agent lives them: enrol once, report in, rotate, get revoked.

func newTestRouter(t *testing.T) (*gin.Engine, *agentpki.Authority) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	authority, err := agentpki.New(agentpki.Options{
		Dir:   t.TempDir(),
		Store: agentpki.NewMemoryStore(),
	})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	engine := gin.New()
	RegisterAgentPKIRoutes(engine.Group("/api/v1"), NewAgentPKIHandler(authority, nil, nil))
	return engine, authority
}

func newAgentCSR(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a key: %v", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "node-1.example.vn"},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}, key)
	if err != nil {
		t.Fatalf("cannot create a certificate request: %v", err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

// post sends a JSON body and returns the status and the decoded data envelope.
func post(t *testing.T, engine *gin.Engine, path string, body []byte, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
	return rec.Code, envelope.Data
}

func signedHeaders(t *testing.T, agentID, serial string, key *ecdsa.PrivateKey, body []byte) map[string]string {
	t.Helper()
	hdr, err := agentpki.SignRequest(agentID, serial, key, time.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign the request: %v", err)
	}
	return map[string]string{
		agentpki.HeaderAgentID:   hdr.AgentID,
		agentpki.HeaderSerial:    hdr.Serial,
		agentpki.HeaderTimestamp: hdr.Timestamp,
		agentpki.HeaderNonce:     hdr.Nonce,
		agentpki.HeaderSignature: hdr.Signature,
	}
}

func TestAgentLifecycleOverHTTP(t *testing.T) {
	engine, authority := newTestRouter(t)

	invite, err := authority.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	key, csrPEM := newAgentCSR(t)

	enrolBody, _ := json.Marshal(map[string]string{
		"token":         invite.Token,
		"csr_pem":       csrPEM,
		"hostname":      "node-1.example.vn",
		"agent_version": "test",
	})
	status, data := post(t, engine, "/api/v1/agent-pki/enrol", enrolBody, nil)
	if status != http.StatusCreated {
		t.Fatalf("enrolment returned %d, want 201", status)
	}
	agentID, _ := data["agent_id"].(string)
	serial, _ := data["serial"].(string)
	if agentID == "" || serial == "" {
		t.Fatalf("enrolment returned no identity: %v", data)
	}
	if capem, _ := data["ca_pem"].(string); capem == "" {
		t.Fatal("enrolment did not return the CA certificate, so the agent has no trust anchor")
	}

	// The same token again is refused: it was spent by the call above.
	status, _ = post(t, engine, "/api/v1/agent-pki/enrol", enrolBody, nil)
	if status != http.StatusConflict {
		t.Fatalf("a reused enrolment token returned %d, want 409", status)
	}

	// Reporting in, signed with the key that never left the managed server.
	statusBody := []byte(`{"agent_version":"test"}`)
	code, data := post(t, engine, "/api/v1/agent-pki/status", statusBody,
		signedHeaders(t, agentID, serial, key, statusBody))
	if code != http.StatusOK {
		t.Fatalf("the status report returned %d, want 200", code)
	}
	if _, ok := data["denied_serials"]; !ok {
		t.Fatal("the status reply carries no deny list, so an agent cannot learn about a revoked panel certificate")
	}

	// An unsigned report is refused.
	code, _ = post(t, engine, "/api/v1/agent-pki/status", statusBody, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("an unsigned status report returned %d, want 401", code)
	}

	// Rotation, authenticated by the certificate the agent already holds.
	_, nextCSR := newAgentCSR(t)
	renewBody, _ := json.Marshal(map[string]string{"csr_pem": nextCSR})
	code, data = post(t, engine, "/api/v1/agent-pki/renew", renewBody,
		signedHeaders(t, agentID, serial, key, renewBody))
	if code != http.StatusOK {
		t.Fatalf("renewal returned %d, want 200", code)
	}
	if next, _ := data["serial"].(string); next == "" || next == serial {
		t.Fatalf("renewal did not issue a new serial: %v", data["serial"])
	}

	// The previous certificate still authenticates a report: the overlap window
	// is open, so a rotation whose answer was lost has not stranded the agent.
	code, _ = post(t, engine, "/api/v1/agent-pki/status", statusBody,
		signedHeaders(t, agentID, serial, key, statusBody))
	if code != http.StatusOK {
		t.Fatalf("the previous certificate was refused during the overlap window: %d", code)
	}

	// Revocation bites immediately.
	if err := authority.Revoke(context.Background(), agentID, "test"); err != nil {
		t.Fatalf("revocation failed: %v", err)
	}
	code, _ = post(t, engine, "/api/v1/agent-pki/status", statusBody,
		signedHeaders(t, agentID, serial, key, statusBody))
	if code != http.StatusForbidden {
		t.Fatalf("a revoked agent's report returned %d, want 403", code)
	}
}

func TestExpiredEnrolmentTokenIsRefusedOverHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	authority, err := agentpki.New(agentpki.Options{
		Dir:          t.TempDir(),
		Store:        agentpki.NewMemoryStore(),
		EnrolmentTTL: time.Minute,
		Now:          func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("cannot create the authority: %v", err)
	}
	engine := gin.New()
	RegisterAgentPKIRoutes(engine.Group("/api/v1"), NewAgentPKIHandler(authority, nil, nil))

	invite, err := authority.MintEnrolment(context.Background(), "", "node-1.example.vn", "operator", time.Minute)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	now = now.Add(2 * time.Minute)

	_, csrPEM := newAgentCSR(t)
	body, _ := json.Marshal(map[string]string{"token": invite.Token, "csr_pem": csrPEM})
	code, _ := post(t, engine, "/api/v1/agent-pki/enrol", body, nil)
	if code != http.StatusGone {
		t.Fatalf("an expired enrolment token returned %d, want 410", code)
	}
}

func TestEnrolmentWithAGarbageTokenIsRefused(t *testing.T) {
	engine, _ := newTestRouter(t)
	_, csrPEM := newAgentCSR(t)
	body, _ := json.Marshal(map[string]string{"token": "not-a-token", "csr_pem": csrPEM})
	code, _ := post(t, engine, "/api/v1/agent-pki/enrol", body, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("a garbage token returned %d, want 401", code)
	}
}

// TestRoutesRegisterFromTheDefaultLocation exercises the exact expression that
// router.go installs, so the one line it has to carry is known to compile and
// to produce a working CA on disk.
func TestRoutesRegisterFromTheDefaultLocation(t *testing.T) {
	t.Setenv("VKAI_SSL_ROOT", t.TempDir())
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	RegisterAgentPKIRoutes(engine.Group("/api/v1"), NewAgentPKIHandlerFromEnv(nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-pki/ca", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the CA endpoint answered %d, want 200", rec.Code)
	}
	var envelope struct {
		Data struct {
			CAPEM       string `json:"ca_pem"`
			Fingerprint string `json:"ca_fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("the CA endpoint returned an unreadable body: %v", err)
	}
	if !strings.Contains(envelope.Data.CAPEM, "BEGIN CERTIFICATE") || envelope.Data.Fingerprint == "" {
		t.Fatal("the CA endpoint returned no usable trust anchor")
	}
}
