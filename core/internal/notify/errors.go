package notify

import (
	"errors"
	"fmt"
	"net/http"
)

// PermanentError marks a failure that retrying cannot fix: a rejected
// credential, a malformed configuration, an address the remote side refuses.
//
// The distinction matters in both directions. Retrying a permanent failure
// five times over half an hour delays the dead letter that tells an operator
// their channel is broken. Treating a temporary failure as permanent throws
// away an alert because a mail server was restarting.
type PermanentError struct {
	Err error
}

// Error implements error.
func (e *PermanentError) Error() string {
	if e.Err == nil {
		return "permanent failure"
	}
	return e.Err.Error()
}

// Unwrap lets errors.Is and errors.As see through to the cause.
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent marks an error as not worth retrying. It is idempotent.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	var already *PermanentError
	if errors.As(err, &already) {
		return err
	}
	return &PermanentError{Err: err}
}

// IsPermanent reports whether an error is marked as not worth retrying.
func IsPermanent(err error) bool {
	var permanent *PermanentError
	return errors.As(err, &permanent)
}

// classifyHTTPStatus turns a response status into an error of the right kind.
// It returns nil for a success.
//
// The split is the usual one with two deliberate exceptions: 408 and 429 are
// 4xx codes that mean "come back later", so they are retried. Everything else
// in the 4xx range means the request itself is wrong, and sending it again
// unchanged will be wrong again.
func classifyHTTPStatus(status int, detail string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests:
		return fmt.Errorf("remote returned %d %s: %s", status, http.StatusText(status), detail)
	case status >= 400 && status < 500:
		return Permanent(fmt.Errorf("remote rejected the request with %d %s: %s",
			status, http.StatusText(status), detail))
	default:
		return fmt.Errorf("remote returned %d %s: %s", status, http.StatusText(status), detail)
	}
}
