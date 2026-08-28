package backup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(t *testing.T, seed byte) *Key {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	k, err := NewKey(raw)
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t, 1)

	// Sizes either side of the chunk boundary, because an off-by-one in the
	// terminator only shows up when the payload is an exact multiple.
	for _, size := range []int{0, 1, 4096, defaultChunkSize - 1, defaultChunkSize, defaultChunkSize + 1, 2*defaultChunkSize + 17} {
		plain := make([]byte, size)
		if _, err := io.ReadFull(rand.Reader, plain); err != nil {
			t.Fatalf("rand: %v", err)
		}

		var sealed bytes.Buffer
		if err := Encrypt(&sealed, bytes.NewReader(plain), key); err != nil {
			t.Fatalf("Encrypt(%d bytes): %v", size, err)
		}
		if sealed.Len() <= size {
			t.Fatalf("Encrypt(%d bytes) produced %d bytes, expected growth", size, sealed.Len())
		}

		r, err := NewDecryptReader(bytes.NewReader(sealed.Bytes()), key)
		if err != nil {
			t.Fatalf("NewDecryptReader(%d bytes): %v", size, err)
		}
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read back %d bytes: %v", size, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("round trip of %d bytes did not return the same bytes", size)
		}
	}
}

func TestEncryptedStreamNeverContainsPlaintext(t *testing.T) {
	key := testKey(t, 7)
	secret := bytes.Repeat([]byte("DB_PASSWORD=hunter2;"), 500)

	var sealed bytes.Buffer
	if err := Encrypt(&sealed, bytes.NewReader(secret), key); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(sealed.Bytes(), []byte("hunter2")) {
		t.Fatal("the encrypted stream contains the plaintext; encryption is not happening before the bytes leave")
	}
}

func TestDecryptWithWrongKeyIsRefusedBeforeAnyOutput(t *testing.T) {
	right := testKey(t, 1)
	wrong := testKey(t, 2)

	var sealed bytes.Buffer
	if err := Encrypt(&sealed, strings.NewReader("the only copy of the customer database"), right); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	r, err := NewDecryptReader(bytes.NewReader(sealed.Bytes()), wrong)
	if !errors.Is(err, ErrWrongKey) {
		t.Fatalf("expected ErrWrongKey, got reader=%v err=%v", r, err)
	}
	if r != nil {
		t.Fatal("a reader was returned for the wrong key; a restore could have started writing")
	}
	// The message has to name both keys, or an operator holding three key
	// files cannot tell which one this archive wants.
	if !strings.Contains(err.Error(), right.ID()) || !strings.Contains(err.Error(), wrong.ID()) {
		t.Fatalf("error does not identify the keys: %v", err)
	}
}

func TestDecryptDetectsCorruption(t *testing.T) {
	key := testKey(t, 3)
	plain := bytes.Repeat([]byte("payload"), 5000)

	var sealed bytes.Buffer
	if err := Encrypt(&sealed, bytes.NewReader(plain), key); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	t.Run("flipped ciphertext byte", func(t *testing.T) {
		corrupt := append([]byte(nil), sealed.Bytes()...)
		corrupt[headerLen+40] ^= 0x01
		r, err := NewDecryptReader(bytes.NewReader(corrupt), key)
		if err != nil {
			t.Fatalf("header should still open: %v", err)
		}
		if _, err := io.ReadAll(r); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("expected ErrCorrupt, got %v", err)
		}
	})

	t.Run("flipped header byte", func(t *testing.T) {
		corrupt := append([]byte(nil), sealed.Bytes()...)
		// Inside the wrapped data key, past magic, version and key id.
		corrupt[len(magic)+1+keyIDLen+wrapNonceLen+2] ^= 0x01
		if _, err := NewDecryptReader(bytes.NewReader(corrupt), key); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("expected ErrCorrupt, got %v", err)
		}
	})

	t.Run("truncated archive", func(t *testing.T) {
		// Cut the terminating chunk off. Without the final-chunk marker this
		// would read back as a shorter but perfectly valid payload, which is
		// the failure that makes a partial upload look like a good backup.
		full := sealed.Bytes()
		truncated := full[:len(full)-40]
		r, err := NewDecryptReader(bytes.NewReader(truncated), key)
		if err != nil {
			t.Fatalf("header should still open: %v", err)
		}
		if _, err := io.ReadAll(r); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("expected ErrCorrupt for a truncated archive, got %v", err)
		}
	})

	t.Run("reordered chunks", func(t *testing.T) {
		var small bytes.Buffer
		chunkA := bytes.Repeat([]byte("A"), defaultChunkSize)
		chunkB := bytes.Repeat([]byte("B"), defaultChunkSize)
		if err := Encrypt(&small, bytes.NewReader(append(chunkA, chunkB...)), key); err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		raw := small.Bytes()
		// Two equal-sized sealed chunks follow the header; swap them.
		sealedLen := 4 + defaultChunkSize + 16
		first := append([]byte(nil), raw[headerLen:headerLen+sealedLen]...)
		second := append([]byte(nil), raw[headerLen+sealedLen:headerLen+2*sealedLen]...)
		swapped := append([]byte(nil), raw[:headerLen]...)
		swapped = append(swapped, second...)
		swapped = append(swapped, first...)
		swapped = append(swapped, raw[headerLen+2*sealedLen:]...)

		r, err := NewDecryptReader(bytes.NewReader(swapped), key)
		if err != nil {
			t.Fatalf("header should still open: %v", err)
		}
		if _, err := io.ReadAll(r); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("expected ErrCorrupt for reordered chunks, got %v", err)
		}
	})
}

func TestPeekKeyIDNamesTheKeyWithoutHavingIt(t *testing.T) {
	key := testKey(t, 9)
	var sealed bytes.Buffer
	if err := Encrypt(&sealed, strings.NewReader("x"), key); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	id, err := PeekKeyID(sealed.Bytes())
	if err != nil {
		t.Fatalf("PeekKeyID: %v", err)
	}
	if id != key.ID() {
		t.Fatalf("PeekKeyID returned %q, want %q", id, key.ID())
	}
	if _, err := PeekKeyID([]byte("not an archive at all")); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("expected ErrNotEncrypted, got %v", err)
	}
}

func TestKeyIDDoesNotRevealTheKey(t *testing.T) {
	key := testKey(t, 11)
	id, err := hex.DecodeString(key.ID())
	if err != nil {
		t.Fatalf("key id is not hex: %v", err)
	}
	if bytes.Contains(key.raw[:], id) {
		t.Fatal("the key id appears inside the key material")
	}
	if len(id) != keyIDLen {
		t.Fatalf("key id is %d bytes, want %d", len(id), keyIDLen)
	}
	// Two keys differing in one byte must not share an ID.
	other := testKey(t, 12)
	if other.ID() == key.ID() {
		t.Fatal("two different keys produced the same key id")
	}
}

func TestLoadKeyRefusesAKeyStoredInsideTheBackupTree(t *testing.T) {
	backupRoot := t.TempDir()
	keyFile := filepath.Join(backupRoot, "backup.key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("ab", 32)), 0o400); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_, err := LoadKey(LoadKeyOptions{File: keyFile, ForbiddenRoot: backupRoot})
	if err == nil {
		t.Fatal("a key file inside the backup root was accepted")
	}
	if !strings.Contains(err.Error(), "protects nothing") {
		t.Fatalf("the error does not explain the problem: %v", err)
	}

	// The same file outside the backup tree is fine.
	elsewhere := filepath.Join(t.TempDir(), "backup.key")
	if err := os.WriteFile(elsewhere, []byte(strings.Repeat("ab", 32)), 0o400); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if _, err := LoadKey(LoadKeyOptions{File: elsewhere, ForbiddenRoot: backupRoot}); err != nil {
		t.Fatalf("a key outside the backup root was refused: %v", err)
	}
}

func TestLoadKeyRefusesAWorldReadableKeyFile(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "backup.key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("cd", 32)), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if _, err := LoadKey(LoadKeyOptions{File: keyFile}); err == nil {
		t.Fatal("a mode 0644 key file was accepted")
	}
}

func TestLoadKeyHasNoDefault(t *testing.T) {
	t.Setenv(EnvKeyFile, "")
	t.Setenv(EnvKey, "")
	if _, err := LoadKey(LoadKeyOptions{}); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey with nothing configured, got %v", err)
	}
}

func TestLoadKeyFromEnvironment(t *testing.T) {
	k, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	t.Setenv(EnvKeyFile, "")
	t.Setenv(EnvKey, hex.EncodeToString(k.raw[:]))

	loaded, err := LoadKey(LoadKeyOptions{})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if !loaded.Equal(k) {
		t.Fatal("the key loaded from the environment is not the key that was set")
	}
}

func TestEncryptWithoutAKeyIsRefused(t *testing.T) {
	if err := Encrypt(io.Discard, strings.NewReader("x"), nil); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if _, err := NewDecryptReader(strings.NewReader("x"), nil); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
}
