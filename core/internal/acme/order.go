package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// newOrderRequest is the newOrder payload of RFC 8555 section 7.4, extended with
// the "profile" member the CA uses to select a certificate profile. The member
// is omitted entirely when no profile is requested, because a CA that does not
// understand it rejects an explicit null.
type newOrderRequest struct {
	Identifiers []Identifier `json:"identifiers"`
	Profile     string       `json:"profile,omitempty"`
	NotBefore   string       `json:"notBefore,omitempty"`
	NotAfter    string       `json:"notAfter,omitempty"`
}

// finalizeRequest is the finalize payload: the CSR in base64url DER.
type finalizeRequest struct {
	CSR string `json:"csr"`
}

// Obtain runs a full issuance: register the account if needed, create an order
// for ids, satisfy every HTTP-01 challenge through solver, finalize with a
// freshly generated ECDSA P-256 key and download the certificate chain.
//
// profile selects a certificate profile from the CA's meta.profiles and may be
// empty. Let's Encrypt issues certificates for IP identifiers only under
// ProfileShortLived, so an order containing an IdentifierIP without that profile
// is rejected by the CA.
//
// The returned chainPEM is the leaf followed by the issuer chain, exactly as the
// CA sent it, and keyPEM is the matching SEC 1 EC private key. Neither is written
// to disk: where the panel's certificate lives is the caller's decision.
//
// The whole call is bounded by Config.Timeout. Every token presented is cleaned
// up before returning, including on failure.
func (c *Client) Obtain(ctx context.Context, ids []Identifier, profile string, solver ChallengeSolver) (chainPEM, keyPEM []byte, err error) {
	if len(ids) == 0 {
		return nil, nil, errors.New("acme: Obtain needs at least one identifier")
	}
	if solver == nil {
		return nil, nil, errors.New("acme: Obtain needs a ChallengeSolver")
	}
	if err := validateIdentifiers(ids); err != nil {
		return nil, nil, err
	}

	timeout := c.cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := c.Account(ctx); err != nil {
		return nil, nil, err
	}
	if err := c.checkProfile(ctx, profile); err != nil {
		return nil, nil, err
	}

	order, err := c.newOrder(ctx, ids, profile)
	if err != nil {
		return nil, nil, err
	}

	if err := c.authorizeOrder(ctx, order, solver); err != nil {
		return nil, nil, err
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("acme: generate certificate key: %w", err)
	}
	csrDER, err := buildCSR(certKey, ids)
	if err != nil {
		return nil, nil, err
	}

	order, err = c.finalize(ctx, order, csrDER)
	if err != nil {
		return nil, nil, err
	}
	if order.Certificate == "" {
		return nil, nil, fmt.Errorf("acme: order %s is valid but carries no certificate URL", order.URL)
	}

	chainPEM, err = c.downloadCertificate(ctx, order.Certificate)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = encodeECPrivateKeyPEM(certKey)
	if err != nil {
		return nil, nil, err
	}
	return chainPEM, keyPEM, nil
}

// validateIdentifiers rejects identifier types this package cannot order and IP
// values that are not addresses, so the mistake surfaces here rather than as a
// generic "malformed" from the CA.
func validateIdentifiers(ids []Identifier) error {
	for _, id := range ids {
		switch id.Type {
		case IdentifierDNS:
			if strings.TrimSpace(id.Value) == "" {
				return errors.New("acme: dns identifier has an empty value")
			}
		case IdentifierIP:
			if net.ParseIP(id.Value) == nil {
				return fmt.Errorf("acme: ip identifier %q is not a valid IP address", id.Value)
			}
		default:
			return fmt.Errorf("acme: unsupported identifier type %q, want %q or %q", id.Type, IdentifierDNS, IdentifierIP)
		}
	}
	return nil
}

// checkProfile verifies the requested profile against meta.profiles. A CA that
// advertises no profiles at all is left alone: the member is then simply passed
// through and the CA decides.
func (c *Client) checkProfile(ctx context.Context, profile string) error {
	if profile == "" {
		return nil
	}
	profiles, err := c.Profiles(ctx)
	if err != nil {
		return err
	}
	if len(profiles) == 0 || profiles.Has(profile) {
		return nil
	}
	return fmt.Errorf("acme: %s does not offer profile %q, available profiles: %s",
		c.directoryURL, profile, strings.Join(profiles.Names(), ", "))
}

// newOrder creates an order for the identifiers, optionally under a profile.
func (c *Client) newOrder(ctx context.Context, ids []Identifier, profile string) (*Order, error) {
	dir, err := c.Directory(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(newOrderRequest{Identifiers: ids, Profile: profile})
	if err != nil {
		return nil, fmt.Errorf("acme: marshal newOrder request: %w", err)
	}
	resp, err := c.post(ctx, dir.NewOrder, payload, false)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusCreated && resp.Status != http.StatusOK {
		return nil, resp.toError(dir.NewOrder)
	}
	var order Order
	if err := json.Unmarshal(resp.Body, &order); err != nil {
		return nil, fmt.Errorf("acme: parse newOrder response: %w", err)
	}
	order.URL = resp.Header.Get("Location")
	if order.URL == "" {
		return nil, fmt.Errorf("acme: newOrder response from %s has no Location header", dir.NewOrder)
	}
	if order.Status == statusInvalid {
		return nil, orderError(&order)
	}
	return &order, nil
}

// authorizeOrder satisfies every pending authorization of the order over
// HTTP-01. Tokens are cleaned up before returning, whatever the outcome.
func (c *Client) authorizeOrder(ctx context.Context, order *Order, solver ChallengeSolver) (err error) {
	var presented []string
	defer func() {
		for _, token := range presented {
			if cleanupErr := solver.CleanUp(token); cleanupErr != nil && err == nil {
				err = fmt.Errorf("acme: clean up challenge token: %w", cleanupErr)
			}
		}
	}()

	for _, authzURL := range order.Authorizations {
		authz, err := c.getAuthorization(ctx, authzURL)
		if err != nil {
			return err
		}
		switch authz.Status {
		case statusValid:
			// A cached authorization from an earlier order: nothing to prove.
			continue
		case statusPending:
		default:
			return fmt.Errorf("acme: authorization %s for %s is %s and cannot be used",
				authzURL, authz.Identifier, authz.Status)
		}

		challenge, err := findHTTP01(authz, authzURL)
		if err != nil {
			return err
		}
		keyAuth, err := keyAuthorization(challenge.Token, &c.key.PublicKey)
		if err != nil {
			return err
		}
		if err := solver.Present(challenge.Token, keyAuth); err != nil {
			return fmt.Errorf("acme: present http-01 challenge for %s: %w", authz.Identifier, err)
		}
		presented = append(presented, challenge.Token)

		if err := c.acceptChallenge(ctx, challenge.URL); err != nil {
			return fmt.Errorf("acme: accept http-01 challenge for %s: %w", authz.Identifier, err)
		}
		if err := c.waitAuthorization(ctx, authzURL); err != nil {
			return err
		}
	}
	return nil
}

// findHTTP01 picks the http-01 challenge out of an authorization.
//
// HTTP-01 is the only challenge this package implements. For an IP identifier
// RFC 8738 allows only HTTP-01 and TLS-ALPN-01, and TLS-ALPN-01 would require
// taking over port 443 from the customer websites on the same machine.
func findHTTP01(authz *Authorization, authzURL string) (*Challenge, error) {
	offered := make([]string, 0, len(authz.Challenges))
	for i := range authz.Challenges {
		ch := &authz.Challenges[i]
		offered = append(offered, ch.Type)
		if ch.Type == ChallengeHTTP01 {
			if ch.Token == "" {
				return nil, fmt.Errorf("acme: http-01 challenge for %s has an empty token", authz.Identifier)
			}
			return ch, nil
		}
	}
	return nil, fmt.Errorf("acme: authorization %s for %s offers no http-01 challenge, only: %s",
		authzURL, authz.Identifier, strings.Join(offered, ", "))
}

// getAuthorization reads an authorization with POST-as-GET.
func (c *Client) getAuthorization(ctx context.Context, url string) (*Authorization, error) {
	resp, err := c.post(ctx, url, nil, false)
	if err != nil {
		return nil, err
	}
	var authz Authorization
	if err := json.Unmarshal(resp.Body, &authz); err != nil {
		return nil, fmt.Errorf("acme: parse authorization %s: %w", url, err)
	}
	return &authz, nil
}

// acceptChallenge tells the CA the challenge is ready for validation. The
// payload is an empty JSON object, not an empty payload, which is what
// distinguishes this from a POST-as-GET.
func (c *Client) acceptChallenge(ctx context.Context, url string) error {
	_, err := c.post(ctx, url, []byte("{}"), false)
	return err
}

// waitAuthorization polls an authorization until it becomes valid, with bounded
// exponential backoff and the deadline already on ctx.
func (c *Client) waitAuthorization(ctx context.Context, url string) error {
	delay := c.newBackoff()
	for {
		authz, err := c.getAuthorization(ctx, url)
		if err != nil {
			return err
		}
		switch authz.Status {
		case statusValid:
			return nil
		case statusPending, statusProcessing:
		case statusInvalid:
			return authorizationError(authz, url)
		default:
			return fmt.Errorf("acme: authorization %s for %s is %s", url, authz.Identifier, authz.Status)
		}
		if err := sleepContext(ctx, delay.next()); err != nil {
			return fmt.Errorf("acme: waiting for authorization %s: %w", url, err)
		}
	}
}

// finalize submits the CSR and polls the order until the certificate is ready.
func (c *Client) finalize(ctx context.Context, order *Order, csrDER []byte) (*Order, error) {
	// The order must be ready before it will accept a CSR. A CA that has just
	// validated the last authorization may still report "pending" for a moment.
	current := order
	delay := c.newBackoff()
	for current.Status == statusPending {
		if err := sleepContext(ctx, delay.next()); err != nil {
			return nil, fmt.Errorf("acme: waiting for order %s to become ready: %w", order.URL, err)
		}
		var err error
		current, err = c.getOrder(ctx, order.URL)
		if err != nil {
			return nil, err
		}
		if current.Status == statusInvalid {
			return nil, orderError(current)
		}
	}

	payload, err := json.Marshal(finalizeRequest{CSR: base64url(csrDER)})
	if err != nil {
		return nil, fmt.Errorf("acme: marshal finalize request: %w", err)
	}
	resp, err := c.post(ctx, current.Finalize, payload, false)
	if err != nil {
		return nil, err
	}
	var finalized Order
	if err := json.Unmarshal(resp.Body, &finalized); err != nil {
		return nil, fmt.Errorf("acme: parse finalize response from %s: %w", current.Finalize, err)
	}
	finalized.URL = order.URL

	return c.waitOrder(ctx, &finalized)
}

// waitOrder polls an order until it is valid.
func (c *Client) waitOrder(ctx context.Context, order *Order) (*Order, error) {
	current := order
	delay := c.newBackoff()
	for {
		switch current.Status {
		case statusValid:
			return current, nil
		case statusInvalid:
			return nil, orderError(current)
		case statusPending, statusReady, statusProcessing:
		default:
			return nil, fmt.Errorf("acme: order %s is %s", current.URL, current.Status)
		}
		if err := sleepContext(ctx, delay.next()); err != nil {
			return nil, fmt.Errorf("acme: waiting for order %s: %w", current.URL, err)
		}
		next, err := c.getOrder(ctx, current.URL)
		if err != nil {
			return nil, err
		}
		current = next
	}
}

// getOrder reads an order with POST-as-GET.
func (c *Client) getOrder(ctx context.Context, url string) (*Order, error) {
	resp, err := c.post(ctx, url, nil, false)
	if err != nil {
		return nil, err
	}
	var order Order
	if err := json.Unmarshal(resp.Body, &order); err != nil {
		return nil, fmt.Errorf("acme: parse order %s: %w", url, err)
	}
	order.URL = url
	return &order, nil
}

// downloadCertificate fetches the issued chain with POST-as-GET. The body is the
// leaf followed by the issuer chain in PEM, which is exactly what a TLS server
// wants in its certificate file.
func (c *Client) downloadCertificate(ctx context.Context, url string) ([]byte, error) {
	resp, err := c.post(ctx, url, nil, false)
	if err != nil {
		return nil, err
	}
	if len(resp.Body) == 0 {
		return nil, fmt.Errorf("acme: certificate at %s is empty", url)
	}
	if !strings.Contains(string(resp.Body), "BEGIN CERTIFICATE") {
		return nil, fmt.Errorf("acme: certificate at %s is not PEM encoded", url)
	}
	return resp.Body, nil
}

// buildCSR produces a DER CSR for the identifiers, signed with key.
//
// The subject is left empty on purpose: every name goes into the subjectAltName
// extension, DNS names as dNSName and IP identifiers as iPAddress per RFC 8738,
// and Let's Encrypt ignores the common name anyway.
func buildCSR(key *ecdsa.PrivateKey, ids []Identifier) ([]byte, error) {
	tmpl := &x509.CertificateRequest{
		Subject:            pkix.Name{},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	for _, id := range ids {
		switch id.Type {
		case IdentifierDNS:
			tmpl.DNSNames = append(tmpl.DNSNames, id.Value)
		case IdentifierIP:
			ip := net.ParseIP(id.Value)
			if ip == nil {
				return nil, fmt.Errorf("acme: ip identifier %q is not a valid IP address", id.Value)
			}
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		default:
			return nil, fmt.Errorf("acme: unsupported identifier type %q", id.Type)
		}
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, fmt.Errorf("acme: create CSR: %w", err)
	}
	return der, nil
}

// orderError turns a failed order into the CA's problem document when there is
// one, so the reason reaches the log verbatim.
func orderError(order *Order) error {
	if order.Error != nil {
		return fmt.Errorf("acme: order %s is invalid: %w", order.URL, order.Error)
	}
	return fmt.Errorf("acme: order %s is invalid and the CA gave no reason", order.URL)
}

// authorizationError reports why an authorization failed, preferring the
// challenge-level problem document because that is where the CA explains what it
// saw when it fetched the token.
func authorizationError(authz *Authorization, url string) error {
	for i := range authz.Challenges {
		if ch := &authz.Challenges[i]; ch.Error != nil {
			return fmt.Errorf("acme: authorization %s for %s failed on the %s challenge: %w",
				url, authz.Identifier, ch.Type, ch.Error)
		}
	}
	return fmt.Errorf("acme: authorization %s for %s is invalid and the CA gave no reason", url, authz.Identifier)
}

// backoff produces a bounded exponential delay sequence for status polling.
type backoff struct {
	current time.Duration
	max     time.Duration
}

// newBackoff builds a backoff from the client configuration.
func (c *Client) newBackoff() *backoff {
	start := c.cfg.PollInterval
	if start <= 0 {
		start = defaultPollInterval
	}
	max := c.cfg.MaxPollInterval
	if max <= 0 {
		max = defaultMaxPollInterval
	}
	if max < start {
		max = start
	}
	return &backoff{current: start, max: max}
}

// next returns the delay to use now and doubles it for the following call, up to
// the cap.
func (b *backoff) next() time.Duration {
	d := b.current
	if b.current < b.max {
		b.current *= 2
		if b.current > b.max {
			b.current = b.max
		}
	}
	return d
}

// sleepContext waits for d unless ctx ends first, in which case the context
// error is returned. This is what makes Config.Timeout an overall deadline
// rather than a per-request one.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
