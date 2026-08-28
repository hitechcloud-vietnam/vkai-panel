package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestPath is where the manifest lives inside the archive. It is the first
// tar entry, so a reader that only wants to know what is in an archive reads
// one entry and stops - which is what makes a dry run cheap.
const ManifestPath = ".vkai/manifest.json"

// ManifestVersion is the manifest schema version. A restore refuses a version
// it does not understand rather than guessing.
const ManifestVersion = 1

// What a backup covers. Each kind carries its own retention, because the
// answers to "how many generations of this do we keep" are genuinely different:
// a website tree is large and changes slowly, a database dump is small and
// changes constantly, and the panel's own configuration is tiny and the one
// you need when the machine is gone.
const (
	KindWebsite  = "website"
	KindDatabase = "database"
	KindFiles    = "files"
	KindConfig   = "config"
)

// ValidKind reports whether kind is one this package knows how to handle.
func ValidKind(kind string) bool {
	switch kind {
	case KindWebsite, KindDatabase, KindFiles, KindConfig:
		return true
	}
	return false
}

// Entry types, kept as strings so the manifest stays readable when an operator
// opens it during an incident.
const (
	EntryFile    = "file"
	EntryDir     = "dir"
	EntrySymlink = "symlink"
)

// Entry is one member of the archive.
type Entry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

// Manifest describes everything the archive contains, and is written inside
// the archive itself.
//
// It exists so that verification has something to check the extracted bytes
// against that travelled with them. A checksum kept only in the panel database
// proves nothing about an archive sitting in a bucket - and if the panel
// database is the thing that was lost, it is not there to be consulted.
type Manifest struct {
	Version   int       `json:"version"`
	Kind      string    `json:"kind"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	// SingleFile is true when Source was one file rather than a directory, as
	// a database dump is. The archive writer needs to know, and it must not
	// infer it: a directory containing exactly one file whose name happens to
	// equal the directory's own name would fool any inference, and the
	// resulting archive would try to read the directory as a file.
	SingleFile bool  `json:"single_file,omitempty"`
	FileCount  int   `json:"file_count"`
	DirCount   int   `json:"dir_count"`
	LinkCount  int   `json:"link_count"`
	TotalSize  int64 `json:"total_size"`
	// Skipped records members of the source tree that are not backed up:
	// sockets, devices, FIFOs. They are named rather than silently dropped so
	// a restore that is missing something can be explained.
	Skipped []string `json:"skipped,omitempty"`
	Entries []Entry  `json:"entries"`
}

// FileEntries returns just the regular files, which are the entries that carry
// a checksum and therefore the ones verification compares.
func (m *Manifest) FileEntries() []Entry {
	out := make([]Entry, 0, m.FileCount)
	for _, e := range m.Entries {
		if e.Type == EntryFile {
			out = append(out, e)
		}
	}
	return out
}

// Lookup finds an entry by its archive-relative path.
func (m *Manifest) Lookup(path string) (Entry, bool) {
	for _, e := range m.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return Entry{}, false
}

// ScanOptions controls a scan of a source tree.
type ScanOptions struct {
	// Source is the file or directory to back up.
	Source string
	// Kind is what this backup is: website, database, files or config.
	Kind string
	// Exclude is consulted with the archive-relative path of every candidate.
	// Returning true leaves it out of the backup entirely.
	Exclude func(relPath string) bool
	// Tracker receives scan progress. It may be nil.
	Tracker *Tracker
	// FollowSymlinks is deliberately absent. A backup records a symlink as a
	// symlink; following one would copy the target into the archive under the
	// link's name and turn a restore into a silent structural change.
}

// ScanTree walks a source and produces the manifest for it, hashing every
// regular file as it goes.
//
// This is a full read of the source. It is the price of being able to say
// later, with evidence, that a restore produced the same bytes; a backup tool
// that records only sizes and modification times can detect a lost file but
// not a corrupted one, and corruption is the failure that silently survives
// into every subsequent generation.
func ScanTree(ctx context.Context, opts ScanOptions) (*Manifest, error) {
	if strings.TrimSpace(opts.Source) == "" {
		return nil, fmt.Errorf("backup: no source to scan")
	}
	if !ValidKind(opts.Kind) {
		return nil, fmt.Errorf("backup: unknown backup kind %q", opts.Kind)
	}
	source := filepath.Clean(opts.Source)

	info, err := os.Lstat(source)
	if err != nil {
		return nil, fmt.Errorf("backup: cannot read the backup source: %w", err)
	}

	m := &Manifest{
		Version:   ManifestVersion,
		Kind:      opts.Kind,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	}

	opts.Tracker.SetPhase(PhaseScanning, "reading "+source)

	// A single file source - a database dump, one configuration file - is
	// archived under its own base name, so the archive is self-describing and
	// a restore does not need to be told what the file was called.
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("backup: source %q is neither a directory nor a regular file", source)
		}
		sum, err := hashFile(ctx, source)
		if err != nil {
			return nil, err
		}
		m.Entries = append(m.Entries, Entry{
			Path:   filepath.Base(source),
			Type:   EntryFile,
			Size:   info.Size(),
			Mode:   uint32(info.Mode().Perm()),
			SHA256: sum,
		})
		m.FileCount = 1
		m.TotalSize = info.Size()
		m.SingleFile = true
		opts.Tracker.SetTotals(1, info.Size())
		opts.Tracker.Advance(1, info.Size())
		return m, nil
	}

	walkErr := filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("backup: cannot read %q: %w", p, err)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if p == source {
			return nil
		}

		rel, relErr := filepath.Rel(source, p)
		if relErr != nil {
			return fmt.Errorf("backup: cannot place %q inside %q: %w", p, source, relErr)
		}
		rel = filepath.ToSlash(rel)

		if opts.Exclude != nil && opts.Exclude(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		fi, statErr := d.Info()
		if statErr != nil {
			return fmt.Errorf("backup: cannot stat %q: %w", p, statErr)
		}

		switch {
		case fi.Mode().IsDir():
			m.Entries = append(m.Entries, Entry{
				Path: rel,
				Type: EntryDir,
				Mode: uint32(fi.Mode().Perm()),
			})
			m.DirCount++
		case fi.Mode()&os.ModeSymlink != 0:
			target, linkErr := os.Readlink(p)
			if linkErr != nil {
				return fmt.Errorf("backup: cannot read the symlink %q: %w", p, linkErr)
			}
			m.Entries = append(m.Entries, Entry{
				Path: rel,
				Type: EntrySymlink,
				Mode: uint32(fi.Mode().Perm()),
				Link: target,
			})
			m.LinkCount++
		case fi.Mode().IsRegular():
			sum, hashErr := hashFile(ctx, p)
			if hashErr != nil {
				return hashErr
			}
			m.Entries = append(m.Entries, Entry{
				Path:   rel,
				Type:   EntryFile,
				Size:   fi.Size(),
				Mode:   uint32(fi.Mode().Perm()),
				SHA256: sum,
			})
			m.FileCount++
			m.TotalSize += fi.Size()
			opts.Tracker.Advance(1, fi.Size())
		default:
			// Sockets, devices and FIFOs. There is nothing meaningful to copy
			// and nothing meaningful to restore, so they are named in the
			// manifest and left out of the archive.
			m.Skipped = append(m.Skipped, rel)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// A stable order makes two scans of an unchanged tree produce byte
	// identical manifests, which is what lets an operator diff them.
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	sort.Strings(m.Skipped)

	opts.Tracker.SetTotals(m.FileCount, m.TotalSize)
	return m, nil
}

// hashFile returns the lowercase hex SHA-256 of a file, giving up promptly if
// the operation was cancelled.
func hashFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("backup: cannot open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	buf := make([]byte, 256<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("backup: cannot read %q: %w", path, readErr)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile is the exported form of hashFile, used by verification to recompute
// the checksum of a file it has just written.
func HashFile(ctx context.Context, path string) (string, error) { return hashFile(ctx, path) }
