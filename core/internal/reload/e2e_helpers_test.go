package reload_test

// Small shims kept apart from the tests themselves so the tests read as what
// they check rather than as plumbing.

import (
	"context"
	"os"
	"time"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// contextCompat is a context with a deadline and nothing else, for the one call
// that needs one at shutdown.
type contextCompat struct {
	deadline time.Time
}

func (c contextCompat) Deadline() (time.Time, bool) { return c.deadline, true }
func (c contextCompat) Done() <-chan struct{}       { return nil }
func (c contextCompat) Err() error {
	if time.Now().After(c.deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
func (c contextCompat) Value(any) any { return nil }
