package twofactor

import (
	"strings"
	"testing"
)

// TestGenerateRecoveryCodesShape checks the format people have to read off
// paper: grouped, unambiguous, unique and long enough not to be guessable.
func TestGenerateRecoveryCodesShape(t *testing.T) {
	codes, err := GenerateRecoveryCodes(DefaultRecoveryCodeCount)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != DefaultRecoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), DefaultRecoveryCodeCount)
	}

	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate recovery code %q in one batch", code)
		}
		seen[code] = true

		if len(code) != recoveryCodeLength+1 {
			t.Fatalf("code %q is %d characters, want %d with one separator", code, len(code), recoveryCodeLength+1)
		}
		if strings.Count(code, "-") != 1 {
			t.Fatalf("code %q is not grouped for reading", code)
		}
		for _, r := range strings.ReplaceAll(code, "-", "") {
			if !strings.ContainsRune(recoveryAlphabet, r) {
				t.Fatalf("code %q contains %q, which is not in the unambiguous alphabet", code, r)
			}
		}
		// A recovery code must never be mistakable for a TOTP code, since the
		// verifier routes by shape.
		if isDigits(strings.ReplaceAll(code, "-", "")) && len(strings.ReplaceAll(code, "-", "")) == DefaultDigits {
			t.Fatalf("code %q is indistinguishable from a TOTP code", code)
		}
	}
}

// TestRecoveryCodeHashing checks a code is stored only as a hash and that
// human typing variations still match.
func TestRecoveryCodeHashing(t *testing.T) {
	codes, err := GenerateRecoveryCodes(1)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	code := codes[0]

	hash, err := HashRecoveryCode(code)
	if err != nil {
		t.Fatalf("HashRecoveryCode: %v", err)
	}
	if strings.Contains(hash, NormaliseRecoveryCode(code)) {
		t.Fatal("the stored hash contains the code itself")
	}
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Fatalf("recovery codes are not stored with the project's password hashing: %q", hash)
	}

	for _, variant := range []string{
		code,
		strings.ToLower(code),
		strings.ReplaceAll(code, "-", ""),
		" " + code + " ",
		strings.ReplaceAll(code, "-", " "),
	} {
		if !CheckRecoveryCode(variant, hash) {
			t.Errorf("CheckRecoveryCode rejected the variant %q", variant)
		}
	}

	if CheckRecoveryCode("00000-00000", hash) && NormaliseRecoveryCode("00000-00000") != NormaliseRecoveryCode(code) {
		t.Error("CheckRecoveryCode accepted an unrelated code")
	}
}

// TestNormaliseRecoveryCode pins the Crockford substitutions, so a code
// written down as O or l still works.
func TestNormaliseRecoveryCode(t *testing.T) {
	cases := map[string]string{
		"abcde-fghjk":   "ABCDEFGHJK",
		"O1I-Ll U":      "01111V",
		" 12345 67890 ": "1234567890",
	}
	for input, want := range cases {
		if got := NormaliseRecoveryCode(input); got != want {
			t.Errorf("NormaliseRecoveryCode(%q) = %q, want %q", input, got, want)
		}
	}
}
