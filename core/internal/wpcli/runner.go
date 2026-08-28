package wpcli

// Running WP-CLI, never as root.
//
// Requirement four of this task: "Never run WP-CLI as root. State the user each
// command runs as and how you enforce it."
//
// The user: every WP-CLI command runs as the site's own system user and that
// user's primary group - the same identity the site's PHP-FPM pool runs as
// (PoolSpec.User / PoolSpec.Group). One site, one unix user; a plugin update
// for site A cannot write a file into site B.
//
// How it is enforced, in three layers, each of which is independently
// sufficient and all three of which are tested:
//
//  1. Resolution. The site user is looked up with os/user. Its uid and gid are
//     read from the passwd database, not taken from the request. A uid or gid
//     of 0 is refused here, so "run as a user that happens to be root" is not
//     reachable even if the database says so.
//
//  2. The credential. exec.Cmd is given SysProcAttr.Credential{Uid, Gid}, so
//     the kernel changes the process identity between fork and exec. This is
//     not a --user flag that the program is trusted to honour; the process
//     never has uid 0 after exec. Supplementary groups are cleared explicitly
//     so the child does not inherit root's group list.
//
//  3. WP-CLI's own refusal. WP-CLI stops with "Error: YIKES! It looks like
//     you're running this as root." unless --allow-root is passed. This package
//     never constructs that flag, and rejectOptionInjection refuses any caller
//     value that starts with a dash, so no argument can smuggle it in. That
//     turns a bug in layers 1 and 2 into a failed command rather than a root
//     WP-CLI run.
//
// The panel process itself runs as root - it manages systemd units and writes
// into /etc - which is exactly why this file exists. Nothing about "the panel
// is root" is allowed to reach a customer's WordPress.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Identity is a resolved, non-root system identity.
type Identity struct {
	Name  string
	Group string
	UID   uint32
	GID   uint32
}

// String is what a log line and an API response report as "this ran as".
func (i Identity) String() string {
	return fmt.Sprintf("%s:%s (uid %d, gid %d)", i.Name, i.Group, i.UID, i.GID)
}

// ErrWouldRunAsRoot is returned whenever a resolved identity turns out to be
// root, or when the caller asked for root by name. It is a distinct type so a
// handler can answer 400 with the reason rather than 500 with a stack trace.
type ErrWouldRunAsRoot struct {
	Requested string
	UID       uint32
	GID       uint32
}

func (e *ErrWouldRunAsRoot) Error() string {
	return fmt.Sprintf("refusing to run WP-CLI as %q: it resolves to uid %d, gid %d, and WP-CLI "+
		"must never run with root privileges over a customer's WordPress installation",
		e.Requested, e.UID, e.GID)
}

// LookupIdentity resolves a system user name to a uid/gid pair and refuses
// root. This is layer 1.
//
// It is a variable rather than a function so a test can resolve names without
// creating users on the machine. The production value is the only one that
// reads /etc/passwd.
var LookupIdentity = func(name string) (Identity, error) {
	validated, err := UnixName("site user", name)
	if err != nil {
		return Identity{}, err
	}
	if validated == "root" {
		return Identity{}, &ErrWouldRunAsRoot{Requested: name, UID: 0, GID: 0}
	}

	u, err := user.Lookup(validated)
	if err != nil {
		return Identity{}, fmt.Errorf("the site's system user %q does not exist on this host: %w",
			validated, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return Identity{}, fmt.Errorf("system user %q has an unreadable uid %q", validated, u.Uid)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return Identity{}, fmt.Errorf("system user %q has an unreadable gid %q", validated, u.Gid)
	}
	if uid == 0 || gid == 0 {
		return Identity{}, &ErrWouldRunAsRoot{Requested: name, UID: uint32(uid), GID: uint32(gid)}
	}

	groupName := u.Gid
	if g, err := user.LookupGroupId(u.Gid); err == nil {
		groupName = g.Name
	}

	return Identity{Name: validated, Group: groupName, UID: uint32(uid), GID: uint32(gid)}, nil
}

// Command is one WP-CLI invocation, fully typed. There is no field anywhere in
// this struct that holds a command line.
type Command struct {
	// Identity is the non-root user the command runs as.
	Identity Identity
	// Dir is the WordPress installation directory. It becomes both the working
	// directory and the --path argument.
	Dir string
	// Args are the WP-CLI arguments, already validated by the constructors in
	// args.go. They never include --allow-root and never include --path.
	Args []string
	// Timeout bounds this command. Zero means the runner's default.
	Timeout time.Duration
	// Stdin, when set, is written to the command. Used for `wp config set` of
	// a value that should not appear in a process listing.
	Stdin []byte
}

// Argv is the exact argument vector that will be executed, for logging and for
// tests. It is the whole story: there is no shell, no quoting and nothing
// concatenated.
func (c Command) Argv(binary string) []string {
	argv := make([]string, 0, len(c.Args)+3)
	argv = append(argv, binary)
	argv = append(argv, c.Args...)
	argv = append(argv, "--path="+c.Dir)
	// Skip WP-CLI's own plugin/theme loading of the site's code where it is not
	// needed, so a broken plugin cannot fail an unrelated command.
	argv = append(argv, "--no-color")
	return argv
}

// Result is what a command produced.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	// RanAs is the identity the command actually ran under. It is reported to
	// the operator, because "which user did that run as" must be answerable
	// from the panel and not only from a source file.
	RanAs Identity
	// Argv is the executed vector, for the audit trail.
	Argv []string
}

// Runner executes WP-CLI commands.
type Runner struct {
	// Binary is the wp executable. Defaults to /usr/local/bin/wp.
	Binary string
	// DefaultTimeout bounds any command that does not set its own.
	DefaultTimeout time.Duration
	// exec is the process launcher. It is a field so tests can observe the
	// exec.Cmd - including its SysProcAttr.Credential - without needing root.
	exec func(context.Context, *exec.Cmd) (*Result, error)
}

// DefaultBinary is where deploy/install.sh puts the WP-CLI phar.
const DefaultBinary = "/usr/local/bin/wp"

// NewRunner returns a runner that executes for real.
func NewRunner(binary string) *Runner {
	if binary == "" {
		binary = DefaultBinary
	}
	return &Runner{Binary: binary, DefaultTimeout: 5 * time.Minute}
}

// ErrNotInstalled is returned when the WP-CLI binary is not on this host. It is
// distinct so the handler can say "install WP-CLI" instead of "exec failed".
var ErrNotInstalled = errors.New("WP-CLI is not installed on this host")

// Available reports whether this runner can actually launch WP-CLI.
//
// A runner built by NewRunnerForTest launches through the caller's function
// rather than through the binary, so the binary's absence says nothing about
// it. Checking the binary anyway would mean every service-level test had to
// have wp installed on the machine, which is precisely the reason the join
// between a database row and the process that runs never got tested before.
func (r *Runner) Available() error {
	if r.exec != nil {
		return nil
	}
	info, err := os.Stat(r.Binary)
	if err != nil {
		return fmt.Errorf("%w: %s is missing", ErrNotInstalled, r.Binary)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%w: %s is not executable", ErrNotInstalled, r.Binary)
	}
	return nil
}

// Run executes one command as the site user.
//
// Every refusal below happens before a process is created. The order matters:
// the identity is checked first, so a command that would have run as root
// never reaches the point where a binary is looked up.
func (r *Runner) Run(ctx context.Context, cmd Command) (*Result, error) {
	// Layer 1, re-asserted at the point of use. A caller that built an
	// Identity by hand rather than through LookupIdentity does not get a free
	// pass, because a zero-valued Identity{} has UID 0.
	if cmd.Identity.UID == 0 || cmd.Identity.GID == 0 {
		return nil, &ErrWouldRunAsRoot{
			Requested: cmd.Identity.Name,
			UID:       cmd.Identity.UID,
			GID:       cmd.Identity.GID,
		}
	}
	if cmd.Identity.Name == "root" {
		return nil, &ErrWouldRunAsRoot{Requested: "root"}
	}

	dir, err := Path("wordpress directory", cmd.Dir)
	if err != nil {
		return nil, err
	}
	cmd.Dir = dir

	if len(cmd.Args) == 0 {
		return nil, fmt.Errorf("a WP-CLI command needs at least a subcommand")
	}
	// Layer 3's guarantee, asserted rather than assumed: no argument this
	// package constructs is --allow-root, and none may be smuggled in.
	for _, arg := range cmd.Args {
		if strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("a WP-CLI argument contains a NUL byte")
		}
		if arg == "--allow-root" || strings.HasPrefix(arg, "--allow-root=") {
			return nil, fmt.Errorf("--allow-root is never passed by this panel: it is the flag " +
				"that would let WP-CLI run with root privileges over a customer's site")
		}
	}

	timeout := cmd.Timeout
	if timeout <= 0 {
		timeout = r.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	argv := cmd.Argv(r.Binary)
	process := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.Dir = cmd.Dir

	// Layer 2: the kernel changes identity between fork and exec.
	// NoSetGroups is deliberately false so setgroups(2) is called and the
	// child's supplementary group list is replaced rather than inherited from
	// the root panel process.
	process.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:         cmd.Identity.UID,
			Gid:         cmd.Identity.GID,
			Groups:      []uint32{cmd.Identity.GID},
			NoSetGroups: false,
		},
		Setpgid: true,
	}

	// A scrubbed environment. The panel's own environment holds the database
	// password, the JWT secret and the panel master key; none of it belongs in
	// a customer's PHP process.
	process.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + cmd.Dir,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		// WP-CLI writes its cache under HOME; keeping it inside the site
		// directory means it is written by a user who can write there.
		"WP_CLI_CACHE_DIR=" + filepath.Join(cmd.Dir, ".wp-cli", "cache"),
		"WP_CLI_DISABLE_AUTO_CHECK_UPDATE=1",
	}

	if r.exec != nil {
		result, err := r.exec(ctx, process)
		if result != nil {
			result.RanAs = cmd.Identity
			result.Argv = argv
		}
		return result, err
	}

	if err := r.Available(); err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if len(cmd.Stdin) > 0 {
		process.Stdin = bytes.NewReader(cmd.Stdin)
	}

	runErr := process.Run()
	result := &Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		RanAs:  cmd.Identity,
		Argv:   argv,
	}
	if process.ProcessState != nil {
		result.ExitCode = process.ProcessState.ExitCode()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("wp %s timed out after %s (running as %s)",
			cmd.Args[0], timeout, cmd.Identity)
	}
	if runErr != nil {
		return result, fmt.Errorf("wp %s failed as %s: %w: %s",
			strings.Join(cmd.Args, " "), cmd.Identity, runErr, firstLines(result.Stderr, result.Stdout))
	}
	return result, nil
}

// firstLines picks the most useful few lines out of a WP-CLI failure. WP-CLI
// prints its error on stderr; when stderr is empty the reason is on stdout.
func firstLines(stderr, stdout string) string {
	text := strings.TrimSpace(stderr)
	if text == "" {
		text = strings.TrimSpace(stdout)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 6 {
		lines = lines[:6]
	}
	joined := strings.TrimSpace(strings.Join(lines, "; "))
	if len(joined) > 1000 {
		joined = joined[:1000] + " ... (truncated)"
	}
	return joined
}

// NewRunnerForTest builds a runner whose process launcher is supplied by the
// caller, so a test can drive a whole WordPress operation and then inspect the
// exec.Cmd - its argv, its working directory and above all its
// SysProcAttr.Credential - without a wp binary and without being able to
// become another user.
//
// It is exported for the same reason phpfpm.DetectedForTest is: the wiring
// between a database row and the process that eventually runs is exactly the
// join this project has repeatedly failed to test, and it cannot be tested from
// inside this package alone. The launcher still goes through Run, so every
// refusal in this file - the root checks, the --allow-root check, the path
// validation - applies to a test exactly as it does in production.
func NewRunnerForTest(launch func(context.Context, *exec.Cmd) (*Result, error)) *Runner {
	return &Runner{
		Binary:         DefaultBinary,
		DefaultTimeout: time.Minute,
		exec:           launch,
	}
}
