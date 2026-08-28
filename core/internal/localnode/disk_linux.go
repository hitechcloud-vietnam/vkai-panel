//go:build linux

package localnode

import "syscall"

// totalDiskBytes returns the size of the filesystem holding path.
//
// statfs is used rather than shelling out to df: no process is started and no
// argument reaches a shell. The agent cannot do this - it is built without cgo
// and without golang.org/x/sys - but the panel is a Linux service and can.
func totalDiskBytes(path string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	if st.Blocks == 0 || st.Bsize <= 0 {
		return 0, false
	}
	total := int64(st.Blocks) * int64(st.Bsize)
	if total <= 0 {
		return 0, false
	}
	return total, true
}
