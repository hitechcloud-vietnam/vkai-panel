package backup

// Writing and reading a backup archive.
//
// An archive is: tar -> gzip -> (optionally) AES-256-GCM, in that order.
//
// The order is the interesting part. Compressing before encrypting is what
// makes the archive small; encrypting before it reaches a Destination is what
// keeps plaintext off the wire and out of somebody else's bucket. Doing it the
// other way round - encrypt then compress - would produce an archive that does
// not compress at all, which is how you can tell a tool has never been run on
// real data.
//
// A restore is planned before it is applied. ExtractArchive with DryRun set
// reads exactly one tar entry, the manifest, and reports what a real run would
// overwrite. That shortcut is sound only because a real run refuses any member
// the manifest does not list and errors on any manifest entry the archive does
// not contain, so the two lists can never drift apart unnoticed.
//
// Extraction runs as root. The rules that keep it inside the destination are
// the ones internal/upgrade arrived at the hard way, with one deliberate
// difference: a backup of a customer's website tree legitimately contains
// symlinks, so this package restores them rather than refusing them. It
// restores them LAST, after every directory and regular file is on disk, so no
// write performed by this extraction can ever pass through a link this
// extraction created. Combined with O_NOFOLLOW on every file and a real
// directory check on every path component, that closes the symlink-then-write
// attack without having to reason about link targets as strings - which the
// upgrade package documents at length is unsound.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// maxManifestBytes bounds the manifest a restore will parse. A manifest is
// JSON produced by this package; 256 MiB is far beyond any real tree and still
// refuses an archive whose first entry is a decompression bomb.
const maxManifestBytes = 256 << 20

// CreateOptions describes a backup to write.
type CreateOptions struct {
	Source  string
	Kind    string
	Key     *Key
	Exclude func(relPath string) bool
	Tracker *Tracker

	// Manifest, when set, is used instead of scanning the source again. The
	// caller that already scanned - to show the operator what will be backed
	// up - passes it back here rather than reading the tree twice.
	Manifest *Manifest
}

// CreateResult is what was written.
type CreateResult struct {
	Manifest  *Manifest `json:"manifest"`
	Bytes     int64     `json:"bytes"`
	SHA256    string    `json:"sha256"`
	Encrypted bool      `json:"encrypted"`
	KeyID     string    `json:"key_id,omitempty"`
}

// hashWriter counts and digests everything that reaches the destination, so the
// SHA-256 recorded for an archive is the digest of the bytes as they were
// actually written - after compression and after encryption. It is the digest a
// later download is checked against.
type hashWriter struct {
	dst    io.Writer
	n      int64
	digest interface {
		io.Writer
		Sum([]byte) []byte
	}
}

func newHashWriter(dst io.Writer) *hashWriter {
	return &hashWriter{dst: dst, digest: sha256.New()}
}

func (h *hashWriter) Write(p []byte) (int, error) {
	n, err := h.dst.Write(p)
	if n > 0 {
		h.n += int64(n)
		h.digest.Write(p[:n])
	}
	return n, err
}

func (h *hashWriter) sum() string { return hex.EncodeToString(h.digest.Sum(nil)) }

// CreateArchive writes a backup of opts.Source to dst.
func CreateArchive(ctx context.Context, dst io.Writer, opts CreateOptions) (*CreateResult, error) {
	if dst == nil {
		return nil, errors.New("backup: no destination writer")
	}

	manifest := opts.Manifest
	if manifest == nil {
		scanned, err := ScanTree(ctx, ScanOptions{
			Source:  opts.Source,
			Kind:    opts.Kind,
			Exclude: opts.Exclude,
			Tracker: opts.Tracker,
		})
		if err != nil {
			return nil, err
		}
		manifest = scanned
	}

	counted := newHashWriter(dst)

	// The encryptor pulls, the tar writer pushes, so a pipe joins them. Errors
	// have to cross the pipe in both directions or a failure on one side
	// deadlocks the other.
	var (
		sink    io.Writer = counted
		pw      *io.PipeWriter
		encDone chan error
	)
	if opts.Key != nil {
		var pr *io.PipeReader
		pr, pw = io.Pipe()
		encDone = make(chan error, 1)
		go func() {
			err := Encrypt(counted, pr, opts.Key)
			// Unblock the writer if encryption failed part way through.
			_ = pr.CloseWithError(err)
			encDone <- err
		}()
		sink = pw
	}

	opts.Tracker.SetPhase(PhaseArchiving, "writing archive")
	opts.Tracker.SetTotals(manifest.FileCount, manifest.TotalSize)

	writeErr := writeTarGz(ctx, sink, manifest, opts.Tracker)

	if pw != nil {
		// Closing with the error propagates it to Encrypt, which stops rather
		// than sealing a truncated archive as if it were complete.
		if writeErr != nil {
			_ = pw.CloseWithError(writeErr)
		} else {
			_ = pw.Close()
		}
		if encErr := <-encDone; encErr != nil && writeErr == nil {
			writeErr = encErr
		}
	}
	if writeErr != nil {
		return nil, writeErr
	}

	return &CreateResult{
		Manifest:  manifest,
		Bytes:     counted.n,
		SHA256:    counted.sum(),
		Encrypted: opts.Key != nil,
		KeyID:     opts.Key.ID(),
	}, nil
}

func writeTarGz(ctx context.Context, sink io.Writer, m *Manifest, tracker *Tracker) error {
	// The tracker counts source bytes read, not compressed bytes written, so
	// that a percentage relates to something the operator can see on disk.
	// The writer here only carries cancellation.
	gz := gzip.NewWriter(&ctxWriter{ctx: ctx, dst: sink})
	tw := tar.NewWriter(gz)

	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: could not encode the manifest: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     ManifestPath,
		Mode:     0o600,
		Size:     int64(len(manifestJSON)),
		ModTime:  m.CreatedAt,
		Typeflag: tar.TypeReg,
	}); err != nil {
		return fmt.Errorf("backup: could not write the manifest header: %w", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		return fmt.Errorf("backup: could not write the manifest: %w", err)
	}

	base := m.Source

	for _, entry := range m.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}

		source := filepath.Join(base, filepath.FromSlash(entry.Path))
		if m.SingleFile {
			source = base
		}

		switch entry.Type {
		case EntryDir:
			if err := tw.WriteHeader(&tar.Header{
				Name:     entry.Path + "/",
				Mode:     int64(entry.Mode),
				ModTime:  m.CreatedAt,
				Typeflag: tar.TypeDir,
			}); err != nil {
				return fmt.Errorf("backup: could not write the directory %q: %w", entry.Path, err)
			}
		case EntrySymlink:
			if err := tw.WriteHeader(&tar.Header{
				Name:     entry.Path,
				Mode:     int64(entry.Mode),
				ModTime:  m.CreatedAt,
				Typeflag: tar.TypeSymlink,
				Linkname: entry.Link,
			}); err != nil {
				return fmt.Errorf("backup: could not write the symlink %q: %w", entry.Path, err)
			}
		case EntryFile:
			if err := writeFileEntry(ctx, tw, source, entry, m.CreatedAt, tracker); err != nil {
				return err
			}
		default:
			return fmt.Errorf("backup: manifest entry %q has unknown type %q", entry.Path, entry.Type)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("backup: could not finish the archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("backup: could not finish compression: %w", err)
	}
	return nil
}

func writeFileEntry(ctx context.Context, tw *tar.Writer, source string, entry Entry, modTime time.Time, tracker *Tracker) error {
	f, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("backup: cannot open %q: %w", source, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("backup: cannot stat %q: %w", source, err)
	}
	// The file may have changed size since the scan. The manifest is the
	// authority for what this archive claims to contain, so a size change is
	// refused rather than written under a checksum that no longer describes it.
	if info.Size() != entry.Size {
		return fmt.Errorf(
			"backup: %q changed while the backup was running (scanned %d bytes, now %d); re-run the backup",
			source, entry.Size, info.Size())
	}

	if err := tw.WriteHeader(&tar.Header{
		Name:     entry.Path,
		Mode:     int64(entry.Mode),
		Size:     entry.Size,
		ModTime:  modTime,
		Typeflag: tar.TypeReg,
	}); err != nil {
		return fmt.Errorf("backup: could not write the header for %q: %w", entry.Path, err)
	}

	written, err := io.Copy(tw, &ctxReader{ctx: ctx, src: f})
	if err != nil {
		return fmt.Errorf("backup: could not archive %q: %w", source, err)
	}
	if written != entry.Size {
		return fmt.Errorf("backup: %q produced %d bytes, expected %d", source, written, entry.Size)
	}
	tracker.Advance(1, written)
	return nil
}

// ctxWriter fails the next write once the operation has been cancelled, which
// is what unwinds tar, gzip and the encryptor together instead of leaving one
// of them blocked on a pipe.
type ctxWriter struct {
	ctx context.Context
	dst io.Writer
}

func (w *ctxWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.dst.Write(p)
}

// ctxReader is the same idea on the read side, so a cancelled backup stops
// mid-file rather than at the next file boundary.
type ctxReader struct {
	ctx context.Context
	src io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(p)
}

// archiveReader opens the decrypt/decompress/untar pipeline over src.
type archiveReader struct {
	tr     *tar.Reader
	gz     *gzip.Reader
	closer func() error
}

func openArchive(src io.Reader, key *Key) (*archiveReader, error) {
	stream := src
	if key != nil {
		decrypted, err := NewDecryptReader(src, key)
		if err != nil {
			return nil, err
		}
		stream = decrypted
	} else {
		// No key was supplied. If this is an encrypted archive, say so and
		// name the key it needs, rather than letting gzip fail with "invalid
		// header" - which reads like corruption and sends an operator looking
		// for the wrong problem at the worst possible moment.
		header := make([]byte, len(magic)+1+keyIDLen)
		n, err := io.ReadFull(src, header)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("backup: could not read the archive: %w", err)
		}
		if keyID, peekErr := PeekKeyID(header[:n]); peekErr == nil {
			return nil, fmt.Errorf(
				"%w: this archive is encrypted with key %s, but no key was supplied", ErrNoKey, keyID)
		}
		stream = io.MultiReader(bytes.NewReader(header[:n]), src)
	}
	gz, err := gzip.NewReader(stream)
	if err != nil {
		return nil, fmt.Errorf("backup: archive is not readable (%w)", err)
	}
	return &archiveReader{tr: tar.NewReader(gz), gz: gz, closer: gz.Close}, nil
}

// ReadManifest reads the manifest out of an archive without extracting it.
func ReadManifest(src io.Reader, key *Key) (*Manifest, error) {
	ar, err := openArchive(src, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ar.closer() }()
	return readManifestEntry(ar.tr)
}

func readManifestEntry(tr *tar.Reader) (*Manifest, error) {
	header, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("backup: archive is empty or unreadable: %w", err)
	}
	if header.Name != ManifestPath {
		return nil, fmt.Errorf(
			"backup: archive does not begin with %s (first member is %q); it was not written by this panel",
			ManifestPath, header.Name)
	}
	if header.Size > maxManifestBytes {
		return nil, fmt.Errorf("backup: manifest is %d bytes, which is not plausible", header.Size)
	}

	data, err := io.ReadAll(io.LimitReader(tr, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("backup: could not read the manifest: %w", err)
	}
	if int64(len(data)) > maxManifestBytes {
		return nil, fmt.Errorf("backup: manifest is larger than %d bytes", maxManifestBytes)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("backup: manifest is not valid JSON: %w", err)
	}
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf(
			"backup: archive manifest is version %d, this panel understands version %d",
			m.Version, ManifestVersion)
	}
	return &m, nil
}
