package ops

// Reading a log file, and measuring what a directory occupies.
//
// These are the two things the panel asked /execute for most often: `tail` on a
// site's error log, and `du` on a site's document root. Both are now named
// operations with typed arguments, and neither takes a program name, a shell
// fragment or a flag from the caller.
//
// # Why a path argument is not the same as an arbitrary command
//
// It is close enough to deserve the same suspicion. A path argument that is not
// constrained is a way to read /etc/shadow, /root/.ssh/id_rsa, or the agent's
// own private key, which would make "read a log file" equivalent to taking the
// host - the same failure /execute had, wearing a narrower name.
//
// Three things constrain it:
//
//  1. The requested path must lie under one of a small set of roots. The check
//     compares whole path elements, so /var/logging is not inside /var/log.
//  2. The path is then resolved through every symlink, and the result must lie
//     under a root as well. Without this, a symlink planted inside /var/log
//     pointing at /etc/shadow would pass the first check and read the target.
//  3. The opened file must be a regular file. A named pipe under an allowed
//     root would otherwise block the operation forever, and a device node would
//     be read as though it were text.
//
// The roots themselves are configurable, because a panel that manages sites in
// a non-default location cannot read their logs otherwise, but "/" is refused
// as a root: a configuration mistake must not be able to reopen the hole this
// operation was written to close.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/metrics"
)

// DefaultLogRoots is where this product and the servers it manages keep logs.
var DefaultLogRoots = []string{
	"/vkai-panel/logs",
	"/var/log",
	"/www/wwwlogs",
}

// DefaultDiskRoots is where a hosting node keeps data whose size an operator
// asks about. It is wider than the log roots because the answer is a number of
// bytes rather than the contents of a file, and narrower than the filesystem
// because there is no reason for the panel to measure /proc.
// /tmp is deliberately absent: the shipped systemd unit sets PrivateTmp, so a
// measurement of /tmp would describe the agent's own private namespace and not
// the /tmp anything else on the host can see. /home is present because a
// hosting node commonly keeps sites there, and an operator who wants it
// measured relaxes ProtectHome in the unit; until they do, the operation fails
// honestly rather than reporting an empty directory.
var DefaultDiskRoots = []string{
	"/backup",
	"/home",
	"/opt",
	"/srv",
	"/usr",
	"/var",
	"/vkai-panel",
	"/www",
}

// Bounds on log.read. A log file on a busy site is measured in gigabytes; the
// window this operation reads is fixed regardless.
const (
	defaultLogLines  = 200
	maxLogLines      = 5000
	defaultLogBytes  = 256 << 10
	maxLogBytes      = 4 << 20
	maxLogListedFile = 500
	maxLogListDepth  = 4
)

// Bounds on disk.usage. A recursive measurement of a directory with millions of
// files would hold a worker for minutes; it stops at the limit and says that it
// did rather than returning a number that is quietly wrong.
const (
	defaultUsageMaxEntries = 200000
	maxUsageMaxEntries     = 2000000
	usageDeadline          = 60 * time.Second
)

// ============================================================
// READING A LOG FILE
// ============================================================

// LogReadArgs is the argument object of log.read.
type LogReadArgs struct {
	// Path is the absolute path of the file, which must resolve under one of
	// the configured log roots.
	Path string `json:"path"`
	// Lines is how many lines from the end of the file to return.
	Lines int `json:"lines,omitempty"`
	// MaxBytes caps how much of the tail of the file is read.
	MaxBytes int64 `json:"max_bytes,omitempty"`
}

// LogReadResult is what log.read returns.
type LogReadResult struct {
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
	Lines      []string  `json:"lines"`
	LineCount  int       `json:"line_count"`
	// Truncated says that the file is longer than the window returned, so a
	// reader does not mistake the first line returned for the start of the log.
	Truncated bool  `json:"truncated"`
	BytesRead int64 `json:"bytes_read"`
}

func (r *Registry) logRead(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args LogReadArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	resolved, err := resolveUnder(args.Path, r.logRoots(), "log")
	if err != nil {
		return nil, err
	}

	// Zero means "use the default"; a negative number is a caller that computed
	// something wrong, and answering it with the default would hide that.
	if args.Lines < 0 || args.Lines > maxLogLines {
		return nil, fmt.Errorf("%w: lines must be between 1 and %d, or omitted for %d",
			ErrInvalidArgument, maxLogLines, defaultLogLines)
	}
	if args.MaxBytes < 0 || args.MaxBytes > maxLogBytes {
		return nil, fmt.Errorf("%w: max_bytes must be between 1 and %d, or omitted for %d",
			ErrInvalidArgument, maxLogBytes, defaultLogBytes)
	}
	lines := args.Lines
	if lines == 0 {
		lines = defaultLogLines
	}
	window := args.MaxBytes
	if window == 0 {
		window = defaultLogBytes
	}

	// Checked before the open, so that a directory or a device node is refused
	// as an invalid argument rather than producing an obscure read error.
	if info, statErr := os.Lstat(resolved); statErr != nil {
		return nil, fmt.Errorf("%w: %s cannot be examined: %v", ErrInvalidArgument, resolved, statErr)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidArgument, resolved)
	}

	// O_NONBLOCK so that a named pipe planted under an allowed root cannot hold
	// this request open until the write timeout expires; see openNonBlock.
	file, err := os.OpenFile(resolved, os.O_RDONLY|openNonBlock, 0)
	if err != nil {
		return nil, fmt.Errorf("ops: cannot open %s: %w", resolved, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("ops: cannot stat %s: %w", resolved, err)
	}
	// Checked again on the open file descriptor, not on the path: between
	// resolving the path and opening it, what is at that name could have
	// changed.
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidArgument, resolved)
	}

	size := info.Size()
	readFrom := int64(0)
	toRead := size
	if size > window {
		readFrom = size - window
		toRead = window
	}
	buffer := make([]byte, toRead)
	read, err := file.ReadAt(buffer, readFrom)
	if err != nil && read == 0 {
		return nil, fmt.Errorf("ops: cannot read %s: %w", resolved, err)
	}
	buffer = buffer[:read]

	result := LogReadResult{
		Path:       resolved,
		SizeBytes:  size,
		ModifiedAt: info.ModTime().UTC(),
		BytesRead:  int64(read),
		Truncated:  readFrom > 0,
	}
	text := strings.Split(strings.TrimRight(string(buffer), "\n"), "\n")
	// Reading from the middle of the file almost certainly begins mid-line.
	// That fragment is dropped rather than returned as though it were a
	// complete log entry.
	if readFrom > 0 && len(text) > 0 {
		text = text[1:]
	}
	if len(text) > lines {
		text = text[len(text)-lines:]
		result.Truncated = true
	}
	if len(text) == 1 && text[0] == "" {
		text = nil
	}
	result.Lines = text
	result.LineCount = len(text)

	r.record(ctx, "log.read", audit.OutcomeExecuted,
		fmt.Sprintf("read the last %d line(s) of %s", result.LineCount, resolved))
	return result, nil
}

// ============================================================
// LISTING THE LOG FILES THAT CAN BE READ
// ============================================================

// LogFile is one entry in the log.list result.
type LogFile struct {
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

// LogListResult is what log.list returns.
type LogListResult struct {
	// Roots is the set of directories this agent will read a log from. It is
	// reported so that the panel can show an operator why a file it expected is
	// not listed, instead of the file merely being absent.
	Roots []string  `json:"roots"`
	Files []LogFile `json:"files"`
	// Truncated is set when there are more log files than the listing returns.
	Truncated bool `json:"truncated"`
}

// logList enumerates the readable log files. It exists so the panel does not
// have to guess a path and so a UI can offer what is actually there: without
// it, a panel would either hard-code paths that differ per distribution, or ask
// the agent to run `find`, which is the arbitrary command problem again.
func (r *Registry) logList(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	if err := noArguments(raw); err != nil {
		return nil, err
	}
	roots := r.logRoots()
	result := LogListResult{Roots: roots}

	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				// An unreadable subdirectory is skipped rather than failing the
				// listing: /var/log holds directories that only their own
				// daemon can enter, and none of them should cost the operator
				// the rest of the list.
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				if strings.Count(path, string(os.PathSeparator))-rootDepth >= maxLogListDepth {
					return fs.SkipDir
				}
				return nil
			}
			// Symlinks are not followed by WalkDir and are not listed: a listed
			// symlink would be refused by log.read anyway, once it resolved
			// outside the roots, and listing it would be misleading.
			if !entry.Type().IsRegular() || !looksLikeLog(entry.Name()) {
				return nil
			}
			if len(result.Files) >= maxLogListedFile {
				result.Truncated = true
				return fs.SkipAll
			}
			info, statErr := entry.Info()
			if statErr != nil {
				return nil
			}
			result.Files = append(result.Files, LogFile{
				Path:       path,
				SizeBytes:  info.Size(),
				ModifiedAt: info.ModTime().UTC(),
			})
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) && !os.IsNotExist(walkErr) {
			continue
		}
	}
	sort.Slice(result.Files, func(a, b int) bool { return result.Files[a].Path < result.Files[b].Path })
	r.record(ctx, "log.list", audit.OutcomeExecuted,
		fmt.Sprintf("listed %d log file(s) under %s", len(result.Files), strings.Join(roots, ", ")))
	return result, nil
}

// looksLikeLog decides what is offered in the listing. It is a convenience for
// the panel's file picker and nothing more: log.read's own validation does not
// consult it, so a log with an unusual name is still readable by path.
func looksLikeLog(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".log"), strings.HasSuffix(lower, ".err"), strings.HasSuffix(lower, ".out"):
		return true
	case strings.HasSuffix(lower, "_log"), strings.HasSuffix(lower, "-log"):
		return true
	case lower == "syslog", lower == "messages", lower == "dmesg", lower == "auth.log":
		return true
	default:
		return false
	}
}

// ============================================================
// DISK USAGE FOR A PATH
// ============================================================

// DiskUsageArgs is the argument object of disk.usage.
type DiskUsageArgs struct {
	// Path is the absolute path to measure, which must resolve under one of the
	// configured data roots.
	Path string `json:"path"`
	// Recursive measures the total size of a directory tree. Without it, only
	// the filesystem the path sits on is reported, which costs one syscall.
	Recursive bool `json:"recursive,omitempty"`
	// MaxEntries bounds a recursive measurement.
	MaxEntries int `json:"max_entries,omitempty"`
}

// DiskUsageResult is what disk.usage returns.
type DiskUsageResult struct {
	Path string `json:"path"`
	// Filesystem is the capacity of the filesystem holding the path, which is
	// the number that says whether the next write will succeed.
	Filesystem metrics.Mount `json:"filesystem"`
	// Entry is the path itself. It is absent unless the path was measured.
	Entry *PathUsage `json:"entry,omitempty"`
}

// PathUsage is the size of one path.
type PathUsage struct {
	IsDirectory bool  `json:"is_directory"`
	SizeBytes   int64 `json:"size_bytes"`
	Files       int64 `json:"files,omitempty"`
	Directories int64 `json:"directories,omitempty"`
	// Truncated says the walk hit its bound, so SizeBytes is a lower bound and
	// not the answer. A partial total presented as a total is how a full disk
	// gets missed.
	Truncated bool   `json:"truncated,omitempty"`
	StoppedAt string `json:"stopped_because,omitempty"`
}

func (r *Registry) diskUsage(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args DiskUsageArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	resolved, err := resolveUnder(args.Path, r.diskRoots(), "data")
	if err != nil {
		return nil, err
	}
	if args.MaxEntries < 0 || args.MaxEntries > maxUsageMaxEntries {
		return nil, fmt.Errorf("%w: max_entries must be between 1 and %d, or omitted for %d",
			ErrInvalidArgument, maxUsageMaxEntries, defaultUsageMaxEntries)
	}
	maxEntries := args.MaxEntries
	if maxEntries == 0 {
		maxEntries = defaultUsageMaxEntries
	}

	mount, err := r.collector().MountFor(resolved)
	if err != nil {
		return nil, fmt.Errorf("ops: cannot determine the filesystem holding %s: %w", resolved, err)
	}
	result := DiskUsageResult{Path: resolved, Filesystem: mount}

	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, fmt.Errorf("ops: cannot stat %s: %w", resolved, err)
	}
	switch {
	case !info.IsDir():
		result.Entry = &PathUsage{SizeBytes: info.Size()}
	case args.Recursive:
		usage := walkUsage(ctx, resolved, maxEntries)
		result.Entry = &usage
	}

	detail := fmt.Sprintf("measured %s on %s", resolved, mount.Mountpoint)
	if result.Entry != nil && result.Entry.IsDirectory {
		detail = fmt.Sprintf("walked %s (%d file(s), %d byte(s))", resolved, result.Entry.Files, result.Entry.SizeBytes)
	}
	r.record(ctx, "disk.usage", audit.OutcomeExecuted, detail)
	return result, nil
}

// walkUsage adds up a directory tree. Symlinks are counted at their own size
// and not followed, so a symlink loop cannot make this run forever and a
// symlink out of the tree cannot make a directory appear to contain something
// that is not in it.
func walkUsage(ctx context.Context, root string, maxEntries int) PathUsage {
	usage := PathUsage{IsDirectory: true}
	deadline := time.Now().Add(usageDeadline)
	entries := 0

	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subdirectory contributes nothing and does not stop
			// the measurement, but it does make the total a lower bound.
			usage.Truncated = true
			if usage.StoppedAt == "" {
				usage.StoppedAt = "part of the tree could not be read"
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			usage.Truncated = true
			usage.StoppedAt = "the request was cancelled"
			return fs.SkipAll
		}
		entries++
		if entries > maxEntries {
			usage.Truncated = true
			usage.StoppedAt = fmt.Sprintf("the limit of %d entries was reached", maxEntries)
			return fs.SkipAll
		}
		if time.Now().After(deadline) {
			usage.Truncated = true
			usage.StoppedAt = fmt.Sprintf("the measurement exceeded %s", usageDeadline)
			return fs.SkipAll
		}
		if entry.IsDir() {
			usage.Directories++
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		usage.Files++
		usage.SizeBytes += info.Size()
		return nil
	})
	return usage
}

// ============================================================
// PATH VALIDATION
// ============================================================

// resolveUnder is the gate every path argument passes through. It returns the
// fully resolved path, or ErrInvalidArgument describing what was wrong with the
// one that was asked for.
func resolveUnder(requested string, roots []string, kind string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidArgument)
	}
	if strings.ContainsRune(requested, 0) {
		return "", fmt.Errorf("%w: path contains a NUL byte", ErrInvalidArgument)
	}
	if !filepath.IsAbs(requested) {
		return "", fmt.Errorf("%w: %q is not an absolute path", ErrInvalidArgument, requested)
	}
	cleaned := filepath.Clean(requested)
	if !withinAny(cleaned, roots) {
		return "", fmt.Errorf("%w: %s is outside the %s directories this agent will read (%s)",
			ErrInvalidArgument, cleaned, kind, strings.Join(roots, ", "))
	}

	// Only now is the filesystem touched. Doing the prefix check first means a
	// path outside the roots is refused without this agent revealing whether it
	// exists.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s does not exist", ErrInvalidArgument, cleaned)
		}
		return "", fmt.Errorf("%w: %s cannot be resolved: %v", ErrInvalidArgument, cleaned, err)
	}
	if !withinAny(resolved, roots) {
		// The interesting case: a symlink inside an allowed directory whose
		// target is not. Named for what it is, because it is far more likely to
		// be an attempt than an accident.
		return "", fmt.Errorf("%w: %s resolves to %s, which is outside the %s directories this agent will read",
			ErrInvalidArgument, cleaned, resolved, kind)
	}
	return resolved, nil
}

func withinAny(path string, roots []string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if path == root || strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/") {
			return true
		}
	}
	return false
}

// sanitiseRoots drops anything that cannot safely be a root. "/" is refused
// outright: a root of "/" makes every path on the host readable and turns
// log.read back into the arbitrary read that this operation replaced.
func sanitiseRoots(roots []string, fallback []string) []string {
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == "." || root == "/" || !filepath.IsAbs(root) || seen[root] {
			continue
		}
		seen[root] = true
		out = append(out, root)
	}
	if len(out) == 0 {
		return fallback
	}
	sort.Strings(out)
	return out
}
