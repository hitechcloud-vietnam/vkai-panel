// Package audit holds the tamper-evident format of the panel's audit log: the
// canonical byte encoding of an entry, the two hashes computed over it, and the
// verification of an exported chain.
//
// Nothing in this package talks to a database. That is deliberate. The same
// definitions have to hold in three places - here, in PL/pgSQL inside
// migrations/pending/audit_chain.sql, and in the Python reference verifier an
// outside auditor runs - and the only way to keep three implementations
// agreeing is for the definition to be small enough to state exactly.
//
// # The format
//
// Every hashed field is framed as F(s): an 8-byte big-endian unsigned length of
// the UTF-8 encoding of s, followed by those bytes. Framing every field, rather
// than joining them with a separator, is what stops a value that contains the
// separator from impersonating a field boundary.
//
//	content_hash = hex(SHA-256(
//	    F("vkai-audit-content-v1") ||
//	    F(id) || F(tenant_id) || F(user_id) || F(action) || F(resource) ||
//	    F(resource_id) || F(details) || F(ip_address) || F(user_agent) ||
//	    F(status) || F(created_at)))
//
//	entry_hash = hex(SHA-256(
//	    F("vkai-audit-chain-v1") || F(prev_hash) || F(tenant_id) ||
//	    F(seq) || F(content_hash)))
//
// UUIDs are lowercase and hyphenated, and empty when the column is NULL.
// details is PostgreSQL's own jsonb rendering of the column - keys sorted, ": "
// and ", " separators - obtained by the writer with RETURNING details::text and
// never re-serialised by anything on either side of the wire, so no JSON
// canonicalisation disagreement is possible. created_at is UTC with exactly six
// fractional digits and a trailing Z. seq is decimal ASCII and starts at 1.
// prev_hash is the entry_hash of seq-1, or 64 '0' characters for the first
// entry in a chain.
//
// # Why two levels
//
// entry_hash depends on nothing but the previous hash, the tenant, the sequence
// number and content_hash, so the chain can be walked without reading the audit
// table at all - that is what makes verification affordable on a large log.
// content_hash binds the row's contents and carries no ordering dependency, so
// any subrange of the expensive half can be checked on its own.
//
// # Changing this
//
// The domain strings are the format version. Changing what is hashed, or the
// order, invalidates every chain ever written under the old definition. If it
// ever has to change, the new domain string goes alongside the old one and
// entries keep the version they were written under - it is not a migration.
package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	// ContentDomain and ChainDomain are the format version. See the note above
	// about changing them.
	ContentDomain = "vkai-audit-content-v1"
	ChainDomain   = "vkai-audit-chain-v1"

	// ExportFormat names the bundle an outside auditor is handed.
	ExportFormat = "vkai-audit-export-v1"

	// HashAlgorithm is stated in every export so a verifier never has to guess.
	HashAlgorithm = "SHA-256"

	// TimeLayout is the rendering of created_at that goes into the hash: UTC,
	// six fractional digits (PostgreSQL timestamps hold microseconds, so six is
	// exact rather than rounded), trailing Z.
	TimeLayout = "2006-01-02T15:04:05.000000Z"

	// HashHexLen is the length of a hash as it appears everywhere in this
	// format: lowercase hex of 32 bytes.
	HashHexLen = 64
)

// GenesisHash is the prev_hash of the first entry in a chain.
var GenesisHash = strings.Repeat("0", HashHexLen)

// FormatTime renders a timestamp the way the hash sees it.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

// appendField writes F(s) - see the package comment.
func appendField(dst []byte, s string) []byte {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(s)))
	dst = append(dst, length[:]...)
	return append(dst, s...)
}

// Content is the hashed projection of one audit_logs row. Every field is
// already a string in exactly the rendering that is hashed, because that is
// also the rendering that goes into an export: an auditor takes the strings out
// of the file verbatim and never has to reproduce a type conversion.
type Content struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	UserID     string `json:"user_id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Details    string `json:"details"`
	IPAddress  string `json:"ip_address"`
	UserAgent  string `json:"user_agent"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// CanonicalBytes is the exact byte string content_hash is taken over.
func (c Content) CanonicalBytes() []byte {
	// 12 fields, 8 bytes of framing each, plus the payloads. One allocation.
	size := 12 * 8
	for _, s := range [...]string{
		ContentDomain, c.ID, c.TenantID, c.UserID, c.Action, c.Resource,
		c.ResourceID, c.Details, c.IPAddress, c.UserAgent, c.Status, c.CreatedAt,
	} {
		size += len(s)
	}

	buf := make([]byte, 0, size)
	buf = appendField(buf, ContentDomain)
	buf = appendField(buf, c.ID)
	buf = appendField(buf, c.TenantID)
	buf = appendField(buf, c.UserID)
	buf = appendField(buf, c.Action)
	buf = appendField(buf, c.Resource)
	buf = appendField(buf, c.ResourceID)
	buf = appendField(buf, c.Details)
	buf = appendField(buf, c.IPAddress)
	buf = appendField(buf, c.UserAgent)
	buf = appendField(buf, c.Status)
	buf = appendField(buf, c.CreatedAt)
	return buf
}

// ContentHash is the lowercase hex SHA-256 of CanonicalBytes.
func (c Content) ContentHash() string {
	sum := sha256.Sum256(c.CanonicalBytes())
	return hex.EncodeToString(sum[:])
}

// EntryCanonicalBytes is the exact byte string entry_hash is taken over.
func EntryCanonicalBytes(prevHash, tenantID string, seq int64, contentHash string) []byte {
	seqText := strconv.FormatInt(seq, 10)

	size := 5 * 8
	for _, s := range [...]string{ChainDomain, prevHash, tenantID, seqText, contentHash} {
		size += len(s)
	}

	buf := make([]byte, 0, size)
	buf = appendField(buf, ChainDomain)
	buf = appendField(buf, prevHash)
	buf = appendField(buf, tenantID)
	buf = appendField(buf, seqText)
	buf = appendField(buf, contentHash)
	return buf
}

// EntryHash links one entry to the one before it.
func EntryHash(prevHash, tenantID string, seq int64, contentHash string) string {
	sum := sha256.Sum256(EntryCanonicalBytes(prevHash, tenantID, seq, contentHash))
	return hex.EncodeToString(sum[:])
}
