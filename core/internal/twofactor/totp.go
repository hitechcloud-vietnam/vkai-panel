// Package twofactor implements time-based one-time password (TOTP) second
// factors for panel accounts.
//
// The algorithm is RFC 6238 (TOTP) on top of RFC 4226 (HOTP): HMAC-SHA-1 over
// a big-endian counter, dynamically truncated to a fixed number of decimal
// digits. It is about forty lines of arithmetic and is implemented here with
// the standard library rather than pulled in as a dependency.
//
// Everything an authenticator app needs to agree with the panel - the hash,
// the digit count and the time step - is stated explicitly in the provisioning
// URI, so the defaults below are not merely internal constants.
package twofactor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// SecretBytes is the shared secret length. RFC 4226 section 4 requires at
	// least 128 bits and recommends 160, which is also the HMAC-SHA-1 block
	// output size and what every authenticator app expects.
	SecretBytes = 20

	// DefaultDigits and DefaultPeriod are the values every mainstream
	// authenticator app assumes when the provisioning URI omits them.
	DefaultDigits = 6
	DefaultPeriod = 30 * time.Second

	// DefaultSkew is how many adjacent time steps on each side of the current
	// one are accepted, to absorb clock drift between the panel and the phone.
	// One step each way is a three step window: 90 seconds in total. Widening
	// this multiplies the number of codes valid at any instant, so it is not a
	// free knob.
	DefaultSkew = 1

	// AlgorithmSHA1 is the only hash used here. RFC 6238 permits SHA-256 and
	// SHA-512, but authenticator apps in the field silently assume SHA-1, and
	// a second factor that fails to enrol is worse than one built on a hash
	// whose collision weakness does not apply to HMAC.
	AlgorithmSHA1 = "SHA1"
)

// secretEncoding is unpadded upper-case base32 (RFC 4648), the encoding every
// otpauth:// consumer expects. Padding is stripped because several popular
// authenticator apps reject '=' inside the secret parameter.
var secretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a fresh 160-bit shared secret from the system CSPRNG.
func GenerateSecret() ([]byte, error) {
	secret := make([]byte, SecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate two-factor secret: %w", err)
	}
	return secret, nil
}

// EncodeSecret renders a secret as the base32 string shown to the user for
// manual entry.
func EncodeSecret(secret []byte) string {
	return secretEncoding.EncodeToString(secret)
}

// DecodeSecret parses a base32 secret. Spaces and lower case are tolerated
// because people retype secrets by hand, and padding is tolerated because
// other implementations emit it.
func DecodeSecret(encoded string) ([]byte, error) {
	cleaned := strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, encoded))
	cleaned = strings.TrimRight(cleaned, "=")

	if cleaned == "" {
		return nil, ErrInvalidSecret
	}

	secret, err := secretEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, ErrInvalidSecret
	}
	if len(secret) == 0 {
		return nil, ErrInvalidSecret
	}
	return secret, nil
}

// Step returns the RFC 6238 time step counter for t: the number of whole
// periods elapsed since the Unix epoch (T0 = 0).
func Step(t time.Time, period time.Duration) uint64 {
	if period <= 0 {
		period = DefaultPeriod
	}
	seconds := t.Unix()
	if seconds < 0 {
		seconds = 0
	}
	return uint64(seconds) / uint64(period/time.Second)
}

// StepStart returns the wall-clock instant at which a step began.
func StepStart(step uint64, period time.Duration) time.Time {
	if period <= 0 {
		period = DefaultPeriod
	}
	return time.Unix(int64(step)*int64(period/time.Second), 0).UTC()
}

// HOTP is RFC 4226: HMAC-SHA-1 of the counter under the shared secret,
// dynamically truncated to `digits` decimal digits.
func HOTP(secret []byte, counter uint64, digits int) string {
	if digits <= 0 {
		digits = DefaultDigits
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Dynamic truncation: the low nibble of the last byte selects a four byte
	// window, whose top bit is masked off so the result is always positive.
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	return fmt.Sprintf("%0*d", digits, value%pow10(digits))
}

// CodeAt returns the code a correctly synchronised authenticator shows at t.
func CodeAt(secret []byte, t time.Time, digits int, period time.Duration) string {
	return HOTP(secret, Step(t, period), digits)
}

// pow10 returns 10^n for the digit counts TOTP allows (6 to 10). Computing it
// in a loop keeps the modulus exact rather than relying on float math.
func pow10(n int) uint32 {
	result := uint32(1)
	for i := 0; i < n && i < 9; i++ {
		result *= 10
	}
	return result
}

// MatchStep looks for a time step within +/- skew of now whose code equals the
// submitted one, and returns it.
//
// The comparison is constant time. A timing oracle on a six digit code is not
// theoretical: an attacker who can distinguish "wrong at digit 1" from "wrong
// at digit 5" turns a million guesses into sixty.
//
// It deliberately does not decide whether the code may be used: replay
// rejection needs durable per-user state and lives in the store, so that two
// concurrent requests cannot both win. See Service.verifyCode.
func MatchStep(secret []byte, code string, now time.Time, digits int, period time.Duration, skew int) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(code) == 0 || len(secret) == 0 {
		return 0, false
	}
	if digits <= 0 {
		digits = DefaultDigits
	}
	if len(code) != digits {
		return 0, false
	}
	if skew < 0 {
		skew = 0
	}

	current := Step(now, period)

	// Walk outward from the current step so the common case - a phone with an
	// accurate clock - matches first.
	for distance := 0; distance <= skew; distance++ {
		for _, step := range candidateSteps(current, distance) {
			candidate := HOTP(secret, step, digits)
			if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
				return step, true
			}
		}
	}
	return 0, false
}

// candidateSteps returns the steps exactly `distance` away from current,
// guarding the underflow at the epoch.
func candidateSteps(current uint64, distance int) []uint64 {
	if distance == 0 {
		return []uint64{current}
	}
	steps := make([]uint64, 0, 2)
	if current >= uint64(distance) {
		steps = append(steps, current-uint64(distance))
	}
	steps = append(steps, current+uint64(distance))
	return steps
}

// ProvisioningURI builds the otpauth:// URI that authenticator apps read from a
// QR code. The label is "issuer:account" and the issuer is repeated as a
// parameter, which is what the de facto Key Uri Format requires; the algorithm,
// digit count and period are stated explicitly rather than left to the app's
// defaults.
func ProvisioningURI(issuer, account string, secret []byte, digits int, period time.Duration) string {
	if digits <= 0 {
		digits = DefaultDigits
	}
	if period <= 0 {
		period = DefaultPeriod
	}

	issuer = sanitiseLabel(issuer)
	account = sanitiseLabel(account)

	label := account
	if issuer != "" {
		label = issuer + ":" + account
	}

	query := url.Values{}
	query.Set("secret", EncodeSecret(secret))
	if issuer != "" {
		query.Set("issuer", issuer)
	}
	query.Set("algorithm", AlgorithmSHA1)
	query.Set("digits", fmt.Sprintf("%d", digits))
	query.Set("period", fmt.Sprintf("%d", int(period/time.Second)))

	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + label,
		RawQuery: query.Encode(),
	}
	return u.String()
}

// sanitiseLabel removes the characters that would break the otpauth label
// grammar. A colon inside an account name silently re-splits the label in some
// apps, so it is stripped rather than escaped.
func sanitiseLabel(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch r {
		case ':', '/', '?', '#', '\n', '\r':
			return -1
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, value)
	return value
}
