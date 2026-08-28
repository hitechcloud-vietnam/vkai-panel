//go:build !unix

package upgrade

// oNoFollow has no equivalent outside unix. The upgrader only runs on Linux -
// it drives systemd - so this exists to keep the package compiling for tooling
// that builds every platform, not because extraction is supported here.
const oNoFollow = 0
