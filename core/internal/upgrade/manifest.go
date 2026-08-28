package upgrade

// The release manifest and the feed it is served from.
//
// IMPORTANT: the wire format below is shared with the release tooling that
// produces the feed. The JSON field names - version, released_at,
// min_upgrade_from, tarball_url, sha256, changelog_url - are the contract; the
// Go struct is free to change around them but those names are not.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return nil
}

// ParsedVersion returns the manifest version already parsed.
func (m Manifest) ParsedVersion() (Version, error) { return ParseVersion(m.Version) }

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
	return parseFeed(body)
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
	for i, m := range releases {
		if err := m.Validate(); err != nil {
			return nil, fmt.Errorf("release feed entry %d: %w", i, err)
		}
	}
	return releases, nil
}
