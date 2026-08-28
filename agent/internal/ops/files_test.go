package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/audit"
)

// sandbox builds a registry whose log and data roots are a temporary directory,
// so the path validation can be exercised without the tests depending on what
// is in /var/log on the machine they run on.
type sandbox struct {
	registry *Registry
	root     string
	logDir   string
	dataDir  string
	record   *audit.Log
	auditLog string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	// The temporary directory is resolved through any symlink up front, because
	// the operations return resolved paths and the assertions compare against
	// them.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("cannot resolve the temporary directory: %v", err)
	}
	logDir := filepath.Join(root, "logs")
	dataDir := filepath.Join(root, "data")
	outside := filepath.Join(root, "outside")
	for _, dir := range []string{logDir, dataDir, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("cannot create %s: %v", dir, err)
		}
	}
	auditPath := filepath.Join(root, "operations.log")
	record := audit.Open(audit.Options{Path: auditPath, Fallback: log.New(io.Discard, "", 0)})
	t.Cleanup(func() { _ = record.Close() })

	registry := New(Deps{
		Run:           (&fakeRunner{}).run,
		Logger:        log.New(io.Discard, "", 0),
		Audit:         record,
		LogRoots:      []string{logDir},
		DiskRoots:     []string{dataDir},
		ApplyDenyList: func([]string) {},
		Info:          func() AgentInfo { return AgentInfo{} },
	})
	return &sandbox{registry: registry, root: root, logDir: logDir, dataDir: dataDir,
		record: record, auditLog: auditPath}
}

func (s *sandbox) write(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
	return path
}

// call dispatches with an identified caller, the way the control channel does.
func (s *sandbox) call(t *testing.T, name string, args any) (interface{}, error) {
	t.Helper()
	raw := json.RawMessage(nil)
	if args != nil {
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatalf("cannot encode the arguments: %v", err)
		}
		raw = encoded
	}
	ctx := audit.WithActor(context.Background(), audit.Actor{
		Name: "vkai-panel", Serial: "0a1b", Address: "10.0.0.5:44321",
	})
	return s.registry.Dispatch(ctx, name, raw)
}

func (s *sandbox) auditContents(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(s.auditLog)
	if err != nil {
		t.Fatalf("cannot read the operation record: %v", err)
	}
	return string(data)
}

// ============================================================
// log.read
// ============================================================

// An unconstrained path argument would make "read a log file" equivalent to
// reading the agent's own private key, which is the same failure /execute had
// wearing a narrower name.
func TestLogReadRefusesEveryPathOutsideItsRoots(t *testing.T) {
	box := newSandbox(t)
	box.write(t, filepath.Join(box.logDir, "site.log"), "hello\n")
	box.write(t, filepath.Join(box.root, "outside", "secret"), "credentials\n")

	for _, name := range []string{
		"",            // nothing at all
		"site.log",    // relative, so not anchored anywhere
		"/etc/shadow", // the obvious target
		"/etc/passwd", //
		filepath.Join(box.root, "outside", "secret"),              // inside the sandbox but outside the roots
		box.logDir + "/../outside/secret",                         // traversal out of a root
		box.logDir + "-sibling/site.log",                          // a string prefix of a root, not a path prefix
		filepath.Join(box.logDir, "site.log") + "\x00/etc/shadow", // a NUL byte
		filepath.Join(box.logDir, "does-not-exist.log"),
		box.logDir, // a directory, not a file
	} {
		_, err := box.call(t, "log.read", LogReadArgs{Path: name})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("log.read accepted %q (error was %v)", name, err)
		}
	}
}

// The interesting case, because it passes a naive prefix check: a symlink
// planted inside an allowed directory whose target is not.
func TestLogReadRefusesASymlinkThatEscapesItsRoots(t *testing.T) {
	box := newSandbox(t)
	secret := box.write(t, filepath.Join(box.root, "outside", "id_rsa"), "PRIVATE KEY\n")
	link := filepath.Join(box.logDir, "innocent.log")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("this filesystem does not support symlinks: %v", err)
	}

	_, err := box.call(t, "log.read", LogReadArgs{Path: link})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("log.read followed a symlink out of its roots (error was %v)", err)
	}
	if !strings.Contains(err.Error(), "resolves to") {
		t.Fatalf("the refusal does not say that the path resolved elsewhere: %v", err)
	}
}

// A named pipe under an allowed root would block the operation forever, and a
// device node would be read as though it were text.
func TestLogReadRefusesAnythingThatIsNotARegularFile(t *testing.T) {
	box := newSandbox(t)
	fifo := filepath.Join(box.logDir, "pipe.log")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("this platform cannot create a FIFO: %v", err)
	}
	_, err := box.call(t, "log.read", LogReadArgs{Path: fifo})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("log.read accepted a named pipe (error was %v)", err)
	}
}

func TestLogReadReturnsTheEndOfTheFile(t *testing.T) {
	box := newSandbox(t)
	var body strings.Builder
	for n := 1; n <= 500; n++ {
		fmt.Fprintf(&body, "line %d\n", n)
	}
	path := box.write(t, filepath.Join(box.logDir, "nginx", "error.log"), body.String())

	result, err := box.call(t, "log.read", LogReadArgs{Path: path, Lines: 10})
	if err != nil {
		t.Fatalf("log.read failed: %v", err)
	}
	read := result.(LogReadResult)
	if read.LineCount != 10 {
		t.Fatalf("log.read returned %d lines, want 10", read.LineCount)
	}
	if read.Lines[9] != "line 500" {
		t.Fatalf("the last line is %q, want \"line 500\"", read.Lines[9])
	}
	if read.Lines[0] != "line 491" {
		t.Fatalf("the first line returned is %q, want \"line 491\"", read.Lines[0])
	}
	if !read.Truncated {
		t.Fatal("a window over a longer file is not flagged as truncated")
	}
	if read.Path != path {
		t.Fatalf("the result reports %q as the path read, want %q", read.Path, path)
	}
}

func TestLogReadOnAShortFileReturnsAllOfItAndSaysItIsComplete(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "small.log"), "one\ntwo\nthree\n")
	result, err := box.call(t, "log.read", LogReadArgs{Path: path})
	if err != nil {
		t.Fatalf("log.read failed: %v", err)
	}
	read := result.(LogReadResult)
	if read.LineCount != 3 || read.Truncated {
		t.Fatalf("a three line file came back as %+v", read)
	}
}

// Reading from the middle of a large file begins mid-line. Returning that
// fragment as though it were a complete entry is a lie a log viewer cannot see
// through.
func TestLogReadDropsThePartialFirstLineOfItsWindow(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "big.log"),
		strings.Repeat("a", 100)+"\n"+"second line\n"+"third line\n")

	result, err := box.call(t, "log.read", LogReadArgs{Path: path, MaxBytes: 40})
	if err != nil {
		t.Fatalf("log.read failed: %v", err)
	}
	read := result.(LogReadResult)
	for _, line := range read.Lines {
		if strings.HasPrefix(line, "aaa") {
			t.Fatalf("the truncated first line was returned as a whole entry: %q", line)
		}
	}
	if !read.Truncated {
		t.Fatal("a windowed read is not flagged as truncated")
	}
}

func TestLogReadRefusesAWindowLargerThanItWillEverServe(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "site.log"), "hello\n")
	for _, args := range []LogReadArgs{
		{Path: path, Lines: maxLogLines + 1},
		{Path: path, MaxBytes: maxLogBytes + 1},
	} {
		if _, err := box.call(t, "log.read", args); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("log.read accepted %+v (error was %v)", args, err)
		}
	}
}

func TestLogReadRefusesAnArgumentItDoesNotUnderstand(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "site.log"), "hello\n")
	_, err := box.registry.Dispatch(context.Background(), "log.read",
		json.RawMessage(fmt.Sprintf(`{"path":%q,"follow":true}`, path)))
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("log.read accepted an argument it does not implement (error was %v)", err)
	}
}

// ============================================================
// log.list
// ============================================================

func TestLogListOffersTheFilesUnderItsRootsAndNothingElse(t *testing.T) {
	box := newSandbox(t)
	box.write(t, filepath.Join(box.logDir, "nginx", "access.log"), "x\n")
	box.write(t, filepath.Join(box.logDir, "syslog"), "x\n")
	box.write(t, filepath.Join(box.logDir, "nginx", "nginx.conf"), "x\n")
	box.write(t, filepath.Join(box.root, "outside", "other.log"), "x\n")

	result, err := box.call(t, "log.list", nil)
	if err != nil {
		t.Fatalf("log.list failed: %v", err)
	}
	listing := result.(LogListResult)
	found := map[string]bool{}
	for _, file := range listing.Files {
		found[file.Path] = true
	}
	if !found[filepath.Join(box.logDir, "nginx", "access.log")] || !found[filepath.Join(box.logDir, "syslog")] {
		t.Fatalf("log.list did not offer the log files it can read: %+v", listing.Files)
	}
	if found[filepath.Join(box.logDir, "nginx", "nginx.conf")] {
		t.Fatal("log.list offered a configuration file")
	}
	if found[filepath.Join(box.root, "outside", "other.log")] {
		t.Fatal("log.list offered a file outside its roots")
	}
	if len(listing.Roots) != 1 || listing.Roots[0] != box.logDir {
		t.Fatalf("log.list reports its roots as %v, want [%s]", listing.Roots, box.logDir)
	}
}

// ============================================================
// disk.usage
// ============================================================

func TestDiskUsageRefusesAPathOutsideItsRoots(t *testing.T) {
	box := newSandbox(t)
	for _, path := range []string{"", "relative/path", "/etc", filepath.Join(box.root, "outside")} {
		if _, err := box.call(t, "disk.usage", DiskUsageArgs{Path: path}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("disk.usage accepted %q (error was %v)", path, err)
		}
	}
}

func TestDiskUsageReportsTheFilesystemAndTheSizeOfThePath(t *testing.T) {
	box := newSandbox(t)
	site := filepath.Join(box.dataDir, "example.com")
	box.write(t, filepath.Join(site, "index.html"), strings.Repeat("a", 1000))
	box.write(t, filepath.Join(site, "assets", "app.js"), strings.Repeat("b", 2000))

	result, err := box.call(t, "disk.usage", DiskUsageArgs{Path: site, Recursive: true})
	if err != nil {
		t.Fatalf("disk.usage failed: %v", err)
	}
	usage := result.(DiskUsageResult)
	if usage.Filesystem.Mountpoint == "" || usage.Filesystem.TotalBytes == nil {
		t.Fatalf("the filesystem holding the path was not reported: %+v", usage.Filesystem)
	}
	if usage.Entry == nil || !usage.Entry.IsDirectory {
		t.Fatalf("the directory was not measured: %+v", usage.Entry)
	}
	if usage.Entry.SizeBytes != 3000 {
		t.Fatalf("the directory measured %d bytes, want 3000", usage.Entry.SizeBytes)
	}
	if usage.Entry.Files != 2 {
		t.Fatalf("the directory holds %d files, want 2", usage.Entry.Files)
	}
	if usage.Entry.Truncated {
		t.Fatalf("a complete measurement was flagged as truncated: %s", usage.Entry.StoppedAt)
	}
}

func TestDiskUsageWithoutRecursiveDoesNotWalkADirectory(t *testing.T) {
	box := newSandbox(t)
	site := filepath.Join(box.dataDir, "example.com")
	box.write(t, filepath.Join(site, "index.html"), "x")

	result, err := box.call(t, "disk.usage", DiskUsageArgs{Path: site})
	if err != nil {
		t.Fatalf("disk.usage failed: %v", err)
	}
	if usage := result.(DiskUsageResult); usage.Entry != nil {
		t.Fatalf("a non-recursive call measured the tree anyway: %+v", usage.Entry)
	}
}

// A total that stopped early and is presented as a total is how a full disk
// gets missed.
func TestDiskUsageSaysWhenItStoppedShort(t *testing.T) {
	box := newSandbox(t)
	site := filepath.Join(box.dataDir, "busy")
	for n := 0; n < 50; n++ {
		box.write(t, filepath.Join(site, fmt.Sprintf("file-%02d", n)), "0123456789")
	}
	result, err := box.call(t, "disk.usage", DiskUsageArgs{Path: site, Recursive: true, MaxEntries: 10})
	if err != nil {
		t.Fatalf("disk.usage failed: %v", err)
	}
	usage := result.(DiskUsageResult)
	if !usage.Entry.Truncated {
		t.Fatalf("a measurement that hit its limit was not flagged: %+v", usage.Entry)
	}
	if usage.Entry.StoppedAt == "" {
		t.Fatal("a truncated measurement does not say why it stopped")
	}
	if usage.Entry.SizeBytes >= 500 {
		t.Fatalf("the truncated measurement returned %d bytes, which is the whole tree", usage.Entry.SizeBytes)
	}
}

func TestDiskUsageRefusesAnUnreasonableEntryLimit(t *testing.T) {
	box := newSandbox(t)
	if _, err := box.call(t, "disk.usage",
		DiskUsageArgs{Path: box.dataDir, MaxEntries: maxUsageMaxEntries + 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("disk.usage accepted an unbounded entry limit (error was %v)", err)
	}
}

// ============================================================
// THE RECORD ON THE NODE
// ============================================================

// Every operation must leave evidence on the machine it ran on, naming the
// certificate that asked for it.
func TestEveryPathOperationRecordsWhoAskedAndWhatItTouched(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "site.log"), "hello\n")
	if _, err := box.call(t, "log.read", LogReadArgs{Path: path}); err != nil {
		t.Fatalf("log.read failed: %v", err)
	}
	contents := box.auditContents(t)
	if !strings.Contains(contents, "vkai-panel") || !strings.Contains(contents, "0a1b") {
		t.Fatalf("the record does not name the caller: %s", contents)
	}
	if !strings.Contains(contents, path) {
		t.Fatalf("the record does not name the file that was read: %s", contents)
	}
	if !strings.Contains(contents, `"operation":"log.read"`) {
		t.Fatalf("the record does not name the operation: %s", contents)
	}
}

func TestServiceControlRecordsTheExactCommandItRan(t *testing.T) {
	box := newSandbox(t)
	if _, err := box.call(t, "service.control", ServiceControlArgs{Name: "nginx", Action: "restart"}); err != nil {
		t.Fatalf("service.control failed: %v", err)
	}
	contents := box.auditContents(t)
	if !strings.Contains(contents, "systemctl restart nginx") {
		t.Fatalf("the record does not hold the command line that ran: %s", contents)
	}
	if !strings.Contains(contents, "vkai-panel") {
		t.Fatalf("the record does not name who asked for the restart: %s", contents)
	}
}

// ============================================================
// ROOT CONFIGURATION
// ============================================================

// A configuration mistake must not be able to reopen the hole these operations
// were written to close.
func TestSlashCannotBeConfiguredAsARoot(t *testing.T) {
	roots := sanitiseRoots([]string{"/", "//", "relative", "", "/var/log", "/var/log"}, DefaultLogRoots)
	for _, root := range roots {
		if root == "/" {
			t.Fatalf("\"/\" survived as a root: %v", roots)
		}
		if !filepath.IsAbs(root) {
			t.Fatalf("%q survived as a root", root)
		}
	}
	if len(roots) != 1 || roots[0] != "/var/log" {
		t.Fatalf("the roots were sanitised to %v, want [/var/log]", roots)
	}
	// A configuration that names nothing usable falls back to the defaults
	// rather than to no constraint at all.
	if got := sanitiseRoots([]string{"/"}, DefaultLogRoots); len(got) != len(DefaultLogRoots) {
		t.Fatalf("a configuration of only \"/\" produced %v", got)
	}
}

// A negative count is a caller that computed something wrong. Answering it with
// the default would hide the mistake in the panel that made it.
func TestANegativeCountIsRefusedRatherThanTreatedAsTheDefault(t *testing.T) {
	box := newSandbox(t)
	path := box.write(t, filepath.Join(box.logDir, "site.log"), "hello\n")
	for _, args := range []any{
		LogReadArgs{Path: path, Lines: -1},
		LogReadArgs{Path: path, MaxBytes: -1},
	} {
		if _, err := box.call(t, "log.read", args); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("log.read accepted %+v (error was %v)", args, err)
		}
	}
	if _, err := box.call(t, "disk.usage",
		DiskUsageArgs{Path: box.dataDir, MaxEntries: -1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("disk.usage accepted a negative entry limit (error was %v)", err)
	}
}
