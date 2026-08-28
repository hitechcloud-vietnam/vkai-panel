package upgrade

// Extracting the release tarball.
//
// A tar archive is a list of instructions to write files at paths of the
// archive's choosing, and the classic bugs - "../../etc/passwd",
// "/etc/cron.d/x", a symlink to /vkai-panel/etc/.env followed by a write
// through it - are all just paths that leave the destination directory.
// Extraction runs as root, so a member that escapes the staging directory is a
// remote root write.
//
// # Why this no longer reasons about link targets as strings
//
// The first version of this file validated a symlink's target by resolving it
// lexically: path.Clean(path.Join(dir(member), target)). That is unsound, and
// an archive escapes it in four members:
//
//	a       -> "."     lexically dest/a, really dest
//	a/b     -> ".."    lexically dest, really dest/b, pointing at dest's parent
//	b/c     -> ".."    one more level up
//	b/c/d/etc/cron.d/pwn               a regular file, written outside dest
//
// path.Clean collapses "<symlink>/.." to nothing; the kernel resolves the
// symlink first and then applies "..". Every hop buys one directory level and
// the hops chain, so no amount of string cleverness fixes it - the two
// resolvers simply do not agree.
//
// The fix is not a better string check. Symlinks and hard links are refused
// outright: a VKAI Panel release tarball is a directory of files and has never
// contained a link, so refusing them costs nothing and removes the entire class
// of bug, including the variants nobody has thought of yet. The alternative -
// keeping links and resolving every path against an opened directory file
// descriptor with openat(2)/O_NOFOLLOW so the kernel's own resolution is what
// gets checked - is the right answer for a general-purpose extractor, but it is
// considerably more code, it is not portable without build tags for every
// syscall, and it defends a feature this archive does not use. Refusing is the
// change least likely to be subtly wrong.
//
// Everything below is then belt and braces on top of that decision, because
// extraction runs as root:
//
//   - member names are still checked for absolute paths, "..", backslashes and
//     control characters, and the joined path is still proved to be inside the
//     destination;
//   - every directory is created one component at a time, and a component that
//     already exists must be a real directory, never a symlink;
//   - every regular file is opened O_CREATE|O_EXCL|O_NOFOLLOW, so the write
//     cannot follow a link that is already on disk and cannot land on an
//     existing file;
//   - modes come from this package, not from the archive: a member carrying
//     setuid, setgid or the sticky bit is refused rather than quietly stripped;
//   - the archive's sha256 is checked, in constant time, against the same open
//     file descriptor that is about to be decompressed - not against the path,
//     which something else could swap in between.
//
// The first bad member aborts the whole extraction rather than being skipped:
// an archive containing a traversal is not a release with one bad file in it,
// it is not a release.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The only modes an extracted release may have. They are decided here rather
// than taken from the archive: a release is code and configuration owned by
// root, executable or not, and nothing in it needs to be writable by anyone
// else or to carry a privilege bit.
const (
	extractedDirMode  os.FileMode = 0o755
	extractedFileMode os.FileMode = 0o644
	extractedExecMode os.FileMode = 0o755
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

// checkArchiveMode refuses a member whose recorded mode carries a privilege
// bit. Stripping it silently would be the friendlier behaviour and the wrong
// one: a release tarball with a setuid binary in it is not a release that was
// built the way we think it was, and installing the rest of it as root on a
// customer's machine is not a decision this package should be making.
func checkArchiveMode(member string, mode os.FileMode) error {
	switch {
	case mode&os.ModeSetuid != 0:
		return &UnsafeArchiveError{Member: member, Reason: "member is setuid"}
	case mode&os.ModeSetgid != 0:
		return &UnsafeArchiveError{Member: member, Reason: "member is setgid"}
	case mode&os.ModeSticky != 0:
		return &UnsafeArchiveError{Member: member, Reason: "member has the sticky bit set"}
	}
	return nil
}

// extractedMode is the mode a regular member is written with: the archive
// decides only whether the file is executable, nothing else.
func extractedMode(mode os.FileMode) os.FileMode {
	if mode.Perm()&0o111 != 0 {
		return extractedExecMode
	}
	return extractedFileMode
}

// extractOptions bound what an extraction may do.
type extractOptions struct {
	// ExpectedSHA256 is the digest the archive must have, lowercase hex. It
	// is mandatory: extraction is the step that runs a parser over
	// attacker-influenced bytes as root, and there is no path through this
	// function that reaches the gzip reader without checking it first.
	ExpectedSHA256 string
	MaxBytes       int64
	MaxEntries     int
}

// extractTarGz unpacks a gzipped tar into dest.
//
// The archive is verified here, against the open file descriptor that is then
// decompressed, rather than trusting a check some caller made earlier against
// the path. Callers do verify at download time as well - that is what deletes a
// bad download before anything opens it - but this function is the one that
// hands bytes to a decompressor, so it does not accept "someone already looked
// at that file" as an answer.
func extractTarGz(archivePath, dest string, opts extractOptions) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	if err := verifyOpenArchive(f, archivePath, opts.ExpectedSHA256); err != nil {
		return err
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read %s as gzip: %w", archivePath, err)
	}
	defer func() { _ = gz.Close() }()

	if err := prepareDest(dest); err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	var (
		total   int64
		entries int
		files   int
	)

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

		// A pax global header carries no file. "git archive" and "tar
		// --format=pax" both emit one, so refusing it would refuse most
		// of the ways a release tarball is actually produced.
		typeflag := hdr.Typeflag
		if typeflag == '\x00' { // the obsolete regular-file flag
			typeflag = tar.TypeReg
		}
		if typeflag == tar.TypeXGlobalHeader {
			continue
		}

		// "tar -C dir ." names the destination itself as "./". It is not
		// an escape and it is not something to create; it is a no-op.
		if typeflag == tar.TypeDir && isDestItself(hdr.Name) {
			continue
		}

		cleanName, err := checkArchiveMember(hdr.Name)
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, cleanName)
		if err != nil {
			return err
		}
		if err := checkArchiveMode(hdr.Name, hdr.FileInfo().Mode()); err != nil {
			return err
		}

		switch typeflag {
		case tar.TypeDir:
			if err := ensureDir(dest, cleanName); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := ensureDir(dest, path.Dir(cleanName)); err != nil {
				return err
			}
			remaining := int64(-1)
			if opts.MaxBytes > 0 {
				remaining = opts.MaxBytes - total
				if remaining <= 0 {
					return fmt.Errorf("archive %s expands past the %d byte limit", archivePath, opts.MaxBytes)
				}
			}
			n, err := writeRegular(target, tr, extractedMode(hdr.FileInfo().Mode()), remaining)
			total += n
			if err != nil {
				return err
			}
			if opts.MaxBytes > 0 && total > opts.MaxBytes {
				return fmt.Errorf("archive %s expands past the %d byte limit", archivePath, opts.MaxBytes)
			}
			files++

		case tar.TypeSymlink, tar.TypeLink:
			// See the file comment: no release contains a link, and a
			// link is the one member type whose safety cannot be
			// decided from the archive alone.
			return &UnsafeArchiveError{
				Member: hdr.Name,
				Target: hdr.Linkname,
				Reason: "the archive contains a link; a release tarball may only contain regular files and directories",
			}

		default:
			// Character and block devices, FIFOs and sockets have no
			// business in a release tarball, and creating them needs
			// privileges the upgrade should not be exercising.
			return &UnsafeArchiveError{
				Member: hdr.Name,
				Reason: fmt.Sprintf("unsupported archive member type %q", string(rune(typeflag))),
			}
		}
	}

	if files == 0 {
		return fmt.Errorf("archive %s contains no files", archivePath)
	}
	if err := os.Chmod(dest, extractedDirMode); err != nil {
		return fmt.Errorf("set mode on %s: %w", dest, err)
	}
	return nil
}

// verifyOpenArchive proves that the bytes behind f are the release the manifest
// named, then rewinds f so the caller reads exactly what was hashed.
//
// Hashing the file descriptor rather than the path is the whole point: a check
// made against a path and a read made against the same path are two resolutions
// of that name, and anything that can write to /vkai-panel/tmp between them
// chooses what gets extracted. There is only ever one open file here.
//
// The comparison is constant time. The value being compared is public, so this
// is cheap insurance rather than a fix for a known oracle, but a digest
// comparison that leaks its prefix through timing is the kind of thing that
// becomes exploitable once someone can ask for a retry.
func verifyOpenArchive(f *os.File, archivePath, expected string) error {
	want, err := decodeSHA256(expected)
	if err != nil {
		return fmt.Errorf("refusing to extract %s: %w", archivePath, err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash %s: %w", archivePath, err)
	}
	got := h.Sum(nil)

	if subtle.ConstantTimeCompare(want, got) != 1 {
		return &ChecksumMismatchError{
			Path:     archivePath,
			Expected: hex.EncodeToString(want),
			Actual:   hex.EncodeToString(got),
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind %s after verifying it: %w", archivePath, err)
	}
	return nil
}

// decodeSHA256 turns a manifest's hex digest into bytes, refusing anything that
// is not exactly a sha256.
func decodeSHA256(s string) ([]byte, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return nil, errors.New("no expected sha256 was supplied")
	}
	if len(trimmed) != sha256.Size*2 {
		return nil, fmt.Errorf("expected sha256 %q is not 64 hex characters", s)
	}
	raw, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("expected sha256 %q is not hexadecimal", s)
	}
	return raw, nil
}

// isDestItself reports whether a member names the destination directory rather
// than something inside it.
func isDestItself(name string) bool {
	clean := path.Clean(strings.TrimSuffix(name, "/"))
	return clean == "." || clean == ""
}

// prepareDest creates the destination directory and proves it is a real
// directory rather than a symlink someone left pointing elsewhere.
func prepareDest(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), extractedDirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	if err := os.Mkdir(dest, 0o700); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		info, lerr := os.Lstat(dest)
		if lerr != nil {
			return fmt.Errorf("inspect %s: %w", dest, lerr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &UnsafeArchiveError{Member: dest, Reason: "the extraction destination is a symlink"}
		}
		if !info.IsDir() {
			return &UnsafeArchiveError{Member: dest, Reason: "the extraction destination is not a directory"}
		}
	}
	return nil
}

// ensureDir creates rel underneath dest, one component at a time.
//
// os.MkdirAll would do this in one call and would happily walk through a
// symlink on the way. Each component is created instead, and a component that
// already exists has to be a real directory - which, since links are refused
// above, can only be one this extraction made.
func ensureDir(dest, rel string) error {
	if rel == "." || rel == "" {
		return nil
	}
	cur := filepath.Clean(dest)
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		err := os.Mkdir(cur, extractedDirMode)
		if err == nil {
			continue
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create directory %s: %w", cur, err)
		}
		info, lerr := os.Lstat(cur)
		if lerr != nil {
			return fmt.Errorf("inspect %s: %w", cur, lerr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &UnsafeArchiveError{Member: rel, Reason: fmt.Sprintf("%s is a symlink; the extraction refuses to write through it", cur)}
		}
		if !info.IsDir() {
			return &UnsafeArchiveError{Member: rel, Reason: fmt.Sprintf("%s already exists and is not a directory", cur)}
		}
	}
	return nil
}

// writeRegular writes one file, refusing to write more than limit bytes when
// limit is non-negative. It returns how much it wrote even on error, so the
// caller's running total stays honest.
//
// The open is O_CREATE|O_EXCL|O_NOFOLLOW. O_EXCL means an archive cannot
// overwrite a file it already extracted - two members with one name is not a
// release either - and, together with O_NOFOLLOW, it means the write cannot
// land on the far end of a symlink that is already on disk. Both matter because
// this runs as root: without them, one link in the staging directory turns a
// release file into a write to anywhere on the filesystem.
func writeRegular(target string, r io.Reader, mode os.FileMode, limit int64) (int64, error) {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY|oNoFollow, mode)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return 0, &UnsafeArchiveError{Member: target, Reason: "the archive writes this path twice, or something is already there"}
		}
		// ELOOP from O_NOFOLLOW arrives here: the path is a symlink.
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
	// The open mode is masked by the process umask, so the mode is set
	// explicitly afterwards. Chmod on a path could in principle follow a
	// link, but the file was just created with O_EXCL|O_NOFOLLOW and links
	// cannot be created by this extraction, so the name still refers to the
	// file that was written.
	if err := os.Chmod(target, mode); err != nil {
		return n, fmt.Errorf("set mode on %s: %w", target, err)
	}
	return n, nil
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
