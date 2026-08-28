package upgrade

// The real implementations behind Deps. Each is small and does nothing but talk
// to the operating system, which is what makes the rest of the package testable
// without one.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ExecRunner runs commands with os/exec, inheriting the panel process's
// environment so that credentials already exported from /vkai-panel/etc/.env
// (PGPASSWORD and friends) reach pg_dump without being written to a file or a
// command line.
type ExecRunner struct{}

// Run executes name with args and returns its combined output. The output is
// returned on failure too, because systemctl explains itself on stderr and that
// explanation is the only thing the operator can act on.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %v: %w: %s", name, args, err, trimOutput(out))
	}
	return out, nil
}

func trimOutput(out []byte) string {
	const max = 2000
	s := string(out)
	if len(s) > max {
		s = s[:max] + "... (truncated)"
	}
	return s
}

// SystemClock is the real clock.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep blocks for d.
func (SystemClock) Sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}

// processAlive reports whether pid is a live process.
//
// Signal 0 performs the permission and existence checks without delivering
// anything. EPERM means the process exists but belongs to someone else, which
// still counts as alive - treating it as dead is how two upgrades end up
// running at once.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
