package twofactor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// keyLabel gives the two-factor key its own domain. The panel master key is
// also used for other field encryption, and reusing one AES key across
// unrelated fields means one nonce-reuse bug anywhere compromises everything.
const keyLabel = "vkai-panel/two-factor/totp-secret/v1"

// CurrentKeyVersion is written next to every ciphertext so a future key
// rotation can tell which records still need re-wrapping.
const CurrentKeyVersion = 1

// SecretBox encrypts TOTP shared secrets for storage.
//
// A TOTP secret is a bearer credential: whoever holds it can mint valid codes
// forever, without the phone. Storing it beside the username would mean one
// database dump hands over every second factor in the fleet, so it is sealed
// with AES-256-GCM under a key derived from panel configuration and never
// written in the clear.
type SecretBox struct {
	aead    cipher.AEAD
	version int
}

// NewSecretBox derives the two-factor encryption key from the panel master key
// and returns a sealer for it. The master key never becomes an AES key
// directly: it is run through HMAC-SHA-256 with a fixed label, so this key is
// distinct from every other key derived from the same master.
func NewSecretBox(master []byte) (*SecretBox, error) {
	if len(master) < 16 {
		return nil, errors.New("two-factor master key must be at least 16 bytes")
	}

	key := DeriveKey(master, keyLabel)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("two-factor cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("two-factor AEAD: %w", err)
	}

	return &SecretBox{aead: aead, version: CurrentKeyVersion}, nil
}

// DeriveKey returns a 256-bit sub-key for a labelled purpose.
func DeriveKey(master []byte, label string) []byte {
	mac := hmac.New(sha256.New, master)
	mac.Write([]byte(label))
	return mac.Sum(nil)
}

// MasterKeyFromEnv reads the panel master key from VKAI_SECRET_KEY, the same
// variable the rest of the panel uses for stored credentials, as 32 bytes in
// hex or base64.
//
// There is deliberately no fallback. A default key would mean every VKAI Panel
// installation on earth shares one key, and a stolen database from any of them
// would decrypt the second factors of all of them.
func MasterKeyFromEnv() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("VKAI_SECRET_KEY"))
	if raw == "" {
		return nil, errors.New("VKAI_SECRET_KEY is not set: cannot store two-factor secrets safely")
	}
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.StdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	return nil, errors.New("VKAI_SECRET_KEY must be a 32 byte key encoded as hex or base64")
}

// Seal encrypts a secret and returns "<version>:<base64(nonce||ciphertext)>".
// The version prefix is what makes key rotation possible later without
// guessing which key a row was written with.
func (b *SecretBox) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("two-factor nonce: %w", err)
	}

	// The version is authenticated as additional data so a stored record
	// cannot be relabelled to point at a different key.
	aad := []byte(strconv.Itoa(b.version))
	sealed := b.aead.Seal(nonce, nonce, plaintext, aad)

	return strconv.Itoa(b.version) + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal. Any failure - wrong key, truncated row, edited
// ciphertext - is one error: the secret is gone and the user must re-enrol.
func (b *SecretBox) Open(encoded string) ([]byte, error) {
	version, payload, found := strings.Cut(encoded, ":")
	if !found {
		return nil, ErrSecretUnreadable
	}
	if version != strconv.Itoa(b.version) {
		return nil, ErrSecretUnreadable
	}

	sealed, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrSecretUnreadable
	}
	if len(sealed) < b.aead.NonceSize() {
		return nil, ErrSecretUnreadable
	}

	nonce, ciphertext := sealed[:b.aead.NonceSize()], sealed[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, []byte(version))
	if err != nil {
		return nil, ErrSecretUnreadable
	}
	return plaintext, nil
}

// Version reports the key version this box writes.
func (b *SecretBox) Version() int { return b.version }
