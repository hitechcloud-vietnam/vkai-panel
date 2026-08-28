package backup

// AWS Signature Version 4, implemented against the published algorithm.
//
// WHY THIS IS HAND WRITTEN
//
// The panel builds with the standard library and what is already in go.mod. The
// AWS SDK is not there and adding it costs upwards of a hundred modules for the
// four HTTP verbs an object store needs. SigV4 is a documented, deterministic,
// keyed-hash construction with no negotiation and no state: given the same
// request and the same clock it produces the same string, which means it can be
// pinned to a published test vector and stay pinned.
//
// WHAT IS AND IS NOT PROVEN
//
// sigv4_test.go checks this implementation against the worked example in the
// AWS documentation - the GET Object request with the example credentials -
// down to the canonical request, the string to sign and the final signature.
// That pins the algorithm. s3_test.go then drives the whole client against a
// server that reconstructs the canonical request from the bytes it actually
// received and recomputes the signature, which pins the client to what it
// signs: a header signed but not sent, a query parameter dropped, a path
// encoded differently on the two sides all fail there.
//
// What neither proves is interoperability with a real S3 implementation. That
// needs a real endpoint and is stated as an open item rather than implied.
//
// SCOPE
//
// Header-based signing of a request whose payload hash is known before the
// request is sent. Streaming chunked signing (STREAMING-AWS4-HMAC-SHA256-
// PAYLOAD) is not implemented: the client spools an archive it cannot seek so
// it can hash it, which costs a temporary file and removes a whole category of
// subtle signing bugs. Presigned URLs are not implemented because nothing here
// needs one.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	sigV4Algorithm = "AWS4-HMAC-SHA256"
	// emptyPayloadSHA256 is sha256 of zero bytes, the payload hash of every
	// GET, HEAD and DELETE this client sends.
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	amzDateFormat  = "20060102T150405Z"
	credDateFormat = "20060102"
)

// sigV4Credentials is one set of static credentials.
type sigV4Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// signV4 signs req in place: it sets the x-amz-date, x-amz-content-sha256 and
// Authorization headers (and x-amz-security-token when the credentials carry
// one).
//
// It returns the canonical request and the string to sign. Production ignores
// both; the tests do not, and returning them is what makes it possible to check
// the intermediate values against the documented example instead of only
// checking the final hex string.
func signV4(req *http.Request, payloadSHA256 string, creds sigV4Credentials, region, service string, signTime time.Time) (canonicalRequest, stringToSign string) {
	signTime = signTime.UTC()
	amzDate := signTime.Format(amzDateFormat)
	dateStamp := signTime.Format(credDateFormat)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadSHA256)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	// The signed header set is built from the request as it stands, plus host,
	// which never appears in Header. Signing a header the request does not
	// carry, or carrying one that was not signed, is the commonest way to get
	// a SignatureDoesNotMatch that looks like a key problem.
	headers := map[string]string{"host": strings.TrimSpace(host)}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		headers[lower] = canonicalHeaderValue(strings.Join(values, ","))
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headers[name])
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest = strings.Join([]string{
		req.Method,
		canonicalURI(req.URL.EscapedPath()),
		canonicalQuery(req.URL.RawQuery),
		canonicalHeaders.String(),
		signedHeaders,
		payloadSHA256,
	}, "\n")

	scope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign = strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigV4Algorithm, creds.AccessKeyID, scope, signedHeaders, signature))

	return canonicalRequest, stringToSign
}

// canonicalHeaderValue collapses runs of spaces and trims, as the algorithm
// requires. Values inside quotes are meant to be left alone; this client never
// sends a quoted header, so the simple rule is the correct one here.
func canonicalHeaderValue(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// canonicalURI takes the already-escaped path from the URL.
//
// S3 is the one service that does NOT normalise the path before signing and
// does NOT double-encode it, which is why the path is taken as the URL carries
// it rather than being re-encoded here. The client builds that path with
// uriEncode(segment, false) per segment, so the two agree by construction.
func canonicalURI(escapedPath string) string {
	if escapedPath == "" {
		return "/"
	}
	return escapedPath
}

// canonicalQuery re-encodes and sorts the query string. It parses the raw
// query by hand rather than through url.Values because Values loses the
// distinction between "a=" and "a", and because the ordering rule is over the
// encoded forms, not the decoded ones.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type kv struct{ k, v string }
	var pairs []kv
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		key, value, _ := strings.Cut(part, "=")
		pairs = append(pairs, kv{k: uriEncode(unescapeQueryComponent(key), true), v: uriEncode(unescapeQueryComponent(value), true)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})

	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.k+"="+p.v)
	}
	return strings.Join(out, "&")
}

// unescapeQueryComponent decodes %XX and '+' the way a query component is
// decoded, so that a component can be re-encoded under the stricter SigV4
// rules. A malformed escape is left as written: refusing here would turn a
// signing helper into a validator, and the request will fail at the endpoint
// anyway.
func unescapeQueryComponent(s string) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '+':
			b.WriteByte(' ')
		case s[i] == '%' && i+2 < len(s):
			hi, ok1 := fromHex(s[i+1])
			lo, ok2 := fromHex(s[i+2])
			if !ok1 || !ok2 {
				b.WriteByte(s[i])
				continue
			}
			b.WriteByte(hi<<4 | lo)
			i += 2
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func fromHex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// uriEncode is the encoding the algorithm specifies: unreserved characters
// pass through, everything else becomes uppercase percent escapes. It is not
// url.QueryEscape, which turns a space into '+' and leaves some sub-delimiters
// alone - both of which produce a signature the endpoint will not agree with.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteByte(c)
		case c == '/':
			if encodeSlash {
				b.WriteString("%2F")
			} else {
				b.WriteByte('/')
			}
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
