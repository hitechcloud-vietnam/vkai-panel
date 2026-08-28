package backup

// The S3-compatible destination: Amazon S3, and every object store that speaks
// the same four verbs - MinIO, Wasabi, Backblaze B2's S3 endpoint, DigitalOcean
// Spaces, Vultr, Contabo. All of them are reached through one endpoint URL and
// a path-style flag, because that is the only thing that actually differs
// between them for this handful of operations.

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// S3Config describes one bucket.
type S3Config struct {
	// Endpoint is the service URL: https://s3.eu-west-1.amazonaws.com for
	// Amazon, https://minio.internal:9000 for a self-hosted store.
	Endpoint string
	Region   string
	Bucket   string
	// Prefix is prepended to every key, so one bucket can hold the backups of
	// several panels without them being able to list each other's by accident.
	Prefix string

	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// PathStyle addresses the bucket as endpoint/bucket/key instead of
	// bucket.endpoint/key. Every self-hosted store needs it; Amazon accepts
	// it. It defaults off so that Amazon gets the addressing it prefers.
	PathStyle bool

	// SpoolDir is where a payload that cannot be re-read is staged so its
	// digest can be computed before the request is signed. It must be on a
	// filesystem with room for one archive. Empty means the system temporary
	// directory.
	SpoolDir string

	HTTPClient *http.Client
	// Now exists so a test can pin the signing clock. Nil means time.Now.
	Now func() time.Time
}

// S3Destination stores archives in an S3-compatible bucket.
type S3Destination struct {
	cfg      S3Config
	endpoint *url.URL
	client   *http.Client
	now      func() time.Time
}

// NewS3Destination validates a configuration and returns a destination.
//
// It performs no network call: a destination that cannot be constructed is a
// configuration error and must be reported as one, while a destination that
// cannot be reached is an operational error and is reported by Probe.
func NewS3Destination(cfg S3Config) (*S3Destination, error) {
	var missing []string
	if strings.TrimSpace(cfg.Endpoint) == "" {
		missing = append(missing, "endpoint")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		missing = append(missing, "region")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		missing = append(missing, "bucket")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		missing = append(missing, "access key id")
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		missing = append(missing, "secret access key")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("backup: S3 destination is missing %s", strings.Join(missing, ", "))
	}

	endpoint, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("backup: S3 endpoint %q is not a URL: %w", cfg.Endpoint, err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("backup: S3 endpoint %q must be http or https", cfg.Endpoint)
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("backup: S3 endpoint %q has no host", cfg.Endpoint)
	}
	if strings.ContainsAny(cfg.Bucket, "/\\ \x00\n\r") {
		return nil, fmt.Errorf("backup: S3 bucket name %q contains an invalid character", cfg.Bucket)
	}
	if cfg.Prefix != "" {
		if err := ValidateKey(strings.TrimSuffix(cfg.Prefix, "/")); err != nil {
			return nil, fmt.Errorf("backup: S3 prefix is not usable: %w", err)
		}
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &S3Destination{cfg: cfg, endpoint: endpoint, client: client, now: now}, nil
}

func (d *S3Destination) Kind() string { return "s3" }

// Describe never includes a credential: it goes in log lines and API responses.
func (d *S3Destination) Describe() string {
	location := fmt.Sprintf("s3 bucket %s at %s", d.cfg.Bucket, d.endpoint.String())
	if d.cfg.Prefix != "" {
		location += " under " + d.cfg.Prefix + "/"
	}
	return location
}

func (d *S3Destination) creds() sigV4Credentials {
	return sigV4Credentials{
		AccessKeyID:     d.cfg.AccessKeyID,
		SecretAccessKey: d.cfg.SecretAccessKey,
		SessionToken:    d.cfg.SessionToken,
	}
}

// fullKey applies the configured prefix.
func (d *S3Destination) fullKey(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if d.cfg.Prefix == "" {
		return key, nil
	}
	return strings.TrimSuffix(d.cfg.Prefix, "/") + "/" + key, nil
}

// requestURL builds the URL for a key, encoding each path segment exactly the
// way the signer will. Path and RawPath are both set so that the escaped form
// the signer reads is the escaped form that goes on the wire; letting net/url
// re-derive one from the other is how a key containing a '+' or a space ends
// up signed one way and sent another.
func (d *S3Destination) requestURL(fullKey string, query url.Values) *url.URL {
	u := *d.endpoint

	var rawSegments, rawPath []string
	if d.cfg.PathStyle {
		rawSegments = append(rawSegments, d.cfg.Bucket)
		rawPath = append(rawPath, d.cfg.Bucket)
	} else {
		u.Host = d.cfg.Bucket + "." + u.Host
	}
	if fullKey != "" {
		for _, segment := range strings.Split(fullKey, "/") {
			rawSegments = append(rawSegments, uriEncode(segment, false))
			rawPath = append(rawPath, segment)
		}
	}

	u.Path = "/" + strings.Join(rawPath, "/")
	u.RawPath = "/" + strings.Join(rawSegments, "/")
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return &u
}

// do signs and sends one request.
func (d *S3Destination) do(ctx context.Context, method string, u *url.URL, body io.Reader, contentLength int64, payloadSHA256 string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("backup: cannot build the %s request: %w", method, err)
	}
	// http.NewRequestWithContext parses the URL again from its string form,
	// which loses RawPath when the encodings differ. Put the URL we built
	// back, so what is signed is what is sent.
	req.URL = u
	req.Host = u.Host
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	signV4(req, payloadSHA256, d.creds(), d.cfg.Region, "s3", d.now())

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup: %s %s failed: %w", method, redactURL(u), err)
	}
	return resp, nil
}

func redactURL(u *url.URL) string {
	clean := *u
	clean.RawQuery = ""
	clean.User = nil
	return clean.String()
}

// s3Error is the error document every S3-compatible store returns.
type s3Error struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
	Key     string   `xml:"Key"`
}

// ErrObjectNotFound is returned when the object is not in the bucket. It is a
// distinct error because retention and verification both have to tell "the
// generation is gone" apart from "the object store is broken".
var ErrObjectNotFound = errors.New("backup: object not found in the destination")

func (d *S3Destination) checkResponse(resp *http.Response, method string, u *url.URL) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrObjectNotFound, redactURL(u))
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var parsed s3Error
	if err := xml.Unmarshal(body, &parsed); err == nil && parsed.Code != "" {
		return fmt.Errorf("backup: %s %s refused by the object store: %s (%s)",
			method, redactURL(u), parsed.Code, parsed.Message)
	}
	return fmt.Errorf("backup: %s %s returned HTTP %d", method, redactURL(u), resp.StatusCode)
}

// Put uploads an object.
//
// The payload has to be digested before the request can be signed, because
// this client signs with the real payload hash rather than UNSIGNED-PAYLOAD -
// an unsigned payload is only safe over TLS and would let a proxy alter an
// archive in flight without the signature noticing. A reader that can seek is
// digested in place; anything else is spooled to a file first. Archives are
// large, so the spool file is deleted before the function returns whatever
// happens.
func (d *S3Destination) Put(ctx context.Context, key string, r io.Reader, size int64) (ObjectInfo, error) {
	fullKey, err := d.fullKey(key)
	if err != nil {
		return ObjectInfo{}, err
	}

	body, length, digest, cleanup, err := d.prepareBody(ctx, r, size)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer cleanup()

	u := d.requestURL(fullKey, nil)
	resp, err := d.do(ctx, http.MethodPut, u, body, length, digest)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := d.checkResponse(resp, http.MethodPut, u); err != nil {
		return ObjectInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	return ObjectInfo{
		Key:     key,
		Size:    length,
		ModTime: d.now().UTC(),
		ETag:    strings.Trim(resp.Header.Get("ETag"), `"`),
	}, nil
}

func (d *S3Destination) prepareBody(ctx context.Context, r io.Reader, size int64) (io.Reader, int64, string, func(), error) {
	noop := func() {}

	if seeker, ok := r.(io.ReadSeeker); ok {
		start, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			digest, n, err := digestReader(ctx, seeker)
			if err != nil {
				return nil, 0, "", noop, err
			}
			if _, err := seeker.Seek(start, io.SeekStart); err != nil {
				return nil, 0, "", noop, fmt.Errorf("backup: cannot rewind the archive after digesting it: %w", err)
			}
			if size >= 0 && size != n {
				return nil, 0, "", noop, fmt.Errorf("backup: archive is %d bytes, expected %d", n, size)
			}
			return seeker, n, digest, noop, nil
		}
	}

	spool, err := os.CreateTemp(d.cfg.SpoolDir, "vkai-upload-*")
	if err != nil {
		return nil, 0, "", noop, fmt.Errorf("backup: cannot stage the upload: %w", err)
	}
	cleanup := func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}
	if err := spool.Chmod(0o600); err != nil {
		cleanup()
		return nil, 0, "", noop, fmt.Errorf("backup: cannot set the mode of the upload staging file: %w", err)
	}
	if _, err := io.Copy(spool, &ctxReader{ctx: ctx, src: r}); err != nil {
		cleanup()
		return nil, 0, "", noop, fmt.Errorf("backup: cannot stage the upload: %w", err)
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, 0, "", noop, fmt.Errorf("backup: cannot rewind the upload staging file: %w", err)
	}
	digest, n, err := digestReader(ctx, spool)
	if err != nil {
		cleanup()
		return nil, 0, "", noop, err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, 0, "", noop, fmt.Errorf("backup: cannot rewind the upload staging file: %w", err)
	}
	if size >= 0 && size != n {
		cleanup()
		return nil, 0, "", noop, fmt.Errorf("backup: archive is %d bytes, expected %d", n, size)
	}
	return spool, n, digest, cleanup, nil
}

func digestReader(ctx context.Context, r io.Reader) (string, int64, error) {
	h := newHashWriter(io.Discard)
	n, err := io.Copy(h, &ctxReader{ctx: ctx, src: r})
	if err != nil {
		return "", 0, fmt.Errorf("backup: cannot digest the archive: %w", err)
	}
	return h.sum(), n, nil
}

func (d *S3Destination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullKey, err := d.fullKey(key)
	if err != nil {
		return nil, err
	}
	u := d.requestURL(fullKey, nil)
	resp, err := d.do(ctx, http.MethodGet, u, nil, 0, emptyPayloadSHA256)
	if err != nil {
		return nil, err
	}
	if err := d.checkResponse(resp, http.MethodGet, u); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *S3Destination) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	fullKey, err := d.fullKey(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	u := d.requestURL(fullKey, nil)
	resp, err := d.do(ctx, http.MethodHead, u, nil, 0, emptyPayloadSHA256)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := d.checkResponse(resp, http.MethodHead, u); err != nil {
		return ObjectInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	info := ObjectInfo{Key: key, Size: resp.ContentLength, ETag: strings.Trim(resp.Header.Get("ETag"), `"`)}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			info.ModTime = t.UTC()
		}
	}
	return info, nil
}

// listBucketResult is the ListObjectsV2 response.
type listBucketResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		LastModified time.Time `xml:"LastModified"`
		ETag         string    `xml:"ETag"`
	} `xml:"Contents"`
}

// List enumerates the objects under a prefix, following continuation tokens.
//
// A bucket holding years of generations is paginated, and a retention pass that
// only ever saw the first page would keep deleting the same thousand objects
// and never reach the old ones.
func (d *S3Destination) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := prefix
	if d.cfg.Prefix != "" {
		fullPrefix = strings.TrimSuffix(d.cfg.Prefix, "/") + "/" + prefix
	}

	var (
		out   []ObjectInfo
		token string
	)
	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if page > 10000 {
			return nil, errors.New("backup: the object store did not stop paginating")
		}

		query := url.Values{}
		query.Set("list-type", "2")
		query.Set("max-keys", "1000")
		if fullPrefix != "" {
			query.Set("prefix", fullPrefix)
		}
		if token != "" {
			query.Set("continuation-token", token)
		}

		u := d.requestURL("", query)
		resp, err := d.do(ctx, http.MethodGet, u, nil, 0, emptyPayloadSHA256)
		if err != nil {
			return nil, err
		}
		if err := d.checkResponse(resp, http.MethodGet, u); err != nil {
			return nil, err
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("backup: cannot read the object listing: %w", readErr)
		}

		var parsed listBucketResult
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("backup: cannot parse the object listing: %w", err)
		}
		for _, item := range parsed.Contents {
			key := item.Key
			if d.cfg.Prefix != "" {
				key = strings.TrimPrefix(key, strings.TrimSuffix(d.cfg.Prefix, "/")+"/")
			}
			out = append(out, ObjectInfo{
				Key:     key,
				Size:    item.Size,
				ModTime: item.LastModified.UTC(),
				ETag:    strings.Trim(item.ETag, `"`),
			})
		}

		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			break
		}
		token = parsed.NextContinuationToken
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Delete removes an object. S3 answers 204 whether or not the key was there,
// which is the behaviour retention wants: re-running a prune converges.
func (d *S3Destination) Delete(ctx context.Context, key string) error {
	fullKey, err := d.fullKey(key)
	if err != nil {
		return err
	}
	u := d.requestURL(fullKey, nil)
	resp, err := d.do(ctx, http.MethodDelete, u, nil, 0, emptyPayloadSHA256)
	if err != nil {
		return err
	}
	if err := d.checkResponse(resp, http.MethodDelete, u); err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil
		}
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return nil
}

// Probe writes a small object, reads it back, checks it and deletes it.
//
// It is what the "test this destination" button calls. Testing a destination by
// listing it is not a test: listing succeeds on a bucket with no write
// permission, and the first thing the operator would learn about that is a
// failed backup at 03:00.
func Probe(ctx context.Context, dest Destination, keyPrefix string) error {
	key := keyPrefix + "/.vkai-probe"
	if err := ValidateKey(key); err != nil {
		return err
	}
	payload := fmt.Sprintf("vkai-panel destination probe %s", time.Now().UTC().Format(time.RFC3339Nano))

	if _, err := dest.Put(ctx, key, strings.NewReader(payload), int64(len(payload))); err != nil {
		return fmt.Errorf("destination is not writable: %w", err)
	}
	defer func() { _ = dest.Delete(ctx, key) }()

	rc, err := dest.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("destination is not readable: %w", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(io.LimitReader(rc, int64(len(payload))+1))
	if err != nil {
		return fmt.Errorf("destination could not return the probe object: %w", err)
	}
	if string(got) != payload {
		return errors.New("destination returned different bytes than were written to it")
	}
	return nil
}
