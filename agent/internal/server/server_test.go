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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/audit"
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
	base, _ := startWithRecord(t, allowRawExec)
	return base
}

// startWithRecord returns the listener's base URL and the path of the node's
// operation record, so a test can assert what the agent wrote down about what
// it was asked to do.
func startWithRecord(t *testing.T, allowRawExec bool) (string, string) {
	t.Helper()
	recordPath := filepath.Join(t.TempDir(), "operations.log")
	record := audit.Open(audit.Options{Path: recordPath, Fallback: log.New(io.Discard, "", 0)})
	t.Cleanup(func() { _ = record.Close() })

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
		Audit:     record,
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
	return "https://" + listener.Addr().String(), recordPath
}

func recorded(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the operation record: %v", err)
	}
	return string(data)
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

// The record is written by the agent, on the node, so an operator can audit
// what this machine was told to do without asking the panel - which is the
// party whose compromise is the thing being investigated.
func TestEveryOperationIsRecordedOnTheNode(t *testing.T) {
	base, recordPath := startWithRecord(t, false)

	if status, _ := call(t, http.MethodPost, base+"/v1/ops/system.info", ""); status != http.StatusOK {
		t.Fatalf("system.info answered %d", status)
	}
	contents := recorded(t, recordPath)
	if !strings.Contains(contents, `"operation":"system.info"`) {
		t.Fatalf("the operation was not recorded: %s", contents)
	}
	if !strings.Contains(contents, `"outcome":"ok"`) {
		t.Fatalf("the outcome was not recorded: %s", contents)
	}
	if !strings.Contains(contents, `"actor"`) {
		t.Fatalf("the record does not name who asked: %s", contents)
	}
}

// A refusal is the line an operator most wants to find: it is what an attempt
// to reach past the named operations looks like.
func TestARefusedOperationIsRecordedWithItsArguments(t *testing.T) {
	base, recordPath := startWithRecord(t, false)

	call(t, http.MethodPost, base+"/v1/ops/service.control", `{"name":"sshd","action":"restart"}`)
	call(t, http.MethodPost, base+"/v1/ops/rm", `{"args":["-rf","/"]}`)
	call(t, http.MethodPost, base+"/execute", `{"command":"id"}`)

	contents := recorded(t, recordPath)
	for _, want := range []string{
		`"operation":"service.control"`,
		"sshd",
		`"operation":"rm"`,
		`"operation":"execute"`,
		`"outcome":"refused"`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("the record is missing %s:\n%s", want, contents)
		}
	}
}

// End to end over the real listener, against the real host: the agent must
// return a measured CPU percentage rather than the load-average approximation
// it used to send under that name.
func TestSystemMetricsOverTheControlChannelReturnsRealNumbers(t *testing.T) {
	base := start(t, false)
	status, body := call(t, http.MethodPost, base+"/v1/ops/system.metrics", "")
	if status != http.StatusOK || !body.OK {
		t.Fatalf("system.metrics answered %d (ok=%v, error=%q)", status, body.OK, body.Error)
	}

	// The result travels as JSON, so it is decoded the way the panel decodes it.
	encoded, err := json.Marshal(body.Result)
	if err != nil {
		t.Fatalf("cannot re-encode the result: %v", err)
	}
	var decoded struct {
		CPUPercent *float64 `json:"cpu_percent"`
		RAMTotal   *int64   `json:"ram_total"`
		Sample     struct {
			CPU struct {
				Available       bool     `json:"available"`
				Cores           int      `json:"cores"`
				IntervalSeconds *float64 `json:"interval_seconds"`
				UsagePercent    *float64 `json:"usage_percent"`
			} `json:"cpu"`
			Disks struct {
				Available bool `json:"available"`
				Mounts    []struct {
					Mountpoint string `json:"mountpoint"`
					IsRoot     bool   `json:"is_root"`
				} `json:"mounts"`
			} `json:"disks"`
		} `json:"sample"`
		Unavailable []string `json:"unavailable"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("the result does not decode: %v\n%s", err, encoded)
	}

	if len(decoded.Unavailable) > 0 {
		t.Skipf("this host cannot collect %v, so there is nothing to assert about it", decoded.Unavailable)
	}
	if !decoded.Sample.CPU.Available || decoded.Sample.CPU.UsagePercent == nil {
		t.Fatalf("no CPU percentage came back: %s", encoded)
	}
	if *decoded.Sample.CPU.UsagePercent < 0 || *decoded.Sample.CPU.UsagePercent > 100 {
		t.Fatalf("the CPU percentage is %.2f", *decoded.Sample.CPU.UsagePercent)
	}
	if decoded.Sample.CPU.IntervalSeconds == nil || *decoded.Sample.CPU.IntervalSeconds <= 0 {
		t.Fatal("the CPU percentage came back without the interval it was measured over")
	}
	if decoded.Sample.CPU.Cores < 1 {
		t.Fatalf("the host reports %d cores", decoded.Sample.CPU.Cores)
	}
	// The flat field the panel already decodes must carry the same number.
	if decoded.CPUPercent == nil || *decoded.CPUPercent != *decoded.Sample.CPU.UsagePercent {
		t.Fatalf("the flat cpu_percent field disagrees with the sample: %s", encoded)
	}
	if decoded.RAMTotal == nil || *decoded.RAMTotal <= 0 {
		t.Fatalf("no memory total came back: %s", encoded)
	}
	if !decoded.Sample.Disks.Available || len(decoded.Sample.Disks.Mounts) == 0 {
		t.Fatalf("no filesystem came back: %s", encoded)
	}
}
