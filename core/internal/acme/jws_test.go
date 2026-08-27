package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
)

// P-256 generator point, used as a fixed public key so the thumbprint below is a
// stable vector rather than something recomputed from a random key.
const (
	vectorX = "6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296"
	vectorY = "4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5"

	// vectorCanonicalJWK is the RFC 7638 canonical form: members in
	// lexicographic order, no whitespace.
	vectorCanonicalJWK = `{"crv":"P-256","kty":"EC","x":"axfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpY","y":"T-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU"}`

	// vectorThumbprint is base64url(SHA256(vectorCanonicalJWK)).
	vectorThumbprint = "xx0BcA-wMohw8atYDJOe6peGModklG2wRHBlXHMvl0M"

	// vectorToken is the challenge token from the RFC 8555 examples.
	vectorToken = "LoqXcYV8q5ONbJQxbmR7SCTNo3tiAXDfowyjxAjEuX0"

	// vectorKeyAuth is token || '.' || thumbprint, per RFC 8555 section 8.1.
	vectorKeyAuth = vectorToken + "." + vectorThumbprint
)

// vectorPublicKey builds the fixed P-256 public key used by the thumbprint
// vector.
func vectorPublicKey(t *testing.T) *ecdsa.PublicKey {
	t.Helper()
	x, ok := new(big.Int).SetString(vectorX, 16)
	if !ok {
		t.Fatal("could not parse the vector X coordinate")
	}
	y, ok := new(big.Int).SetString(vectorY, 16)
	if !ok {
		t.Fatal("could not parse the vector Y coordinate")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
}

func TestPublicJWKIsCanonical(t *testing.T) {
	jwk, err := publicJWK(vectorPublicKey(t))
	if err != nil {
		t.Fatalf("publicJWK: %v", err)
	}
	encoded, err := json.Marshal(jwk)
	if err != nil {
		t.Fatalf("marshal JWK: %v", err)
	}
	if string(encoded) != vectorCanonicalJWK {
		t.Fatalf("canonical JWK mismatch\n got: %s\nwant: %s", encoded, vectorCanonicalJWK)
	}
}

func TestThumbprintAndKeyAuthorizationVector(t *testing.T) {
	pub := vectorPublicKey(t)

	tp, err := thumbprint(pub)
	if err != nil {
		t.Fatalf("thumbprint: %v", err)
	}
	if tp != vectorThumbprint {
		t.Fatalf("thumbprint mismatch\n got: %s\nwant: %s", tp, vectorThumbprint)
	}

	// Recompute independently from the canonical form to prove the vector is
	// really SHA-256 over the canonical JWK and not a copied constant.
	sum := sha256.Sum256([]byte(vectorCanonicalJWK))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); want != tp {
		t.Fatalf("thumbprint is not SHA-256 of the canonical JWK: got %s, want %s", tp, want)
	}

	keyAuth, err := keyAuthorization(vectorToken, pub)
	if err != nil {
		t.Fatalf("keyAuthorization: %v", err)
	}
	if keyAuth != vectorKeyAuth {
		t.Fatalf("key authorization mismatch\n got: %s\nwant: %s", keyAuth, vectorKeyAuth)
	}
}

func TestJWKCoordinatesAreLeftPadded(t *testing.T) {
	// A coordinate that fits in fewer than 32 bytes must still be encoded as 32
	// bytes; trimming it would change the thumbprint and make every key
	// authorization unverifiable.
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     big.NewInt(1),
		Y:     big.NewInt(2),
	}
	jwk, err := publicJWK(pub)
	if err != nil {
		t.Fatalf("publicJWK: %v", err)
	}
	x, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		t.Fatalf("decode x: %v", err)
	}
	y, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		t.Fatalf("decode y: %v", err)
	}
	if len(x) != 32 || len(y) != 32 {
		t.Fatalf("coordinates are not left-padded to 32 bytes: len(x)=%d len(y)=%d", len(x), len(y))
	}
}

// TestSignES256IsRawConcatenationNotDER is the regression test for the single
// most common defect in hand-written ACME clients: signing with
// ecdsa.SignASN1 and shipping the DER SEQUENCE where JOSE requires the fixed
// width r || s form. A CA answers a DER signature with a bare "malformed", so
// nothing but a test catches this.
func TestSignES256IsRawConcatenationNotDER(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	const size = 32

	// Repeat: r or s occasionally has a leading zero byte, and only a padded
	// encoding keeps the length constant across those cases.
	for i := 0; i < 200; i++ {
		data := []byte("signing input")
		sig, err := signES256(key, data)
		if err != nil {
			t.Fatalf("signES256: %v", err)
		}
		if len(sig) != 2*size {
			t.Fatalf("signature is %d bytes, want exactly %d (2 * curve size)", len(sig), 2*size)
		}

		// Splitting the signature in half must yield r and s that verify. A DER
		// signature split the same way could not.
		digest := sha256.Sum256(data)
		r := new(big.Int).SetBytes(sig[:size])
		s := new(big.Int).SetBytes(sig[size:])
		if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
			t.Fatal("signature does not verify when split as raw r || s, so it is not the JOSE form")
		}
	}

	// And confirm the DER form really is different, so the test would fail if
	// signES256 were changed to SignASN1.
	digest := sha256.Sum256([]byte("signing input"))
	der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("SignASN1: %v", err)
	}
	var parsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &parsed); err != nil {
		t.Fatalf("SignASN1 output is not ASN.1, cannot compare: %v", err)
	}
	if len(der) == 2*size {
		t.Skip("DER signature happened to be 64 bytes; the raw-split check above already covers the behaviour")
	}
}

func TestEncodeJWSHeaderSelection(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwk, err := publicJWK(&key.PublicKey)
	if err != nil {
		t.Fatalf("publicJWK: %v", err)
	}

	t.Run("jwk carries no kid", func(t *testing.T) {
		body, err := encodeJWS(key, protectedHeader{
			Alg:   "ES256",
			JWK:   jwk,
			Nonce: "nonce-1",
			URL:   "https://ca.example/new-account",
		}, []byte(`{"termsOfServiceAgreed":true}`))
		if err != nil {
			t.Fatalf("encodeJWS: %v", err)
		}
		header, payload := decodeJWSForTest(t, body)
		if header.Alg != "ES256" {
			t.Fatalf("alg is %q, want ES256", header.Alg)
		}
		if header.JWK == nil {
			t.Fatal("newAccount request must carry jwk")
		}
		if header.Kid != "" {
			t.Fatalf("newAccount request must not carry kid, got %q", header.Kid)
		}
		if header.Nonce != "nonce-1" || header.URL != "https://ca.example/new-account" {
			t.Fatalf("nonce/url not carried: %+v", header)
		}
		if string(payload) != `{"termsOfServiceAgreed":true}` {
			t.Fatalf("payload mismatch: %s", payload)
		}
	})

	t.Run("kid carries no jwk", func(t *testing.T) {
		body, err := encodeJWS(key, protectedHeader{
			Alg:   "ES256",
			Kid:   "https://ca.example/acct/1",
			Nonce: "nonce-2",
			URL:   "https://ca.example/new-order",
		}, []byte(`{}`))
		if err != nil {
			t.Fatalf("encodeJWS: %v", err)
		}
		header, _ := decodeJWSForTest(t, body)
		if header.JWK != nil {
			t.Fatal("a kid request must not carry jwk")
		}
		if header.Kid != "https://ca.example/acct/1" {
			t.Fatalf("kid is %q", header.Kid)
		}
	})

	t.Run("post-as-get has an empty payload", func(t *testing.T) {
		body, err := encodeJWS(key, protectedHeader{
			Alg:   "ES256",
			Kid:   "https://ca.example/acct/1",
			Nonce: "nonce-3",
			URL:   "https://ca.example/authz/1",
		}, nil)
		if err != nil {
			t.Fatalf("encodeJWS: %v", err)
		}
		var msg jwsMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("unmarshal JWS: %v", err)
		}
		if msg.Payload != "" {
			t.Fatalf("POST-as-GET payload must be the empty string, got %q", msg.Payload)
		}
	})

	t.Run("both jwk and kid is rejected", func(t *testing.T) {
		if _, err := encodeJWS(key, protectedHeader{Alg: "ES256", JWK: jwk, Kid: "x", Nonce: "n", URL: "u"}, nil); err == nil {
			t.Fatal("expected an error when both jwk and kid are set")
		}
		if _, err := encodeJWS(key, protectedHeader{Alg: "ES256", Nonce: "n", URL: "u"}, nil); err == nil {
			t.Fatal("expected an error when neither jwk nor kid is set")
		}
	})
}

// decodeJWSForTest splits a flattened JWS into its decoded header and payload.
func decodeJWSForTest(t *testing.T, body []byte) (protectedHeader, []byte) {
	t.Helper()
	var msg jwsMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("unmarshal JWS: %v", err)
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(msg.Protected)
	if err != nil {
		t.Fatalf("decode protected header: %v", err)
	}
	var header protectedHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("parse protected header: %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(msg.Payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return header, payload
}
