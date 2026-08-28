//go:build windows

package ops

// Windows has no O_NONBLOCK and no named pipes on this path. The agent is a
// Linux service; this file exists so the package still compiles for anyone
// building the whole module on a Windows workstation.
const openNonBlock = 0
