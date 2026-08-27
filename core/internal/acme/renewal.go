package acme

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// renewalFraction is the share of a certificate's lifetime that must still be
// left for it to count as fresh. Below one third remaining, it is due.
const renewalFraction = 3

// NeedsRenewal reports whether cert should be replaced, which is true once less
// than one third of its total lifetime remains.
//
// The rule is proportional rather than a fixed number of days because the panel
// deals with two very different lifetimes. A 90 day certificate renews with
// about 30 days left, the usual comfortable margin. A six day certificate from
// the shortlived profile - the only way to get a certificate for an IP address -
// renews with about two days left, which still leaves several retry
// opportunities before it expires.
//
// A certificate that is already expired, or whose validity window is degenerate,
// needs renewal. A nil certificate does too: there is nothing to serve.
func NeedsRenewal(cert *x509.Certificate, now time.Time) bool {
	if cert == nil {
		return true
	}
	lifetime := cert.NotAfter.Sub(cert.NotBefore)
	if lifetime <= 0 {
		return true
	}
	remaining := cert.NotAfter.Sub(now)
	if remaining <= 0 {
		return true
	}
	// Compare with integer arithmetic so the boundary is exact: renewal is due
	// once remaining*3 < lifetime, that is, once less than a third is left.
	return remaining*renewalFraction < lifetime
}

// RenewAfter returns the instant at which NeedsRenewal starts reporting true,
// which is the point where exactly one third of the lifetime remains. A caller
// scheduling a timer wants this rather than a poll loop.
func RenewAfter(cert *x509.Certificate) (time.Time, error) {
	if cert == nil {
		return time.Time{}, errors.New("acme: nil certificate")
	}
	lifetime := cert.NotAfter.Sub(cert.NotBefore)
	if lifetime <= 0 {
		return time.Time{}, fmt.Errorf("acme: certificate validity window is empty (notBefore %s, notAfter %s)",
			cert.NotBefore, cert.NotAfter)
	}
	return cert.NotAfter.Add(-lifetime / renewalFraction), nil
}

// ParseCertificateChainPEM parses a PEM chain as returned by Obtain and returns
// every certificate in it, leaf first.
func ParseCertificateChainPEM(chainPEM []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := chainPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("acme: parse certificate from chain: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("acme: no certificate found in PEM chain")
	}
	return certs, nil
}

// LeafFromChainPEM returns the leaf certificate of a PEM chain, which is the one
// NeedsRenewal should be asked about.
func LeafFromChainPEM(chainPEM []byte) (*x509.Certificate, error) {
	certs, err := ParseCertificateChainPEM(chainPEM)
	if err != nil {
		return nil, err
	}
	return certs[0], nil
}
