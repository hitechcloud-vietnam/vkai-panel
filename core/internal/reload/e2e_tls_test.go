package reload_test

// The certificate half, driven the same way: a real PEM pasted through the real
// endpoint, then a real TLS handshake against the port to see which certificate
// actually came back.
//
// Checking the response body is not enough here. The response says what was
// saved; the handshake says what is being served, and the difference between
// those two is precisely the defect that shipped once already - a certificate
// machine that was built, configured and never asked to do anything.

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
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/handler"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/reload"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// tlsPanel is a running panel that terminates TLS.
type tlsPanel struct {
	*panelUnderTest
	tlsSwitch *reload.TLSSwitch
}

func startTLSPanel(t *testing.T) *tlsPanel {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	t.Setenv("VKAI_SSL_ROOT", dir+"/ssl")

	cfg := config.DefaultPanelAccess()
	cfg.Enabled = true
	cfg.Bind = "127.0.0.1"
	cfg.Port = freePort(t)
	cfg.Entrance = "/vkai_test_door"
	cfg.EntranceEnabled = true
	cfg.SessionTTLSeconds = 3600
	cfg.AllowedIPs = []string{}
	cfg.TrustedProxies = []string{}
	cfg.TLS.Enabled = true
	cfg.TLS.Mode = config.TLSModeSelfSigned
	cfg.TLS.SelfSigned = true
	cfg.TLS.CertFile, cfg.TLS.KeyFile = config.ManagedPanelCertPaths()
	cfg.StateFile = dir + "/panel_access.json"

	logger := zap.NewNop()

	sup := reload.New(reload.Options{Config: cfg, Logger: logger})
	reload.SetDefault(sup)
	t.Cleanup(func() { reload.SetDefault(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	tlsSwitch, err := reload.NewTLSSwitch(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("certificate manager: %v", err)
	}
	t.Cleanup(tlsSwitch.Stop)

	svc := service.NewPanelSettingsService(cfg, nil, logger)
	settings := handler.NewPanelSettingsHandler(svc, logger)

	engine := gin.New()
	engine.GET("/api/v1/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/api/v1/panel/settings", settings.Get)
	engine.PUT("/api/v1/panel/settings", settings.Update)

	guard, err := reload.NewGuardSwitch(cfg, "test-secret-that-is-long-enough-1234", engine, logger)
	if err != nil {
		t.Fatalf("access gate: %v", err)
	}

	rebinder, err := reload.NewRebinder(reload.RebinderOptions{
		Handler:      guard,
		TLSConfig:    tlsSwitch.TLSConfig(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  10 * time.Second,
		DrainTimeout: 2 * time.Second,
		Address:      func(c *config.PanelAccessConfig) string { return c.ListenAddr() },
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("listener: %v", err)
	}

	sup.Register(reload.NewStateFile(logger))
	sup.Register(tlsSwitch)
	sup.Register(guard)
	sup.Register(rebinder)
	sup.SetProbe(reload.Probes{rebinder, guard})
	sup.SetCertificateReloader(tlsSwitch.Reload)
	sup.Adopt(cfg)

	if err := rebinder.Start(cfg); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = rebinder.Shutdown(contextWithTimeout(2 * time.Second)) })

	return &tlsPanel{
		panelUnderTest: &panelUnderTest{t: t, cfg: cfg, sup: sup, rebinder: rebinder, stateFile: cfg.StateFile},
		tlsSwitch:      tlsSwitch,
	}
}

// putTLS sends the request over HTTPS, the way the panel is now reached.
func (p *tlsPanel) putTLS(body string) (int, map[string]any) {
	p.t.Helper()

	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the certificate under test is the subject, not the trust anchor
		},
	}

	url := fmt.Sprintf("https://127.0.0.1:%d%s/api/v1/panel/settings", p.cfg.Port, p.cfg.Entrance)
	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		p.t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		p.t.Fatalf("PUT %s: %v", url, err)
	}
	defer response.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		p.t.Fatalf("decode: %v", err)
	}
	return response.StatusCode, decoded
}

// servedCertificate is what the listener actually presents on the wire.
func servedCertificate(t *testing.T, port int) *x509.Certificate {
	t.Helper()

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp",
		fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
	if err != nil {
		t.Fatalf("TLS handshake against the panel: %v", err)
	}
	defer conn.Close()

	chain := conn.ConnectionState().PeerCertificates
	if len(chain) == 0 {
		t.Fatal("the panel presented no certificate")
	}
	return chain[0]
}

// TestPastedCertificateIsServedWithoutARestart is the whole feature in one
// test: PEM text goes in through the API, and the very next handshake presents
// it.
func TestPastedCertificateIsServedWithoutARestart(t *testing.T) {
	panel := startTLSPanel(t)

	before := servedCertificate(t, panel.cfg.Port)

	certPEM, keyPEM := makeCertificate(t, "vkai-pasted-under-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	body, err := json.Marshal(map[string]any{
		"tls_certificate": string(certPEM),
		"tls_private_key": string(keyPEM),
		"tls_accept_risk": true,
		"confirm":         true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	status, response := panel.putTLS(string(body))
	if status != http.StatusOK {
		t.Fatalf("expected 200 when pasting a certificate, got %d: %v", status, response)
	}

	// Nothing in the response may carry the private key.
	if raw, _ := json.Marshal(response); strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatal("the response echoed private key material")
	}

	after := servedCertificate(t, panel.cfg.Port)
	if after.Subject.CommonName != "vkai-pasted-under-test" {
		t.Fatalf("the panel is still serving %q, not the certificate that was pasted", after.Subject.CommonName)
	}
	if after.Subject.CommonName == before.Subject.CommonName {
		t.Fatal("the served certificate did not change at all")
	}

	// The key is on disk, readable by nobody else.
	data, _ := response["data"].(map[string]any)
	settings, _ := data["settings"].(map[string]any)
	tlsView, _ := settings["tls"].(map[string]any)

	keyFile, _ := tlsView["key_file"].(string)
	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("the private key was not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the private key is mode %o, expected 600", perm)
	}
	if config.IsManagedCertPath(keyFile) {
		t.Fatal("the pasted key was written over the pair the panel renews for itself")
	}
	if mode, _ := tlsView["mode"].(string); mode != config.TLSModeCustom {
		t.Fatalf("the panel did not switch to the custom mode: %q", mode)
	}
}

// TestMismatchedKeyIsRefusedBeforeAnythingIsWritten covers the failure that
// looks, from outside, exactly like the panel being down.
func TestMismatchedKeyIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	panel := startTLSPanel(t)
	before := servedCertificate(t, panel.cfg.Port)

	certPEM, _ := makeCertificate(t, "cert-one", nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour*24))
	_, otherKeyPEM := makeCertificate(t, "cert-two", nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour*24))

	body, _ := json.Marshal(map[string]any{
		"tls_certificate": string(certPEM),
		"tls_private_key": string(otherKeyPEM),
		"tls_accept_risk": true,
		"confirm":         true,
	})

	status, response := panel.putTLS(string(body))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for a mismatched pair, got %d: %v", status, response)
	}

	after := servedCertificate(t, panel.cfg.Port)
	if after.Subject.CommonName != before.Subject.CommonName {
		t.Fatal("a refused certificate changed what the panel serves")
	}
}

// TestExpiredCertificateIsRefused proves the validity check runs against the
// real endpoint and not only in a unit test of the parser.
func TestExpiredCertificateIsRefused(t *testing.T) {
	panel := startTLSPanel(t)

	certPEM, keyPEM := makeCertificate(t, "long-expired", nil, []net.IP{net.ParseIP("127.0.0.1")},
		time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))

	body, _ := json.Marshal(map[string]any{
		"tls_certificate": string(certPEM),
		"tls_private_key": string(keyPEM),
		"tls_accept_risk": true,
		"confirm":         true,
	})

	status, response := panel.putTLS(string(body))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for an expired certificate, got %d: %v", status, response)
	}
	apiError, _ := response["error"].(map[string]any)
	if details, _ := apiError["details"].(string); !strings.Contains(details, config.CertCheckValidity) {
		t.Fatalf("the refusal does not name the validity check: %v", apiError)
	}
}

// TestWrongHostnameNeedsAnExplicitOverride: saving a certificate for the wrong
// name locks the operator out exactly like a bad port change does.
func TestWrongHostnameNeedsAnExplicitOverride(t *testing.T) {
	panel := startTLSPanel(t)

	certPEM, keyPEM := makeCertificate(t, "elsewhere", []string{"panel.somewhere-else.example"}, nil,
		time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	body, _ := json.Marshal(map[string]any{
		"tls_certificate": string(certPEM),
		"tls_private_key": string(keyPEM),
		"confirm":         true,
	})

	status, response := panel.putTLS(string(body))
	if status != http.StatusConflict {
		t.Fatalf("expected 409 for a certificate that does not cover the panel's host, got %d: %v", status, response)
	}
	apiError, _ := response["error"].(map[string]any)
	if code, _ := apiError["code"].(string); code != "TLS_RISK_ACKNOWLEDGEMENT_REQUIRED" {
		t.Fatalf("expected an acknowledgement to be required, got %q", code)
	}

	data, _ := response["data"].(map[string]any)
	if check, _ := data["check"].(string); check != config.CertCheckHostnames {
		t.Fatalf("the refusal does not name the hostname check: %v", data)
	}

	// With the risk acknowledged the same request goes through.
	body, _ = json.Marshal(map[string]any{
		"tls_certificate": string(certPEM),
		"tls_private_key": string(keyPEM),
		"tls_accept_risk": true,
		"confirm":         true,
	})
	if status, response = panel.putTLS(string(body)); status != http.StatusOK {
		t.Fatalf("the acknowledged certificate was still refused: %d %v", status, response)
	}
}

// makeCertificate builds a self-signed pair for a test.
func makeCertificate(t *testing.T, commonName string, dnsNames []string, ips []net.IP, notBefore, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("key encoding: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
