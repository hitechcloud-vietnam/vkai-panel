package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

func testHasher(t *testing.T) *KeyHasher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	hasher, err := NewKeyHasher(key)
	if err != nil {
		t.Fatalf("NewKeyHasher: %v", err)
	}
	return hasher
}

func TestGenerateAPIKeyShape(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if !strings.HasPrefix(key, APIKeyPrefixMarker) {
		t.Fatalf("key %q does not carry the panel's marker", key)
	}
	body := strings.TrimPrefix(key, APIKeyPrefixMarker)
	if len(body) != 32 {
		t.Fatalf("key body is %d characters, want 32 (128 bits)", len(body))
	}
	if _, err := hex.DecodeString(body); err != nil {
		t.Fatalf("key body is not hexadecimal: %v", err)
	}

	other, _ := GenerateAPIKey()
	if other == key {
		t.Fatal("two generated keys were identical")
	}
}

func TestHashIsNotReversibleAndVerifies(t *testing.T) {
	hasher := testHasher(t)
	key, _ := GenerateAPIKey()

	stored := hasher.Hash(key)
	if strings.Contains(stored, key) || strings.Contains(stored, strings.TrimPrefix(key, APIKeyPrefixMarker)) {
		t.Fatal("the stored digest contains the key itself")
	}
	if !strings.HasPrefix(stored, "hmac-sha256$") {
		t.Fatalf("stored digest %q does not name its algorithm", stored)
	}

	ok, upgrade, err := hasher.VerifyStoredHash(key, stored)
	if err != nil || !ok {
		t.Fatalf("a correct key did not verify: ok=%v upgrade=%v err=%v", ok, upgrade, err)
	}
	if upgrade {
		t.Fatal("a freshly written digest was reported as needing an upgrade")
	}

	wrong, _ := GenerateAPIKey()
	ok, _, err = hasher.VerifyStoredHash(wrong, stored)
	if err != nil || ok {
		t.Fatalf("a different key verified against the digest: ok=%v err=%v", ok, err)
	}
}

func TestHashIsPeppered(t *testing.T) {
	// The point of the pepper: a database dump alone does not let an attacker
	// test a guess, because the digest depends on a key that is not in the
	// database.
	first := testHasher(t)

	second := make([]byte, 32)
	for i := range second {
		second[i] = byte(255 - i)
	}
	other, err := NewKeyHasher(second)
	if err != nil {
		t.Fatalf("NewKeyHasher: %v", err)
	}

	key, _ := GenerateAPIKey()
	if first.Hash(key) == other.Hash(key) {
		t.Fatal("the digest does not depend on the panel master key")
	}
	if first.Hash(key) == LegacyDigest(key) {
		t.Fatal("the digest is a bare SHA-256 of the key")
	}
}

func TestVerifyAcceptsALegacyDigestAndAsksForAnUpgrade(t *testing.T) {
	hasher := testHasher(t)
	key, _ := GenerateAPIKey()

	ok, upgrade, err := hasher.VerifyStoredHash(key, LegacyDigest(key))
	if err != nil {
		t.Fatalf("a legacy SHA-256 digest errored: %v", err)
	}
	if !ok {
		t.Fatal("a key stored under the previous scheme stopped working")
	}
	if !upgrade {
		t.Fatal("the caller was not told to rewrite the digest in the current format")
	}
}

func TestVerifyRefusesAPlaintextDigest(t *testing.T) {
	// service/multi_user.go used to store the key itself in key_hash. Such a
	// row is not a badly stored credential, it is a live secret in a table
	// that goes into every backup, and this refuses to authenticate against it
	// so that the key has to be re-minted.
	hasher := testHasher(t)
	key, _ := GenerateAPIKey()

	ok, _, err := hasher.VerifyStoredHash(key, key)
	if ok {
		t.Fatal("a row whose key_hash is the key itself authenticated")
	}
	if err == nil {
		t.Fatal("the caller was not told the digest format is unsupported")
	}

	if _, _, err := hasher.VerifyStoredHash(key, ""); err == nil {
		t.Fatal("an empty digest did not report an error")
	}
	if _, _, err := hasher.VerifyStoredHash(key, "bcrypt$whatever"); err == nil {
		t.Fatal("an unknown algorithm marker did not report an error")
	}
}

func TestLookupPrefixesCoverBothConventions(t *testing.T) {
	key := "vk_live_0123456789abcdef0123456789abcdef"
	prefixes := APIKeyLookupPrefixes(key)

	if len(prefixes) != 2 {
		t.Fatalf("prefixes = %v, want the 12 character form and the 8 character one", prefixes)
	}
	if prefixes[0] != key[:12] {
		t.Fatalf("prefixes[0] = %q, want %q", prefixes[0], key[:12])
	}
	if prefixes[1] != key[:8] {
		t.Fatalf("prefixes[1] = %q, want the legacy 8 character prefix %q", prefixes[1], key[:8])
	}
	if APIKeyPrefix(key) != key[:12] {
		t.Fatalf("APIKeyPrefix stores %q", APIKeyPrefix(key))
	}
}

func TestFingerprintIsStableAndShort(t *testing.T) {
	hasher := testHasher(t)
	key, _ := GenerateAPIKey()

	first := hasher.Fingerprint(key)
	if first != hasher.Fingerprint(key) {
		t.Fatal("the fingerprint is not stable")
	}
	if len(first) != 16 {
		t.Fatalf("fingerprint is %d characters", len(first))
	}
	if strings.Contains(hasher.Hash(key), key) {
		t.Fatal("the fingerprint's source contains the key")
	}
}

func TestKeyHasherFromEnvRefusesAMissingMasterKey(t *testing.T) {
	t.Setenv("VKAI_SECRET_KEY", "")
	if _, err := NewKeyHasherFromEnv(); err == nil {
		t.Fatal("a hasher was built without a master key; the panel would have silently fallen back to an unpeppered digest")
	}

	t.Setenv("VKAI_SECRET_KEY", "too short")
	if _, err := NewKeyHasherFromEnv(); err == nil {
		t.Fatal("a short master key was accepted")
	}

	t.Setenv("VKAI_SECRET_KEY", strings.Repeat("ab", 32))
	if _, err := NewKeyHasherFromEnv(); err != nil {
		t.Fatalf("a valid 32 byte hex master key was refused: %v", err)
	}
}
