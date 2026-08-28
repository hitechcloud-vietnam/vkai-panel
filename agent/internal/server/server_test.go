package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/ops"
)

// These tests are about the routing surface: what the agent answers and what it
// no longer answers. Who is allowed to ask is tested in internal/pki.

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate a key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "agent-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cannot create a certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("cannot parse a certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}
}

func start(t *testing.T, allowRawExec bool) string {
	t.Helper()
	registry := ops.New(ops.Deps{
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("active"), nil
		},
		AllowRawExec:  allowRawExec,
		Logger:        log.New(io.Discard, "", 0),
		ApplyDenyList: func([]string) {},
		Info:          func() ops.AgentInfo { return ops.AgentInfo{Version: "test"} },
	})
	srv, err := New(Options{
		Addr:      "127.0.0.1:0",
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{selfSigned(t)}},
		Registry:  registry,
		Logger:    log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("cannot build the server: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	go func() { _ = srv.http.ServeTLS(listener, "", "") }()
	t.Cleanup(func() { _ = srv.http.Close() })
	return "https://" + listener.Addr().String()
}

func call(t *testing.T, method, url string, body string) (int, ops.Response) {
	t.Helper()
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("cannot build a request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	var decoded ops.Response
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

func TestTheExecuteEndpointIsGone(t *testing.T) {
	base := start(t, false)
	for _, path := range []string{"/execute", "/execute/", "/v1/ops/exec.raw"} {
		status, body := call(t, http.MethodPost, base+path, `{"command":"id"}`)
		if status != http.StatusNotFound {
			t.Fatalf("%s answered %d, want 404", path, status)
		}
		if body.OK {
			t.Fatalf("%s reported success", path)
		}
	}
}

func TestNamedOperationRuns(t *testing.T) {
	base := start(t, false)
	status, body := call(t, http.MethodPost, base+"/v1/ops/system.info", "")
	if status != http.StatusOK || !body.OK {
		t.Fatalf("system.info answered %d (ok=%v, error=%q)", status, body.OK, body.Error)
	}
}

func TestAnUnknownOperationIsRefusedWithoutRunningAnything(t *testing.T) {
	base := start(t, false)
	status, body := call(t, http.MethodPost, base+"/v1/ops/rm", `{"args":["-rf","/"]}`)
	if status != http.StatusNotFound || body.OK {
		t.Fatalf("an unknown operation answered %d (ok=%v)", status, body.OK)
	}
}

func TestAnInvalidArgumentIsARefusalNotAFailure(t *testing.T) {
	base := start(t, false)
	status, body := call(t, http.MethodPost, base+"/v1/ops/service.control",
		`{"name":"sshd","action":"restart"}`)
	if status != http.StatusBadRequest || body.OK {
		t.Fatalf("service.control on an unmanaged unit answered %d (ok=%v)", status, body.OK)
	}
}

func TestTheOperationListIsDiscoverable(t *testing.T) {
	base := start(t, false)
	status, body := call(t, http.MethodGet, base+"/v1/operations", "")
	if status != http.StatusOK || !body.OK {
		t.Fatalf("the operation list answered %d", status)
	}
	names, ok := body.Result.([]any)
	if !ok || len(names) == 0 {
		t.Fatalf("the operation list is empty: %v", body.Result)
	}
}
