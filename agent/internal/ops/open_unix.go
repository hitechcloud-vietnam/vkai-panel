//go:build !windows

package ops

import "syscall"

// openNonBlock is added to the flags log.read opens a file with.
//
// Opening a named pipe for reading blocks until something opens the other end,
// and a named pipe can be created under /var/log by anything that can write
// there. Without this flag, one log.read against such a path would hold the
// request until the agent's write timeout expired - and would do it again on
// every retry. The mode check that follows the open rejects the pipe; this flag
// is what guarantees the open itself returns so that the check can run.
const openNonBlock = syscall.O_NONBLOCK
