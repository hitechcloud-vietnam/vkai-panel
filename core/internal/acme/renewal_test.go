package acme

import (
	"crypto/x509"
	"testing"
	"time"
)

// certWithLifetime builds a bare x509.Certificate carrying only the validity
// window, which is all NeedsRenewal looks at.
func certWithLifetime(notBefore time.Time, lifetime time.Duration) *x509.Certificate {
	return &x509.Certificate{NotBefore: notBefore, NotAfter: notBefore.Add(lifetime)}
}

func TestNeedsRenewalBoundaries(t *testing.T) {
	issued := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	const (
		shortLived = 6 * 24 * time.Hour  // the "shortlived" profile: ~6 days
		classic    = 90 * 24 * time.Hour // the "classic" profile: 90 days
	)

	tests := []struct {
		name     string
		lifetime time.Duration
		now      time.Time
		want     bool
	}{
		// A six day certificate has a two day renewal threshold.
		{"6d just issued", shortLived, issued, false},
		{"6d halfway", shortLived, issued.Add(3 * 24 * time.Hour), false},
		{"6d exactly one third left", shortLived, issued.Add(4 * 24 * time.Hour), false},
		{"6d a second past one third", shortLived, issued.Add(4*24*time.Hour + time.Second), true},
		{"6d one day left", shortLived, issued.Add(5 * 24 * time.Hour), true},
		{"6d expired", shortLived, issued.Add(shortLived), true},
		{"6d long expired", shortLived, issued.Add(shortLived + 24*time.Hour), true},

		// A ninety day certificate has a thirty day renewal threshold.
		{"90d just issued", classic, issued, false},
		{"90d 31 days left", classic, issued.Add(59 * 24 * time.Hour), false},
		{"90d exactly 30 days left", classic, issued.Add(60 * 24 * time.Hour), false},
		{"90d a second past 30 days left", classic, issued.Add(60*24*time.Hour + time.Second), true},
		{"90d 29 days left", classic, issued.Add(61 * 24 * time.Hour), true},
		{"90d expired", classic, issued.Add(classic), true},

		// A certificate whose validity has not started yet is still fresh.
		{"6d before notBefore", shortLived, issued.Add(-time.Hour), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cert := certWithLifetime(issued, tc.lifetime)
			if got := NeedsRenewal(cert, tc.now); got != tc.want {
				t.Fatalf("NeedsRenewal(lifetime=%s, %s left) = %v, want %v",
					tc.lifetime, cert.NotAfter.Sub(tc.now), got, tc.want)
			}
		})
	}
}

func TestNeedsRenewalDegenerateInputs(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	if !NeedsRenewal(nil, now) {
		t.Fatal("a nil certificate must need renewal")
	}
	zeroWindow := &x509.Certificate{NotBefore: now, NotAfter: now}
	if !NeedsRenewal(zeroWindow, now) {
		t.Fatal("a certificate with an empty validity window must need renewal")
	}
	inverted := &x509.Certificate{NotBefore: now, NotAfter: now.Add(-time.Hour)}
	if !NeedsRenewal(inverted, now) {
		t.Fatal("a certificate with an inverted validity window must need renewal")
	}
}

func TestRenewAfter(t *testing.T) {
	issued := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	shortCert := certWithLifetime(issued, 6*24*time.Hour)
	when, err := RenewAfter(shortCert)
	if err != nil {
		t.Fatalf("RenewAfter: %v", err)
	}
	if want := issued.Add(4 * 24 * time.Hour); !when.Equal(want) {
		t.Fatalf("RenewAfter for a 6 day certificate = %s, want %s", when, want)
	}
	if NeedsRenewal(shortCert, when) {
		t.Fatal("renewal must not be due at exactly one third remaining")
	}
	if !NeedsRenewal(shortCert, when.Add(time.Second)) {
		t.Fatal("renewal must be due one second after the threshold")
	}

	classicCert := certWithLifetime(issued, 90*24*time.Hour)
	when, err = RenewAfter(classicCert)
	if err != nil {
		t.Fatalf("RenewAfter: %v", err)
	}
	if want := issued.Add(60 * 24 * time.Hour); !when.Equal(want) {
		t.Fatalf("RenewAfter for a 90 day certificate = %s, want %s", when, want)
	}

	if _, err := RenewAfter(nil); err == nil {
		t.Fatal("RenewAfter(nil) must fail")
	}
}
