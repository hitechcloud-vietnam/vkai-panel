//go:build !linux

package metrics

// The agent runs on Linux. This file exists so that a developer on macOS can
// still build and test the rest of the agent, and so that the failure is an
// honest "unavailable on this platform" in the report rather than a filesystem
// that appears to have zero bytes.

import "errors"

func statfs(string) (FSStat, error) {
	return FSStat{}, errors.New("filesystem capacity is only collected on Linux")
}
