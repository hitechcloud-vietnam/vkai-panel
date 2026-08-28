//go:build linux

package metrics

// statfs on Linux, from the standard library's syscall package.
//
// This is a direct syscall rather than a df subprocess. df was what the agent
// used, and it meant a capacity report could fail because coreutils was absent
// from a minimal image or because --output was not supported by the busybox df
// that replaced it. It also meant two process spawns on every managed host on
// every report.
//
// golang.org/x/sys/unix would be the usual home for this, but the agent is a
// module with no dependencies at all, deliberately, so that what runs as root
// on a customer's server has no supply chain but the Go standard library.

import "syscall"

func statfs(path string) (FSStat, error) {
	var raw syscall.Statfs_t
	if err := syscall.Statfs(path, &raw); err != nil {
		return FSStat{}, err
	}
	// Bsize is the optimal transfer block size, which is the unit Blocks,
	// Bfree and Bavail are counted in on Linux.
	block := int64(raw.Bsize)
	return FSStat{
		TotalBytes:     int64(raw.Blocks) * block,
		FreeBytes:      int64(raw.Bfree) * block,
		AvailableBytes: int64(raw.Bavail) * block,
		InodesTotal:    int64(raw.Files),
		InodesFree:     int64(raw.Ffree),
	}, nil
}
