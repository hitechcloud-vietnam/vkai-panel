package upgrade

// Downloading and verifying the release tarball.
//
// The invariant this file exists to hold is one sentence: no byte of a release
// archive is interpreted before its sha256 has been checked against the
// manifest. Hashing happens while the download streams to disk, and the file is
// deleted on a mismatch without ever being handed to the gzip or tar readers -
// a decompressor is a parser, and running one over an unverified attacker-
// controlled stream is precisely the thing to avoid.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// download fetches the tarball into the panel's tmp directory and returns the
// path plus its sha256, lowercase hex.
func (u *Upgrader) download(ctx context.Context, m Manifest) (path string, sum string, err error) {
	if err := os.MkdirAll(u.TmpDir(), 0o750); err != nil {
		return "", "", fmt.Errorf("create %s: %w", u.TmpDir(), err)
	}

	dlCtx := ctx
	if u.cfg.DownloadTimeout > 0 {
		var cancel context.CancelFunc
		dlCtx, cancel = context.WithTimeout(ctx, u.cfg.DownloadTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, m.TarballURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("build download request for %s: %w", m.TarballURL, err)
	}
	req.Header.Set("User-Agent", u.cfg.UserAgent)

	resp, err := u.deps.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download %s: %w", m.TarballURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download %s returned HTTP %d", m.TarballURL, resp.StatusCode)
	}

	dest := filepath.Join(u.TmpDir(), fmt.Sprintf("vkai-panel-%s-%d.tar.gz", sanitizeForFilename(m.Version), u.deps.PID))
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return "", "", fmt.Errorf("create %s: %w", dest, err)
	}

	hasher := sha256.New()
	// One byte over the limit is enough to prove the limit was exceeded.
	limited := io.LimitReader(resp.Body, u.cfg.MaxDownloadBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), limited)
	closeErr := f.Close()

	switch {
	case copyErr != nil:
		_ = os.Remove(dest)
		return "", "", fmt.Errorf("download %s: %w", m.TarballURL, copyErr)
	case closeErr != nil:
		_ = os.Remove(dest)
		return "", "", fmt.Errorf("write %s: %w", dest, closeErr)
	case written > u.cfg.MaxDownloadBytes:
		_ = os.Remove(dest)
		return "", "", fmt.Errorf("download %s exceeds the %d byte limit", m.TarballURL, u.cfg.MaxDownloadBytes)
	case written == 0:
		_ = os.Remove(dest)
		return "", "", fmt.Errorf("download %s returned an empty body", m.TarballURL)
	}

	return dest, hex.EncodeToString(hasher.Sum(nil)), nil
}

// verifyChecksum proves the file on disk is the one the manifest describes.
//
// It re-hashes the file rather than trusting the hash computed while streaming,
// because the thing that gets extracted is the file, not the stream. The two
// are the same only if nothing rewrote /vkai-panel/tmp in between, and "only if
// nothing else touched it" is not a property worth assuming about the step that
// decides whether to run a parser over attacker-influenced bytes. The streamed
// hash is still compared, so a swap between the two reads is named as such
// rather than reported as a bad download.
//
// On any mismatch the archive is deleted before returning, so no later step can
// find it and use it.
func (u *Upgrader) verifyChecksum(path, streamed string, m Manifest) error {
	expected := strings.ToLower(strings.TrimSpace(m.SHA256))

	onDisk, err := fileSHA256(path)
	if err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("verify %s: %w", path, err)
	}

	if !strings.EqualFold(onDisk, streamed) {
		_ = os.Remove(path)
		return fmt.Errorf("the download at %s changed between being written and being verified (%s while downloading, %s on disk); refusing to use it",
			path, streamed, onDisk)
	}
	if strings.EqualFold(expected, onDisk) {
		return nil
	}

	_ = os.Remove(path)
	return &ChecksumMismatchError{
		URL:      m.TarballURL,
		Expected: expected,
		Actual:   onDisk,
	}
}

// fileSHA256 hashes a file without holding it in memory.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sanitizeForFilename keeps a version usable as one path segment. Versions are
// validated before they reach here, so this is belt and braces against a feed
// that starts publishing something unexpected.
func sanitizeForFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == '+':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "release"
	}
	return out
}
