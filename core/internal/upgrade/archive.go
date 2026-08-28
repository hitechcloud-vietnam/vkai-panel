package upgrade

// Extracting the release tarball.
//
// A tar archive is a list of instructions to write files at paths of the
// archive's choosing, and the classic bugs - "../../etc/passwd", "/etc/cron.d/x",
// a symlink to /vkai-panel/etc/.env followed by a write through it - are all
// just paths that leave the destination directory. Every member name and every
// link target is therefore checked before anything is created, and the first
// bad member aborts the whole extraction rather than being skipped: an archive
// containing a traversal is not a release with one bad file in it, it is not a
// release.

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// checkArchiveMember validates one member name against the rules that keep an
// extraction inside its destination. It returns the cleaned relative name.
//
// The rules, in the order a hostile archive tends to try them:
//
//	empty or "." .............. nothing to write
//	NUL or newline ............ truncation tricks against anything that logs
//	"/usr/bin/x" .............. absolute, would escape by construction
//	"C:\x" or a backslash ..... absolute on another platform, or a separator
//	                            some tools translate
//	"../x", "a/../../x" ....... traversal
//	a name that still leaves ... the belt-and-braces containment check made
//	the destination after Join   by the caller
func checkArchiveMember(name string) (string, error) {
	if name == "" {
		return "", &UnsafeArchiveError{Member: name, Reason: "empty member name"}
	}
	if strings.ContainsAny(name, "\x00\n\r") {
		return "", &UnsafeArchiveError{Member: name, Reason: "member name contains a control character"}
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", &UnsafeArchiveError{Member: name, Reason: "member name is an absolute path"}
	}
	if strings.Contains(name, `\`) {
		return "", &UnsafeArchiveError{Member: name, Reason: `member name contains a backslash`}
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", &UnsafeArchiveError{Member: name, Reason: "member name looks like a drive-letter absolute path"}
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", &UnsafeArchiveError{Member: name, Reason: `member name contains ".."`}
		}
	}

	clean := path.Clean(name)
	clean = strings.TrimSuffix(clean, "/")
	if clean == "." || clean == "" {
		return "", &UnsafeArchiveError{Member: name, Reason: "member name resolves to the destination directory itself"}
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", &UnsafeArchiveError{Member: name, Reason: "member name escapes the destination directory"}
	}
	return clean, nil
}

// checkLinkTarget validates the target of a symlink or hard link. The target is
// resolved the way the kernel would resolve it - relative to the directory
// holding the link - and has to land inside the destination tree.
func checkLinkTarget(member, cleanName, target string) error {
	if target == "" {
		return &UnsafeArchiveError{Member: member, Target: target, Reason: "link has an empty target"}
	}
	if strings.ContainsAny(target, "\x00\n\r") {
		return &UnsafeArchiveError{Member: member, Target: target, Reason: "link target contains a control character"}
	}
	if strings.HasPrefix(target, "/") || filepath.IsAbs(target) {
		return &UnsafeArchiveError{Member: member, Target: target, Reason: "link target is an absolute path"}
	}
	if strings.Contains(target, `\`) {
		return &UnsafeArchiveError{Member: member, Target: target, Reason: "link target contains a backslash"}
	}
	resolved := path.Clean(path.Join(path.Dir(cleanName), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return &UnsafeArchiveError{Member: member, Target: target, Reason: "link target points outside the destination directory"}
	}
	return nil
}

// safeJoin joins a validated relative name onto dest and proves the result did
// not leave dest. It is redundant given checkArchiveMember, and that is the
// point: it is the check that still holds if the rules above are ever relaxed.
func safeJoin(dest, cleanName string) (string, error) {
	base := filepath.Clean(dest)
	joined := filepath.Clean(filepath.Join(base, filepath.FromSlash(cleanName)))
	if joined != base && !strings.HasPrefix(joined, base+string(filepath.Separator)) {
		return "", &UnsafeArchiveError{Member: cleanName, Reason: fmt.Sprintf("resolves to %s, outside %s", joined, base)}
	}
	return joined, nil
}

// extractOptions bound what an extraction may do.
type extractOptions struct {
	MaxBytes   int64
	MaxEntries int
}

// extractTarGz unpacks a gzipped tar into dest, which must not already exist.
//
// The caller has already verified the archive's checksum. Everything here is
// still defensive, because "the checksum matched" only proves the archive is
// the one the manifest named, not that the release pipeline that produced it
// was well behaved.
func extractTarGz(archivePath, dest string, opts extractOptions) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read %s as gzip: %w", archivePath, err)
	}
	defer func() { _ = gz.Close() }()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}

	tr := tar.NewReader(gz)
	var (
		total   int64
		entries int
		files   int
	)
	// Directory modes are applied after every member is written, because a
	// directory extracted read-only would otherwise stop its own contents
	// from being created.
	type pendingDir struct {
		path string
		mode os.FileMode
	}
	var dirs []pendingDir

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", archivePath, err)
		}

		entries++
		if opts.MaxEntries > 0 && entries > opts.MaxEntries {
			return fmt.Errorf("archive %s has more than %d members", archivePath, opts.MaxEntries)
		}

		cleanName, err := checkArchiveMember(hdr.Name)
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, cleanName)
		if err != nil {
			return err
		}

		mode := sanitizeMode(hdr.FileInfo().Mode().Perm())

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}
			dirs = append(dirs, pendingDir{path: target, mode: mode | 0o700})

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", filepath.Dir(target), err)
			}
			remaining := int64(-1)
			if opts.MaxBytes > 0 {
				remaining = opts.MaxBytes - total
				if remaining <= 0 {
					return fmt.Errorf("archive %s expands past the %d byte limit", archivePath, opts.MaxBytes)
				}
			}
			n, err := writeRegular(target, tr, mode, remaining)
			total += n
			if err != nil {
				return err
			}
			if opts.MaxBytes > 0 && total > opts.MaxBytes {
				return fmt.Errorf("archive %s expands past the %d byte limit", archivePath, opts.MaxBytes)
			}
			files++

		case tar.TypeSymlink:
			if err := checkLinkTarget(hdr.Name, cleanName, hdr.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", filepath.Dir(target), err)
			}
			_ = os.Remove(target)
			if err := os.Symlink(filepath.FromSlash(hdr.Linkname), target); err != nil {
				return fmt.Errorf("create symlink %s: %w", target, err)
			}

		case tar.TypeLink:
			linkClean, err := checkArchiveMember(hdr.Linkname)
			if err != nil {
				return &UnsafeArchiveError{Member: hdr.Name, Target: hdr.Linkname, Reason: "hard link target is not a safe archive path"}
			}
			linkTarget, err := safeJoin(dest, linkClean)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", filepath.Dir(target), err)
			}
			_ = os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				return fmt.Errorf("create hard link %s: %w", target, err)
			}

		default:
			// Character and block devices, FIFOs and sockets have no
			// business in a release tarball, and creating them needs
			// privileges the upgrade should not be exercising.
			return &UnsafeArchiveError{
				Member: hdr.Name,
				Reason: fmt.Sprintf("unsupported archive member type %q", string(rune(hdr.Typeflag))),
			}
		}
	}

	if files == 0 {
		return fmt.Errorf("archive %s contains no files", archivePath)
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return fmt.Errorf("set mode on %s: %w", dirs[i].path, err)
		}
	}
	return nil
}

// writeRegular writes one file, refusing to write more than limit bytes when
// limit is non-negative. It returns how much it wrote even on error, so the
// caller's running total stays honest.
func writeRegular(target string, r io.Reader, mode os.FileMode, limit int64) (int64, error) {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", target, err)
	}
	src := r
	if limit >= 0 {
		src = io.LimitReader(r, limit+1)
	}
	n, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		return n, fmt.Errorf("write %s: %w", target, copyErr)
	}
	if closeErr != nil {
		return n, fmt.Errorf("close %s: %w", target, closeErr)
	}
	if limit >= 0 && n > limit {
		return n, fmt.Errorf("write %s: the file exceeds the remaining extraction byte limit", target)
	}
	if err := os.Chmod(target, mode); err != nil {
		return n, fmt.Errorf("set mode on %s: %w", target, err)
	}
	return n, nil
}

// sanitizeMode strips setuid, setgid, the sticky bit and group/other write off
// an archived mode. A release tarball has no legitimate reason to carry a
// setuid binary into /vkai-panel, and a group-writable file there is a way for
// one compromised service to rewrite another's code.
func sanitizeMode(m os.FileMode) os.FileMode {
	m &^= os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	m &^= 0o022
	m |= 0o400
	return m.Perm()
}

// dirSize adds up the apparent size of every regular file under root. It is
// what preflight compares against the free space, so symlinks are not followed
// and directories cost nothing.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", root, err)
	}
	return total, nil
}
