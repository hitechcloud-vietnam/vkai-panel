package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The key model, stated once so nobody has to infer it from the code.
//
// WHERE THE KEY LIVES
//
// The operator holds one 32 byte master key. The panel finds it in exactly one
// of two places, in this order:
//
//	VKAI_BACKUP_KEY_FILE   path to a file containing the key (hex or base64)
//	VKAI_BACKUP_KEY        the key itself, hex or base64
//
// There is no default and no generated-on-first-use fallback. A panel with no
// key configured refuses to write an encrypted backup rather than writing an
// unencrypted one that the operator believes is encrypted.
//
// The key file must not live inside the backup root. LoadKey enforces that:
// a key stored next to the ciphertext it protects is not encryption, it is
// obfuscation with extra steps, and the check is what stops an operator from
// arriving at that arrangement by accident. The recommended location is
// /vkai-panel/etc/backup.key, mode 0400, owned by root, and copied to wherever
// the operator keeps their other secrets. LoadKey refuses a key file that is
// group or world readable for the same reason.
//
// IF THE OPERATOR LOSES THE KEY
//
// Every archive written with it is unrecoverable. Not "hard to recover" -
// unrecoverable. AES-256-GCM with a random 256 bit data key wrapped under the
// master key has no recovery path, no escrow and no vendor override, and the
// panel deliberately stores nothing that would create one. The panel records
// only the key ID (below), which identifies a key without revealing it.
//
// This is the intended trade. The operator's obligation is to keep a copy of
// the key somewhere that does not burn down with the server. The panel's
// obligation is to say so plainly and to make the failure loud: restoring with
// the wrong key fails with ErrWrongKey before a single byte is written, rather
// than producing garbage that looks like a restore.

const (
	// EnvKeyFile names the file holding the operator's backup key.
	EnvKeyFile = "VKAI_BACKUP_KEY_FILE"
	// EnvKey carries the key inline, for container deployments that inject
	// secrets as environment variables.
	EnvKey = "VKAI_BACKUP_KEY"
)

// keyIDLabel domain-separates the key ID derivation from every other use of
// the key, so the ID can be stored and logged without weakening the key.
const keyIDLabel = "vkai-backup-key-id/v1"

// keyIDLen is the length in bytes of the truncated HMAC used as a key ID.
const keyIDLen = 16

var (
	// ErrNoKey is returned when encryption was asked for but the operator has
	// configured no key.
	ErrNoKey = errors.New("backup: no encryption key configured (set " + EnvKeyFile + " or " + EnvKey + ")")

	// ErrWrongKey is returned when an archive was written under a different
	// key than the one supplied. It is checked before any output is produced.
	ErrWrongKey = errors.New("backup: archive was encrypted with a different key")

	// ErrNotEncrypted is returned when a decrypting reader is pointed at a
	// stream that is not a VKAI encrypted archive.
	ErrNotEncrypted = errors.New("backup: stream is not a VKAI encrypted archive")

	// ErrCorrupt is returned when the ciphertext fails authentication or the
	// stream ends before its terminator. Both mean the same thing to a caller:
	// these bytes are not the bytes that were written.
	ErrCorrupt = errors.New("backup: encrypted archive is corrupt or truncated")
)

// Key is the operator's master key together with its public identifier.
type Key struct {
	raw [32]byte
	id  string
}

// NewKey builds a Key from 32 raw bytes.
func NewKey(raw []byte) (*Key, error) {
	if len(raw) != 32 {
		return nil, fmt.Errorf("backup: encryption key must be 32 bytes, got %d", len(raw))
	}
	k := &Key{}
	copy(k.raw[:], raw)
	mac := hmac.New(sha256.New, k.raw[:])
	mac.Write([]byte(keyIDLabel))
	k.id = hex.EncodeToString(mac.Sum(nil)[:keyIDLen])
	return k, nil
}

// ParseKey decodes a key written as 64 hex characters or as base64.
func ParseKey(s string) (*Key, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrNoKey
	}
	if raw, err := hex.DecodeString(s); err == nil && len(raw) == 32 {
		return NewKey(raw)
	}
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil && len(raw) == 32 {
		return NewKey(raw)
	}
	if raw, err := base64.RawStdEncoding.DecodeString(s); err == nil && len(raw) == 32 {
		return NewKey(raw)
	}
	return nil, errors.New("backup: encryption key must be a 32 byte key encoded as hex or base64")
}

// GenerateKey produces a fresh random key. It is what the panelctl key
// generation command hands the operator; the panel never persists the result.
func GenerateKey() (*Key, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, fmt.Errorf("backup: could not generate a key: %w", err)
	}
	return NewKey(raw)
}

// ID is the public identifier of the key: HMAC-SHA256 of a fixed label under
// the key, truncated to 16 bytes. It is safe to store beside a backup and safe
// to log. It is what lets a restore say "this archive needs a different key"
// instead of "decryption failed".
func (k *Key) ID() string {
	if k == nil {
		return ""
	}
	return k.id
}

// Equal reports whether two keys are the same key, in constant time.
func (k *Key) Equal(other *Key) bool {
	if k == nil || other == nil {
		return k == other
	}
	return subtle.ConstantTimeCompare(k.raw[:], other.raw[:]) == 1
}

// LoadKeyOptions controls where LoadKey looks and how strict it is.
type LoadKeyOptions struct {
	// File overrides EnvKeyFile.
	File string
	// Inline overrides EnvKey.
	Inline string
	// ForbiddenRoot is a directory the key file may not live inside; it is
	// the backup root in production. Empty disables the check.
	ForbiddenRoot string
	// AllowLoosePermissions skips the 0077 mode check. Only tests set it.
	AllowLoosePermissions bool
}

// LoadKey resolves the operator's key from the environment.
func LoadKey(opts LoadKeyOptions) (*Key, error) {
	file := opts.File
	if file == "" {
		file = strings.TrimSpace(os.Getenv(EnvKeyFile))
	}
	if file != "" {
		return loadKeyFile(file, opts)
	}

	inline := opts.Inline
	if inline == "" {
		inline = strings.TrimSpace(os.Getenv(EnvKey))
	}
	if inline == "" {
		return nil, ErrNoKey
	}
	return ParseKey(inline)
}

func loadKeyFile(file string, opts LoadKeyOptions) (*Key, error) {
	if !filepath.IsAbs(file) {
		return nil, fmt.Errorf("backup: %s must be an absolute path, got %q", EnvKeyFile, file)
	}
	clean := filepath.Clean(file)

	if root := strings.TrimSpace(opts.ForbiddenRoot); root != "" {
		base := filepath.Clean(root)
		if clean == base || strings.HasPrefix(clean, base+string(filepath.Separator)) {
			return nil, fmt.Errorf(
				"backup: the encryption key file %q is inside the backup tree %q; "+
					"a key stored beside the archives it protects protects nothing - move it out",
				clean, base)
		}
	}

	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("backup: cannot read the encryption key file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("backup: %q is a directory, not a key file", clean)
	}
	if !opts.AllowLoosePermissions && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf(
			"backup: the encryption key file %q is mode %04o; it must not be readable by group or others (chmod 0400)",
			clean, info.Mode().Perm())
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("backup: cannot read the encryption key file: %w", err)
	}
	return ParseKey(string(data))
}

// ============================================================
// The encrypted stream format
// ============================================================
//
// A random 256 bit data key encrypts the payload; the master key encrypts the
// data key. That is why a master key rotation does not have to rewrite any
// archive, and why the master key is used for one 32 byte operation per
// archive rather than for gigabytes of payload.
//
// Header (93 bytes, all fixed width, nothing length-prefixed):
//
//	 8  magic "VKAIBKP1"
//	 1  format version, currently 1
//	16  key ID of the master key
//	12  nonce for the wrapped data key
//	48  the data key sealed under the master key (32 bytes + 16 byte tag)
//	 8  nonce prefix for the payload chunks
//
// Payload: a sequence of chunks, each
//
//	4  big endian length of the sealed chunk
//	n  AES-256-GCM sealed chunk
//
// The nonce of chunk i is noncePrefix || uint32be(i), so it is unique within
// the archive by construction and the data key is fresh per archive.
//
// Each chunk's additional data is magic || version || uint32be(i) || finalFlag.
// Binding the index stops a chunk from being moved or duplicated. Binding the
// final flag, plus a mandatory terminating chunk, stops truncation: a reader
// that reaches EOF without having seen a chunk marked final returns ErrCorrupt
// rather than a short file that looks complete.

const (
	magic         = "VKAIBKP1"
	formatVersion = 1

	wrapNonceLen   = 12
	wrappedKeyLen  = 32 + 16
	noncePrefixLen = 8

	headerLen = len(magic) + 1 + keyIDLen + wrapNonceLen + wrappedKeyLen + noncePrefixLen

	// defaultChunkSize is the plaintext size of one sealed chunk. 1 MiB keeps
	// the per-chunk overhead under 0.002% and bounds the memory a restore
	// needs regardless of how large the archive is.
	defaultChunkSize = 1 << 20

	// maxChunkSize bounds what a decrypting reader will allocate for a length
	// prefix it has not yet authenticated. Without it a corrupt four byte
	// length is an out-of-memory condition.
	maxChunkSize = 64 << 20
)

func chunkAAD(index uint32, final bool) []byte {
	aad := make([]byte, 0, len(magic)+1+4+1)
	aad = append(aad, magic...)
	aad = append(aad, formatVersion)
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], index)
	aad = append(aad, idx[:]...)
	if final {
		aad = append(aad, 1)
	} else {
		aad = append(aad, 0)
	}
	return aad
}

func chunkNonce(prefix []byte, index uint32) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[noncePrefixLen:], index)
	return nonce
}

// Encrypt copies src to dst as a VKAI encrypted archive under k.
func Encrypt(dst io.Writer, src io.Reader, k *Key) error {
	if k == nil {
		return ErrNoKey
	}

	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return fmt.Errorf("backup: could not generate a data key: %w", err)
	}
	wrapNonce := make([]byte, wrapNonceLen)
	if _, err := io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return fmt.Errorf("backup: could not generate a nonce: %w", err)
	}
	noncePrefix := make([]byte, noncePrefixLen)
	if _, err := io.ReadFull(rand.Reader, noncePrefix); err != nil {
		return fmt.Errorf("backup: could not generate a nonce: %w", err)
	}

	keyID, err := hex.DecodeString(k.ID())
	if err != nil {
		return fmt.Errorf("backup: malformed key id: %w", err)
	}

	header := make([]byte, 0, headerLen)
	header = append(header, magic...)
	header = append(header, formatVersion)
	header = append(header, keyID...)
	header = append(header, wrapNonce...)

	master, err := newGCM(k.raw[:])
	if err != nil {
		return err
	}
	// The header so far is the additional data of the wrap, so an archive
	// cannot be re-labelled with someone else's key ID.
	wrapped := master.Seal(nil, wrapNonce, dataKey, header)
	header = append(header, wrapped...)
	header = append(header, noncePrefix...)

	if len(header) != headerLen {
		return fmt.Errorf("backup: internal error: header is %d bytes, expected %d", len(header), headerLen)
	}
	if _, err := dst.Write(header); err != nil {
		return fmt.Errorf("backup: could not write the archive header: %w", err)
	}

	payload, err := newGCM(dataKey)
	if err != nil {
		return err
	}

	buf := make([]byte, defaultChunkSize)
	var index uint32
	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			if err := writeChunk(dst, payload, noncePrefix, index, false, buf[:n]); err != nil {
				return err
			}
			index++
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
		return fmt.Errorf("backup: could not read the archive payload: %w", readErr)
	}

	// The terminator. Always written, always empty, always marked final.
	return writeChunk(dst, payload, noncePrefix, index, true, nil)
}

func writeChunk(dst io.Writer, aead cipher.AEAD, prefix []byte, index uint32, final bool, plain []byte) error {
	sealed := aead.Seal(nil, chunkNonce(prefix, index), plain, chunkAAD(index, final))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(sealed)))
	if _, err := dst.Write(length[:]); err != nil {
		return fmt.Errorf("backup: could not write the archive: %w", err)
	}
	if _, err := dst.Write(sealed); err != nil {
		return fmt.Errorf("backup: could not write the archive: %w", err)
	}
	return nil
}

// PeekKeyID reads the header of an encrypted archive and returns the ID of the
// key it was written under, without needing that key. It is what lets a
// restore tell the operator which key to fetch.
func PeekKeyID(header []byte) (string, error) {
	if len(header) < len(magic)+1+keyIDLen {
		return "", ErrNotEncrypted
	}
	if string(header[:len(magic)]) != magic {
		return "", ErrNotEncrypted
	}
	if header[len(magic)] != formatVersion {
		return "", fmt.Errorf("backup: archive format version %d is not supported by this panel", header[len(magic)])
	}
	off := len(magic) + 1
	return hex.EncodeToString(header[off : off+keyIDLen]), nil
}

// decryptReader turns an encrypted archive back into the plaintext stream.
type decryptReader struct {
	src     io.Reader
	aead    cipher.AEAD
	prefix  []byte
	index   uint32
	pending []byte
	done    bool
	err     error
}

// NewDecryptReader validates the header, refuses a stream written under a
// different key, and returns a reader over the plaintext.
//
// The wrong-key check happens here, before the caller has been handed a single
// byte, so a restore under the wrong key cannot half-write anything.
func NewDecryptReader(src io.Reader, k *Key) (io.Reader, error) {
	if k == nil {
		return nil, ErrNoKey
	}

	header := make([]byte, headerLen)
	if _, err := io.ReadFull(src, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrNotEncrypted
		}
		return nil, fmt.Errorf("backup: could not read the archive header: %w", err)
	}

	archiveKeyID, err := PeekKeyID(header)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(archiveKeyID), []byte(k.ID())) != 1 {
		return nil, fmt.Errorf("%w (archive key id %s, supplied key id %s)", ErrWrongKey, archiveKeyID, k.ID())
	}

	off := len(magic) + 1 + keyIDLen
	wrapNonce := header[off : off+wrapNonceLen]
	off += wrapNonceLen
	wrapped := header[off : off+wrappedKeyLen]
	off += wrappedKeyLen
	prefix := header[off : off+noncePrefixLen]

	master, err := newGCM(k.raw[:])
	if err != nil {
		return nil, err
	}
	dataKey, err := master.Open(nil, wrapNonce, wrapped, header[:len(magic)+1+keyIDLen+wrapNonceLen])
	if err != nil {
		// The key ID matched but the wrapped key did not authenticate, so the
		// header itself has been tampered with.
		return nil, ErrCorrupt
	}

	payload, err := newGCM(dataKey)
	if err != nil {
		return nil, err
	}

	return &decryptReader{src: src, aead: payload, prefix: prefix}, nil
}

func (d *decryptReader) Read(p []byte) (int, error) {
	for len(d.pending) == 0 {
		if d.err != nil {
			return 0, d.err
		}
		if d.done {
			return 0, io.EOF
		}
		if err := d.next(); err != nil {
			d.err = err
			return 0, err
		}
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	return n, nil
}

func (d *decryptReader) next() error {
	var length [4]byte
	if _, err := io.ReadFull(d.src, length[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// Ended without a chunk marked final: truncated.
			return ErrCorrupt
		}
		return fmt.Errorf("backup: could not read the archive: %w", err)
	}
	size := binary.BigEndian.Uint32(length[:])
	if size < uint32(d.aead.Overhead()) || size > maxChunkSize {
		return ErrCorrupt
	}

	sealed := make([]byte, size)
	if _, err := io.ReadFull(d.src, sealed); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrCorrupt
		}
		return fmt.Errorf("backup: could not read the archive: %w", err)
	}

	nonce := chunkNonce(d.prefix, d.index)

	// A chunk is either an interior chunk or the terminator. Try interior
	// first because that is the common case; the two differ only in one byte
	// of additional data, so a chunk cannot be promoted or demoted.
	if plain, err := d.aead.Open(nil, nonce, sealed, chunkAAD(d.index, false)); err == nil {
		d.index++
		d.pending = plain
		return nil
	}
	plain, err := d.aead.Open(nil, nonce, sealed, chunkAAD(d.index, true))
	if err != nil {
		return ErrCorrupt
	}
	if len(plain) != 0 {
		return ErrCorrupt
	}
	d.done = true
	return nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backup: could not initialise the cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("backup: could not initialise GCM: %w", err)
	}
	return aead, nil
}
