package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// File names used inside Config.AccountDir. Both are written with mode 0600
// because either one is enough to act as the ACME account.
const (
	accountKeyFile  = "account.key"
	accountMetaFile = "account.json"
)

// filePerm is the mode of every file this package writes. The account key, the
// certificate key and the account metadata are all secrets.
const filePerm os.FileMode = 0o600

// dirPerm is the mode used when this package has to create Config.AccountDir.
const dirPerm os.FileMode = 0o700

// accountMeta is the small JSON record kept beside the account key. Kid is the
// account URL the CA assigned; reusing it lets later runs skip newAccount and
// address the existing account directly.
type accountMeta struct {
	// Kid is the account URL, used as the "kid" JWS header field.
	Kid string `json:"kid"`

	// Contact is the mailto: contact list the account was registered with. It
	// is stored so a changed Config.Email can be noticed by a caller.
	Contact []string `json:"contact,omitempty"`

	// DirectoryURL records which CA the account belongs to. An account from the
	// staging CA is meaningless against production, so a mismatch means the
	// stored kid must not be reused.
	DirectoryURL string `json:"directoryURL,omitempty"`
}

// Account is the registered ACME account: the persisted key and the account URL.
type Account struct {
	// URL is the account URL, sent as the JWS "kid" on every request after
	// registration.
	URL string

	// Contact is the contact list the account carries at the CA.
	Contact []string

	// Key is the ES256 account key. It is never transmitted; only its public
	// half travels, as the "jwk" header on newAccount.
	Key *ecdsa.PrivateKey
}

// Thumbprint returns the account key's RFC 7638 thumbprint, the value that forms
// the second half of every key authorization.
func (a *Account) Thumbprint() (string, error) {
	if a == nil || a.Key == nil {
		return "", errors.New("acme: account has no key")
	}
	return thumbprint(&a.Key.PublicKey)
}

// generateAccountKey creates a fresh ES256 (ECDSA P-256) account key.
func generateAccountKey() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("acme: generate account key: %w", err)
	}
	return key, nil
}

// encodeECPrivateKeyPEM serializes an ECDSA private key as SEC 1 "EC PRIVATE KEY"
// PEM, the form both OpenSSL and Go read without ceremony.
func encodeECPrivateKeyPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("acme: marshal EC private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// decodeECPrivateKeyPEM parses a PEM private key. Both SEC 1 "EC PRIVATE KEY" and
// PKCS#8 "PRIVATE KEY" blocks are accepted, since an operator may have dropped in
// a key produced by another tool.
func decodeECPrivateKeyPEM(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("acme: no PEM block found in account key")
	}
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("acme: parse EC private key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("acme: parse PKCS#8 private key: %w", err)
		}
		key, ok := parsed.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("acme: account key is %T, want *ecdsa.PrivateKey", parsed)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("acme: unexpected PEM block %q in account key", block.Type)
	}
}

// writeFileSecret writes data atomically with mode 0600. The temporary file is
// created in the destination directory so the rename cannot cross a filesystem
// boundary, and it is removed if anything fails.
func writeFileSecret(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("acme: create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("acme: create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best effort cleanup; a successful rename makes this a no-op.
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return fmt.Errorf("acme: chmod %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("acme: write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("acme: sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("acme: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("acme: rename %s to %s: %w", tmpName, path, err)
	}
	return nil
}

// loadOrCreateAccountKey returns the account key stored in dir, generating and
// persisting a new one at mode 0600 the first time. The key is generated exactly
// once per directory: an ACME account is bound to its key, so regenerating it
// would silently orphan the account and its authorizations.
func loadOrCreateAccountKey(dir string) (*ecdsa.PrivateKey, error) {
	path := filepath.Join(dir, accountKeyFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		key, perr := decodeECPrivateKeyPEM(data)
		if perr != nil {
			return nil, fmt.Errorf("acme: account key %s is unusable: %w", path, perr)
		}
		if _, cerr := curveName(&key.PublicKey); cerr != nil {
			return nil, fmt.Errorf("acme: account key %s: %w", path, cerr)
		}
		// Repair the mode if something left the key world readable.
		if info, serr := os.Stat(path); serr == nil && info.Mode().Perm() != filePerm {
			if cerr := os.Chmod(path, filePerm); cerr != nil {
				return nil, fmt.Errorf("acme: tighten permissions on %s: %w", path, cerr)
			}
		}
		return key, nil
	case errors.Is(err, os.ErrNotExist):
		key, gerr := generateAccountKey()
		if gerr != nil {
			return nil, gerr
		}
		pemBytes, eerr := encodeECPrivateKeyPEM(key)
		if eerr != nil {
			return nil, eerr
		}
		if werr := writeFileSecret(path, pemBytes); werr != nil {
			return nil, werr
		}
		return key, nil
	default:
		return nil, fmt.Errorf("acme: read account key %s: %w", path, err)
	}
}

// loadAccountMeta reads the stored account metadata. A missing file is not an
// error: it simply means the account has not been registered yet.
func loadAccountMeta(dir string) (*accountMeta, error) {
	path := filepath.Join(dir, accountMetaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("acme: read account metadata %s: %w", path, err)
	}
	var meta accountMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("acme: parse account metadata %s: %w", path, err)
	}
	return &meta, nil
}

// saveAccountMeta persists the account URL so later runs reuse the account
// instead of registering a new one.
func saveAccountMeta(dir string, meta *accountMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("acme: marshal account metadata: %w", err)
	}
	data = append(data, '\n')
	return writeFileSecret(filepath.Join(dir, accountMetaFile), data)
}

// newAccountRequest is the newAccount payload of RFC 8555 section 7.3.
type newAccountRequest struct {
	TermsOfServiceAgreed bool     `json:"termsOfServiceAgreed"`
	Contact              []string `json:"contact,omitempty"`
	OnlyReturnExisting   bool     `json:"onlyReturnExisting,omitempty"`
}

// accountResource is the account object the CA returns.
type accountResource struct {
	Status  string   `json:"status"`
	Contact []string `json:"contact,omitempty"`
	Orders  string   `json:"orders,omitempty"`
}

// contactList turns an email address into the ACME contact array. An empty email
// yields a nil list, which registers an account with no contact.
func contactList(email string) []string {
	if email == "" {
		return nil
	}
	return []string{"mailto:" + email}
}
