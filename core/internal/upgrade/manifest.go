package upgrade

// The release manifest and the feed it is served from.
//
// IMPORTANT: the wire format below is shared with the release tooling that
// produces the feed. The JSON field names - version, released_at,
// min_upgrade_from, tarball_url, sha256, changelog_url, size_bytes, signature -
// are the contract; the Go struct is free to change around them but those names
// are not.
//
// # What a manifest is trusted to decide
//
// A manifest names a URL and a digest, and this package then installs whatever
// that URL serves, as root, and restarts the machine's services onto it.
// Without a signature the only thing standing between "whoever can answer for
// the feed host" and root on every installation is TLS - which means the feed's
// certificate, its CA, its DNS, its CDN and everyone with commit access to the
// bucket behind it. Config.ReleasePublicKeys closes that: when it is set, every
// manifest in the feed has to carry an ed25519 signature over its own contents,
// made by a key that never has to be on the release host at all. It is off by
// default, because turning it on without published keys would brick upgrades on
// existing installations, and that is stated as a residual risk rather than
// quietly assumed away.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Manifest describes one published release.
//
// MinUpgradeFrom is the oldest version that may upgrade straight to this one.
// It exists because some releases carry a migration or a config rewrite that
// only understands the shape left behind by a particular predecessor; jumping
// over that predecessor would run the migration against a layout it has never
// seen. When the running version is older than MinUpgradeFrom the upgrade is
// refused and the operator is told which version to install first.
type Manifest struct {
	Version        string    `json:"version"`
	ReleasedAt     time.Time `json:"released_at"`
	MinUpgradeFrom string    `json:"min_upgrade_from"`
	TarballURL     string    `json:"tarball_url"`
	SHA256         string    `json:"sha256"`
	ChangelogURL   string    `json:"changelog_url"`

	// SizeBytes is the exact size of the tarball, when the release tooling
	// publishes it. It lets the disk check happen before a gigabyte is
	// downloaded rather than after, and a download of a different length is
	// refused without hashing it.
	SizeBytes int64 `json:"size_bytes,omitempty"`

	// Signature is an ed25519 signature over ManifestSigningPayload, hex
	// encoded. Required when Config.ReleasePublicKeys is set, ignored when
	// it is not.
	Signature string `json:"signature,omitempty"`
}

// Validate checks that a manifest can actually be acted on. A feed that is
// syntactically valid JSON but missing a checksum would otherwise only fail
// after the tarball had been downloaded.
func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("manifest has no version")
	}
	if _, err := ParseVersion(m.Version); err != nil {
		return fmt.Errorf("manifest version is unusable: %w", err)
	}
	if m.MinUpgradeFrom != "" {
		if _, err := ParseVersion(m.MinUpgradeFrom); err != nil {
			return fmt.Errorf("manifest min_upgrade_from is unusable: %w", err)
		}
	}
	if strings.TrimSpace(m.TarballURL) == "" {
		return fmt.Errorf("manifest for %s has no tarball_url", m.Version)
	}
	if err := validateChecksum(m.SHA256); err != nil {
		return fmt.Errorf("manifest for %s: %w", m.Version, err)
	}
	if m.SizeBytes < 0 {
		return fmt.Errorf("manifest for %s has a negative size_bytes", m.Version)
	}
	return nil
}

// ParsedVersion returns the manifest version already parsed.
func (m Manifest) ParsedVersion() (Version, error) { return ParseVersion(m.Version) }

// ManifestSigningPayload is the exact byte string a release signature covers.
//
// It is built from the fields rather than from the JSON, because two encoders
// produce two different JSON documents for the same manifest and a signature
// over "whatever bytes arrived" would either break on a whitespace change or
// have to canonicalise JSON, which is a well-known way to get this wrong. The
// release tooling must produce this string exactly; it is part of the contract.
//
//	vkai-panel-release-manifest-v1\n
//	<version>\n
//	<released_at as RFC3339 in UTC, empty when unset>\n
//	<min_upgrade_from>\n
//	<tarball_url>\n
//	<sha256, lowercase>\n
//	<size_bytes as decimal>\n
//	<changelog_url>\n
func ManifestSigningPayload(m Manifest) []byte {
	released := ""
	if !m.ReleasedAt.IsZero() {
		released = m.ReleasedAt.UTC().Format(time.RFC3339)
	}
	fields := []string{
		"vkai-panel-release-manifest-v1",
		strings.TrimSpace(m.Version),
		released,
		strings.TrimSpace(m.MinUpgradeFrom),
		strings.TrimSpace(m.TarballURL),
		strings.ToLower(strings.TrimSpace(m.SHA256)),
		strconv.FormatInt(m.SizeBytes, 10),
		strings.TrimSpace(m.ChangelogURL),
	}
	return []byte(strings.Join(fields, "\n") + "\n")
}

// verifySignature checks a manifest against the configured release keys.
//
// Every listed manifest has to verify, not just the one that is about to be
// installed: the others decide which upgrades are legal - min_upgrade_from and
// the intermediate release the operator is told to install first - so an
// unsigned entry in the list is still an entry that changes what happens.
func (u *Upgrader) verifySignature(m Manifest) error {
	if len(u.releaseKeys) == 0 {
		return nil
	}
	sig, err := hex.DecodeString(strings.TrimSpace(m.Signature))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("release %s is not signed, or its signature is not %d hex-encoded bytes",
			m.Version, ed25519.SignatureSize)
	}
	payload := ManifestSigningPayload(m)
	for _, key := range u.releaseKeys {
		if ed25519.Verify(key, payload, sig) {
			return nil
		}
	}
	return fmt.Errorf("the signature on release %s was not made by any configured release key; refusing to install it", m.Version)
}

func validateChecksum(sum string) error {
	s := strings.TrimSpace(sum)
	if s == "" {
		return fmt.Errorf("no sha256 in manifest")
	}
	if len(s) != 64 {
		return fmt.Errorf("sha256 %q is not 64 hex characters", sum)
	}
	for _, r := range strings.ToLower(s) {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fmt.Errorf("sha256 %q is not hexadecimal", sum)
		}
	}
	return nil
}

// checkDownloadURL refuses a tarball URL that would be fetched over a channel
// nobody authenticates. Without signatures TLS is the only thing making the
// feed's answer trustworthy at all, so a plaintext URL - or one the feed
// redirected to plaintext - is an unauthenticated root install.
func (u *Upgrader) checkDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("tarball_url %q is not a URL: %w", raw, err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if u.cfg.AllowInsecureURLs {
			return nil
		}
		return fmt.Errorf("tarball_url %q is plaintext http; set AllowInsecureURLs only for a mirror on a trusted network", raw)
	default:
		return fmt.Errorf("tarball_url %q has scheme %q; only https is supported", raw, parsed.Scheme)
	}
}

// maxFeedBytes caps what the feed may hand us. The feed is a small JSON
// document; anything larger is a misconfigured URL or a hostile one, and either
// way is not worth buffering.
const maxFeedBytes = 1 << 20 // 1 MiB

// fetchFeed retrieves the feed and returns every release it lists, oldest
// first is not guaranteed - callers sort.
//
// Two shapes are accepted, because both are reasonable things for a release
// pipeline to publish and guessing wrong would be a silent outage:
//
//	{"version": "...", ...}                  a single release
//	[{"version": "..."}, {"version": "..."}] the release history
//	{"releases": [ ... ]}                    the same, wrapped
//
// The history form is what lets an incompatible jump name a real intermediate
// release rather than only quoting min_upgrade_from back at the operator.
func (u *Upgrader) fetchFeed(ctx context.Context) ([]Manifest, error) {
	// The feed is a small document, and the caller's context may have no
	// deadline of its own - a CLI invocation, a cron job. Without this an
	// unresponsive release host would hang the upgrade rather than fail it.
	if u.cfg.FeedTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, u.cfg.FeedTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.FeedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build feed request for %s: %w", u.cfg.FeedURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", u.cfg.UserAgent)

	resp, err := u.deps.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release feed %s: %w", u.cfg.FeedURL, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxFeedBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release feed %s returned HTTP %d", u.cfg.FeedURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read release feed %s: %w", u.cfg.FeedURL, err)
	}
	if len(body) > maxFeedBytes {
		return nil, fmt.Errorf("release feed %s is larger than %d bytes", u.cfg.FeedURL, maxFeedBytes)
	}

	releases, err := parseFeed(body)
	if err != nil {
		return nil, err
	}
	for _, m := range releases {
		if err := u.verifySignature(m); err != nil {
			return nil, err
		}
		if err := u.checkDownloadURL(m.TarballURL); err != nil {
			return nil, fmt.Errorf("release %s: %w", m.Version, err)
		}
	}
	return releases, nil
}

func parseFeed(body []byte) ([]Manifest, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, fmt.Errorf("release feed is empty")
	}

	var releases []Manifest
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(body, &releases); err != nil {
			return nil, fmt.Errorf("decode release feed: %w", err)
		}
	case '{':
		var wrapper struct {
			Releases []Manifest `json:"releases"`
		}
		if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Releases) > 0 {
			releases = wrapper.Releases
			break
		}
		var single Manifest
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, fmt.Errorf("decode release feed: %w", err)
		}
		releases = []Manifest{single}
	default:
		return nil, fmt.Errorf("release feed is not a JSON object or array")
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("release feed lists no releases")
	}
	seen := make(map[string]int, len(releases))
	for i, m := range releases {
		if err := m.Validate(); err != nil {
			return nil, fmt.Errorf("release feed entry %d: %w", i, err)
		}
		// Two entries for one version would make "the newest release"
		// depend on sort stability, and the two entries can name
		// different tarballs. There is no honest way to pick one.
		v, err := m.ParsedVersion()
		if err != nil {
			return nil, fmt.Errorf("release feed entry %d: %w", i, err)
		}
		key := v.Canonical()
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("release feed lists version %s twice, at entries %d and %d", m.Version, first, i)
		}
		seen[key] = i
	}
	return releases, nil
}
