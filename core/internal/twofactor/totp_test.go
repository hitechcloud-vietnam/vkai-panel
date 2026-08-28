package twofactor

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// rfcSecret is the shared secret used by the RFC 4226 and RFC 6238 test
// vectors: the ASCII string "12345678901234567890".
var rfcSecret = []byte("12345678901234567890")

// TestHOTPVectors checks the HOTP values from RFC 4226 appendix D. TOTP is
// HOTP with a time-derived counter, so if these are wrong nothing above them
// can be right.
func TestHOTPVectors(t *testing.T) {
	expected := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}

	for counter, want := range expected {
		got := HOTP(rfcSecret, uint64(counter), 6)
		if got != want {
			t.Errorf("HOTP(counter=%d) = %s, want %s", counter, got, want)
		}
	}
}

// TestTOTPVectors checks the SHA-1 rows of the RFC 6238 appendix B test table.
// The RFC prints eight digit codes; the panel uses six, so the digit count is
// passed explicitly here.
func TestTOTPVectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}

	for _, tc := range cases {
		at := time.Unix(tc.unix, 0).UTC()
		got := CodeAt(rfcSecret, at, 8, DefaultPeriod)
		if got != tc.want {
			t.Errorf("CodeAt(%d) = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

// TestStepMatchesRFCCounters checks the time step derivation itself: RFC 6238
// section 4 states T = floor(unix / 30) with T0 = 0.
func TestStepMatchesRFCCounters(t *testing.T) {
	cases := []struct {
		unix int64
		want uint64
	}{
		{0, 0},
		{29, 0},
		{30, 1},
		{59, 1},
		{1111111109, 37037036},
		{20000000000, 666666666},
	}

	for _, tc := range cases {
		if got := Step(time.Unix(tc.unix, 0), DefaultPeriod); got != tc.want {
			t.Errorf("Step(%d) = %d, want %d", tc.unix, got, tc.want)
		}
	}
}

// TestDriftWindowBoundaries pins the accepted window exactly. With skew 1 the
// previous, current and next steps are accepted and the step two away is not -
// widening this silently would triple the number of codes valid at any instant.
func TestDriftWindowBoundaries(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	current := Step(now, DefaultPeriod)

	cases := []struct {
		name      string
		step      uint64
		skew      int
		wantOK    bool
		wantMatch uint64
	}{
		{"current step", current, 1, true, current},
		{"one step behind", current - 1, 1, true, current - 1},
		{"one step ahead", current + 1, 1, true, current + 1},
		{"two steps behind", current - 2, 1, false, 0},
		{"two steps ahead", current + 2, 1, false, 0},
		{"no drift allowed, previous step", current - 1, 0, false, 0},
		{"no drift allowed, current step", current, 0, true, current},
		{"wide window, two steps behind", current - 2, 2, true, current - 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := HOTP(rfcSecret, tc.step, DefaultDigits)
			step, ok := MatchStep(rfcSecret, code, now, DefaultDigits, DefaultPeriod, tc.skew)
			if ok != tc.wantOK {
				t.Fatalf("MatchStep ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && step != tc.wantMatch {
				t.Fatalf("MatchStep step = %d, want %d", step, tc.wantMatch)
			}
		})
	}
}

// TestMatchStepRejectsMalformedCodes checks the cheap rejections: a code of the
// wrong length or with non-digits never reaches the HMAC.
func TestMatchStepRejectsMalformedCodes(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	valid := CodeAt(rfcSecret, now, DefaultDigits, DefaultPeriod)

	for _, code := range []string{"", "12345", "1234567", valid + "0", "abcdef"} {
		if _, ok := MatchStep(rfcSecret, code, now, DefaultDigits, DefaultPeriod, DefaultSkew); ok {
			t.Errorf("MatchStep accepted malformed code %q", code)
		}
	}
}

// TestSecretRoundTrip checks the base32 presentation is what an authenticator
// app can consume, and survives the tidying a person's typing needs.
func TestSecretRoundTrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) != SecretBytes {
		t.Fatalf("secret is %d bytes, want %d (160 bits)", len(secret), SecretBytes)
	}

	encoded := EncodeSecret(secret)
	if strings.Contains(encoded, "=") {
		t.Errorf("encoded secret %q contains padding, which some apps reject", encoded)
	}

	for _, variant := range []string{encoded, strings.ToLower(encoded), spaceEvery(encoded, 4)} {
		decoded, err := DecodeSecret(variant)
		if err != nil {
			t.Fatalf("DecodeSecret(%q): %v", variant, err)
		}
		if string(decoded) != string(secret) {
			t.Errorf("round trip of %q changed the secret", variant)
		}
	}

	if _, err := DecodeSecret("not base32 !!"); err == nil {
		t.Error("DecodeSecret accepted a non base32 string")
	}
}

// TestProvisioningURI checks the otpauth URI an authenticator app scans: the
// label, the issuer and every parameter the app must agree with.
func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("VKAI Panel", "admin", rfcSecret, 6, 30*time.Second)

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("ProvisioningURI produced an unparseable URI %q: %v", uri, err)
	}
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" {
		t.Fatalf("URI is %q://%q, want otpauth://totp", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/VKAI Panel:admin" {
		t.Errorf("label is %q, want /VKAI Panel:admin", parsed.Path)
	}

	query := parsed.Query()
	if query.Get("secret") != EncodeSecret(rfcSecret) {
		t.Errorf("secret parameter is %q", query.Get("secret"))
	}
	for key, want := range map[string]string{
		"issuer":    "VKAI Panel",
		"algorithm": "SHA1",
		"digits":    "6",
		"period":    "30",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s parameter is %q, want %q", key, got, want)
		}
	}

	// A colon in the account name would re-split the label in some apps.
	withColon := ProvisioningURI("VKAI Panel", "admin:root", rfcSecret, 6, 30*time.Second)
	if strings.Count(withColon, ":") != strings.Count(uri, ":") {
		t.Errorf("account name colon leaked into the label: %q", withColon)
	}
}

func spaceEvery(value string, n int) string {
	var b strings.Builder
	for i, r := range value {
		if i > 0 && i%n == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
