package backup

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// Destination is where a finished archive goes.
//
// It is deliberately the smallest interface that supports the whole feature:
// put an object, get it back, ask what is there, delete a generation. Anything
// larger would be shaped around one provider's API and would make the second
// provider awkward; anything smaller could not express retention.
//
// Every implementation must be safe for concurrent use, and must treat a key
// as an opaque path-like string that ValidateKey has already approved.
type Destination interface {
	// Kind is the stable identifier stored in the database: "local", "s3".
	Kind() string
	// Describe is a human-readable location, safe to log and show in the UI.
	// It must never contain a credential.
	Describe() string
	// Put stores an object. size may be -1 when it is not known in advance.
	Put(ctx context.Context, key string, r io.Reader, size int64) (ObjectInfo, error)
	// Get opens an object for reading. The caller closes it.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Stat reports on one object.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// List returns every object under a prefix, oldest first is not required:
	// callers that care about order sort for themselves.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	// Delete removes one object. Deleting something that is not there is not
	// an error - retention must be safe to re-run.
	Delete(ctx context.Context, key string) error
}

// Google Drive: evaluated, not built. The reasoning, recorded here rather than
// lost in a review thread.
//
// Drive has no static credential. It needs an OAuth 2.0 flow: a client id and
// secret registered by whoever operates the panel, a consent screen that a
// human must click through in a browser, a refresh token stored server-side
// and rotated, and a token endpoint that has to be reachable at 03:00 or the
// backup does not run. That is a fundamentally different shape from the two
// destinations above, which take a credential an operator pastes once.
//
// Three specific problems, in the order they would be hit:
//
//   - The consent flow needs a browser on the operator's machine and a
//     redirect back to the panel. The panel is often on a port that is not
//     publicly reachable, behind the access gate, which is the point of the
//     access gate. Out-of-band flows for this were deprecated by Google.
//   - An unverified OAuth client's refresh tokens expire in seven days. Making
//     them not expire means putting the app through Google's verification, an
//     external review process with an unpredictable timeline, and the panel's
//     backups would depend on its outcome.
//   - Drive's API is not object storage. It has no key namespace - two files
//     can share a name in a folder - so "the newest generation under this
//     prefix", which every retention decision is expressed in, has to be
//     rebuilt out of file ids and appProperties. That is a second, differently
//     shaped retention implementation to keep correct.
//
// The honest recommendation: an operator who wants their backups in Google's
// infrastructure is better served by Google Cloud Storage, which speaks the
// S3-compatible protocol this package already implements and takes an HMAC key
// pair - so it works today by configuring an S3 destination with GCS's
// endpoint. If Drive itself is ever required, it is a Destination
// implementation plus an OAuth token store plus a consent flow in the UI, and
// it should be estimated as its own piece of work rather than smuggled in as
// a third case in a switch.

// ObjectInfo describes one stored object.
type ObjectInfo struct {
	Key     string    `json:"key"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	ETag    string    `json:"etag,omitempty"`
}

// maxKeyLength is the S3 limit, and a sensible one for a filesystem too.
const maxKeyLength = 1024

// ValidateKey is the single gate every destination puts in front of a
// caller-supplied key.
//
// A key becomes a path under a local root and a path in a URL for S3, so the
// rules have to satisfy both: no leading slash, no traversal, no backslash, no
// control characters, nothing that would resolve to the root itself. Getting
// this wrong on the local destination is a write anywhere on the filesystem as
// root; getting it wrong on S3 is a request to an unintended path.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("backup: object key is empty")
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("backup: object key is longer than %d characters", maxKeyLength)
	}
	if strings.ContainsAny(key, "\x00\n\r") {
		return fmt.Errorf("backup: object key %q contains a control character", key)
	}
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("backup: object key %q must be relative", key)
	}
	if strings.Contains(key, `\`) {
		return fmt.Errorf("backup: object key %q contains a backslash", key)
	}
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return fmt.Errorf("backup: object key %q contains %q", key, "..")
		}
	}
	clean := path.Clean(key)
	if clean == "." || clean == "/" || clean != key {
		return fmt.Errorf("backup: object key %q is not in canonical form (expected %q)", key, clean)
	}
	return nil
}

// ObjectKey builds the key an archive is stored under.
//
// The layout is tenant/kind/resource/timestamp-name, so that:
//   - a prefix listing per tenant, per kind or per resource is one call, which
//     is what retention needs;
//   - the timestamp sorts lexicographically in the same order it sorts
//     chronologically, so "the newest generation" is the last key, on every
//     destination, without parsing anything.
func ObjectKey(tenant, kind, resource string, at time.Time, name string) (string, error) {
	for _, part := range []string{tenant, kind, resource, name} {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("backup: cannot build an object key from empty parts")
		}
		if strings.ContainsAny(part, "/\\\x00\n\r") {
			return "", fmt.Errorf("backup: object key part %q contains a separator or control character", part)
		}
	}
	key := fmt.Sprintf("%s/%s/%s/%s-%s", tenant, kind, resource, at.UTC().Format("20060102T150405Z"), name)
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	return key, nil
}
