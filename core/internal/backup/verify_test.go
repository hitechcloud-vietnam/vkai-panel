package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildArchive(t *testing.T, source string, kind string, key *Key) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := CreateArchive(context.Background(), &buf, CreateOptions{Source: source, Kind: kind, Key: key}); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	return buf.Bytes()
}

func TestVerifyPassesOnAGoodArchive(t *testing.T) {
	source := writeTree(t, map[string]string{
		"index.php":         "<?php",
		"wp-config.php":     "define('DB_NAME', 'shop');",
		"uploads/photo.jpg": strings.Repeat("\xff\xd8", 5000),
	})
	key := testKey(t, 101)
	archive := buildArchive(t, source, KindWebsite, key)

	scratch := t.TempDir()
	result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
		ScratchParent: scratch,
		Key:           key,
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed() {
		t.Fatalf("a good archive failed verification: %s", result.Summary())
	}
	if result.FilesRestored != 3 || result.ChecksumsChecked != 3 {
		t.Fatalf("verified %d files, %d checksums; want 3 and 3", result.FilesRestored, result.ChecksumsChecked)
	}
	if result.ArchiveSHA256 == "" || result.ArchiveBytes == 0 {
		t.Fatal("the pass did not record which bytes it checked")
	}
	if sum := sha256.Sum256(archive); result.ArchiveSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("the recorded archive digest is not the digest of the archive")
	}

	// The scratch directory must be gone: verification is not allowed to leave
	// a second copy of customer data lying around.
	entries, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatalf("read scratch: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("verification left %v behind", entries)
	}
}

func TestVerifyDetectsACorruptedArchive(t *testing.T) {
	source := writeTree(t, map[string]string{
		"a.txt": strings.Repeat("a", 40000),
		"b.txt": strings.Repeat("b", 40000),
	})

	t.Run("encrypted archive with a flipped byte", func(t *testing.T) {
		key := testKey(t, 111)
		archive := buildArchive(t, source, KindFiles, key)
		archive[len(archive)/2] ^= 0x40

		result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
			ScratchParent: t.TempDir(), Key: key,
		})
		if err != nil {
			t.Fatalf("Verify returned an error instead of a verdict: %v", err)
		}
		if result.Passed() {
			t.Fatal("a corrupted encrypted archive passed verification")
		}
		if !strings.Contains(result.Summary(), "could not be restored") {
			t.Fatalf("the verdict does not explain the failure: %s", result.Summary())
		}
	})

	t.Run("plain archive with a flipped byte", func(t *testing.T) {
		archive := buildArchive(t, source, KindFiles, nil)
		archive[len(archive)/2] ^= 0x40

		result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
			ScratchParent: t.TempDir(),
		})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if result.Passed() {
			t.Fatal("a corrupted archive passed verification")
		}
	})

	t.Run("truncated archive", func(t *testing.T) {
		key := testKey(t, 112)
		archive := buildArchive(t, source, KindFiles, key)
		result, err := Verify(context.Background(), bytes.NewReader(archive[:len(archive)-60]), VerifyOptions{
			ScratchParent: t.TempDir(), Key: key,
		})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if result.Passed() {
			t.Fatal("a truncated archive passed verification")
		}
	})

	t.Run("archive encrypted under a key nobody has any more", func(t *testing.T) {
		archive := buildArchive(t, source, KindFiles, testKey(t, 113))
		result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
			ScratchParent: t.TempDir(), Key: testKey(t, 114),
		})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if result.Passed() {
			t.Fatal("an archive that cannot be decrypted passed verification")
		}
		if !strings.Contains(result.Summary(), "different key") {
			t.Fatalf("the verdict does not name the problem: %s", result.Summary())
		}
	})
}

// TestVerifyRecomputesChecksumsRatherThanTrustingTheManifest is the test that
// says the pass is worth having. The archive is well formed, decrypts, parses
// and restores; only the recorded checksum of one file is a lie. A pass that
// merely compared file counts, or that trusted the manifest it was handed,
// would report success on it.
func TestVerifyRecomputesChecksumsRatherThanTrustingTheManifest(t *testing.T) {
	good := sha256.Sum256([]byte("the real content"))
	m := &Manifest{
		Version: ManifestVersion, Kind: KindFiles, Source: "/somewhere", CreatedAt: time.Now().UTC(),
		FileCount: 2, TotalSize: int64(len("the real content") + len("tampered")),
		Entries: []Entry{
			{Path: "honest.txt", Type: EntryFile, Size: int64(len("the real content")), Mode: 0o644, SHA256: hex.EncodeToString(good[:])},
			// Same length as the body below, so only a real hash catches it.
			{Path: "tampered.txt", Type: EntryFile, Size: 8, Mode: 0o644, SHA256: strings.Repeat("00", 32)},
		},
	}
	archive := buildHostileArchive(t, m,
		[]tar.Header{
			{Name: "honest.txt", Mode: 0o644, Typeflag: tar.TypeReg},
			{Name: "tampered.txt", Mode: 0o644, Typeflag: tar.TypeReg},
		},
		map[string]string{"honest.txt": "the real content", "tampered.txt": "tampered"})

	result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{ScratchParent: t.TempDir()})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed() {
		t.Fatal("an archive whose recorded checksum does not describe its contents passed verification")
	}
	if len(result.ChecksumMismatches) != 1 || !strings.Contains(result.ChecksumMismatches[0], "tampered.txt") {
		t.Fatalf("the mismatch was not identified: %+v", result.ChecksumMismatches)
	}
	if result.ChecksumsChecked != 2 {
		t.Fatalf("checked %d checksums, want 2", result.ChecksumsChecked)
	}
}

func TestVerifyImportsADatabaseDump(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "shop.sql")
	if err := os.WriteFile(dump, []byte("CREATE TABLE orders (id int);\n"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	key := testKey(t, 121)
	archive := buildArchive(t, dump, KindDatabase, key)

	t.Run("the import succeeds", func(t *testing.T) {
		var importedPath string
		result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
			ScratchParent: t.TempDir(),
			Key:           key,
			DatabaseImport: func(ctx context.Context, path string) error {
				importedPath = path
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !strings.Contains(string(data), "CREATE TABLE orders") {
					return errors.New("the dump handed to the importer is not the dump that was backed up")
				}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if !result.Passed() {
			t.Fatalf("verification failed: %s", result.Summary())
		}
		if !result.DatabaseChecked || !result.DatabaseImported {
			t.Fatalf("the database check was not recorded: %+v", result)
		}
		if !strings.HasSuffix(importedPath, "shop.sql") {
			t.Fatalf("the importer was handed %q", importedPath)
		}
	})

	t.Run("the import fails", func(t *testing.T) {
		result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
			ScratchParent: t.TempDir(),
			Key:           key,
			DatabaseImport: func(ctx context.Context, path string) error {
				return errors.New(`syntax error at or near "CREAT"`)
			},
		})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if result.Passed() {
			t.Fatal("a dump that does not import passed verification")
		}
		if result.DatabaseImported {
			t.Fatal("the result claims the dump imported")
		}
		if !strings.Contains(result.DatabaseError, "syntax error") {
			t.Fatalf("the importer's error was not recorded: %q", result.DatabaseError)
		}
	})
}

func TestVerifyKeepsTheScratchDirectoryOnlyWhenAsked(t *testing.T) {
	source := writeTree(t, map[string]string{"a.txt": "x"})
	archive := buildArchive(t, source, KindFiles, nil)
	archive[len(archive)-5] ^= 0xff

	scratch := t.TempDir()
	result, err := Verify(context.Background(), bytes.NewReader(archive), VerifyOptions{
		ScratchParent: scratch, KeepScratchOnFailure: true,
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed() {
		t.Skip("the corruption landed somewhere the format tolerates; nothing to assert")
	}
	if result.ScratchDir == "" {
		t.Fatal("the failed pass did not report where it left the restored files")
	}
	if _, err := os.Stat(result.ScratchDir); err != nil {
		t.Fatalf("the scratch directory was removed despite KeepScratchOnFailure: %v", err)
	}
}

func TestVerifyNeedsScratchSpace(t *testing.T) {
	if _, err := Verify(context.Background(), bytes.NewReader(nil), VerifyOptions{}); err == nil {
		t.Fatal("verification ran without a scratch directory")
	}
}
