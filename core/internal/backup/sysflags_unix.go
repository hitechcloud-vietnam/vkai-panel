//go:build unix

package backup

import "syscall"

// oNoFollow makes an open refuse to traverse a final symlink. A restore runs
// as root, so following one is a write to wherever the link points - including
// a link that was already on disk before the restore started.
const oNoFollow = syscall.O_NOFOLLOW
