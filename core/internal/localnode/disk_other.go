//go:build !linux

package localnode

// totalDiskBytes has no answer off Linux, so disk_total stays null. The panel
// is only supported on Linux; this file exists so that the package still builds
// under a developer's `go build` on another platform.
func totalDiskBytes(string) (int64, bool) { return 0, false }
