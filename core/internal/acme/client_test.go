package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeCA is an httptest-backed ACME server. It implements enough of RFC 8555 to
// drive a complete issuance and, more importantly, it verifies every JWS the
// client sends: algorithm, nonce, url, jwk/kid selection and - the part that
// matters most - that the signature is the fixed width r || s form rather than
// ASN.1 DER.
type fakeCA struct {
	t          *testing.T
	server     *httptest.Server
	solverBase string

	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate

	// nonces holds the nonces handed out and not yet spent.
	nonces map[string]bool
	// nonceCounter makes each nonce unique.
	nonceCounter int

	accountKey *ecdsa.PublicKey
	accountURL string

	// requests counts hits per path so a test can assert an account is reused.
	requests map[string]int

	// challengeToken is the token handed to the client, and challengeSeen
	// records the key authorization the CA read back from the solver.
	challengeToken string
	challengeSeen  string

	orderIdentifiers []Identifier
	orderProfile     string

	// authzPollsBeforeValid keeps the authorization pending for that many polls
	// so the backoff path is exercised.
	authzPollsBeforeValid int
	authzValid            bool

	// orderReady flips once the authorization is done, orderValid once the CSR
	// has been accepted.
	orderReady bool
	orderValid bool
	certPEM    []byte

	// injectBadNonce makes the first newOrder fail with badNonce so the retry
	// path is exercised.
	injectBadNonce bool
	badNonceFired  bool
}

func newFakeCA(t *testing.T, solverBase string) *fakeCA {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Fake ACME Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create test CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse test CA certificate: %v", err)
	}

	ca := &fakeCA{
		t:                     t,
		solverBase:            solverBase,
		caKey:                 caKey,
		caCert:                caCert,
		nonces:                make(map[string]bool),
		requests:              make(map[string]int),
		challengeToken:        "sJ2Kd1s3Q0v9-fake-token",
		authzPollsBeforeValid: 1,
	}
	ca.server = httptest.NewServer(http.HandlerFunc(ca.handle))
	t.Cleanup(ca.server.Close)
	return ca
}

// url builds an absolute URL on the fake CA.
func (ca *fakeCA) url(path string) string { return ca.server.URL + path }

// issueNonce mints a nonce and attaches it, which every ACME response must do.
func (ca *fakeCA) issueNonce(w http.ResponseWriter) {
	ca.nonceCounter++
	nonce := fmt.Sprintf("nonce-%d", ca.nonceCounter)
	ca.nonces[nonce] = true
	w.Header().Set("Replay-Nonce", nonce)
}

// writeJSON sends a JSON body with a status code.
func (ca *fakeCA) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		ca.t.Errorf("fake CA: write JSON response: %v", err)
	}
}

// writeProblem sends an RFC 7807 problem document.
func (ca *fakeCA) writeProblem(w http.ResponseWriter, status int, prob Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(prob); err != nil {
		ca.t.Errorf("fake CA: write problem document: %v", err)
	}
}

func (ca *fakeCA) handle(w http.ResponseWriter, r *http.Request) {
	ca.requests[r.URL.Path]++
	// Every ACME response carries a fresh nonce; that is what lets the client
	// avoid a newNonce round trip before each request.
	ca.issueNonce(w)

	switch r.URL.Path {
	case "/directory":
		ca.handleDirectory(w, r)
	case "/nonce":
		w.WriteHeader(http.StatusNoContent)
	case "/new-account":
		ca.handleNewAccount(w, r)
	case "/new-order":
		ca.handleNewOrder(w, r)
	case "/order/1":
		ca.handleOrder(w, r)
	case "/order/1/finalize":
		ca.handleFinalize(w, r)
	case "/authz/1":
		ca.handleAuthz(w, r)
	case "/chal/1":
		ca.handleChallenge(w, r)
	case "/cert/1":
		ca.handleCertificate(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (ca *fakeCA) handleDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ca.t.Errorf("fake CA: directory fetched with %s, want GET", r.Method)
	}
	ca.writeJSON(w, http.StatusOK, map[string]any{
		"newNonce":   ca.url("/nonce"),
		"newAccount": ca.url("/new-account"),
		"newOrder":   ca.url("/new-order"),
		"revokeCert": ca.url("/revoke-cert"),
		"keyChange":  ca.url("/key-change"),
		"meta": map[string]any{
			"termsOfService": ca.url("/tos"),
			"website":        ca.url("/"),
			"profiles": map[string]string{
				"classic":    "The same profile as always",
				"shortlived": "Certificates valid for about six days",
				"tlsserver":  "TLS server certificates only",
			},
		},
	})
}

// verify checks the JWS envelope and returns the decoded payload. It reports a
// test failure and answers with a malformed problem document when anything is
// wrong, which is exactly how a real CA would respond.
func (ca *fakeCA) verify(w http.ResponseWriter, r *http.Request) (protectedHeader, []byte, bool) {
	var zero protectedHeader

	if r.Method != http.MethodPost {
		ca.t.Errorf("fake CA: %s used %s, every ACME request must be a POST", r.URL.Path, r.Method)
	}
	if ct := r.Header.Get("Content-Type"); ct != contentTypeJOSE {
		ca.t.Errorf("fake CA: %s has Content-Type %q, want %q", r.URL.Path, ct, contentTypeJOSE)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		ca.t.Errorf("fake CA: read request body: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "unreadable body"})
		return zero, nil, false
	}

	var msg jwsMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		ca.t.Errorf("fake CA: request to %s is not a flattened JWS: %v", r.URL.Path, err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "not a JWS"})
		return zero, nil, false
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(msg.Protected)
	if err != nil {
		ca.t.Errorf("fake CA: protected header is not base64url: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "bad protected header"})
		return zero, nil, false
	}
	var header protectedHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		ca.t.Errorf("fake CA: protected header is not JSON: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "bad protected header"})
		return zero, nil, false
	}

	if header.Alg != "ES256" {
		ca.t.Errorf("fake CA: alg is %q, want ES256", header.Alg)
	}
	if want := ca.url(r.URL.Path); header.URL != want {
		ca.t.Errorf("fake CA: protected url is %q, want %q", header.URL, want)
	}

	if !ca.nonces[header.Nonce] {
		ca.writeProblem(w, http.StatusBadRequest, Problem{
			Type:   errBadNonce,
			Detail: fmt.Sprintf("JWS has an invalid anti-replay nonce: %q", header.Nonce),
		})
		return zero, nil, false
	}
	delete(ca.nonces, header.Nonce)

	// Pick the key: jwk on newAccount, kid on everything afterwards.
	var pub *ecdsa.PublicKey
	switch {
	case header.JWK != nil:
		if header.Kid != "" {
			ca.t.Error("fake CA: a JWS must not carry both jwk and kid")
		}
		pub, err = publicKeyFromJWK(header.JWK)
		if err != nil {
			ca.t.Errorf("fake CA: unusable jwk: %v", err)
			ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "bad jwk"})
			return zero, nil, false
		}
	default:
		if header.Kid == "" {
			ca.t.Errorf("fake CA: request to %s carries neither jwk nor kid", r.URL.Path)
			ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "no key"})
			return zero, nil, false
		}
		if header.Kid != ca.accountURL {
			ca.t.Errorf("fake CA: kid is %q, want %q", header.Kid, ca.accountURL)
		}
		pub = ca.accountKey
	}

	signature, err := base64.RawURLEncoding.DecodeString(msg.Signature)
	if err != nil {
		ca.t.Errorf("fake CA: signature is not base64url: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "bad signature encoding"})
		return zero, nil, false
	}
	// This is the check a real CA performs silently and reports only as
	// "malformed": ES256 signatures are exactly 64 bytes of raw r || s.
	if len(signature) != 64 {
		ca.t.Errorf("fake CA: signature is %d bytes, want 64 (raw r || s, not ASN.1 DER)", len(signature))
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "signature is not the JOSE raw form"})
		return zero, nil, false
	}
	signingInput := msg.Protected + "." + msg.Payload
	digest := sha256.Sum256([]byte(signingInput))
	sigR := new(big.Int).SetBytes(signature[:32])
	sigS := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(pub, digest[:], sigR, sigS) {
		ca.t.Error("fake CA: signature does not verify")
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "signature does not verify"})
		return zero, nil, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(msg.Payload)
	if err != nil {
		ca.t.Errorf("fake CA: payload is not base64url: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:malformed", Detail: "bad payload encoding"})
		return zero, nil, false
	}
	return header, payload, true
}

// publicKeyFromJWK rebuilds a P-256 public key from its JWK form.
func publicKeyFromJWK(jwk *jsonWebKey) (*ecdsa.PublicKey, error) {
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported key %s/%s", jwk.Kty, jwk.Crv)
	}
	x, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, err
	}
	y, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, err
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, fmt.Errorf("coordinates are %d/%d bytes, want 32 each", len(x), len(y))
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}, nil
}

func (ca *fakeCA) handleNewAccount(w http.ResponseWriter, r *http.Request) {
	header, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if header.JWK == nil {
		ca.t.Error("fake CA: newAccount must be signed with jwk, not kid")
	}
	var req newAccountRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		ca.t.Errorf("fake CA: newAccount payload: %v", err)
	}
	if !req.TermsOfServiceAgreed {
		ca.writeProblem(w, http.StatusBadRequest, Problem{
			Type:   "urn:ietf:params:acme:error:malformed",
			Detail: "must agree to terms of service",
		})
		return
	}

	pub, err := publicKeyFromJWK(header.JWK)
	if err != nil {
		ca.t.Fatalf("fake CA: %v", err)
	}
	ca.accountKey = pub
	ca.accountURL = ca.url("/acct/1")

	w.Header().Set("Location", ca.accountURL)
	ca.writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "valid",
		"contact": req.Contact,
		"orders":  ca.url("/acct/1/orders"),
	})
}

func (ca *fakeCA) handleNewOrder(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}

	// Exercise the badNonce recovery path: the first attempt is rejected even
	// though the nonce was fine, and the client must fetch a fresh one and retry
	// exactly once.
	if ca.injectBadNonce && !ca.badNonceFired {
		ca.badNonceFired = true
		ca.writeProblem(w, http.StatusBadRequest, Problem{
			Type:   errBadNonce,
			Detail: "JWS has an invalid anti-replay nonce",
		})
		return
	}

	var req newOrderRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		ca.t.Errorf("fake CA: newOrder payload: %v", err)
	}
	ca.orderIdentifiers = req.Identifiers
	ca.orderProfile = req.Profile

	w.Header().Set("Location", ca.url("/order/1"))
	ca.writeJSON(w, http.StatusCreated, ca.orderBody())
}

// orderBody renders the current order state.
func (ca *fakeCA) orderBody() map[string]any {
	status := statusPending
	switch {
	case ca.orderValid:
		status = statusValid
	case ca.orderReady:
		status = statusReady
	}
	body := map[string]any{
		"status":         status,
		"expires":        time.Now().Add(time.Hour).Format(time.RFC3339),
		"identifiers":    ca.orderIdentifiers,
		"authorizations": []string{ca.url("/authz/1")},
		"finalize":       ca.url("/order/1/finalize"),
	}
	if ca.orderProfile != "" {
		body["profile"] = ca.orderProfile
	}
	if ca.orderValid {
		body["certificate"] = ca.url("/cert/1")
	}
	return body
}

func (ca *fakeCA) handleOrder(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if len(payload) != 0 {
		ca.t.Errorf("fake CA: reading an order must be POST-as-GET, got payload %s", payload)
	}
	ca.writeJSON(w, http.StatusOK, ca.orderBody())
}

func (ca *fakeCA) handleAuthz(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if len(payload) != 0 {
		ca.t.Errorf("fake CA: reading an authorization must be POST-as-GET, got payload %s", payload)
	}

	status := statusPending
	challengeStatus := statusPending
	if ca.authzValid {
		if ca.authzPollsBeforeValid > 0 {
			// Stay pending for a moment so the client's backoff loop runs.
			ca.authzPollsBeforeValid--
			challengeStatus = statusProcessing
		} else {
			status = statusValid
			challengeStatus = statusValid
			ca.orderReady = true
		}
	}

	ca.writeJSON(w, http.StatusOK, map[string]any{
		"status":     status,
		"expires":    time.Now().Add(time.Hour).Format(time.RFC3339),
		"identifier": ca.orderIdentifiers[0],
		"challenges": []map[string]any{
			{
				// A challenge type the client must skip over.
				"type":   "tls-alpn-01",
				"url":    ca.url("/chal/2"),
				"status": statusPending,
				"token":  ca.challengeToken,
			},
			{
				"type":   ChallengeHTTP01,
				"url":    ca.url("/chal/1"),
				"status": challengeStatus,
				"token":  ca.challengeToken,
			},
		},
	})
}

func (ca *fakeCA) handleChallenge(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if string(payload) != "{}" {
		ca.t.Errorf("fake CA: accepting a challenge must send {}, got %q", payload)
	}

	// Actually fetch the token the way a CA would, from the solver the client
	// was given.
	resp, err := http.Get(ca.solverBase + ChallengePath + ca.challengeToken)
	if err != nil {
		ca.t.Errorf("fake CA: fetch challenge response: %v", err)
		ca.writeProblem(w, http.StatusBadRequest, Problem{Type: "urn:ietf:params:acme:error:connection", Detail: err.Error()})
		return
	}
	defer resp.Body.Close()
	served, err := io.ReadAll(resp.Body)
	if err != nil {
		ca.t.Errorf("fake CA: read challenge response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		ca.writeProblem(w, http.StatusBadRequest, Problem{
			Type:   "urn:ietf:params:acme:error:unauthorized",
			Detail: fmt.Sprintf("Invalid response: %d", resp.StatusCode),
		})
		return
	}
	ca.challengeSeen = string(served)

	expected, err := keyAuthorization(ca.challengeToken, ca.accountKey)
	if err != nil {
		ca.t.Fatalf("fake CA: build expected key authorization: %v", err)
	}
	if ca.challengeSeen != expected {
		ca.t.Errorf("fake CA: solver served %q, want %q", ca.challengeSeen, expected)
		ca.writeProblem(w, http.StatusBadRequest, Problem{
			Type:   "urn:ietf:params:acme:error:unauthorized",
			Detail: "key authorization does not match",
		})
		return
	}
	ca.authzValid = true

	ca.writeJSON(w, http.StatusOK, map[string]any{
		"type":   ChallengeHTTP01,
		"url":    ca.url("/chal/1"),
		"status": statusProcessing,
		"token":  ca.challengeToken,
	})
}

func (ca *fakeCA) handleFinalize(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if !ca.orderReady {
		ca.writeProblem(w, http.StatusForbidden, Problem{
			Type:   "urn:ietf:params:acme:error:orderNotReady",
			Detail: "order is not ready for finalization",
		})
		return
	}

	var req finalizeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		ca.t.Errorf("fake CA: finalize payload: %v", err)
	}
	csrDER, err := base64.RawURLEncoding.DecodeString(req.CSR)
	if err != nil {
		ca.t.Errorf("fake CA: CSR is not base64url: %v", err)
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		ca.t.Fatalf("fake CA: parse CSR: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		ca.t.Errorf("fake CA: CSR signature: %v", err)
	}

	// Issue a six day certificate, matching the shortlived profile.
	notBefore := time.Now().Add(-time.Minute).Truncate(time.Second)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "fake leaf"},
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(6 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca.caCert, csr.PublicKey, ca.caKey)
	if err != nil {
		ca.t.Fatalf("fake CA: issue leaf certificate: %v", err)
	}
	ca.certPEM = append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.caCert.Raw})...,
	)
	ca.orderValid = true

	w.Header().Set("Location", ca.url("/order/1"))
	// Answer "processing" once so the client polls the order at least once.
	body := ca.orderBody()
	body["status"] = statusProcessing
	delete(body, "certificate")
	ca.writeJSON(w, http.StatusOK, body)
}

func (ca *fakeCA) handleCertificate(w http.ResponseWriter, r *http.Request) {
	_, payload, ok := ca.verify(w, r)
	if !ok {
		return
	}
	if len(payload) != 0 {
		ca.t.Errorf("fake CA: downloading a certificate must be POST-as-GET, got payload %s", payload)
	}
	w.Header().Set("Content-Type", "application/pem-certificate-chain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(ca.certPEM); err != nil {
		ca.t.Errorf("fake CA: write certificate: %v", err)
	}
}

// testConfig builds a Client configuration pointed at the fake CA with fast
// polling so the test does not sleep for seconds.
func testConfig(ca *fakeCA, dir string) Config {
	return Config{
		DirectoryURL:    ca.url("/directory"),
		AccountDir:      dir,
		Email:           "ops@example.com",
		AgreeTOS:        true,
		Timeout:         30 * time.Second,
		PollInterval:    time.Millisecond,
		MaxPollInterval: 5 * time.Millisecond,
	}
}

func TestObtainHappyPath(t *testing.T) {
	solver, err := NewHTTP01Server("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start http-01 solver: %v", err)
	}
	defer solver.Close()
	solverBase := "http://" + solver.Addr().String()

	ca := newFakeCA(t, solverBase)
	// Also exercise the badNonce retry inside the happy path.
	ca.injectBadNonce = true

	dir := t.TempDir()
	client, err := New(testConfig(ca, dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The directory must expose the profiles so a caller can choose one.
	profiles, err := client.Profiles(context.Background())
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if !profiles.Has(ProfileShortLived) {
		t.Fatalf("profiles = %v, want the shortlived profile", profiles.Names())
	}

	ids := []Identifier{IPIdentifier("203.0.113.10")}
	chainPEM, keyPEM, err := client.Obtain(context.Background(), ids, ProfileShortLived, solver)
	if err != nil {
		t.Fatalf("Obtain: %v", err)
	}

	if !ca.badNonceFired {
		t.Fatal("the badNonce retry path was never exercised")
	}
	if ca.orderProfile != ProfileShortLived {
		t.Fatalf("the CA received profile %q, want %q", ca.orderProfile, ProfileShortLived)
	}
	if len(ca.orderIdentifiers) != 1 || ca.orderIdentifiers[0] != ids[0] {
		t.Fatalf("the CA received identifiers %v, want %v", ca.orderIdentifiers, ids)
	}

	// The solver must have served exactly the RFC 8555 key authorization.
	thumb, err := client.AccountKeyThumbprint()
	if err != nil {
		t.Fatalf("AccountKeyThumbprint: %v", err)
	}
	if want := ca.challengeToken + "." + thumb; ca.challengeSeen != want {
		t.Fatalf("solver served %q, want %q", ca.challengeSeen, want)
	}

	// The chain must parse and cover the ordered identifier.
	certs, err := ParseCertificateChainPEM(chainPEM)
	if err != nil {
		t.Fatalf("ParseCertificateChainPEM: %v", err)
	}
	if len(certs) != 2 {
		t.Fatalf("chain has %d certificates, want 2 (leaf plus issuer)", len(certs))
	}
	leaf := certs[0]
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "203.0.113.10" {
		t.Fatalf("leaf IP SANs = %v", leaf.IPAddresses)
	}

	// The returned key must be the one the leaf was issued for.
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		t.Fatalf("key PEM is not an EC PRIVATE KEY block: %q", keyPEM)
	}
	certKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse returned key: %v", err)
	}
	leafPub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("leaf public key is %T", leaf.PublicKey)
	}
	if !certKey.PublicKey.Equal(leafPub) {
		t.Fatal("the returned private key does not match the issued certificate")
	}

	// A six day certificate is fresh now and due for renewal after four days.
	if NeedsRenewal(leaf, time.Now()) {
		t.Fatal("a freshly issued 6 day certificate must not need renewal")
	}
	if !NeedsRenewal(leaf, time.Now().Add(5*24*time.Hour)) {
		t.Fatal("a 6 day certificate with one day left must need renewal")
	}

	// The challenge must have been withdrawn.
	resp, err := http.Get(solverBase + ChallengePath + ca.challengeToken)
	if err != nil {
		t.Fatalf("probe the solver after cleanup: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("the challenge token is still served after cleanup (HTTP %d)", resp.StatusCode)
	}

	// The account key and metadata must be on disk at 0600.
	for _, name := range []string{accountKeyFile, accountMetaFile} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("%s has mode %o, want 600", name, perm)
		}
	}
}

func TestAccountIsReusedAcrossClients(t *testing.T) {
	solver, err := NewHTTP01Server("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start http-01 solver: %v", err)
	}
	defer solver.Close()

	ca := newFakeCA(t, "http://"+solver.Addr().String())
	dir := t.TempDir()

	first, err := New(testConfig(ca, dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	account, err := first.Account(context.Background())
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if account.URL != ca.url("/acct/1") {
		t.Fatalf("account URL = %q", account.URL)
	}
	firstThumb, err := first.AccountKeyThumbprint()
	if err != nil {
		t.Fatalf("AccountKeyThumbprint: %v", err)
	}

	// A second client over the same directory must reuse both the key and the
	// stored account URL rather than registering again.
	second, err := New(testConfig(ca, dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reused, err := second.Account(context.Background())
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if reused.URL != account.URL {
		t.Fatalf("second client got account URL %q, want %q", reused.URL, account.URL)
	}
	secondThumb, err := second.AccountKeyThumbprint()
	if err != nil {
		t.Fatalf("AccountKeyThumbprint: %v", err)
	}
	if secondThumb != firstThumb {
		t.Fatal("the account key was regenerated instead of reused")
	}
	if ca.requests["/new-account"] != 1 {
		t.Fatalf("newAccount was called %d times, want 1", ca.requests["/new-account"])
	}
}

func TestObtainSurfacesRateLimitError(t *testing.T) {
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/directory", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce-1")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"newNonce":%q,"newAccount":%q,"newOrder":%q,"meta":{"profiles":{"shortlived":"six days"}}}`,
			base+"/nonce", base+"/new-account", base+"/new-order")
	})
	mux.HandleFunc("/nonce", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce-2")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/new-account", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce-3")
		w.Header().Set("Retry-After", "300")
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"type":"urn:ietf:params:acme:error:rateLimited","detail":"too many new accounts from this address"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base = server.URL

	client, err := New(Config{
		DirectoryURL: base + "/directory",
		AccountDir:   t.TempDir(),
		AgreeTOS:     true,
		Timeout:      10 * time.Second,
		PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = client.Obtain(context.Background(),
		[]Identifier{DNSIdentifier("panel.example.com")}, "", &HTTP01Server{tokens: map[string]string{}})
	if err == nil {
		t.Fatal("expected an error")
	}
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("expected a *RateLimitError, got %T: %v", err, err)
	}
	if rl.RetryAfter != 5*time.Minute {
		t.Fatalf("RetryAfter = %s, want 5m", rl.RetryAfter)
	}
}

func TestObtainRejectsUnknownProfile(t *testing.T) {
	solver, err := NewHTTP01Server("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start http-01 solver: %v", err)
	}
	defer solver.Close()

	ca := newFakeCA(t, "http://"+solver.Addr().String())
	client, err := New(testConfig(ca, t.TempDir()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = client.Obtain(context.Background(),
		[]Identifier{DNSIdentifier("panel.example.com")}, "does-not-exist", solver)
	if err == nil {
		t.Fatal("expected an error for a profile the CA does not advertise")
	}
	if ca.requests["/new-order"] != 0 {
		t.Fatal("no order should be created for an unknown profile")
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	if _, err := New(Config{AgreeTOS: true}); err == nil {
		t.Fatal("New must require AccountDir")
	}
	if _, err := New(Config{AccountDir: t.TempDir()}); err == nil {
		t.Fatal("New must require AgreeTOS")
	}
}

func TestWebrootSolver(t *testing.T) {
	root := t.TempDir()
	solver := NewWebrootSolver(root)

	if err := solver.Present("token-1", "token-1.thumb"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	path := filepath.Join(root, ".well-known", "acme-challenge", "token-1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read challenge file: %v", err)
	}
	if string(data) != "token-1.thumb" {
		t.Fatalf("challenge file contains %q", data)
	}

	if err := solver.CleanUp("token-1"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the challenge file was not removed")
	}
	// Cleaning up twice is not an error, because CleanUp runs on failure paths.
	if err := solver.CleanUp("token-1"); err != nil {
		t.Fatalf("second CleanUp: %v", err)
	}
	// A token must never be able to escape the webroot.
	if err := solver.Present("../../etc/passwd", "x"); err == nil {
		t.Fatal("a traversing token must be rejected")
	}
}
