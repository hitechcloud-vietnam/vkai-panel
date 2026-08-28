//go:build unix

package upgrade

import "syscall"

// oNoFollow makes an open refuse to traverse a final symlink. Extraction runs
// as root, so following one is a write to wherever the link points.
const oNoFollow = syscall.O_NOFOLLOW
