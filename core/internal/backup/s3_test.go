package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is an S3-compatible object store, in memory, that authenticates every
// request.
//
// The authentication is the point. It does not check that the client sent
// *some* Authorization header; it rebuilds the canonical request from the bytes
// it actually received - method, escaped path, query, exactly the headers the
// client said it signed - recomputes the signature and compares. A header the
// client signed but did not send, a query parameter dropped on the way, a key
// escaped one way for signing and another way for the URL: all of them come
// back as SignatureDoesNotMatch here, which is what a real endpoint would say.
type fakeS3 struct {
	t      *testing.T
	mu     sync.Mutex
	bucket string
	secret string
	access string
	region string
	// objects is key -> body.
	objects map[string][]byte
	// pageSize forces pagination regardless of the client's max-keys, so the
	// continuation loop is exercised with three objects instead of a thousand.
	pageSize int
	// requests records every path that was served, for tests that assert what
	// the client did rather than only what it got back.
	requests []string
}

func newFakeS3(t *testing.T) *fakeS3 {
	return &fakeS3{
		t:        t,
		bucket:   "vkai-backups",
		secret:   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		access:   "AKIAIOSFODNN7EXAMPLE",
		region:   "eu-west-1",
		objects:  map[string][]byte{},
		pageSize: 1,
	}
}

func (f *fakeS3) start() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	f.t.Cleanup(server.Close)
	return server
}

func (f *fakeS3) config(endpoint string) S3Config {
	return S3Config{
		Endpoint:        endpoint,
		Region:          f.region,
		Bucket:          f.bucket,
		AccessKeyID:     f.access,
		SecretAccessKey: f.secret,
		PathStyle:       true,
	}
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "MalformedRequest", err.Error())
		return
	}

	if err := f.authenticate(r, body); err != nil {
		writeS3Error(w, http.StatusForbidden, "SignatureDoesNotMatch", err.Error())
		return
	}

	prefix := "/" + f.bucket
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "unknown bucket in "+r.URL.Path)
		return
	}
	key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")

	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)

	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		f.list(w, r)
	case r.Method == http.MethodPut:
		f.objects[key] = body
		sum := sha256.Sum256(body)
		w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet, r.Method == http.MethodHead:
		object, ok := f.objects[key]
		if !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey", key)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(object)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(object)
		}
	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed", r.Method)
	}
}

func (f *fakeS3) list(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	token := r.URL.Query().Get("continuation-token")

	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	start := 0
	if token != "" {
		for i, k := range keys {
			if k == token {
				start = i
				break
			}
		}
	}
	end := start + f.pageSize
	truncated := end < len(keys)
	if end > len(keys) {
		end = len(keys)
	}

	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult>`)
	fmt.Fprintf(&out, "<IsTruncated>%t</IsTruncated>", truncated)
	if truncated {
		fmt.Fprintf(&out, "<NextContinuationToken>%s</NextContinuationToken>", xmlEscape(keys[end]))
	}
	for _, k := range keys[start:end] {
		sum := sha256.Sum256(f.objects[k])
		fmt.Fprintf(&out,
			"<Contents><Key>%s</Key><Size>%d</Size><LastModified>%s</LastModified><ETag>&quot;%s&quot;</ETag></Contents>",
			xmlEscape(k), len(f.objects[k]), time.Now().UTC().Format(time.RFC3339), hex.EncodeToString(sum[:]))
	}
	out.WriteString(`</ListBucketResult>`)

	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, out.String())
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func writeS3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message></Error>", xmlEscape(code), xmlEscape(message))
}

func (f *fakeS3) authenticate(r *http.Request, body []byte) error {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return fmt.Errorf("no Authorization header")
	}

	claimed := sha256.Sum256(body)
	if got := r.Header.Get("X-Amz-Content-Sha256"); got != hex.EncodeToString(claimed[:]) {
		return fmt.Errorf("x-amz-content-sha256 is %s but the body hashes to %s", got, hex.EncodeToString(claimed[:]))
	}

	rest, ok := strings.CutPrefix(auth, sigV4Algorithm+" ")
	if !ok {
		return fmt.Errorf("Authorization is not %s", sigV4Algorithm)
	}
	var credential, signedHeaders string
	for _, part := range strings.Split(rest, ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch key {
		case "Credential":
			credential = value
		case "SignedHeaders":
			signedHeaders = value
		}
	}
	if credential == "" || signedHeaders == "" {
		return fmt.Errorf("Authorization is missing Credential or SignedHeaders: %q", auth)
	}
	if !strings.HasPrefix(credential, f.access+"/") {
		return fmt.Errorf("unknown access key in %q", credential)
	}

	signTime, err := time.Parse(amzDateFormat, r.Header.Get("X-Amz-Date"))
	if err != nil {
		return fmt.Errorf("x-amz-date %q: %w", r.Header.Get("X-Amz-Date"), err)
	}

	// Rebuild the request from what arrived, carrying only the headers the
	// client said it signed.
	replica := &http.Request{
		Method: r.Method,
		URL:    &url.URL{Scheme: "http", Host: r.Host, Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: r.URL.RawQuery},
		Host:   r.Host,
		Header: http.Header{},
	}
	for _, name := range strings.Split(signedHeaders, ";") {
		if name == "host" {
			continue
		}
		value := r.Header.Get(name)
		if value == "" && name == "content-length" {
			value = fmt.Sprintf("%d", len(body))
		}
		if value == "" {
			return fmt.Errorf("header %q was signed but not sent", name)
		}
		replica.Header.Set(name, value)
	}

	signV4(replica, r.Header.Get("X-Amz-Content-Sha256"),
		sigV4Credentials{AccessKeyID: f.access, SecretAccessKey: f.secret},
		f.region, "s3", signTime)

	if replica.Header.Get("Authorization") != auth {
		return fmt.Errorf("signature mismatch\n  sent:        %s\n  recomputed:  %s",
			auth, replica.Header.Get("Authorization"))
	}
	return nil
}

func TestS3DestinationRoundTrip(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()

	dest, err := NewS3Destination(fake.config(server.URL))
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	ctx := context.Background()
	payload := bytes.Repeat([]byte("archive bytes "), 5000)
	key := "tenant-a/website/shop/20250101T000000Z-shop.vkab"

	info, err := dest.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("Put reported %d bytes, want %d", info.Size, len(payload))
	}

	rc, err := dest.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("the object read back is not the object written")
	}

	stat, err := dest.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Size != int64(len(payload)) {
		t.Fatalf("Stat reported %d bytes", stat.Size)
	}

	if err := dest.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := dest.Get(ctx, key); err == nil {
		t.Fatal("the object is still readable after being deleted")
	}
	// Deleting again must be quiet: retention re-runs.
	if err := dest.Delete(ctx, key); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestS3PutFromAnUnseekableReader(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()

	cfg := fake.config(server.URL)
	cfg.SpoolDir = t.TempDir()
	dest, err := NewS3Destination(cfg)
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	payload := bytes.Repeat([]byte("streamed "), 1000)
	// A pipe cannot seek, so the client has to spool it to compute the digest.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write(payload)
		_ = pw.Close()
	}()

	if _, err := dest.Put(context.Background(), "tenant/files/x/stream", pr, -1); err != nil {
		t.Fatalf("Put from a pipe: %v", err)
	}

	rc, err := dest.Get(context.Background(), "tenant/files/x/stream")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatal("the streamed object came back different")
	}

	// The spool file must not survive the upload.
	entries, err := readDirNames(cfg.SpoolDir)
	if err != nil {
		t.Fatalf("read spool dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("the upload staging file was left behind: %v", entries)
	}
}

func TestS3KeysWithAwkwardCharactersAreSignedAsSent(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()

	dest, err := NewS3Destination(fake.config(server.URL))
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	// A '+' and a space are where naive escaping and SigV4 escaping disagree.
	for _, key := range []string{
		"tenant/files/site a/backup one.vkab",
		"tenant/files/plus+key/archive.vkab",
		"tenant/files/tilde~key/archive.vkab",
	} {
		body := []byte("payload for " + key)
		if _, err := dest.Put(context.Background(), key, bytes.NewReader(body), int64(len(body))); err != nil {
			t.Fatalf("Put(%q): %v", key, err)
		}
		rc, err := dest.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		got, _ := io.ReadAll(rc)
		_ = rc.Close()
		if string(got) != string(body) {
			t.Fatalf("Get(%q) returned %q", key, string(got))
		}
	}
}

func TestS3ListFollowsContinuationTokens(t *testing.T) {
	fake := newFakeS3(t)
	fake.pageSize = 1 // one object per page: three pages for three objects
	server := fake.start()

	dest, err := NewS3Destination(fake.config(server.URL))
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	want := []string{
		"tenant/website/shop/20250101T000000Z-a.vkab",
		"tenant/website/shop/20250102T000000Z-b.vkab",
		"tenant/website/shop/20250103T000000Z-c.vkab",
	}
	for _, key := range want {
		if _, err := dest.Put(context.Background(), key, strings.NewReader(key), int64(len(key))); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	// Something under a different prefix, which must not be listed.
	if _, err := dest.Put(context.Background(), "tenant/database/other/20250101T000000Z-d.vkab", strings.NewReader("d"), 1); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := dest.List(context.Background(), "tenant/website/shop/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("List returned %d objects, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Key != want[i] {
			t.Fatalf("List[%d] = %q, want %q", i, got[i].Key, want[i])
		}
	}
}

func TestS3PrefixIsolatesPanels(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()

	cfgA := fake.config(server.URL)
	cfgA.Prefix = "panel-a"
	destA, err := NewS3Destination(cfgA)
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}
	cfgB := fake.config(server.URL)
	cfgB.Prefix = "panel-b"
	destB, err := NewS3Destination(cfgB)
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	if _, err := destA.Put(context.Background(), "t/files/x/one", strings.NewReader("a"), 1); err != nil {
		t.Fatalf("Put A: %v", err)
	}
	if _, err := destB.Put(context.Background(), "t/files/x/two", strings.NewReader("b"), 1); err != nil {
		t.Fatalf("Put B: %v", err)
	}

	listA, err := destA.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List A: %v", err)
	}
	if len(listA) != 1 || listA[0].Key != "t/files/x/one" {
		t.Fatalf("panel A can see %+v", listA)
	}
	if _, err := destA.Get(context.Background(), "t/files/x/two"); err == nil {
		t.Fatal("panel A read an object belonging to panel B")
	}
}

func TestS3ReportsStoreErrors(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()
	dest, err := NewS3Destination(fake.config(server.URL))
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	_, err = dest.Get(context.Background(), "tenant/files/x/missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a missing object gave %v", err)
	}

	// A wrong secret must be refused by the store and reported as its error
	// code, not as a generic failure.
	bad := fake.config(server.URL)
	bad.SecretAccessKey = "not-the-right-secret"
	badDest, err := NewS3Destination(bad)
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}
	_, err = badDest.Put(context.Background(), "tenant/files/x/y", strings.NewReader("z"), 1)
	if err == nil || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Fatalf("a bad secret gave %v", err)
	}
}

func TestS3ConfigurationIsValidated(t *testing.T) {
	cases := map[string]S3Config{
		"no endpoint": {Region: "r", Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"},
		"no region":   {Endpoint: "https://s3.example", Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"},
		"no bucket":   {Endpoint: "https://s3.example", Region: "r", AccessKeyID: "a", SecretAccessKey: "s"},
		"no key":      {Endpoint: "https://s3.example", Region: "r", Bucket: "b", SecretAccessKey: "s"},
		"no secret":   {Endpoint: "https://s3.example", Region: "r", Bucket: "b", AccessKeyID: "a"},
		"bad scheme":  {Endpoint: "ftp://s3.example", Region: "r", Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"},
		"bad bucket":  {Endpoint: "https://s3.example", Region: "r", Bucket: "a/b", AccessKeyID: "a", SecretAccessKey: "s"},
	}
	for name, cfg := range cases {
		if _, err := NewS3Destination(cfg); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestS3VirtualHostAddressing(t *testing.T) {
	dest, err := NewS3Destination(S3Config{
		Endpoint:        "https://s3.eu-west-1.amazonaws.com",
		Region:          "eu-west-1",
		Bucket:          "vkai-backups",
		AccessKeyID:     "a",
		SecretAccessKey: "s",
	})
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}
	u := dest.requestURL("tenant/files/x/y z", nil)
	if u.Host != "vkai-backups.s3.eu-west-1.amazonaws.com" {
		t.Fatalf("virtual-host addressing produced host %q", u.Host)
	}
	if u.EscapedPath() != "/tenant/files/x/y%20z" {
		t.Fatalf("escaped path is %q", u.EscapedPath())
	}

	pathStyle, err := NewS3Destination(S3Config{
		Endpoint: "https://minio.internal:9000", Region: "us-east-1", Bucket: "vkai-backups",
		AccessKeyID: "a", SecretAccessKey: "s", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}
	u = pathStyle.requestURL("tenant/files/x/y", nil)
	if u.Host != "minio.internal:9000" || u.EscapedPath() != "/vkai-backups/tenant/files/x/y" {
		t.Fatalf("path-style addressing produced %s%s", u.Host, u.EscapedPath())
	}
}

func TestDestinationProbeChecksWritesNotJustListing(t *testing.T) {
	fake := newFakeS3(t)
	server := fake.start()
	dest, err := NewS3Destination(fake.config(server.URL))
	if err != nil {
		t.Fatalf("NewS3Destination: %v", err)
	}

	if err := Probe(context.Background(), dest, "tenant-a"); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	// The probe must not leave anything behind.
	objects, err := dest.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("the probe left %+v behind", objects)
	}
}
