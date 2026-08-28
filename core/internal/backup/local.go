package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalDestination stores archives on a directory of this machine.
//
// It is the destination every installation has, and the one an operator falls
// back to when the object store is unreachable. It is NOT offsite: an archive
// on the same disk as the data it protects survives a bad deployment and does
// not survive the disk. The panel says so where the operator chooses it; this
// type just refuses to pretend otherwise.
type LocalDestination struct {
	root string
}

// NewLocalDestination opens a local destination rooted at an absolute path,
// creating it if necessary.
func NewLocalDestination(root string) (*LocalDestination, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("backup: local destination needs a root directory")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("backup: local destination root %q must be an absolute path", root)
	}
	clean := filepath.Clean(root)
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return nil, fmt.Errorf("backup: cannot create the local destination %q: %w", clean, err)
	}
	return &LocalDestination{root: clean}, nil
}

// Root is the directory this destination writes into.
func (d *LocalDestination) Root() string { return d.root }

func (d *LocalDestination) Kind() string { return "local" }

func (d *LocalDestination) Describe() string { return "local disk at " + d.root }

func (d *LocalDestination) resolve(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	full := filepath.Join(d.root, filepath.FromSlash(key))
	clean := filepath.Clean(full)
	if !strings.HasPrefix(clean, d.root+string(filepath.Separator)) {
		return "", fmt.Errorf("backup: object key %q resolves outside %s", key, d.root)
	}
	return clean, nil
}

// Put writes the object, atomically: the archive lands under a temporary name
// in the same directory and is renamed into place only once it is complete and
// on disk. A crash halfway through therefore leaves no half-written archive
// that a later restore would happily open and fail on.
func (d *LocalDestination) Put(ctx context.Context, key string, r io.Reader, size int64) (ObjectInfo, error) {
	target, err := d.resolve(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot create %q: %w", filepath.Dir(target), err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".partial-*")
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot stage the archive: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op once the rename has happened
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot set the mode of the staged archive: %w", err)
	}

	written, err := io.Copy(tmp, &ctxReader{ctx: ctx, src: r})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot write the archive: %w", err)
	}
	if size >= 0 && written != size {
		return ObjectInfo{}, fmt.Errorf("backup: archive is %d bytes, expected %d", written, size)
	}
	// fsync before rename: without it the rename can be durable while the
	// contents are not, which is the shape of a backup that is present after
	// a power cut and empty.
	if err := tmp.Sync(); err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot flush the archive to disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot close the staged archive: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot publish the archive: %w", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot stat the archive just written: %w", err)
	}
	return ObjectInfo{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (d *LocalDestination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	target, err := d.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("backup: cannot open %q: %w", key, err)
	}
	return f, nil
}

func (d *LocalDestination) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	target, err := d.resolve(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: cannot stat %q: %w", key, err)
	}
	return ObjectInfo{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (d *LocalDestination) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if prefix != "" {
		if err := ValidateKey(strings.TrimSuffix(prefix, "/")); err != nil {
			return nil, err
		}
	}

	var out []ObjectInfo
	err := filepath.WalkDir(d.root, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(d.root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		// Objects being written are staged under .partial-*; they are not
		// objects yet and retention must never see them as a generation.
		if strings.HasPrefix(filepath.Base(key), ".partial-") {
			return nil
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		out = append(out, ObjectInfo{Key: key, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("backup: cannot list %s: %w", d.root, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Delete removes an object. A key that is not there is not an error: retention
// runs repeatedly and must converge, not fail on its second pass.
func (d *LocalDestination) Delete(ctx context.Context, key string) error {
	target, err := d.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: cannot delete %q: %w", key, err)
	}
	return nil
}
