package twofactor

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

const (
	// DefaultRecoveryCodeCount is how many single-use codes are issued. Ten is
	// the industry norm: enough that losing a couple to a bad printer does not
	// matter, few enough that the user still treats them as precious.
	DefaultRecoveryCodeCount = 10

	// recoveryCodeLength is the number of random characters per code. Ten
	// characters from a 32 symbol alphabet is 50 bits of entropy, which is far
	// beyond guessing range even without the rate limiter.
	recoveryCodeLength = 10

	// recoveryGroupSize is the display grouping, purely so a human can read a
	// code off paper without losing their place.
	recoveryGroupSize = 5

	// DefaultLowRecoveryThreshold is the number of remaining codes at or below
	// which the panel starts warning. Running out silently means the account is
	// one lost phone away from an out-of-band recovery process.
	DefaultLowRecoveryThreshold = 3
)

// recoveryAlphabet is Crockford base32: no I, L, O or U, so a handwritten code
// cannot be misread as a digit and no code can spell an unfortunate word. Its
// length is exactly 32, which is what makes the modulo below unbiased - 256 is
// a whole multiple of 32, so every symbol is equally likely.
const recoveryAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// GenerateRecoveryCodes returns count fresh codes in display form
// ("XXXXX-XXXXX"). They are returned in the clear exactly once, to be shown to
// the user at enrolment; only their hashes are ever stored.
func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = DefaultRecoveryCodeCount
	}

	codes := make([]string, 0, count)
	buf := make([]byte, recoveryCodeLength)

	for i := 0; i < count; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generate recovery code: %w", err)
		}

		var b strings.Builder
		for j, value := range buf {
			if j > 0 && j%recoveryGroupSize == 0 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(value)%len(recoveryAlphabet)])
		}
		codes = append(codes, b.String())
	}

	return codes, nil
}

// NormaliseRecoveryCode puts a typed code into the canonical form that was
// hashed: upper case, no separators, with the character substitutions Crockford
// base32 prescribes so that a code read as "O" or "l" still matches.
func NormaliseRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		switch r {
		case '-', ' ', '\t', '_':
			continue
		case 'O':
			b.WriteRune('0')
		case 'I', 'L':
			b.WriteRune('1')
		case 'U':
			b.WriteRune('V')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HashRecoveryCode hashes a code with the same password hashing the rest of the
// panel uses (bcrypt, via utils). A recovery code is a password: it is a
// low-entropy-looking string a human types, and it must survive a database dump
// without becoming usable.
func HashRecoveryCode(code string) (string, error) {
	return utils.HashPassword(NormaliseRecoveryCode(code))
}

// CheckRecoveryCode reports whether a typed code matches a stored hash.
func CheckRecoveryCode(code, hash string) bool {
	return utils.CheckPassword(NormaliseRecoveryCode(code), hash)
}

// HashRecoveryCodes hashes a whole batch.
func HashRecoveryCodes(codes []string) ([]string, error) {
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := HashRecoveryCode(code)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	return hashes, nil
}
