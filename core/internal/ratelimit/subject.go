package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
)

// Subject is the (address, account) pair an attempt is attributed to. Both
// halves are normalised on construction, because a limiter that can be evaded
// by changing the case of a username or by moving one address along inside a
// block the attacker already owns is decorative.
type Subject struct {
	// Address is the normalised source: an IPv4 address as-is, an IPv6
	// address collapsed to its /64. Anything unparseable is kept verbatim so
	// it still buckets consistently.
	Address string

	// Account is the lower-cased, trimmed account identifier as the caller
	// supplied it. It is only used to derive keys, never logged from here.
	Account string
}

// NewSubject normalises an address and an account into a Subject.
func NewSubject(address, account string) Subject {
	return Subject{
		Address: NormalizeAddress(address),
		Account: NormalizeAccount(account),
	}
}

// NormalizeAddress buckets a source address.
//
// An IPv6 attacker is routinely handed a /64 - eighteen quintillion addresses -
// so counting single IPv6 addresses is the same as not counting at all. IPv4 is
// counted whole: /24 aggregation there would sweep up unrelated customers
// behind the same provider.
func NormalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "unknown"
	}

	// Tolerate a host:port that slipped through.
	if host, _, err := net.SplitHostPort(address); err == nil && host != "" {
		address = host
	}
	address = strings.Trim(address, "[]")

	ip := net.ParseIP(address)
	if ip == nil {
		return strings.ToLower(address)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}

	masked := ip.Mask(net.CIDRMask(64, 128))
	if masked == nil {
		return ip.String()
	}
	return masked.String() + "/64"
}

// NormalizeAccount folds an account identifier to a single canonical form.
// Without this, "Admin", "admin" and " admin " would be three separate
// budgets and an attacker would simply cycle the spelling.
func NormalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

// accountKey hashes the account so plaintext usernames and email addresses do
// not sit in Redis keys, where they show up in SLOWLOG, in MONITOR output and
// in any backup of the instance. This is hygiene, not a security boundary: the
// hash is unsalted so that every panel instance derives the same key.
func accountKey(account string) string {
	if account == "" {
		return "anonymous"
	}
	sum := sha256.Sum256([]byte(account))
	return hex.EncodeToString(sum[:16])
}
