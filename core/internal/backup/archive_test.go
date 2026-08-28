package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// writeTree materialises a map of relative path to contents and returns the
// root. A path ending in "/" is a directory.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", rel, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = "-> " + target
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func TestArchiveRoundTrip(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		name := "plain"
		var key *Key
		if encrypted {
			name = "encrypted"
			key = testKey(t, 21)
		}

		t.Run(name, func(t *testing.T) {
			source := writeTree(t, map[string]string{
				"index.php":            "<?php echo 'hello';",
				"wp-content/uploads/a": "binary-ish\x00\x01\x02",
				"wp-content/empty/":    "",
				"deep/a/b/c/file.txt":  strings.Repeat("x", 100000),
			})
			if err := os.Symlink("../uploads/a", filepath.Join(source, "wp-content", "link")); err != nil {
				t.Fatalf("symlink: %v", err)
			}

			var archive bytes.Buffer
			result, err := CreateArchive(context.Background(), &archive, CreateOptions{
				Source: source,
				Kind:   KindWebsite,
				Key:    key,
			})
			if err != nil {
				t.Fatalf("CreateArchive: %v", err)
			}
			if result.Encrypted != encrypted {
				t.Fatalf("result says encrypted=%v", result.Encrypted)
			}
			if result.Manifest.FileCount != 3 {
				t.Fatalf("manifest counted %d files, want 3", result.Manifest.FileCount)
			}
			if result.Manifest.LinkCount != 1 {
				t.Fatalf("manifest counted %d symlinks, want 1", result.Manifest.LinkCount)
			}
			if result.SHA256 == "" || result.Bytes == 0 {
				t.Fatal("the archive digest or size was not recorded")
			}

			dest := filepath.Join(t.TempDir(), "restored")
			plan, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
				Dest: dest,
				Key:  key,
			})
			if err != nil {
				t.Fatalf("ExtractArchive: %v", err)
			}
			if !plan.Applied {
				t.Fatal("plan says the restore was not applied")
			}
			if plan.FilesWritten != 3 {
				t.Fatalf("restore wrote %d files, want 3", plan.FilesWritten)
			}

			want := readTree(t, source)
			got := readTree(t, dest)
			if len(want) != len(got) {
				t.Fatalf("restored tree has %d entries, source has %d", len(got), len(want))
			}
			for rel, content := range want {
				if got[rel] != content {
					t.Fatalf("restored %s does not match the source", rel)
				}
			}
			if _, err := os.Stat(filepath.Join(dest, "wp-content", "empty")); err != nil {
				t.Fatalf("the empty directory was not restored: %v", err)
			}
		})
	}
}

func TestArchiveOfASingleFile(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "shop.sql")
	if err := os.WriteFile(dump, []byte("CREATE TABLE orders (id int);\n"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	key := testKey(t, 31)
	var archive bytes.Buffer
	result, err := CreateArchive(context.Background(), &archive, CreateOptions{
		Source: dump,
		Kind:   KindDatabase,
		Key:    key,
	})
	if err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	if result.Manifest.FileCount != 1 || result.Manifest.Entries[0].Path != "shop.sql" {
		t.Fatalf("a single file source produced %+v", result.Manifest.Entries)
	}

	dest := filepath.Join(t.TempDir(), "restored")
	if _, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest: dest,
		Key:  key,
	}); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "shop.sql"))
	if err != nil {
		t.Fatalf("read restored dump: %v", err)
	}
	if string(data) != "CREATE TABLE orders (id int);\n" {
		t.Fatalf("restored dump is %q", string(data))
	}
}

func TestDryRunReportsOverwritesAndWritesNothing(t *testing.T) {
	source := writeTree(t, map[string]string{
		"index.php":       "NEW index",
		"config.php":      "NEW config",
		"assets/logo.png": "NEW logo",
		"only-in-backup":  "NEW file",
	})

	key := testKey(t, 41)
	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{
		Source: source, Kind: KindWebsite, Key: key,
	}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	// The destination is a live site: one file differs, one is byte identical,
	// one is missing, and one is there that the backup has never heard of.
	dest := writeTree(t, map[string]string{
		"index.php":         "LIVE index, about to be lost",
		"config.php":        "NEW config",
		"assets/logo.png":   "LIVE logo, about to be lost",
		"uploads/photo.jpg": "LIVE upload the backup does not contain",
	})
	before := readTree(t, dest)

	plan, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest:              dest,
		Key:               key,
		DryRun:            true,
		SurveyDestination: true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}

	if !plan.DryRun || plan.Applied {
		t.Fatal("the plan does not say it was a dry run")
	}
	if after := readTree(t, dest); len(after) != len(before) {
		t.Fatal("the dry run changed the destination")
	}
	for rel, content := range before {
		if readTree(t, dest)[rel] != content {
			t.Fatalf("the dry run modified %s", rel)
		}
	}

	overwritten := map[string]PlanEntry{}
	for _, e := range plan.Overwrites {
		overwritten[e.Path] = e
	}
	if len(overwritten) != 3 {
		t.Fatalf("plan reports %d overwrites, want 3: %+v", len(overwritten), plan.Overwrites)
	}
	if !overwritten["config.php"].Identical {
		t.Fatal("config.php is byte identical but the plan does not say so")
	}
	for _, name := range []string{"index.php", "assets/logo.png"} {
		e := overwritten[name]
		if e.Identical {
			t.Fatalf("%s differs but the plan calls it identical", name)
		}
		if e.ExistingSHA256 == e.IncomingSHA256 && e.ExistingSize == e.IncomingSize {
			t.Fatalf("%s: the plan does not distinguish the two versions", name)
		}
	}
	if len(plan.ChangedOverwrites()) != 2 {
		t.Fatalf("ChangedOverwrites returned %d, want 2", len(plan.ChangedOverwrites()))
	}

	if len(plan.NewFiles) != 1 || plan.NewFiles[0] != "only-in-backup" {
		t.Fatalf("plan reports new files %v, want [only-in-backup]", plan.NewFiles)
	}
	if len(plan.ExistingNotInArchive) != 1 || plan.ExistingNotInArchive[0] != "uploads/photo.jpg" {
		t.Fatalf("plan reports untouched existing files %v, want [uploads/photo.jpg]", plan.ExistingNotInArchive)
	}
}

func TestRestoreRefusesToOverwriteUnlessAsked(t *testing.T) {
	source := writeTree(t, map[string]string{"index.php": "from the backup"})
	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{Source: source, Kind: KindWebsite}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	dest := writeTree(t, map[string]string{"index.php": "the live site"})

	plan, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{Dest: dest})
	if err == nil {
		t.Fatal("a restore that would overwrite a live file went ahead without being asked")
	}
	if plan == nil || len(plan.ChangedOverwrites()) != 1 {
		t.Fatalf("the refusal did not come back with the plan: %+v", plan)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "index.php")); string(data) != "the live site" {
		t.Fatal("the live file was modified by a restore that then refused")
	}

	if _, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest: dest, AllowOverwrite: true,
	}); err != nil {
		t.Fatalf("restore with overwrite allowed: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "index.php")); string(data) != "from the backup" {
		t.Fatal("the restore did not replace the file")
	}
}

func TestRestoreWithTheWrongKeyWritesNothing(t *testing.T) {
	source := writeTree(t, map[string]string{"index.php": "secret"})
	right, wrong := testKey(t, 51), testKey(t, 52)

	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{
		Source: source, Kind: KindWebsite, Key: right,
	}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "restored")
	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest: dest, Key: wrong,
	})
	if !errors.Is(err, ErrWrongKey) {
		t.Fatalf("expected ErrWrongKey, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("the destination directory was created before the key was checked")
	}
}

func TestRestoreOfAnEncryptedArchiveWithoutAKeyFails(t *testing.T) {
	source := writeTree(t, map[string]string{"a.txt": "x"})
	key := testKey(t, 61)
	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{
		Source: source, Kind: KindFiles, Key: key,
	}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest: filepath.Join(t.TempDir(), "restored"),
	})
	if err == nil {
		t.Fatal("an encrypted archive was opened without a key")
	}
	// The message has to say "encrypted" and name the key. Letting gzip fail
	// with "invalid header" would send an operator looking for corruption when
	// the archive is fine and it is the key that is missing.
	if !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if !strings.Contains(err.Error(), key.ID()) {
		t.Fatalf("the error does not name the key the archive needs: %v", err)
	}

	// And a plain archive still opens with no key at all: the peek must not
	// have consumed the bytes it looked at.
	plainSource := writeTree(t, map[string]string{"a.txt": "plain contents"})
	var plain bytes.Buffer
	if _, err := CreateArchive(context.Background(), &plain, CreateOptions{Source: plainSource, Kind: KindFiles}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "plain")
	if _, err := ExtractArchive(context.Background(), bytes.NewReader(plain.Bytes()), ExtractOptions{Dest: dest}); err != nil {
		t.Fatalf("an unencrypted archive would not open: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "a.txt")); string(data) != "plain contents" {
		t.Fatalf("the unencrypted archive restored %q", string(data))
	}
}

// buildHostileArchive writes a tar.gz whose manifest and payload are chosen by
// the test. It is how the traversal and manifest-mismatch defences are driven:
// CreateArchive would never produce these, which is exactly why the restore
// side has to be tested against something that is not CreateArchive.
func buildHostileArchive(t *testing.T, m *Manifest, members []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)

	manifestJSON, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: ManifestPath, Mode: 0o600, Size: int64(len(manifestJSON)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("manifest header: %v", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		t.Fatalf("manifest body: %v", err)
	}

	for i := range members {
		h := members[i]
		body := bodies[h.Name]
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("header %s: %v", h.Name, err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatalf("body %s: %v", h.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return raw.Bytes()
}

func TestRestoreRefusesTraversalMembers(t *testing.T) {
	for _, name := range []string{"../escaped", "/etc/cron.d/pwn", "a/../../escaped", `..\escaped`} {
		m := &Manifest{
			Version: ManifestVersion, Kind: KindFiles, CreatedAt: time.Now(),
			FileCount: 1, TotalSize: 3,
			Entries: []Entry{{Path: name, Type: EntryFile, Size: 3, Mode: 0o644, SHA256: "unused"}},
		}
		archive := buildHostileArchive(t, m,
			[]tar.Header{{Name: name, Mode: 0o644, Typeflag: tar.TypeReg}},
			map[string]string{name: "pwn"})

		dest := filepath.Join(t.TempDir(), "restored")
		_, err := ExtractArchive(context.Background(), bytes.NewReader(archive), ExtractOptions{
			Dest: dest, AllowOverwrite: true,
		})
		var unsafe *UnsafeMemberError
		if !errors.As(err, &unsafe) {
			t.Fatalf("member %q was not refused: %v", name, err)
		}
	}
}

func TestRestoreRefusesAMemberTheManifestDoesNotList(t *testing.T) {
	m := &Manifest{
		Version: ManifestVersion, Kind: KindFiles, CreatedAt: time.Now(),
		FileCount: 1, TotalSize: 4,
		Entries: []Entry{{Path: "expected.txt", Type: EntryFile, Size: 4, Mode: 0o644, SHA256: "x"}},
	}
	archive := buildHostileArchive(t, m,
		[]tar.Header{
			{Name: "expected.txt", Mode: 0o644, Typeflag: tar.TypeReg},
			{Name: "smuggled.php", Mode: 0o644, Typeflag: tar.TypeReg},
		},
		map[string]string{"expected.txt": "good", "smuggled.php": "<?php"})

	dest := filepath.Join(t.TempDir(), "restored")
	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive), ExtractOptions{
		Dest: dest, AllowOverwrite: true,
	})
	var unsafe *UnsafeMemberError
	if !errors.As(err, &unsafe) {
		t.Fatalf("a member absent from the manifest was accepted: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "smuggled.php")); statErr == nil {
		t.Fatal("the smuggled member was written to disk")
	}
}

func TestRestoreReportsAManifestEntryTheArchiveDoesNotContain(t *testing.T) {
	m := &Manifest{
		Version: ManifestVersion, Kind: KindFiles, CreatedAt: time.Now(),
		FileCount: 2, TotalSize: 8,
		Entries: []Entry{
			{Path: "present.txt", Type: EntryFile, Size: 4, Mode: 0o644, SHA256: "x"},
			{Path: "absent.txt", Type: EntryFile, Size: 4, Mode: 0o644, SHA256: "y"},
		},
	}
	archive := buildHostileArchive(t, m,
		[]tar.Header{{Name: "present.txt", Mode: 0o644, Typeflag: tar.TypeReg}},
		map[string]string{"present.txt": "here"})

	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive), ExtractOptions{
		Dest: filepath.Join(t.TempDir(), "restored"), AllowOverwrite: true,
	})
	var mismatch *ManifestMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected a manifest mismatch, got %v", err)
	}
	if len(mismatch.Missing) != 1 || mismatch.Missing[0] != "absent.txt" {
		t.Fatalf("mismatch names %v", mismatch.Missing)
	}
}

func TestRestoreRefusesSetuidMembers(t *testing.T) {
	m := &Manifest{
		Version: ManifestVersion, Kind: KindFiles, CreatedAt: time.Now(),
		FileCount: 1, TotalSize: 2,
		Entries: []Entry{{Path: "rootshell", Type: EntryFile, Size: 2, Mode: 0o755, SHA256: "z"}},
	}
	archive := buildHostileArchive(t, m,
		// 04755: the POSIX setuid bit, as a tar header carries it.
		[]tar.Header{{Name: "rootshell", Mode: 0o4755, Typeflag: tar.TypeReg}},
		map[string]string{"rootshell": "sh"})

	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive), ExtractOptions{
		Dest: filepath.Join(t.TempDir(), "restored"), AllowOverwrite: true,
	})
	var unsafe *UnsafeMemberError
	if !errors.As(err, &unsafe) {
		t.Fatalf("a setuid member was accepted: %v", err)
	}
}

func TestRestoreWillNotWriteThroughAPreExistingSymlink(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "shadow")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	source := writeTree(t, map[string]string{"etc/shadow": "attacker content"})
	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{Source: source, Kind: KindFiles}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	dest := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "etc")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{
		Dest: dest, AllowOverwrite: true,
	})
	if err == nil {
		t.Fatal("the restore followed a symlink that was already in the destination")
	}
	if data, _ := os.ReadFile(victim); string(data) != "original" {
		t.Fatal("a file outside the destination was overwritten")
	}
}

func TestCancellingABackupStopsIt(t *testing.T) {
	source := writeTree(t, map[string]string{
		"big-1": strings.Repeat("a", 2_000_000),
		"big-2": strings.Repeat("b", 2_000_000),
		"big-3": strings.Repeat("c", 2_000_000),
	})

	tracker, ctx := NewTracker(context.Background())
	tracker.OnUpdate(func(p Progress) {
		if p.Phase == PhaseArchiving && p.BytesDone > 0 {
			tracker.Cancel()
		}
	})

	_, err := CreateArchive(ctx, io.Discard, CreateOptions{
		Source: source, Kind: KindFiles, Key: testKey(t, 71), Tracker: tracker,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled backup returned %v, want context.Canceled", err)
	}
}

func TestProgressReachesTheEnd(t *testing.T) {
	source := writeTree(t, map[string]string{
		"a": strings.Repeat("a", 5000),
		"b": strings.Repeat("b", 5000),
	})

	tracker, ctx := NewTracker(context.Background())
	var phases []string
	tracker.OnUpdate(func(p Progress) {
		if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
			phases = append(phases, p.Phase)
		}
	})

	if _, err := CreateArchive(ctx, io.Discard, CreateOptions{
		Source: source, Kind: KindFiles, Tracker: tracker,
	}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	tracker.SetPhase(PhaseDone, "")

	final := tracker.Snapshot()
	if final.Percent != 100 {
		t.Fatalf("final progress is %.1f%%", final.Percent)
	}
	if final.Cancellable {
		t.Fatal("a finished operation still reports itself as cancellable")
	}
	sort.Strings(phases)
	if !containsAll(phases, []string{PhaseArchiving, PhaseDone, PhaseScanning}) {
		t.Fatalf("phases seen: %v", phases)
	}
}

func containsAll(have, want []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestReadManifestDoesNotNeedTheWholeArchive(t *testing.T) {
	source := writeTree(t, map[string]string{"a.txt": "hello", "b/c.txt": "world"})
	key := testKey(t, 81)

	var archive bytes.Buffer
	if _, err := CreateArchive(context.Background(), &archive, CreateOptions{
		Source: source, Kind: KindConfig, Key: key,
	}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}

	m, err := ReadManifest(bytes.NewReader(archive.Bytes()), key)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.Kind != KindConfig || m.FileCount != 2 {
		t.Fatalf("manifest is %+v", m)
	}
	if _, ok := m.Lookup("b/c.txt"); !ok {
		t.Fatal("manifest does not list b/c.txt")
	}
}

// TestArchiveOfADirectoryWhoseOnlyFileSharesItsName covers the case that broke
// the first version of the archive writer: it decided a source was a single
// file by comparing the one entry's name to the source's base name, and a
// directory /x/foo containing a file called "foo" satisfies that test. The
// writer then tried to read the directory as a file.
func TestArchiveOfADirectoryWhoseOnlyFileSharesItsName(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "foo")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "foo"), []byte("contents"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var archive bytes.Buffer
	result, err := CreateArchive(context.Background(), &archive, CreateOptions{Source: source, Kind: KindFiles})
	if err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	if result.Manifest.SingleFile {
		t.Fatal("a directory was archived as a single file")
	}

	dest := filepath.Join(t.TempDir(), "restored")
	if _, err := ExtractArchive(context.Background(), bytes.NewReader(archive.Bytes()), ExtractOptions{Dest: dest}); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "foo"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "contents" {
		t.Fatalf("restored %q", string(data))
	}
}
