package auth

// API key material: how a key is minted, how it is stored, and how a presented
// key is checked against what was stored.
//
// Three properties are being bought here.
//
// 1. The panel never holds a key it could hand back. What is stored is a
//    digest; the key itself exists in the response to the request that created
//    it and nowhere else, ever. An operator who loses a key rotates it.
//
// 2. The digest is HMAC-SHA-256 under a server-side pepper derived from the
//    panel master key (VKAI_SECRET_KEY), not a bare SHA-256. A bare digest of a
//    high-entropy random string is not realistically invertible, but the pepper
//    costs nothing and it means a stolen database dump is not, on its own,
//    enough to test a guess: the attacker needs the file the panel keeps its
//    master key in as well. That is the difference between one compromise and
//    two.
//
// 3. Nothing about the comparison depends on how much of the key was right.
//    Digests are compared with crypto/hmac.Equal.
//
// The stored form carries its algorithm so the format can move again without a
// migration:
//
//	hmac-sha256$<64 hex characters>
//
// A stored value with no "$" is from before this change and is a bare SHA-256
// digest; it still verifies, and the caller is told to write the peppered form
// back. A stored value that is neither is refused - see VerifyStoredHash.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Key format. The prefix is stored in the clear so a presented key can be
// looked up without scanning every row, and so an operator can recognise a key
// in a log line without the log line containing the key.
const (
	// APIKeyPrefixMarker is what every key this panel mints starts with.
	APIKeyPrefixMarker = "vk_live_"
	// APIKeyPrefixLength is how much of a key is stored in the clear.
	APIKeyPrefixLength = 12
	// legacyPrefixLength is what service/multi_user.go used. A key minted
	// through that path has an 8 character prefix, so a lookup that only ever
	// asked for 12 would never find it.
	legacyPrefixLength = 8
	// apiKeyRandomBytes is the entropy in a key: 128 bits.
	apiKeyRandomBytes = 16
)

const hmacAlgorithm = "hmac-sha256"

// ErrKeyHashUnsupported means a stored digest is in a form this panel refuses
// to authenticate against. Today that is exactly one form: a row where the
// "hash" is the key itself.
var ErrKeyHashUnsupported = errors.New("stored API key digest is not in a supported format")

// GenerateAPIKey mints a new key. The caller shows it once.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	return APIKeyPrefixMarker + hex.EncodeToString(buf), nil
}

// APIKeyPrefix is the clear-text prefix stored alongside the digest.
func APIKeyPrefix(rawKey string) string {
	if len(rawKey) < APIKeyPrefixLength {
		return rawKey
	}
	return rawKey[:APIKeyPrefixLength]
}

// APIKeyLookupPrefixes returns every prefix a stored row might carry for this
// key, newest convention first.
//
// There are two conventions in this codebase - 12 characters here and 8 in
// service/multi_user.go - and a lookup that assumed one of them would silently
// fail to find keys minted by the other. Both are asked for; the digest check
// afterwards is what actually decides.
func APIKeyLookupPrefixes(rawKey string) []string {
	var out []string
	if len(rawKey) >= APIKeyPrefixLength {
		out = append(out, rawKey[:APIKeyPrefixLength])
	}
	if len(rawKey) >= legacyPrefixLength {
		candidate := rawKey[:legacyPrefixLength]
		if len(out) == 0 || out[0] != candidate {
			out = append(out, candidate)
		}
	}
	if len(out) == 0 && rawKey != "" {
		out = append(out, rawKey)
	}
	return out
}

// KeyHasher turns a key into the digest that is stored.
type KeyHasher struct {
	pepper []byte
}

// NewKeyHasher builds a hasher over a master key. The pepper is derived with a
// purpose label so the same master key used elsewhere in the panel - the
// two-factor secret box, the credential encryption key - never produces the
// same derived key as this does.
func NewKeyHasher(masterKey []byte) (*KeyHasher, error) {
	if len(masterKey) == 0 {
		return nil, errors.New("API key hashing needs a master key")
	}
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte("vkai-panel/api-key-pepper/v1"))
	return &KeyHasher{pepper: mac.Sum(nil)}, nil
}

// NewKeyHasherFromEnv reads the panel master key from VKAI_SECRET_KEY, the
// same variable the rest of the panel's secret handling uses.
//
// A missing key is an error, not a fallback. Falling back to an unpeppered
// digest would mean the panel quietly stops providing the property it claims
// to provide, and nothing would say so - which is the failure this codebase has
// already had four times.
func NewKeyHasherFromEnv() (*KeyHasher, error) {
	raw := strings.TrimSpace(os.Getenv("VKAI_SECRET_KEY"))
	if raw == "" {
		return nil, errors.New("VKAI_SECRET_KEY is not set: cannot mint or verify API keys safely")
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return NewKeyHasher(decoded)
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return NewKeyHasher(decoded)
	}
	return nil, errors.New("VKAI_SECRET_KEY must be a 32 byte key encoded as hex or base64")
}

// Hash returns the value to store for a key.
func (h *KeyHasher) Hash(rawKey string) string {
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write([]byte(rawKey))
	return hmacAlgorithm + "$" + hex.EncodeToString(mac.Sum(nil))
}

// LegacyDigest is the bare SHA-256 the previous implementation stored. It is
// here so verification can recognise those rows, and nowhere else.
func LegacyDigest(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// VerifyStoredHash checks a presented key against a stored digest.
//
// It returns whether the key matches, and whether the stored digest should be
// rewritten in the current format. Rewriting on successful use is how the last
// bare-SHA-256 row disappears without a migration that would have to know every
// key, which it cannot.
//
// A stored value that is neither the peppered form nor a 64 character hex
// digest is refused with ErrKeyHashUnsupported. That case is real: the second
// key-minting path in service/multi_user.go stored the key itself in the
// key_hash column. Such a row is not a credential that happens to be badly
// stored, it is a plaintext secret sitting in a table that gets backed up, and
// this refuses to authenticate against it so that it has to be re-minted.
func (h *KeyHasher) VerifyStoredHash(rawKey, stored string) (ok bool, needsUpgrade bool, err error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return false, false, ErrKeyHashUnsupported
	}

	if algorithm, digest, found := strings.Cut(stored, "$"); found {
		if algorithm != hmacAlgorithm {
			return false, false, ErrKeyHashUnsupported
		}
		expected := h.Hash(rawKey)
		_, expectedDigest, _ := strings.Cut(expected, "$")
		return hmac.Equal([]byte(digest), []byte(expectedDigest)), false, nil
	}

	// No algorithm marker. The only historic form worth honouring is the bare
	// SHA-256 digest: 64 hex characters and nothing else.
	if len(stored) != 64 || !isHex(stored) {
		return false, false, ErrKeyHashUnsupported
	}
	if hmac.Equal([]byte(stored), []byte(LegacyDigest(rawKey))) {
		return true, true, nil
	}
	return false, false, nil
}

// Fingerprint is a short, non-reversible label for a key, for log lines and
// audit entries. It is derived from the peppered digest, so two log lines can
// be tied to the same key without either of them containing it.
func (h *KeyHasher) Fingerprint(rawKey string) string {
	full := h.Hash(rawKey)
	_, digest, _ := strings.Cut(full, "$")
	if len(digest) >= 16 {
		return digest[:16]
	}
	return digest
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
