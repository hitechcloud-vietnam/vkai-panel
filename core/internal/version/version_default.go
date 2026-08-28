// Code generated from the repository VERSION file by "make sync-version".
// DO NOT EDIT: edit /VERSION and run "make sync-version".
//
// This file exists because Go's //go:embed cannot reach outside the module
// directory (core/), while VERSION has to stay at the repository root where the
// installer, the Makefile, the UI build and the release workflow all read it.
// The copy below is therefore mechanical, and the "Version Check" workflow fails
// any pull request in which it has drifted from VERSION.

package version

// defaultVersion is the version a build reports when no linker flags were
// passed, that is, a plain "go build ./...".
const defaultVersion = "0.3.0"
