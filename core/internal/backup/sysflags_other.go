//go:build !unix

package backup

// oNoFollow has no equivalent outside unix. The panel only runs on Linux, so
// this exists to keep the package compiling for tooling that builds every
// platform, not because a restore is supported here.
const oNoFollow = 0
