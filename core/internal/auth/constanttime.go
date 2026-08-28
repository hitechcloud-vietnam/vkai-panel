package auth

// Constant-time primitives for credential comparison.
//
// A credential check that returns as soon as two bytes differ leaks how much
// of the secret the caller got right. Over enough requests that turns a
// comparison into an oracle that hands the secret over one byte at a time. It
// matters most for the credentials that are compared as opaque strings - API
// keys, agent tokens, one-time codes - because unlike a password those are not
// already going through a slow hash that destroys the timing signal.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"

	"golang.org/x/crypto/bcrypt"
)

// SecureCompare reports whether two strings are equal, in time that does not
// depend on where they first differ.
//
// It hashes both sides before comparing, so the comparison is also independent
// of their lengths - a plain subtle.ConstantTimeCompare returns early when the
// lengths differ and therefore leaks the length of the secret.
func SecureCompare(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// SecureCompareBytes is SecureCompare for byte slices.
func SecureCompareBytes(a, b []byte) bool {
	ha := sha256.Sum256(a)
	hb := sha256.Sum256(b)
	return hmac.Equal(ha[:], hb[:])
}

// decoyHash is a real bcrypt digest of a value nobody holds. It is a package
// level constant so that generating it never shows up in a request's timing.
//
// It was produced with bcrypt.DefaultCost, the same cost utils.HashPassword
// uses, so verifying against it takes the same time as verifying against a
// genuine stored hash.
const decoyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// BurnPasswordTime performs the same work a password verification would,
// against a hash nobody can match, and always reports failure.
//
// It exists for the "no such user" branch of a login. Without it that branch
// returns in microseconds while the "wrong password" branch spends the cost of
// a bcrypt verification, and the difference is a reliable answer to "does this
// account exist?" - which is the first half of every credential attack.
//
// The middleware guard also pads every failed authentication to a fixed floor,
// which covers this from the outside. The two are complementary: the floor
// holds as long as the real path stays under it, and this keeps the two paths
// equal even when it does not.
func BurnPasswordTime() bool {
	_ = bcrypt.CompareHashAndPassword([]byte(decoyHash), []byte("vkai-panel-no-such-account"))
	return false
}
