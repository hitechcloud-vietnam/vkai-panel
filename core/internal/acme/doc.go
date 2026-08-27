// Package acme is a self-contained ACME v2 (RFC 8555) client used to obtain and
// renew the certificate the panel serves on its own port.
//
// # Why this exists instead of shelling out to certbot
//
// The panel is reachable in two ways before an operator has pointed a domain at
// the machine: by IP address, and by hostname. A certificate for an IP address
// is possible today (RFC 8738 defines the "ip" identifier type), but Let's
// Encrypt only issues one through the "shortlived" certificate profile, and the
// resulting certificate lives about six days. Selecting a profile means sending
// a "profile" member in the newOrder request, which is a recent addition to the
// protocol.
//
// The certbot packaged by the distributions the panel must install on does not
// speak that. Ubuntu 24.04 ships certbot 2.9.0, which has no notion of ACME
// profiles at all, so it cannot ask for "shortlived" and therefore cannot get a
// certificate for an IP address. The panel supports nine OS families, each
// pinning a different certbot version, so "install a newer certbot" is not a
// portable answer either - it would make the panel's own TLS depend on the
// slowest-moving package in each distribution.
//
// This package therefore implements the small slice of ACME the panel needs,
// using only the Go standard library:
//
//   - directory discovery, including meta.profiles so the caller can see which
//     profiles the CA offers before asking for one;
//   - an ES256 account key generated once and kept at 0600 in a caller-supplied
//     directory, reusing the account URL (kid) on later runs;
//   - orders for both "dns" and "ip" identifiers, with an optional profile;
//   - the HTTP-01 challenge only - it is the one challenge type that works for
//     an IP identifier without occupying port 443, and DNS-01 is not defined for
//     IP identifiers at all.
//
// # Ports
//
// The panel listens on its own port (8888 by default) because 80 and 443 belong
// to the customer websites hosted on the same machine. HTTP-01 validation still
// has to happen on port 80, so this package does not open a socket by itself:
// the caller supplies a ChallengeSolver and decides whether to drop the token
// into an existing webroot served by nginx or to bind port 80 for the few
// seconds validation takes. Solvers for both are provided in solver.go.
//
// # Certbot is still used elsewhere
//
// Customer website certificates keep going through certbot exactly as before.
// This package is only for the panel's own certificate, where the profile
// requirement makes the distribution certbot unusable.
//
// # Typical use
//
//	client, err := acme.New(acme.Config{
//	        DirectoryURL: acme.LetsEncryptProduction,
//	        AccountDir:   "/usr/local/vkai/ssl/acme",
//	        Email:        "ops@example.com",
//	        AgreeTOS:     true,
//	})
//	if err != nil {
//	        return err
//	}
//
//	// Optional: confirm the CA still offers the profile before ordering.
//	profiles, err := client.Profiles(ctx)
//	if err != nil {
//	        return err
//	}
//	if !profiles.Has(acme.ProfileShortLived) {
//	        return fmt.Errorf("CA no longer offers %q", acme.ProfileShortLived)
//	}
//
//	solver, err := acme.NewHTTP01Server(":80")
//	if err != nil {
//	        return err
//	}
//	defer solver.Close()
//
//	chainPEM, keyPEM, err := client.Obtain(ctx,
//	        []acme.Identifier{acme.IPIdentifier("203.0.113.10")},
//	        acme.ProfileShortLived,
//	        solver,
//	)
//
// The caller writes chainPEM and keyPEM to the paths its TLS configuration
// points at and reloads, then re-runs when NeedsRenewal reports that less than a
// third of the certificate's lifetime is left - roughly every two days for a
// six-day shortlived certificate, and about thirty days before expiry for a
// classic ninety-day one.
//
// # Diagnosing failures
//
// Errors from this package carry the CA's problem document verbatim, so the type
// URN, the detail and any subproblems all reach the log in one line. Use
// errors.As to pick out *acme.Problem for the URN, or *acme.RateLimitError when
// the CA is throttling: it carries the Retry-After the CA asked for, so a
// scheduler can back off for exactly that long instead of retrying blindly.
package acme
