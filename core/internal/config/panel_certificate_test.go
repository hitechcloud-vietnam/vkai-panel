package config

// Certificate validation, checked one failure at a time.
//
// Each of these is a way an operator's paste can be wrong, and each one has a
// different consequence: a mismatched pair fails every handshake, an expired
// certificate is refused by browsers, a missing intermediate works on the
// machine that has it cached and nowhere else. A single "invalid certificate"
// message would leave the operator guessing between them.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func pair(t *testing.T, cn string, dns []string, ips []net.IP, notBefore, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dns,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func checkOf(t *testing.T, err error) string {
	t.Helper()
	var certErr *CertificateError
	if !errors.As(err, &certErr) {
		t.Fatalf("expected a certificate error, got %v", err)
	}
	return certErr.Check
}

func TestInspectCertificatePairAcceptsAGoodPair(t *testing.T) {
	certPEM, keyPEM := pair(t, "panel", []string{"panel.example.com"}, []net.IP{net.ParseIP("127.0.0.1")},
		time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour))

	inspection, err := InspectCertificatePair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("a valid pair was refused: %v", err)
	}
	if inspection.KeyType != "ECDSA P-256" {
		t.Fatalf("key type %q", inspection.KeyType)
	}
	if !inspection.SelfSigned {
		t.Fatal("a self-signed certificate was not recognised as one")
	}
	if inspection.ChainComplete {
		t.Fatal("a self-signed certificate must never count as a complete chain")
	}
	if !inspection.CoversHost("panel.example.com") || !inspection.CoversHost("127.0.0.1") {
		t.Fatalf("host coverage is wrong: dns=%v ip=%v", inspection.DNSNames, inspection.IPAddresses)
	}
	if inspection.CoversHost("other.example.com") {
		t.Fatal("a host the certificate does not name was reported as covered")
	}
}

func TestInspectCertificatePairRejectsAMismatchedKey(t *testing.T) {
	certPEM, _ := pair(t, "one", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	_, otherKey := pair(t, "two", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	_, err := InspectCertificatePair(certPEM, otherKey)
	if got := checkOf(t, err); got != CertCheckKeyMatch {
		t.Fatalf("expected the %s check, got %s", CertCheckKeyMatch, got)
	}
}

func TestInspectCertificatePairRejectsAnExpiredCertificate(t *testing.T) {
	certPEM, keyPEM := pair(t, "old", nil, nil, time.Now().Add(-48*time.Hour), time.Now().Add(-time.Hour))

	_, err := InspectCertificatePair(certPEM, keyPEM)
	if got := checkOf(t, err); got != CertCheckValidity {
		t.Fatalf("expected the %s check, got %s", CertCheckValidity, got)
	}
}

func TestInspectCertificatePairRejectsANotYetValidCertificate(t *testing.T) {
	certPEM, keyPEM := pair(t, "future", nil, nil, time.Now().Add(24*time.Hour), time.Now().Add(48*time.Hour))

	_, err := InspectCertificatePair(certPEM, keyPEM)
	if got := checkOf(t, err); got != CertCheckValidity {
		t.Fatalf("expected the %s check, got %s", CertCheckValidity, got)
	}
}

func TestInspectCertificatePairWarnsAboutAnImminentExpiry(t *testing.T) {
	certPEM, keyPEM := pair(t, "soon", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(5*24*time.Hour))

	inspection, err := InspectCertificatePair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("a valid certificate was refused: %v", err)
	}
	if !inspection.ExpiringSoon {
		t.Fatal("a certificate expiring in five days was not flagged")
	}
	if len(inspection.Warnings) == 0 {
		t.Fatal("no warning was produced for an imminent expiry")
	}
}

func TestInspectCertificatePairRejectsAnEncryptedKey(t *testing.T) {
	certPEM, _ := pair(t, "panel", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	encrypted := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-256-CBC,0123"},
		Bytes:   []byte("not really a key"),
	})

	_, err := InspectCertificatePair(certPEM, encrypted)
	if got := checkOf(t, err); got != CertCheckKeyEncrypted {
		t.Fatalf("expected the %s check, got %s", CertCheckKeyEncrypted, got)
	}
	if !strings.Contains(err.Error(), "openssl rsa") {
		t.Fatalf("the message does not tell the operator how to fix it: %v", err)
	}
}

func TestInspectCertificatePairRejectsSwappedBoxes(t *testing.T) {
	certPEM, keyPEM := pair(t, "panel", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	if got := checkOf(t, mustFail(t, keyPEM, keyPEM)); got != CertCheckCertPEM {
		t.Fatalf("a key in the certificate box gave the %s check", got)
	}
	if got := checkOf(t, mustFail(t, certPEM, certPEM)); got != CertCheckKeyPEM {
		t.Fatalf("a certificate in the key box gave the %s check", got)
	}
}

func TestInspectCertificatePairRejectsEmptyInput(t *testing.T) {
	certPEM, keyPEM := pair(t, "panel", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	if got := checkOf(t, mustFail(t, []byte("hello"), keyPEM)); got != CertCheckCertPEM {
		t.Fatalf("plain text in the certificate box gave the %s check", got)
	}
	if got := checkOf(t, mustFail(t, certPEM, []byte(""))); got != CertCheckKeyPEM {
		t.Fatalf("an empty key box gave the %s check", got)
	}
}

func TestInspectCertificatePairAcceptsAnRSAKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "rsa-panel"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("key encoding: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	inspection, err := InspectCertificatePair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("a PKCS#8 RSA pair was refused: %v", err)
	}
	if inspection.KeyType != "RSA" || inspection.KeyBits != 2048 {
		t.Fatalf("key reported as %s %d", inspection.KeyType, inspection.KeyBits)
	}
}

func TestCoversHostHonoursOneWildcardLabel(t *testing.T) {
	inspection := &CertificateInspection{DNSNames: []string{"*.example.com"}}

	if !inspection.CoversHost("panel.example.com") {
		t.Fatal("a wildcard did not match its own label")
	}
	if inspection.CoversHost("a.b.example.com") {
		t.Fatal("a wildcard matched two labels")
	}
	if inspection.CoversHost("example.com") {
		t.Fatal("a wildcard matched the bare domain")
	}
}

func TestInstallCustomPairRestoresThePreviousPairOnRollback(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := dir+"/panel.crt", dir+"/panel.key"

	firstCert, firstKey := pair(t, "first", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := InstallCustomPair(certFile, keyFile, firstCert, firstKey); err != nil {
		t.Fatalf("install: %v", err)
	}

	secondCert, secondKey := pair(t, "second", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	backup, err := InstallCustomPair(certFile, keyFile, secondCert, secondKey)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	inspection, err := InspectCertificateFile(certFile)
	if err != nil || inspection.Subject != "CN=second" {
		t.Fatalf("the second certificate was not installed: %v %v", inspection, err)
	}

	backup.Restore(certFile, keyFile)

	inspection, err = InspectCertificateFile(certFile)
	if err != nil || inspection.Subject != "CN=first" {
		t.Fatalf("the rollback did not restore the first certificate: %v %v", inspection, err)
	}
}

func TestInstallCustomPairWritesTheKeyUnreadableByAnybodyElse(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := dir+"/panel.crt", dir+"/panel.key"

	certPEM, keyPEM := pair(t, "panel", nil, nil, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := InstallCustomPair(certFile, keyFile, certPEM, keyPEM); err != nil {
		t.Fatalf("install: %v", err)
	}

	info, err := osStat(keyFile)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the private key is mode %o, expected 600", perm)
	}
}

func mustFail(t *testing.T, certPEM, keyPEM []byte) error {
	t.Helper()
	_, err := InspectCertificatePair(certPEM, keyPEM)
	if err == nil {
		t.Fatal("expected a refusal, got none")
	}
	return err
}
