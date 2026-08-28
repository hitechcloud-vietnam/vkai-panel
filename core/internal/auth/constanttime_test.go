package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSecureCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "", true},
		{"vkai_key_value", "vkai_key_value", true},
		{"vkai_key_value", "vkai_key_valuf", false},
		{"vkai_key_value", "vkai_key_valu", false},
		{"vkai_key_value", "", false},
		{"a", strings.Repeat("a", 1000), false},
	}
	for _, tc := range cases {
		if got := SecureCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("SecureCompare(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSecureCompareBytesAgreesWithSecureCompare(t *testing.T) {
	pairs := [][2]string{{"", ""}, {"abc", "abc"}, {"abc", "abd"}, {"abc", "abcd"}}
	for _, p := range pairs {
		if SecureCompareBytes([]byte(p[0]), []byte(p[1])) != SecureCompare(p[0], p[1]) {
			t.Errorf("the byte and string forms disagree on %q vs %q", p[0], p[1])
		}
	}
}

// TestSecureCompareDoesNotLeakThePrefixLength is a smoke test, not a proof: a
// timing assertion on a shared CI runner cannot be made reliable. What it does
// catch is somebody replacing the implementation with ==, which produces a
// difference far larger than the tolerance.
func TestSecureCompareDoesNotLeakThePrefixLength(t *testing.T) {
	const secret = "vkai_live_0123456789abcdef0123456789abcdef"
	almost := secret[:len(secret)-1] + "!"
	nothing := strings.Repeat("!", len(secret))

	measure := func(candidate string) time.Duration {
		start := time.Now()
		for i := 0; i < 20000; i++ {
			SecureCompare(secret, candidate)
		}
		return time.Since(start)
	}

	// Warm up so the first measurement does not pay for page faults.
	measure(nothing)

	near := measure(almost)
	far := measure(nothing)

	ratio := float64(near) / float64(far)
	if ratio < 0.5 || ratio > 2.0 {
		t.Fatalf("comparing a near-miss took %v and a complete miss took %v (ratio %.2f); "+
			"that difference is large enough to be an oracle", near, far, ratio)
	}
}

func TestBurnPasswordTimeAlwaysFailsAndCostsRealWork(t *testing.T) {
	if BurnPasswordTime() {
		t.Fatal("BurnPasswordTime must never report success")
	}

	// The point of it is the cost. A bcrypt verification at the default cost
	// is milliseconds, not nanoseconds; if this ever became free it would stop
	// hiding the difference it exists to hide.
	start := time.Now()
	BurnPasswordTime()
	elapsed := time.Since(start)

	if elapsed < time.Millisecond {
		t.Fatalf("BurnPasswordTime took %v, which is too fast to stand in for a real "+
			"password verification", elapsed)
	}
}
