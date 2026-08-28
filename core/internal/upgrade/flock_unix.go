//go:build unix

package upgrade

import (
	"errors"
	"os"
	"syscall"
)

// flockExclusiveNonBlocking takes an exclusive advisory lock on f without
// waiting. It reports whether the lock was taken; an error means the call
// itself failed, not that someone else holds the lock.
//
// The lock belongs to the open file description, so it is released by closing f
// and by the process exiting for any reason, including being killed. That is
// the property this package needs: an upgrade that was OOM-killed leaves no
// lock behind to break.
func flockExclusiveNonBlocking(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			// A signal arrived. The lock was not taken; ask again.
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}
