package backup

import (
	"net/http"
	"testing"
	"time"
)

// The worked example from the AWS Signature Version 4 documentation for
// Amazon S3: "GET Object" against examplebucket, with the example credentials
// that appear throughout those pages.
//
// This is the only external fact this package depends on, and it is the reason
// a hand-written signer is defensible at all: the algorithm is pinned to a
// published vector rather than to what this implementation happens to do.
const (
	docExampleAccessKey = "AKIAIOSFODNN7EXAMPLE"
	docExampleSecret    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	docExampleCanonicalRequest = "GET\n" +
		"/test.txt\n" +
		"\n" +
		"host:examplebucket.s3.amazonaws.com\n" +
		"range:bytes=0-9\n" +
		"x-amz-content-sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n" +
		"x-amz-date:20130524T000000Z\n" +
		"\n" +
		"host;range;x-amz-content-sha256;x-amz-date\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	docExampleStringToSign = "AWS4-HMAC-SHA256\n" +
		"20130524T000000Z\n" +
		"20130524/us-east-1/s3/aws4_request\n" +
		"7344ae5b7ee6c3e7e6b0fe0640412a37625d1fbfff95c48bbb2dc43964946972"

	docExampleSignature = "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
)

func TestSignV4MatchesThePublishedExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Range", "bytes=0-9")

	signTime := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	canonical, toSign := signV4(req, emptyPayloadSHA256, sigV4Credentials{
		AccessKeyID:     docExampleAccessKey,
		SecretAccessKey: docExampleSecret,
	}, "us-east-1", "s3", signTime)

	if canonical != docExampleCanonicalRequest {
		t.Fatalf("canonical request does not match the documented one:\n got:\n%s\nwant:\n%s", canonical, docExampleCanonicalRequest)
	}
	if toSign != docExampleStringToSign {
		t.Fatalf("string to sign does not match the documented one:\n got:\n%s\nwant:\n%s", toSign, docExampleStringToSign)
	}

	want := "AWS4-HMAC-SHA256 Credential=" + docExampleAccessKey +
		"/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=" + docExampleSignature
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization header does not match:\n got: %s\nwant: %s", got, want)
	}
}

func TestSigningKeyDerivation(t *testing.T) {
	// The intermediate signing key for the same example, so a failure points
	// at the derivation rather than at the whole signature.
	key := deriveSigningKey(docExampleSecret, "20130524", "us-east-1", "s3")
	if len(key) != 32 {
		t.Fatalf("signing key is %d bytes", len(key))
	}
	// Deriving twice must give the same key; deriving for another day must not.
	if string(key) != string(deriveSigningKey(docExampleSecret, "20130524", "us-east-1", "s3")) {
		t.Fatal("signing key derivation is not deterministic")
	}
	if string(key) == string(deriveSigningKey(docExampleSecret, "20130525", "us-east-1", "s3")) {
		t.Fatal("the signing key does not depend on the date")
	}
	if string(key) == string(deriveSigningKey(docExampleSecret, "20130524", "eu-west-1", "s3")) {
		t.Fatal("the signing key does not depend on the region")
	}
}

func TestURIEncode(t *testing.T) {
	cases := []struct {
		in          string
		encodeSlash bool
		want        string
	}{
		{"simple", false, "simple"},
		{"a/b/c", false, "a/b/c"},
		{"a/b/c", true, "a%2Fb%2Fc"},
		{"with space", true, "with%20space"},
		{"tilde~dash-dot.under_", true, "tilde~dash-dot.under_"},
		{"plus+and&amp", true, "plus%2Band%26amp"},
		{"e=mc2", true, "e%3Dmc2"},
		{"caf\xc3\xa9", true, "caf%C3%A9"},
	}
	for _, c := range cases {
		if got := uriEncode(c.in, c.encodeSlash); got != c.want {
			t.Fatalf("uriEncode(%q, %v) = %q, want %q", c.in, c.encodeSlash, got, c.want)
		}
	}
}

func TestCanonicalQuerySortsAndEncodes(t *testing.T) {
	// list-type comes before prefix, and an empty value still carries its '='.
	got := canonicalQuery("prefix=tenant%2Fwebsite&list-type=2&continuation-token=&max-keys=1000")
	want := "continuation-token=&list-type=2&max-keys=1000&prefix=tenant%2Fwebsite"
	if got != want {
		t.Fatalf("canonicalQuery = %q, want %q", got, want)
	}
	if canonicalQuery("") != "" {
		t.Fatal("an empty query must canonicalise to an empty string")
	}
}

func TestCanonicalHeaderValueCollapsesWhitespace(t *testing.T) {
	if got := canonicalHeaderValue("  a   b  "); got != "a b" {
		t.Fatalf("canonicalHeaderValue = %q, want %q", got, "a b")
	}
}
