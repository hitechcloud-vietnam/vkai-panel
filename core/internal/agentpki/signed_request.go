package agentpki

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Requests that travel from the agent to the panel - renewal and status - are
// authenticated by a signature made with the private key of the certificate the
// agent already holds, not by mutual TLS.
//
// The reason is deployment, not cryptography. The panel's own HTTPS listener
// serves the browser UI with a public certificate and cannot demand a client
// certificate from every request without breaking that. A signature over the
// request achieves the same thing here: only the holder of the agent's private
// key can produce it, the panel checks it against the public key it issued, and
// the deny list applies exactly as it does on a handshake.
//
// The signed message is:
//
//	agent id \n serial \n timestamp \n nonce \n hex(sha256(body))
//
// A stale timestamp is refused outside a five minute window in either
// direction, and each nonce is accepted once inside that window, so a captured
// request cannot be replayed.
const (
	HeaderAgentID   = "X-VKAI-Agent-Id"
	HeaderSerial    = "X-VKAI-Agent-Serial"
	HeaderTimestamp = "X-VKAI-Timestamp"
	HeaderNonce     = "X-VKAI-Nonce"
	HeaderSignature = "X-VKAI-Signature"

	// maxSkew is how far a request timestamp may be from the panel's clock.
	maxSkew = 5 * time.Minute
)

// ErrBadSignature is returned for every failure of the signature itself. It is
// deliberately one error: a caller that cannot sign gets no help narrowing down
// why.
var ErrBadSignature = errors.New("agentpki: the request signature is not valid")

// SignedHeaders is the set of headers that authenticate an agent request.
type SignedHeaders struct {
	AgentID   string
	Serial    string
	Timestamp string
	Nonce     string
	Signature string
}

// SignedHeadersFrom reads the headers off an inbound request.
func SignedHeadersFrom(h http.Header) SignedHeaders {
	return SignedHeaders{
		AgentID:   h.Get(HeaderAgentID),
		Serial:    h.Get(HeaderSerial),
		Timestamp: h.Get(HeaderTimestamp),
		Nonce:     h.Get(HeaderNonce),
		Signature: h.Get(HeaderSignature),
	}
}

// Apply writes the headers onto an outbound request.
func (s SignedHeaders) Apply(h http.Header) {
	h.Set(HeaderAgentID, s.AgentID)
	h.Set(HeaderSerial, s.Serial)
	h.Set(HeaderTimestamp, s.Timestamp)
	h.Set(HeaderNonce, s.Nonce)
	h.Set(HeaderSignature, s.Signature)
}

// SigningMessage builds the exact bytes both sides sign. It is exported because
// the agent, which is a separate module, must produce the same bytes.
func SigningMessage(agentID, serial, timestamp, nonce string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		agentID,
		serial,
		timestamp,
		nonce,
		hex.EncodeToString(sum[:]),
	}, "\n"))
}

// SignRequest produces the headers for one request. This is the reference
// implementation; the agent carries its own copy of the same six lines.
func SignRequest(agentID, serial string, key crypto.Signer, now time.Time, body []byte) (SignedHeaders, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return SignedHeaders{}, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := now.UTC().Format(time.RFC3339Nano)
	digest := sha256.Sum256(SigningMessage(agentID, serial, timestamp, nonce, body))
	sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return SignedHeaders{}, err
	}
	return SignedHeaders{
		AgentID:   agentID,
		Serial:    serial,
		Timestamp: timestamp,
		Nonce:     nonce,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// VerifySignedRequest authenticates an agent request. It applies the same three
// rules as a handshake - known agent, not revoked, the certificate we issued
// (with the overlap window) - and then the signature.
func (a *Authority) VerifySignedRequest(ctx context.Context, hdr SignedHeaders, body []byte) (*AgentRecord, error) {
	if hdr.AgentID == "" || hdr.Serial == "" || hdr.Timestamp == "" || hdr.Nonce == "" || hdr.Signature == "" {
		return nil, ErrBadSignature
	}
	stamp, err := time.Parse(time.RFC3339Nano, hdr.Timestamp)
	if err != nil {
		return nil, ErrBadSignature
	}
	now := a.now()
	if stamp.Before(now.Add(-maxSkew)) || stamp.After(now.Add(maxSkew)) {
		return nil, fmt.Errorf("%w: the request timestamp is outside the accepted window", ErrBadSignature)
	}
	if !a.replay.accept(hdr.Nonce, now) {
		return nil, fmt.Errorf("%w: this request has already been seen", ErrBadSignature)
	}

	rec, err := a.checkIssued(ctx, hdr.AgentID, hdr.Serial, "", now)
	if err != nil {
		return nil, err
	}
	certRec, err := a.acceptSerial(rec, hdr.Serial, "", now)
	if err != nil {
		return nil, err
	}
	if now.After(certRec.NotAfter) {
		return nil, fmt.Errorf("%w: the certificate has expired", ErrWrongCertificate)
	}

	pub, err := x509.ParsePKIXPublicKey(certRec.PublicKeyDER)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(hdr.Signature)
	if err != nil {
		return nil, ErrBadSignature
	}
	digest := sha256.Sum256(SigningMessage(hdr.AgentID, hdr.Serial, hdr.Timestamp, hdr.Nonce, body))
	if err := verifySignature(pub, digest[:], sig); err != nil {
		return nil, err
	}
	return rec, nil
}

func verifySignature(pub any, digest, sig []byte) error {
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, digest, sig) {
			return ErrBadSignature
		}
		return nil
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest, sig); err != nil {
			return ErrBadSignature
		}
		return nil
	default:
		return ErrBadSignature
	}
}

// ============================================================
// REPLAY GUARD
// ============================================================

// replayGuard remembers the nonces seen inside the skew window. It is bounded
// by that window rather than by a count: nothing outside it can be replayed,
// because the timestamp check has already refused it.
type replayGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newReplayGuard() *replayGuard {
	return &replayGuard{seen: make(map[string]time.Time)}
}

// accept records a nonce and reports whether it is new.
func (g *replayGuard) accept(nonce string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.seen[nonce]; ok {
		return false
	}
	cutoff := now.Add(-maxSkew * 2)
	for key, at := range g.seen {
		if at.Before(cutoff) {
			delete(g.seen, key)
		}
	}
	g.seen[nonce] = now
	return true
}
