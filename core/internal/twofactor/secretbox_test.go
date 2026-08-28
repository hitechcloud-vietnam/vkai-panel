package twofactor

import (
	"strings"
	"testing"
)

func testMaster(seed byte) []byte {
	master := make([]byte, 32)
	for i := range master {
		master[i] = seed + byte(i)
	}
	return master
}

// TestSecretBoxRoundTrip checks the sealed form is opaque, versioned and
// reversible.
func TestSecretBoxRoundTrip(t *testing.T) {
	box, err := NewSecretBox(testMaster(1))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}

	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	sealed, err := box.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !strings.HasPrefix(sealed, "1:") {
		t.Fatalf("sealed value %q does not carry a key version", sealed)
	}
	if strings.Contains(sealed, string(secret)) {
		t.Fatal("the sealed value contains the plaintext secret")
	}

	opened, err := box.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(opened) != string(secret) {
		t.Fatal("Open returned a different secret")
	}

	// Sealing twice must not produce the same ciphertext: a deterministic
	// ciphertext would let anyone with a database dump spot two accounts that
	// share a secret.
	again, err := box.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if again == sealed {
		t.Fatal("sealing the same secret twice produced identical ciphertext")
	}
}

// TestSecretBoxRejectsTampering: GCM authenticates, so an edited row must fail
// rather than decrypt to something else.
func TestSecretBoxRejectsTampering(t *testing.T) {
	box, err := NewSecretBox(testMaster(2))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	secret, _ := GenerateSecret()
	sealed, _ := box.Seal(secret)

	corrupted := []byte(sealed)
	corrupted[len(corrupted)-1] ^= 0x01
	for _, bad := range []string{
		string(corrupted),
		strings.TrimPrefix(sealed, "1:"),
		"2:" + strings.SplitN(sealed, ":", 2)[1],
		"1:not-base64",
		"1:",
		"",
	} {
		if _, err := box.Open(bad); err == nil {
			t.Errorf("Open accepted a damaged value %q", bad)
		}
	}
}

// TestSecretBoxKeysAreSeparated: a different master key cannot read the
// ciphertext, and the two-factor key is not the master key itself.
func TestSecretBoxKeysAreSeparated(t *testing.T) {
	first, _ := NewSecretBox(testMaster(3))
	second, _ := NewSecretBox(testMaster(9))

	secret, _ := GenerateSecret()
	sealed, _ := first.Seal(secret)

	if _, err := second.Open(sealed); err == nil {
		t.Fatal("a box built from a different master key opened the ciphertext")
	}

	master := testMaster(3)
	derived := DeriveKey(master, keyLabel)
	if string(derived) == string(master) {
		t.Fatal("the encryption key is the master key verbatim")
	}
	if string(derived) == string(DeriveKey(master, "some-other-purpose")) {
		t.Fatal("two labelled sub-keys collide")
	}
	if len(derived) != 32 {
		t.Fatalf("derived key is %d bytes, want 32 for AES-256", len(derived))
	}
}

// TestNewSecretBoxRejectsWeakMaster: a short key must fail loudly rather than
// be silently stretched.
func TestNewSecretBoxRejectsWeakMaster(t *testing.T) {
	if _, err := NewSecretBox([]byte("short")); err == nil {
		t.Fatal("NewSecretBox accepted a five byte master key")
	}
}
