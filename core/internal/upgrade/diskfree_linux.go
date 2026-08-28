//go:build linux

package upgrade

import (
	"fmt"
	"syscall"
)

// diskFree reports the bytes available at path to an unprivileged writer.
//
// Bavail is used rather than Bfree on purpose: Bfree includes the reserve that
// only root may dip into, and counting it would let preflight pass on a
// filesystem that the panel's own user cannot actually write to.
func diskFree(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	if st.Bsize <= 0 {
		return 0, fmt.Errorf("statfs %s reported a block size of %d", path, st.Bsize)
	}
	return st.Bavail * uint64(st.Bsize), nil
}
