package auth

// Binding a session to where and what it was issued to.
//
// A panel session is a bearer token. Whoever holds it is the operator, from
// anywhere, forever - which is why a token copied out of a laptop, a log
// aggregator or a browser extension is worth as much as the password that
// produced it. Binding narrows that: the token is only worth something when it
// is presented from the same place, by the same thing, that it was issued to.
//
// The obvious implementation of that idea makes the product unusable, and it
// is worth being precise about why. A session pinned to an exact IP address
// dies every time a phone moves between wifi and the mobile network, every time
// a carrier-grade NAT pool rotates, every time a corporate egress cluster
// rebalances - none of which the user can see, all of which look to them like
// the panel logging them out at random. Support then turns the feature off, and
// the panel ends up with a security control that exists in the settings page
// and nowhere else.
//
// So the two dimensions are treated differently, because they behave
// differently:
//
//   - The DEVICE is stable within a session and is enforced hard. A session
//     issued to a browser and then presented by curl, by a script, or by a
//     different browser is refused and the session is ended. The fingerprint is
//     taken from the User-Agent with version numbers removed, so a browser that
//     updates itself under the user does not read as a different device - that
//     being the one legitimate way a User-Agent changes mid-session.
//
//   - The NETWORK is not stable and is enforced by consequence, not by
//     refusal. The default policy compares networks rather than addresses (a
//     /24 for IPv4, a /48 for IPv6, which covers a NAT pool or an egress
//     cluster), and when the session does move outside its network it keeps
//     working for reads and stops working for writes until the operator proves
//     the password again. A moved session can still be watched; it cannot
//     delete a database.
//
// The cost of that choice, stated plainly: an attacker with a stolen token and
// a different address can read everything the account can read until the
// operator notices. The alternative - refusing the request outright - buys the
// difference between "read" and "nothing" at the price of ending a real
// session every time a real user changes network, which happens far more often
// than tokens are stolen. Operators who want that trade can have it:
// VKAI_SESSION_IP_BINDING=strict pins to the exact address and refuses.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EnvSessionIPBinding selects the network half of the policy.
const EnvSessionIPBinding = "VKAI_SESSION_IP_BINDING"

// EnvSessionDeviceBinding turns the device half off. It exists for an
// installation sitting behind something that rewrites User-Agent headers;
// nothing else should touch it.
const EnvSessionDeviceBinding = "VKAI_SESSION_DEVICE_BINDING"

// Network binding modes.
const (
	// IPBindingOff ignores the source address entirely. Device binding still
	// applies.
	IPBindingOff = "off"
	// IPBindingWarn records and audits a move and never refuses anything.
	IPBindingWarn = "warn"
	// IPBindingNetwork is the default: a move within the session's /24 (or
	// /48 for IPv6) is nothing, a move outside it keeps reads and costs the
	// operator a password before the next write.
	IPBindingNetwork = "network"
	// IPBindingStrict pins the session to the exact address it was issued to
	// and refuses anything else.
	IPBindingStrict = "strict"
)

// BindingPolicy is the resolved configuration.
type BindingPolicy struct {
	// IPMode is one of the IPBinding* constants.
	IPMode string
	// DeviceBinding enforces the device fingerprint. On by default.
	DeviceBinding bool
}

// DefaultBindingPolicy is what an installation that configures nothing gets.
func DefaultBindingPolicy() BindingPolicy {
	return BindingPolicy{IPMode: IPBindingNetwork, DeviceBinding: true}
}

// BindingPolicyFromEnv resolves the policy from the environment. An
// unrecognised value falls back to the default rather than to "off": a typo
// must not disable the control.
func BindingPolicyFromEnv() BindingPolicy {
	policy := DefaultBindingPolicy()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvSessionIPBinding))) {
	case IPBindingOff:
		policy.IPMode = IPBindingOff
	case IPBindingWarn:
		policy.IPMode = IPBindingWarn
	case IPBindingNetwork:
		policy.IPMode = IPBindingNetwork
	case IPBindingStrict:
		policy.IPMode = IPBindingStrict
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvSessionDeviceBinding))) {
	case "0", "false", "no", "off":
		policy.DeviceBinding = false
	}

	return policy
}

// Verdict is what the policy decides about one request.
type Verdict int

const (
	// VerdictAllow: the request comes from where and what the session was
	// issued to.
	VerdictAllow Verdict = iota
	// VerdictAllowChanged: the origin moved, the policy tolerates it, and the
	// move is worth recording.
	VerdictAllowChanged
	// VerdictReauthenticate: the origin moved far enough that a write must
	// wait for the password. Reads are allowed.
	VerdictReauthenticate
	// VerdictRefuse: the request is not this session's. Refuse it and end the
	// session.
	VerdictRefuse
)

// Binding is what was recorded when the session was established.
type Binding struct {
	IP          string
	Network     string
	Fingerprint string
}

// Observation is what the current request looks like.
type Observation struct {
	IP          string
	Fingerprint string
	Method      string
}

// Decision is the policy's answer plus the reason, which goes into the audit
// entry and into the error the client sees.
type Decision struct {
	Verdict Verdict
	Reason  string
	// OriginMoved is true when the source address left the bound network (or,
	// in strict mode, changed at all). It is what increments the session's
	// move counter.
	OriginMoved bool
}

// Evaluate applies the policy to one request.
func (p BindingPolicy) Evaluate(bound Binding, seen Observation) Decision {
	// The device is checked first and hardest. A different device is never a
	// network artefact; it is a different program holding the token.
	if p.DeviceBinding && bound.Fingerprint != "" && seen.Fingerprint != bound.Fingerprint {
		return Decision{
			Verdict: VerdictRefuse,
			Reason:  "device_changed",
		}
	}

	if p.IPMode == IPBindingOff || seen.IP == "" || bound.IP == "" {
		return Decision{Verdict: VerdictAllow}
	}

	if seen.IP == bound.IP {
		return Decision{Verdict: VerdictAllow}
	}

	if p.IPMode == IPBindingStrict {
		return Decision{
			Verdict:     VerdictRefuse,
			Reason:      "address_changed",
			OriginMoved: true,
		}
	}

	// Same network is the common case for a real user: a NAT pool handing out
	// a neighbouring address, an egress cluster rebalancing.
	if bound.Network != "" && NetworkOf(seen.IP) == bound.Network {
		return Decision{Verdict: VerdictAllow}
	}

	if p.IPMode == IPBindingWarn {
		return Decision{
			Verdict:     VerdictAllowChanged,
			Reason:      "network_changed",
			OriginMoved: true,
		}
	}

	// IPBindingNetwork: reads continue, writes wait for a password.
	if ActionForMethod(seen.Method) == ActionRead {
		return Decision{
			Verdict:     VerdictAllowChanged,
			Reason:      "network_changed",
			OriginMoved: true,
		}
	}
	return Decision{
		Verdict:     VerdictReauthenticate,
		Reason:      "network_changed",
		OriginMoved: true,
	}
}

// versionNumbers matches the version-looking runs in a User-Agent.
//
// Stripping them is what keeps a browser's own auto-update from reading as a
// stolen token. "Chrome/126.0.6478.127" and "Chrome/127.0.6533.72" become the
// same string; "Chrome" and "curl" do not.
var versionNumbers = regexp.MustCompile(`\d+(\.\d+)*`)

// DeviceFingerprint reduces a User-Agent to a stable identifier for the client
// program, and hashes it. The hash is what is stored: the raw header is kept
// alongside it for the operator's own "which devices are signed in" screen, but
// the comparison is over the normalised form.
func DeviceFingerprint(userAgent string) string {
	normalised := versionNumbers.ReplaceAllString(strings.ToLower(strings.TrimSpace(userAgent)), "")
	normalised = strings.Join(strings.Fields(normalised), " ")
	sum := sha256.Sum256([]byte("vkai-panel/device/v1|" + normalised))
	return hex.EncodeToString(sum[:])
}

// NetworkOf returns the network a source address belongs to, in CIDR form: a
// /24 for IPv4 and a /48 for IPv6.
//
// Those sizes are chosen to cover the ways one client's address legitimately
// changes without covering "somewhere else entirely". A /24 is the granularity
// at which NAT pools and egress clusters hand out addresses; a /48 is the
// smallest block an IPv6 site is normally delegated.
func NetworkOf(address string) string {
	ip := net.ParseIP(strings.Trim(strings.TrimSpace(address), "[]"))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return (&net.IPNet{IP: v4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(48, 128)), Mask: net.CIDRMask(48, 128)}).String()
}

// AddressInCIDRs reports whether an address falls inside any of the given
// networks. A bare address is accepted as a single-host network, so an operator
// can write "203.0.113.7" as well as "203.0.113.0/24".
//
// An empty list means "no restriction". An entry that does not parse is
// ignored, which for an allow list means it grants nothing - a typo can only
// narrow.
func AddressInCIDRs(address string, cidrs []string) bool {
	if len(cidrs) == 0 {
		return true
	}
	ip := net.ParseIP(strings.Trim(strings.TrimSpace(address), "[]"))
	if ip == nil {
		return false
	}
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(raw); err == nil {
			if network.Contains(ip) {
				return true
			}
			continue
		}
		if single := net.ParseIP(raw); single != nil && single.Equal(ip) {
			return true
		}
	}
	return false
}

// ValidateCIDRs checks an operator-supplied allow list and returns it in
// canonical form. It exists so a typo is refused when the key is created
// rather than discovered when the integration stops working.
func ValidateCIDRs(cidrs []string) ([]string, error) {
	out := make([]string, 0, len(cidrs))
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(raw); err == nil {
			out = append(out, network.String())
			continue
		}
		if ip := net.ParseIP(raw); ip != nil {
			out = append(out, ip.String())
			continue
		}
		return nil, &InvalidCIDRError{Value: raw}
	}
	return out, nil
}

// InvalidCIDRError names the entry that did not parse.
type InvalidCIDRError struct{ Value string }

func (e *InvalidCIDRError) Error() string {
	return "not an IP address or CIDR block: " + e.Value
}

// ---------------------------------------------------------------------------
// The contract between the request path and whatever stores sessions
// ---------------------------------------------------------------------------
//
// These types live here rather than in the service that implements them so
// that the middleware enforcing the policy depends on this package and not on
// the data layer. The middleware asks a question and acts on the answer; it
// does not know there is a database behind it.

// SessionRequest describes one request whose access token has already been
// validated. Everything in it comes from the token or from the request itself.
type SessionRequest struct {
	// TokenID is the access token's jti. It identifies the session.
	TokenID string
	UserID  uuid.UUID
	// TenantID is the tenant the token was issued for.
	TenantID uuid.UUID
	// ExpiresAt is when the token stops being valid. The session row lives
	// exactly that long.
	ExpiresAt time.Time
	// IP is the resolved source address - the peer, or a forwarded address
	// from a proxy the operator has named. Never a header taken on trust.
	IP        string
	UserAgent string
	Method    string
	Path      string
}

// SessionVerdict is the answer.
type SessionVerdict struct {
	// SessionID is the session this request belongs to.
	SessionID uuid.UUID
	// Allow is whether the request may proceed.
	Allow bool
	// ReauthRequired distinguishes "this session is over" from "this session
	// needs the password again before it can change anything". The client is
	// told which, because the two need different things from the user.
	ReauthRequired bool
	// Reason is a short machine-readable cause: device_changed,
	// address_changed, network_changed, revoked, expired.
	Reason string
	// Established is true when this request created the session record.
	Established bool
}

// SessionEvaluator decides whether an authenticated request may proceed. The
// implementation is service.SessionService.
type SessionEvaluator interface {
	EvaluateSession(ctx context.Context, req SessionRequest) (SessionVerdict, error)
}
