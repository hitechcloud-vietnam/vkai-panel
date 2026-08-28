package agentpki

import "github.com/hitechcloud-vietnam/vkai-panel/internal/config"

// sslRoot is the one place this package touches the filesystem layout. Every
// absolute path in the panel is declared in internal/config/paths.go and
// nowhere else, so the CA directory is derived from it rather than written out.
func sslRoot() string { return config.SSLRoot() }
