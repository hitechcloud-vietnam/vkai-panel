//go:build !linux

package upgrade

import "fmt"

// diskFree has no portable implementation. The panel only ships on Linux; this
// build exists so that "go build ./..." on a developer's machine still works,
// and it fails loudly rather than pretending the disk is empty.
func diskFree(path string) (uint64, error) {
	return 0, fmt.Errorf("free space for %s cannot be measured on this platform; inject Deps.DiskFree", path)
}
