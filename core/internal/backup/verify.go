package backup

// Proving a backup is restorable.
//
// This is the part of a backup system that is normally skipped, and it is the
// part that decides whether the rest of it was worth writing. A backup that has
// never been restored is not a backup, it is a hypothesis - and it is worse
// than having none, because the operator has stopped worrying.
//
// WHAT THIS PASS DOES
//
// It takes a real archive from a real destination and restores it into a
// scratch directory that no live service uses. Then it reads every file it just
// wrote back off the disk, recomputes the SHA-256 of each one, and compares it
// against the manifest that travelled inside the archive. It counts the files
// and the bytes. If the archive is a database dump and an importer was
// supplied, it runs the dump into a throwaway database and reports whether the
// import succeeded. Then it deletes the scratch directory and records what
// happened.
//
// WHAT IT THEREFORE PROVES
//
//   - The bytes at the destination are the bytes that were written: they
//     decrypted under the operator's key, they decompressed, the tar parsed,
//     and every file's checksum matched. Bit rot, a truncated upload, a
//     half-written object, an archive encrypted under a key nobody still has -
//     all of them fail here rather than during an incident.
//   - The archive is internally consistent: the manifest and the payload are
//     the same list of files, in both directions.
//   - For a database backup with an importer, the dump is syntactically valid
//     and a database server accepts it end to end.
//
// WHAT IT DOES NOT PROVE - stated plainly, because a verification pass that is
// oversold is worse than none:
//
//   - It does not prove the backup contains the right data. If the site was
//     already broken, or the wrong directory was configured as the source, this
//     pass will happily confirm that the wrong thing was archived perfectly.
//   - It does not prove a restore into the LIVE location will work. Scratch
//     space is not the document root: permissions, ownership, SELinux labels,
//     free space and a running web server are all different there.
//   - For a database, a successful import is not a working application. It says
//     the schema and the rows loaded, not that the application starts.
//   - It says nothing about backups other than the one it checked, and nothing
//     about this one at any later moment. It is evidence with a timestamp.
//
// The result is written to the database and surfaced through the API precisely
// so that "last verified" is a date an operator can look at, rather than a
// belief.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Verification outcomes.
const (
	VerifyPassed = "passed"
	VerifyFailed = "failed"
)

// DatabaseImporter runs a restored SQL dump into a throwaway database. It
// returns nil when the import succeeded.
//
// It is a function rather than an interface because the only implementation
// that matters lives in the service layer, where the database credentials are,
// and because a test needs to be able to fail it on demand.
type DatabaseImporter func(ctx context.Context, dumpPath string) error

// VerifyOptions controls one verification pass.
type VerifyOptions struct {
	// ScratchParent is the directory the throwaway restore happens under. It
	// must not be inside a live document root, and it needs room for the
	// uncompressed archive.
	ScratchParent string
	// Key decrypts the archive.
	Key *Key
	// DatabaseImport, when set and when the archive is a database backup, is
	// run against the restored dump.
	DatabaseImport DatabaseImporter
	// KeepScratchOnFailure leaves the restored files behind when the pass
	// fails, so an operator can look at them. On success they always go.
	KeepScratchOnFailure bool
	Tracker              *Tracker
}

// VerifyResult is the evidence.
type VerifyResult struct {
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`

	ArchiveBytes  int64  `json:"archive_bytes"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Kind          string `json:"kind"`
	ArchiveSource string `json:"archive_source"`

	FilesExpected int   `json:"files_expected"`
	FilesRestored int   `json:"files_restored"`
	BytesExpected int64 `json:"bytes_expected"`
	BytesRestored int64 `json:"bytes_restored"`

	ChecksumsChecked   int      `json:"checksums_checked"`
	ChecksumMismatches []string `json:"checksum_mismatches,omitempty"`
	MissingFiles       []string `json:"missing_files,omitempty"`
	UnexpectedFiles    []string `json:"unexpected_files,omitempty"`

	DatabaseChecked  bool   `json:"database_checked"`
	DatabaseImported bool   `json:"database_imported"`
	DatabaseError    string `json:"database_error,omitempty"`

	// Failures is the list an operator reads first: every reason this pass did
	// not pass, in the order they were found.
	Failures []string `json:"failures,omitempty"`
	// ScratchDir is set only when the scratch directory was deliberately left
	// behind after a failure.
	ScratchDir string `json:"scratch_dir,omitempty"`
}

// Passed reports whether the archive restored cleanly.
func (r *VerifyResult) Passed() bool { return r != nil && r.Status == VerifyPassed }

// Summary is one line for a log or a notification.
func (r *VerifyResult) Summary() string {
	if r == nil {
		return "no verification has been run"
	}
	if r.Passed() {
		line := fmt.Sprintf("restored %d file(s), %d byte(s); %d checksum(s) matched",
			r.FilesRestored, r.BytesRestored, r.ChecksumsChecked)
		if r.DatabaseChecked {
			line += "; the database dump imported"
		}
		return line
	}
	if len(r.Failures) == 0 {
		return "verification failed"
	}
	return "verification failed: " + strings.Join(r.Failures, "; ")
}

func (r *VerifyResult) fail(format string, args ...any) {
	r.Status = VerifyFailed
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

// Verify restores an archive into scratch space and checks it.
//
// A failed verification is a RESULT, not an error: it is the answer to the
// question that was asked, and it has to be recorded. The error return is
// reserved for not being able to run the check at all - no scratch space, no
// key configured - because that is the case where there is nothing to record.
func Verify(ctx context.Context, src io.Reader, opts VerifyOptions) (*VerifyResult, error) {
	if src == nil {
		return nil, errors.New("backup: nothing to verify")
	}

	parent := opts.ScratchParent
	if strings.TrimSpace(parent) == "" {
		return nil, errors.New("backup: verification needs a scratch directory")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("backup: cannot create the verification scratch area %q: %w", parent, err)
	}
	scratch, err := os.MkdirTemp(parent, "verify-*")
	if err != nil {
		return nil, fmt.Errorf("backup: cannot create a verification scratch directory: %w", err)
	}

	result := &VerifyResult{Status: VerifyPassed, StartedAt: time.Now().UTC()}
	keepScratch := false
	defer func() {
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
		if keepScratch {
			result.ScratchDir = scratch
			return
		}
		_ = os.RemoveAll(scratch)
	}()

	opts.Tracker.SetPhase(PhaseVerifying, "restoring into scratch space")

	// Digest the archive as it is consumed, so the pass records WHICH bytes it
	// checked. Without it "verified" cannot be tied to a specific object, and a
	// later re-upload would silently inherit the old verdict.
	digest := newHashWriter(io.Discard)
	counted := io.TeeReader(src, digest)

	plan, extractErr := ExtractArchive(ctx, counted, ExtractOptions{
		Dest:           scratch,
		Key:            opts.Key,
		AllowOverwrite: true,
		Tracker:        opts.Tracker,
	})
	// Drain whatever is left so the digest covers the whole object even when
	// extraction stopped early; a short read here is not itself a failure.
	_, _ = io.Copy(io.Discard, counted)
	result.ArchiveBytes = digest.n
	result.ArchiveSHA256 = digest.sum()

	if extractErr != nil {
		if errors.Is(extractErr, context.Canceled) || errors.Is(extractErr, context.DeadlineExceeded) {
			return nil, extractErr
		}
		result.fail("the archive could not be restored: %v", extractErr)
		keepScratch = opts.KeepScratchOnFailure
		return result, nil
	}

	manifest := plan.Manifest()
	if manifest == nil {
		result.fail("the archive carried no manifest")
		keepScratch = opts.KeepScratchOnFailure
		return result, nil
	}

	result.Kind = manifest.Kind
	result.ArchiveSource = manifest.Source
	result.FilesExpected = manifest.FileCount
	result.BytesExpected = manifest.TotalSize

	opts.Tracker.SetPhase(PhaseVerifying, "recomputing checksums")
	opts.Tracker.SetTotals(manifest.FileCount, manifest.TotalSize)

	if err := checkRestoredFiles(ctx, scratch, manifest, result, opts.Tracker); err != nil {
		return nil, err
	}

	if manifest.Kind == KindDatabase && opts.DatabaseImport != nil {
		opts.Tracker.SetPhase(PhaseVerifying, "importing the dump into a scratch database")
		result.DatabaseChecked = true
		dump, err := findSQLDump(scratch, manifest)
		if err != nil {
			result.DatabaseError = err.Error()
			result.fail("no SQL dump to import: %v", err)
		} else if err := opts.DatabaseImport(ctx, dump); err != nil {
			result.DatabaseError = err.Error()
			result.fail("the dump did not import: %v", err)
		} else {
			result.DatabaseImported = true
		}
	}

	keepScratch = !result.Passed() && opts.KeepScratchOnFailure
	return result, nil
}

// checkRestoredFiles walks what was written and holds it to the manifest in
// both directions: nothing in the manifest missing, nothing on disk extra,
// every checksum recomputed from the bytes on disk.
func checkRestoredFiles(ctx context.Context, scratch string, m *Manifest, result *VerifyResult, tracker *Tracker) error {
	expected := make(map[string]Entry, m.FileCount)
	for _, entry := range m.FileEntries() {
		expected[entry.Path] = entry
	}

	found := make(map[string]struct{}, len(expected))
	err := filepath.WalkDir(scratch, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Symlinks are restored but carry no checksum: their content is the
		// target string, which the manifest already records verbatim.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(scratch, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		entry, known := expected[rel]
		if !known {
			result.UnexpectedFiles = append(result.UnexpectedFiles, rel)
			return nil
		}
		found[rel] = struct{}{}

		result.FilesRestored++
		result.BytesRestored += info.Size()

		if info.Size() != entry.Size {
			result.ChecksumMismatches = append(result.ChecksumMismatches,
				fmt.Sprintf("%s: restored %d bytes, manifest says %d", rel, info.Size(), entry.Size))
			return nil
		}

		sum, err := hashFile(ctx, p)
		if err != nil {
			return err
		}
		result.ChecksumsChecked++
		if sum != entry.SHA256 {
			result.ChecksumMismatches = append(result.ChecksumMismatches,
				fmt.Sprintf("%s: restored file hashes to %s, manifest says %s", rel, sum, entry.SHA256))
		}
		tracker.Advance(1, info.Size())
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		result.fail("the restored files could not be read back: %v", err)
		return nil
	}

	for path := range expected {
		if _, ok := found[path]; !ok {
			result.MissingFiles = append(result.MissingFiles, path)
		}
	}
	sort.Strings(result.MissingFiles)
	sort.Strings(result.UnexpectedFiles)
	sort.Strings(result.ChecksumMismatches)

	if len(result.MissingFiles) > 0 {
		result.fail("%d file(s) in the manifest were not restored", len(result.MissingFiles))
	}
	if len(result.UnexpectedFiles) > 0 {
		result.fail("%d restored file(s) are not in the manifest", len(result.UnexpectedFiles))
	}
	if len(result.ChecksumMismatches) > 0 {
		result.fail("%d restored file(s) do not match their recorded checksum", len(result.ChecksumMismatches))
	}
	if result.FilesRestored != result.FilesExpected {
		result.fail("restored %d file(s), the manifest lists %d", result.FilesRestored, result.FilesExpected)
	}
	return nil
}

// findSQLDump locates the dump inside a restored database backup.
func findSQLDump(scratch string, m *Manifest) (string, error) {
	var candidates []string
	for _, entry := range m.FileEntries() {
		if strings.HasSuffix(strings.ToLower(entry.Path), ".sql") {
			candidates = append(candidates, entry.Path)
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("the archive contains no .sql file")
	case 1:
		return filepath.Join(scratch, filepath.FromSlash(candidates[0])), nil
	default:
		sort.Strings(candidates)
		return "", fmt.Errorf("the archive contains %d .sql files (%s); a database backup must contain exactly one",
			len(candidates), strings.Join(candidates, ", "))
	}
}
