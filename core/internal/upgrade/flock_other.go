//go:build !unix

package upgrade

import (
	"errors"
	"os"
)

// flockExclusiveNonBlocking has no portable equivalent outside unix, and this
// package drives systemd, so there is no platform off unix where an upgrade is
// meaningful. Refusing here is better than silently falling back to a lock
// protocol with a race in it.
func flockExclusiveNonBlocking(*os.File) (bool, error) {
	return false, errors.New("the upgrade lock requires flock(2), which this platform does not provide")
}
