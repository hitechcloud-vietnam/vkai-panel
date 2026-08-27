package acme

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// jsonWebKey is the public half of an ES256 account key in JWK form.
//
// The field order matters. RFC 7638 computes a key thumbprint over a JSON object
// whose members appear in lexicographic order with no whitespace, and for an EC
// key that order is exactly crv, kty, x, y. encoding/json emits struct fields in
// declaration order, so declaring them sorted makes MarshalJSON produce the
// canonical form directly. Do not reorder these fields.
type jsonWebKey struct {
	Crv string `json:"crv"`
	Kty string `json:"kty"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// base64url encodes without padding, as every ACME and JOSE field requires.
func base64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// keySize returns the byte length of one coordinate on the key's curve, which is
// also the length each half of a raw ECDSA signature must be padded to.
func keySize(key *ecdsa.PublicKey) int {
	return (key.Curve.Params().BitSize + 7) / 8
}

// publicJWK converts an ECDSA public key into its JWK representation. The
// coordinates are fixed-width big-endian integers, left-padded to the curve
// size; trimming leading zero bytes would produce a different thumbprint and
// therefore a key authorization the CA cannot verify.
func publicJWK(pub *ecdsa.PublicKey) (*jsonWebKey, error) {
	if pub == nil || pub.Curve == nil {
		return nil, errors.New("acme: nil ECDSA public key")
	}
	crv, err := curveName(pub)
	if err != nil {
		return nil, err
	}
	size := keySize(pub)
	x := make([]byte, size)
	y := make([]byte, size)
	pub.X.FillBytes(x)
	pub.Y.FillBytes(y)
	return &jsonWebKey{
		Crv: crv,
		Kty: "EC",
		X:   base64url(x),
		Y:   base64url(y),
	}, nil
}

// curveName maps a curve to its JOSE name. Only P-256 is supported because the
// package signs exclusively with ES256.
func curveName(pub *ecdsa.PublicKey) (string, error) {
	switch pub.Curve.Params().Name {
	case "P-256":
		return "P-256", nil
	default:
		return "", fmt.Errorf("acme: unsupported curve %q, only P-256 (ES256) is supported", pub.Curve.Params().Name)
	}
}

// thumbprint returns the RFC 7638 SHA-256 thumbprint of the account key,
// base64url encoded. This is the second half of a key authorization.
func thumbprint(pub *ecdsa.PublicKey) (string, error) {
	jwk, err := publicJWK(pub)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(jwk)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return base64url(sum[:]), nil
}

// keyAuthorization builds the key authorization of RFC 8555 section 8.1:
//
//	token || '.' || base64url(SHA256(canonical JWK))
//
// For an HTTP-01 challenge this exact string is what must be served at
// /.well-known/acme-challenge/<token>.
func keyAuthorization(token string, pub *ecdsa.PublicKey) (string, error) {
	tp, err := thumbprint(pub)
	if err != nil {
		return "", err
	}
	return token + "." + tp, nil
}

// protectedHeader is the JWS protected header of an ACME request. Exactly one of
// JWK and Kid is set: JWK on newAccount and on revocation by certificate key,
// Kid on every request made once the account URL is known.
type protectedHeader struct {
	Alg   string      `json:"alg"`
	JWK   *jsonWebKey `json:"jwk,omitempty"`
	Kid   string      `json:"kid,omitempty"`
	Nonce string      `json:"nonce"`
	URL   string      `json:"url"`
}

// jwsMessage is the flattened JSON serialization ACME requires.
type jwsMessage struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// signES256 signs data with ECDSA over SHA-256 and returns the JOSE signature
// form: the raw concatenation r || s, each value left-padded with zeros to the
// curve byte size, so the result is always exactly 2*keySize bytes.
//
// This is deliberately not crypto/ecdsa.SignASN1. An ASN.1 DER signature is a
// variable-length SEQUENCE of two INTEGERs and is what most Go code reaches for
// first, but JOSE (RFC 7518 section 3.4) requires the fixed-width raw form. A CA
// handed a DER signature rejects the request with a generic "malformed" error
// that says nothing about the real cause, which makes this the single most
// common defect in hand-written ACME clients.
func signES256(key *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("acme: nil signing key")
	}
	if _, err := curveName(&key.PublicKey); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return nil, fmt.Errorf("acme: ECDSA sign: %w", err)
	}
	size := keySize(&key.PublicKey)
	sig := make([]byte, 2*size)
	r.FillBytes(sig[:size])
	s.FillBytes(sig[size:])
	return sig, nil
}

// encodeJWS builds the flattened JWS an ACME server expects.
//
// payload is the already-serialized request body. It must be nil for a
// POST-as-GET request, which is encoded as an empty payload string rather than
// as the four bytes "null".
func encodeJWS(key *ecdsa.PrivateKey, header protectedHeader, payload []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("acme: nil signing key")
	}
	if (header.JWK == nil) == (header.Kid == "") {
		return nil, errors.New("acme: JWS protected header must carry exactly one of jwk and kid")
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("acme: marshal protected header: %w", err)
	}
	encodedHeader := base64url(headerJSON)

	encodedPayload := ""
	if payload != nil {
		encodedPayload = base64url(payload)
	}

	signingInput := encodedHeader + "." + encodedPayload
	signature, err := signES256(key, []byte(signingInput))
	if err != nil {
		return nil, err
	}
	return json.Marshal(jwsMessage{
		Protected: encodedHeader,
		Payload:   encodedPayload,
		Signature: base64url(signature),
	})
}
