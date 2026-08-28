package phpfpm

// Command execution.
//
// Every external command in this package is built as an argv: a program name
// and a slice of arguments, handed to exec.CommandContext, never to a shell.
// There is no function here that takes a command line as a string, because a
// function like that is the one an attacker-controlled pool name eventually
// reaches.

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes one external command. It is an interface so a test can drive
// the whole rollback path - write, validate, reload, fail, restore - without a
// php-fpm binary or a systemd on the machine.
type Runner interface {
	// Run executes name with args and returns the combined output. The
	// returned error is non-nil when the command exits non-zero.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner is the real one.
type ExecRunner struct {
	// Timeout bounds a single command. A package manager install is slow, so
	// callers that need longer pass their own context; this is the floor.
	Timeout time.Duration
}

// NewExecRunner returns a runner with a sensible default timeout.
func NewExecRunner() *ExecRunner {
	return &ExecRunner{Timeout: 10 * time.Minute}
}

// Run executes the command. Note the absence of "sh -c": args go to the
// process as a vector, so a pool name of `; rm -rf /` is one argument that the
// program rejects, not two commands that the shell runs.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	// A package manager that stops to ask a question hangs the panel. Every
	// caller that needs non-interactivity passes the flag; this makes apt's
	// frontend match.
	cmd.Env = append(cmd.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.Bytes(), fmt.Errorf("%s timed out after %s: %s", name, timeout, trimOutput(out.String()))
	}
	if err != nil {
		return out.Bytes(), fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, trimOutput(out.String()))
	}
	return out.Bytes(), nil
}

// trimOutput keeps an error message readable. A failing php-fpm -t prints a
// handful of lines; a failing dnf prints hundreds.
func trimOutput(s string) string {
	s = strings.TrimSpace(s)
	const limit = 2000
	if len(s) <= limit {
		return s
	}
	return s[:limit] + " ... (truncated)"
}
