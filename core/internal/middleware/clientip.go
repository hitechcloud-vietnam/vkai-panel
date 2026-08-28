package middleware

// Source address resolution for the credential guard.
//
// gin's c.ClientIP() is not usable here. SetTrustedProxies is never called on
// this engine, so gin trusts every peer and honours whatever X-Forwarded-For
// the client sends. That is harmless for a log line and fatal for a rate
// limiter: an attacker would put a fresh random address in the header on every
// request and the per-address dimension would count to one, forever.
//
// So the guard resolves the address itself. The immediate peer is the only
// thing the network guarantees; a forwarded header is believed only when the
// peer is a proxy the operator has named. The panel's shipped nginx sits on
// loopback in front of the API, which is why loopback is the default trust
// list and why the default is not "any private address": a container network
// or a hostile neighbour on the same LAN is not a proxy.

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// EnvTrustedProxies names the addresses or CIDR blocks whose forwarded headers
// are believed, comma separated. Unset means loopback only.
const EnvTrustedProxies = "VKAI_TRUSTED_PROXIES"

var defaultTrustedProxies = []string{"127.0.0.0/8", "::1/128"}

// ClientIPResolver turns a request into the source address the limiter counts.
type ClientIPResolver struct {
	trusted []*net.IPNet
}

// NewClientIPResolver builds a resolver from a list of addresses or CIDR
// blocks. Entries that do not parse are dropped: a typo must not silently
// widen the trust list to everything.
func NewClientIPResolver(proxies []string) *ClientIPResolver {
	r := &ClientIPResolver{}
	for _, raw := range proxies {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(raw); err == nil {
			r.trusted = append(r.trusted, network)
			continue
		}
		if ip := net.ParseIP(raw); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			r.trusted = append(r.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return r
}

var (
	defaultResolverOnce sync.Once
	defaultResolver     *ClientIPResolver
)

// DefaultClientIPResolver is the process-wide resolver, built from
// VKAI_TRUSTED_PROXIES on first use.
func DefaultClientIPResolver() *ClientIPResolver {
	defaultResolverOnce.Do(func() {
		list := defaultTrustedProxies
		if raw := strings.TrimSpace(os.Getenv(EnvTrustedProxies)); raw != "" {
			list = strings.Split(raw, ",")
		}
		defaultResolver = NewClientIPResolver(list)
	})
	return defaultResolver
}

// Trusted reports whether an address is a configured proxy.
func (r *ClientIPResolver) Trusted(address string) bool {
	ip := net.ParseIP(strings.Trim(strings.TrimSpace(address), "[]"))
	if ip == nil {
		return false
	}
	for _, network := range r.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Resolve returns the source address to attribute the request to.
//
// It walks X-Forwarded-For from the right - the end the nearest proxy appends
// to - and returns the first entry that is not itself a trusted proxy. Entries
// to the left of that one were written by somebody outside the trusted chain
// and are therefore attacker-controlled, so they are never used.
func (r *ClientIPResolver) Resolve(req *http.Request) string {
	if req == nil {
		return ""
	}

	peer := hostOf(req.RemoteAddr)
	if !r.Trusted(peer) {
		return peer
	}

	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.Trim(strings.TrimSpace(parts[i]), "[]")
			if candidate == "" {
				continue
			}
			if net.ParseIP(candidate) == nil {
				// A forged, unparseable entry ends the walk: continuing past
				// it would let the client choose which entry we believe.
				break
			}
			if r.Trusted(candidate) {
				continue
			}
			return candidate
		}
	}

	if real := strings.Trim(strings.TrimSpace(req.Header.Get("X-Real-IP")), "[]"); real != "" {
		if net.ParseIP(real) != nil {
			return real
		}
	}

	return peer
}

// AuthClientIP is the address the credential guard counts against.
func AuthClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return DefaultClientIPResolver().Resolve(c.Request)
}

func hostOf(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(remoteAddr, "[]")
}
