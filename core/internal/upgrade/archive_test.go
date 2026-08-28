package upgrade

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckArchiveMemberRejectsUnsafeNames is the table of everything a tar
// member may not be called. Each entry is a real technique, not a hypothetical.
func TestCheckArchiveMemberRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	unsafe := []struct {
		name string
		why  string
	}{
		{"/etc/cron.d/backdoor", "absolute path"},
		{"//etc/passwd", "absolute path with a doubled separator"},
		{"../outside", "leading traversal"},
		{"../../../../vkai-panel/etc/.env", "traversal reaching the config directory"},
		{"core/../../outside", "traversal in the middle"},
		{"core/..", "traversal as the final element"},
		{"..", "traversal alone"},
		{`C:\Windows\system32`, "drive-letter absolute path"},
		{`core\..\..\outside`, "backslash separators"},
		{"core/bin\x00/evil", "embedded NUL"},
		{"core/bin\n/evil", "embedded newline"},
		{"", "empty name"},
		{".", "the destination directory itself"},
		{"./", "the destination directory itself, written as a directory"},
	}
	for _, tc := range unsafe {
		got, err := checkArchiveMember(tc.name)
		var unsafeErr *UnsafeArchiveError
		if !errors.As(err, &unsafeErr) {
			t.Errorf("checkArchiveMember(%q) = %q, %v; want *UnsafeArchiveError (%s)", tc.name, got, err, tc.why)
		}
	}

	safe := []struct{ in, want string }{
		{"VERSION", "VERSION"},
		{"core/vkai-api", "core/vkai-api"},
		{"./core/vkai-api", "core/vkai-api"},
		{"panel/.next/static/chunk.js", "panel/.next/static/chunk.js"},
		{"core/", "core"},
		{"a/b/c/d/e", "a/b/c/d/e"},
		{"file..name", "file..name"},
	}
	for _, tc := range safe {
		got, err := checkArchiveMember(tc.in)
		if err != nil {
			t.Errorf("checkArchiveMember(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("checkArchiveMember(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCheckLinkTargetRejectsEscapes(t *testing.T) {
	t.Parallel()

	unsafe := []struct{ member, clean, target string }{
		{"core/env", "core/env", "/vkai-panel/etc/.env"},
		{"core/env", "core/env", "../../etc/.env"},
		{"env", "env", "../etc/.env"},
		{"core/x", "core/x", `..\..\etc`},
		{"core/x", "core/x", ""},
		{"core/x", "core/x", "ok\x00/../.."},
	}
	for _, tc := range unsafe {
		err := checkLinkTarget(tc.member, tc.clean, tc.target)
		var unsafeErr *UnsafeArchiveError
		if !errors.As(err, &unsafeErr) {
			t.Errorf("checkLinkTarget(%q -> %q) = %v; want *UnsafeArchiveError", tc.member, tc.target, err)
		}
	}

	safe := []struct{ member, clean, target string }{
		{"core/current", "core/current", "vkai-api"},
		{"core/a/b", "core/a/b", "../c"},
		{"panel/link", "panel/link", "../core/vkai-api"},
	}
	for _, tc := range safe {
		if err := checkLinkTarget(tc.member, tc.clean, tc.target); err != nil {
			t.Errorf("checkLinkTarget(%q -> %q): unexpected error %v", tc.member, tc.target, err)
		}
	}
}

// TestExtractRefusesUnsafeArchives proves the check is actually wired into the
// extraction, and that nothing lands outside the destination when it fires.
func TestExtractRefusesUnsafeArchives(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		entries []tarEntry
	}{
		{
			name: "traversal out of the destination",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "../escaped.txt", Body: "owned"},
			},
		},
		{
			name: "absolute member path",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "/absolute.txt", Body: "owned"},
			},
		},
		{
			name: "symlink pointing at the panel config",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/env", Typeflag: tar.TypeSymlink, Linkname: "../../../etc/.env"},
			},
		},
		{
			name: "symlink with an absolute target",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/shadow", Typeflag: tar.TypeSymlink, Linkname: "/etc/shadow"},
			},
		},
		{
			name: "hard link escaping the destination",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/link", Typeflag: tar.TypeLink, Linkname: "../../../etc/.env"},
			},
		},
		{
			name: "device node",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/mem", Typeflag: tar.TypeChar},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			// A canary beside the destination: if extraction escapes by
			// one level this is what it would overwrite.
			canary := filepath.Join(base, "escaped.txt")
			mustWriteFile(t, canary, "original\n")

			archive := filepath.Join(base, "release.tar.gz")
			if err := os.WriteFile(archive, buildTarGz(t, tc.entries), 0o600); err != nil {
				t.Fatalf("write archive: %v", err)
			}
			dest := filepath.Join(base, "dest")

			err := extractTarGz(archive, dest, extractOptions{MaxBytes: 1 << 20, MaxEntries: 100})
			var unsafeErr *UnsafeArchiveError
			if !errors.As(err, &unsafeErr) {
				t.Fatalf("extractTarGz = %v, want *UnsafeArchiveError", err)
			}

			body, readErr := os.ReadFile(canary)
			if readErr != nil {
				t.Fatalf("read canary: %v", readErr)
			}
			if string(body) != "original\n" {
				t.Fatalf("extraction escaped the destination: canary now %q", body)
			}
			if exists(filepath.Join(base, "absolute.txt")) {
				t.Fatal("extraction wrote outside the destination")
			}
		})
	}
}

func TestExtractHappyPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive := filepath.Join(base, "release.tar.gz")
	if err := os.WriteFile(archive, releaseTarball(t, "1.1.0"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	dest := filepath.Join(base, "dest")

	if err := extractTarGz(archive, dest, extractOptions{MaxBytes: 1 << 20, MaxEntries: 100}); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dest, "VERSION"))
	if err != nil || strings.TrimSpace(string(body)) != "1.1.0" {
		t.Fatalf("VERSION = %q, %v", body, err)
	}
	info, err := os.Stat(filepath.Join(dest, "core", "vkai-api"))
	if err != nil {
		t.Fatalf("stat vkai-api: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("vkai-api lost its executable bit: mode %v", info.Mode().Perm())
	}
	if info.Mode().Perm()&0o022 != 0 {
		t.Errorf("vkai-api is group or world writable: mode %v", info.Mode().Perm())
	}
}

func TestSanitizeModeStripsPrivilegeBits(t *testing.T) {
	t.Parallel()
	got := sanitizeMode(os.FileMode(0o777) | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if got&os.ModeSetuid != 0 || got&os.ModeSetgid != 0 || got&os.ModeSticky != 0 {
		t.Errorf("sanitizeMode kept a privilege bit: %v", got)
	}
	if got.Perm()&0o022 != 0 {
		t.Errorf("sanitizeMode kept group/other write: %v", got.Perm())
	}
	if got.Perm()&0o700 != 0o700 {
		t.Errorf("sanitizeMode = %v, want the owner bits preserved", got.Perm())
	}
}

func TestExtractRejectsOversizedArchive(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive := filepath.Join(base, "release.tar.gz")
	entries := []tarEntry{{Name: "big", Body: strings.Repeat("a", 4096)}}
	if err := os.WriteFile(archive, buildTarGz(t, entries), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	err := extractTarGz(archive, filepath.Join(base, "dest"), extractOptions{MaxBytes: 1024, MaxEntries: 100})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("extractTarGz = %v, want a size-limit error", err)
	}
}

func TestExtractRejectsTooManyEntries(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive := filepath.Join(base, "release.tar.gz")
	var entries []tarEntry
	for i := 0; i < 10; i++ {
		entries = append(entries, tarEntry{Name: "f" + string(rune('0'+i)), Body: "x"})
	}
	if err := os.WriteFile(archive, buildTarGz(t, entries), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	err := extractTarGz(archive, filepath.Join(base, "dest"), extractOptions{MaxBytes: 1 << 20, MaxEntries: 3})
	if err == nil || !strings.Contains(err.Error(), "members") {
		t.Fatalf("extractTarGz = %v, want an entry-count error", err)
	}
}

// ------------------------------------------------------- checksum refusal

// TestRunRefusesChecksumMismatch is requirement 2: a tarball that does not hash
// to what the manifest promised is deleted without ever being opened, and the
// installation is left exactly as it was.
func TestRunRefusesChecksumMismatch(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	m := env.publish("1.1.0", "")
	// Republish the same release with a checksum that does not match the
	// bytes being served - a tampered mirror, or a corrupted CDN object.
	tampered := m
	tampered.SHA256 = sha256Hex([]byte("something else entirely"))
	env.publishManifests(tampered)

	res, err := env.u.Run(context.Background())

	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Run error = %v, want *ChecksumMismatchError", err)
	}
	if mismatch.Expected != tampered.SHA256 || mismatch.Actual != m.SHA256 {
		t.Errorf("mismatch = %+v, want expected %s actual %s", mismatch, tampered.SHA256, m.SHA256)
	}

	// Nothing was extracted, nothing was switched.
	if res.Switched || res.Succeeded {
		t.Errorf("result = %+v, want nothing switched", res)
	}
	if exists(filepath.Join(env.root, "releases", "1.1.0")) {
		t.Error("an unverified archive was extracted into the releases directory")
	}
	if env.currentTarget() != "1.0.0" {
		t.Errorf("current = %s, want 1.0.0", env.currentTarget())
	}
	if len(env.runner.callsMatching("systemctl restart")) != 0 {
		t.Error("services were restarted despite the checksum mismatch")
	}

	// The downloaded file is gone.
	tmpEntries, _ := os.ReadDir(filepath.Join(env.root, "tmp"))
	for _, e := range tmpEntries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			t.Errorf("the unverified download %s was left on disk", e.Name())
		}
	}

	if !env.hasStep(StepVerify, StatusFailed) {
		t.Errorf("expected the verify step to be reported as failed, got %v", env.steps())
	}
	if env.hasStep(StepStage, StatusStarted) {
		t.Error("staging started for an archive that failed verification")
	}
}

// The verification reads the file that is about to be extracted, not just the
// stream that produced it. A file swapped in /vkai-panel/tmp between the two
// must be reported as a swap rather than as a bad download.
func TestVerifyChecksumRehashesTheFileOnDisk(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)

	body := []byte("the real release")
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	m := Manifest{TarballURL: "https://example.test/r.tar.gz", SHA256: sha256Hex(body)}

	// Matching manifest and matching stream: accepted, file kept.
	if err := env.u.verifyChecksum(path, sha256Hex(body), m); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
	if !exists(path) {
		t.Fatal("a verified archive was deleted")
	}

	// The stream hashed to something else: the file was replaced underneath.
	err := env.u.verifyChecksum(path, sha256Hex([]byte("what we downloaded")), m)
	if err == nil || !strings.Contains(err.Error(), "changed between being written and being verified") {
		t.Fatalf("verifyChecksum = %v, want a swap to be named as one", err)
	}
	if exists(path) {
		t.Fatal("the suspect archive was left on disk")
	}
}
