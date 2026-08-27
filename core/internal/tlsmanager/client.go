package tlsmanager

// The ACME surface this package depends on.
//
// Only three declarations are needed to order a certificate: an identifier, a
// challenge solver and a client that turns the two into a PEM chain. Keeping
// them here rather than importing them means the TLS lifecycle can be built,
// tested and reviewed without the ACME protocol implementation existing yet,
// and it keeps the dependency pointing one way: internal/acme never needs to
// know that a panel certificate is what it is issuing.

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/acme"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Identifier is one ACME identifier: {"dns", "panel.example.com"} or
// {"ip", "116.118.2.44"}.
//
// Aliased rather than redeclared so *acme.Client satisfies Client directly and
// no adapter has to be maintained between the two packages.
type Identifier = acme.Identifier

// Solver publishes and withdraws the answer to a single HTTP-01 challenge.
// Present must make keyAuth retrievable at
// http://<identifier>/.well-known/acme-challenge/<token> before it returns.
type Solver = acme.ChallengeSolver

// Client orders a certificate for one set of identifiers under one ACME
// profile. It returns the full chain and the private key, both PEM encoded.
type Client interface {
	Obtain(ctx context.Context, identifiers []Identifier, profile string, solver Solver) (chainPEM, keyPEM []byte, err error)
}

// ErrACMEClientUnavailable is returned when an ACME client cannot be built.
// The manager treats it like any other issuance failure - it keeps serving the
// certificate already on the wire - so the panel still starts and simply stays
// on its self-signed certificate.
var ErrACMEClientUnavailable = errors.New("tlsmanager: no ACME client is available")

// newACMEClient builds the client used by the "letsencrypt" TLS mode.
//
// The account key lives beside the panel certificate rather than in the
// certificate directory itself: losing it means every future order starts from
// a new account, which counts against the CA's account rate limit.
func newACMEClient(email string, staging bool) (Client, error) {
	directory := acme.LetsEncryptProduction
	if staging {
		directory = acme.LetsEncryptStaging
	}
	client, err := acme.New(acme.Config{
		DirectoryURL: directory,
		AccountDir:   filepath.Join(config.PanelSSLDir(), "acme"),
		Email:        email,
		// The operator agreed by choosing the "letsencrypt" TLS mode; the
		// installer and the settings page both state the terms apply.
		AgreeTOS: true,
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

var _ = context.Background // context stays imported for the Client interface
