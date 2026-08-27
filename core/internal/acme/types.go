package acme

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Directory URLs for the two Let's Encrypt endpoints the panel uses. Staging is
// selectable through Config.Staging or by setting Config.DirectoryURL directly.
const (
	// LetsEncryptProduction is the live ACME directory. Certificates issued from
	// it are publicly trusted and count against the production rate limits.
	LetsEncryptProduction = "https://acme-v02.api.letsencrypt.org/directory"

	// LetsEncryptStaging is the staging ACME directory. Certificates issued from
	// it are not publicly trusted, which makes it the right target for testing
	// an installation without burning production rate limits.
	LetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Certificate profiles advertised by Let's Encrypt in the directory's
// meta.profiles member. Always confirm against Directory.Meta.Profiles at run
// time rather than trusting these constants: the set is CA policy and changes
// without a protocol revision.
const (
	// ProfileClassic is the historical 90 day profile.
	ProfileClassic = "classic"

	// ProfileShortLived issues certificates valid for roughly six days. It is
	// the only profile under which Let's Encrypt issues certificates for IP
	// address identifiers.
	ProfileShortLived = "shortlived"

	// ProfileTLSServer is the reduced-extension profile for TLS server
	// certificates.
	ProfileTLSServer = "tlsserver"
)

// Identifier types defined by RFC 8555 and RFC 8738.
const (
	// IdentifierDNS is the "dns" identifier type of RFC 8555.
	IdentifierDNS = "dns"

	// IdentifierIP is the "ip" identifier type of RFC 8738.
	IdentifierIP = "ip"
)

// ACME error URNs this package reacts to. The full set is open ended, so these
// are matched by exact string and everything else is surfaced verbatim.
const (
	errBadNonce    = "urn:ietf:params:acme:error:badNonce"
	errRateLimited = "urn:ietf:params:acme:error:rateLimited"
)

// Order and authorization status values used by RFC 8555.
const (
	statusPending     = "pending"
	statusReady       = "ready"
	statusProcessing  = "processing"
	statusValid       = "valid"
	statusInvalid     = "invalid"
	statusDeactivated = "deactivated"
	statusExpired     = "expired"
	statusRevoked     = "revoked"
)

// Identifier is a single name an order covers. Type is either IdentifierDNS or
// IdentifierIP and Value is the domain name or the textual IP address.
type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// DNSIdentifier returns a "dns" identifier for the given host name.
func DNSIdentifier(name string) Identifier {
	return Identifier{Type: IdentifierDNS, Value: name}
}

// IPIdentifier returns an "ip" identifier for the given textual IP address.
func IPIdentifier(addr string) Identifier {
	return Identifier{Type: IdentifierIP, Value: addr}
}

// String renders the identifier the way it appears in a problem document, for
// example `dns:panel.example.com`.
func (i Identifier) String() string { return i.Type + ":" + i.Value }

// Profiles is the meta.profiles member of the directory. Let's Encrypt sends an
// object mapping a profile name to a human readable description, but the draft
// also allows a bare array of names, so both shapes are accepted and a profile
// without a description simply carries an empty string.
type Profiles map[string]string

// UnmarshalJSON accepts either {"name":"description"} or ["name", ...].
func (p *Profiles) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*p = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var names []string
		if err := json.Unmarshal(data, &names); err != nil {
			return err
		}
		out := make(Profiles, len(names))
		for _, n := range names {
			out[n] = ""
		}
		*p = out
		return nil
	}
	var obj map[string]string
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*p = Profiles(obj)
	return nil
}

// Names returns the advertised profile names in a stable, sorted order.
func (p Profiles) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has reports whether the CA advertises the named profile.
func (p Profiles) Has(name string) bool {
	_, ok := p[name]
	return ok
}

// DirectoryMeta is the "meta" member of an ACME directory.
type DirectoryMeta struct {
	TermsOfService          string   `json:"termsOfService,omitempty"`
	Website                 string   `json:"website,omitempty"`
	CAAIdentities           []string `json:"caaIdentities,omitempty"`
	ExternalAccountRequired bool     `json:"externalAccountRequired,omitempty"`
	Profiles                Profiles `json:"profiles,omitempty"`
}

// Directory is the resource returned by the ACME directory URL. It tells the
// client where every other endpoint lives and, through Meta.Profiles, which
// certificate profiles the CA is willing to issue under.
type Directory struct {
	NewNonce    string        `json:"newNonce"`
	NewAccount  string        `json:"newAccount"`
	NewOrder    string        `json:"newOrder"`
	NewAuthz    string        `json:"newAuthz,omitempty"`
	RevokeCert  string        `json:"revokeCert,omitempty"`
	KeyChange   string        `json:"keyChange,omitempty"`
	RenewalInfo string        `json:"renewalInfo,omitempty"`
	Meta        DirectoryMeta `json:"meta"`
}

// Order is an ACME order resource. URL is filled in by the client from the
// Location header and is not part of the wire format.
type Order struct {
	URL            string       `json:"-"`
	Status         string       `json:"status"`
	Expires        string       `json:"expires,omitempty"`
	Identifiers    []Identifier `json:"identifiers"`
	Authorizations []string     `json:"authorizations"`
	Finalize       string       `json:"finalize"`
	Certificate    string       `json:"certificate,omitempty"`
	Profile        string       `json:"profile,omitempty"`
	Error          *Problem     `json:"error,omitempty"`
}

// Authorization is an ACME authorization resource: one identifier and the set of
// challenges that would prove control over it.
type Authorization struct {
	Status     string      `json:"status"`
	Expires    string      `json:"expires,omitempty"`
	Identifier Identifier  `json:"identifier"`
	Challenges []Challenge `json:"challenges"`
	Wildcard   bool        `json:"wildcard,omitempty"`
}

// Challenge is a single challenge inside an authorization.
type Challenge struct {
	Type      string   `json:"type"`
	URL       string   `json:"url"`
	Status    string   `json:"status"`
	Token     string   `json:"token"`
	Validated string   `json:"validated,omitempty"`
	Error     *Problem `json:"error,omitempty"`
}

// ChallengeHTTP01 is the only challenge type this package implements. For an IP
// identifier the alternatives are TLS-ALPN-01, which would need port 443, and
// DNS-01, which RFC 8738 does not define for IP identifiers at all.
const ChallengeHTTP01 = "http-01"

// Subproblem is one entry of a problem document's "subproblems" array. The CA
// uses it to attribute a failure to a specific identifier when an order covers
// several.
type Subproblem struct {
	Type       string      `json:"type"`
	Detail     string      `json:"detail,omitempty"`
	Identifier *Identifier `json:"identifier,omitempty"`
}

// Problem is an RFC 7807 problem document as used by RFC 8555. It is returned as
// an error so a failed issuance can be diagnosed from a single log line: Error
// renders the type, the detail and every subproblem verbatim.
type Problem struct {
	Type        string       `json:"type,omitempty"`
	Title       string       `json:"title,omitempty"`
	Status      int          `json:"status,omitempty"`
	Detail      string       `json:"detail,omitempty"`
	Instance    string       `json:"instance,omitempty"`
	Subproblems []Subproblem `json:"subproblems,omitempty"`
}

// Error renders the problem document verbatim, including subproblems.
func (p *Problem) Error() string {
	if p == nil {
		return "acme: <nil problem>"
	}
	var b strings.Builder
	b.WriteString("acme: ")
	if p.Type != "" {
		b.WriteString(p.Type)
	} else {
		b.WriteString("unknown error")
	}
	if p.Status != 0 {
		b.WriteString(" (status ")
		b.WriteString(strconv.Itoa(p.Status))
		b.WriteString(")")
	}
	if p.Detail != "" {
		b.WriteString(": ")
		b.WriteString(p.Detail)
	} else if p.Title != "" {
		b.WriteString(": ")
		b.WriteString(p.Title)
	}
	if p.Instance != "" {
		b.WriteString(" [instance ")
		b.WriteString(p.Instance)
		b.WriteString("]")
	}
	for _, sub := range p.Subproblems {
		b.WriteString("; subproblem ")
		if sub.Identifier != nil {
			b.WriteString(sub.Identifier.String())
			b.WriteString(" ")
		}
		if sub.Type != "" {
			b.WriteString(sub.Type)
		} else {
			b.WriteString("unknown error")
		}
		if sub.Detail != "" {
			b.WriteString(": ")
			b.WriteString(sub.Detail)
		}
	}
	return b.String()
}

// Is lets errors.Is match two problems by their URN type, so a caller can write
// errors.Is(err, &acme.Problem{Type: "urn:ietf:params:acme:error:unauthorized"}).
func (p *Problem) Is(target error) bool {
	other, ok := target.(*Problem)
	if !ok || p == nil || other == nil {
		return false
	}
	return other.Type != "" && other.Type == p.Type
}

// IsRateLimited reports whether the problem is the rate limit URN.
func (p *Problem) IsRateLimited() bool {
	return p != nil && p.Type == errRateLimited
}

// RateLimitError is returned when the CA answers with HTTP 429 or with the
// rateLimited problem type. RetryAfter carries the parsed Retry-After header so
// the caller can back off for exactly as long as the CA asked instead of
// hammering it; it is zero when the CA sent no usable header.
type RateLimitError struct {
	// Problem is the CA's problem document, which usually names the specific
	// rate limit that was hit.
	Problem *Problem

	// RetryAfter is the wait the CA requested. Zero when unknown.
	RetryAfter time.Duration

	// RetryAfterHeader is the raw Retry-After header value, kept verbatim
	// because it may be an HTTP date rather than a number of seconds.
	RetryAfterHeader string

	// StatusCode is the HTTP status that carried the error.
	StatusCode int

	// URL is the endpoint that rejected the request.
	URL string
}

// Error implements error.
func (e *RateLimitError) Error() string {
	detail := "acme: rate limited"
	if e.Problem != nil {
		detail = e.Problem.Error()
	}
	msg := fmt.Sprintf("%s (HTTP %d)", detail, e.StatusCode)
	if e.URL != "" {
		msg += " from " + e.URL
	}
	if e.RetryAfter > 0 {
		msg += fmt.Sprintf("; retry after %s", e.RetryAfter)
	} else if e.RetryAfterHeader != "" {
		msg += "; retry after " + e.RetryAfterHeader
	}
	return msg
}

// Unwrap exposes the underlying problem document to errors.As and errors.Is.
func (e *RateLimitError) Unwrap() error {
	if e.Problem == nil {
		return nil
	}
	return e.Problem
}

// parseRetryAfter converts a Retry-After header into a duration. It understands
// both forms allowed by RFC 9110: a number of seconds and an HTTP date.
func parseRetryAfter(header http.Header, now time.Time) (time.Duration, string) {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0, ""
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, raw
		}
		return time.Duration(secs) * time.Second, raw
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := when.Sub(now); d > 0 {
			return d, raw
		}
		return 0, raw
	}
	return 0, raw
}
