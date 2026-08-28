package agentpki

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================
// A DIRECTORY, STANDING IN FOR THE SERVERS TABLE
// ============================================================

type fakeDirectory struct {
	mu       sync.Mutex
	byToken  map[string]*LegacyServer
	uses     map[string]time.Time
	failWith error
}

func newDirectory(servers ...*LegacyServer) *fakeDirectory {
	d := &fakeDirectory{
		byToken: make(map[string]*LegacyServer),
		uses:    make(map[string]time.Time),
	}
	for i, server := range servers {
		d.byToken[tokenFor(i)] = server
	}
	return d
}

// tokenFor is the shape of the value the old channel actually used: a bare
// uuid, stored in the clear, unchanging.
func tokenFor(i int) string {
	return fmt.Sprintf("3f2504e0-4f89-11d3-9a0c-0305e82c330%d", i)
}

func (d *fakeDirectory) LookupByStaticToken(_ context.Context, token string) (*LegacyServer, error) {
	if d.failWith != nil {
		return nil, d.failWith
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	server, ok := d.byToken[token]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *server
	return &clone, nil
}

func (d *fakeDirectory) ListStaticTokenServers(_ context.Context) ([]LegacyServer, error) {
	if d.failWith != nil {
		return nil, d.failWith
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]LegacyServer, 0, len(d.byToken))
	for _, server := range d.byToken {
		out = append(out, *server)
	}
	return out, nil
}

func (d *fakeDirectory) NoteStaticTokenUse(_ context.Context, serverID string, at time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.uses[serverID] = at
	return nil
}

func (d *fakeDirectory) usedAt(serverID string) time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.uses[serverID]
}

// enrolForServer enrols an agent against one panel server record, which is what
// binds a certificate to the row that used to hold a static token.
func enrolForServer(t *testing.T, a *Authority, serverID, hostname string) *agentIdentity {
	t.Helper()
	invite, err := a.MintEnrolment(context.Background(), serverID, hostname, "test-operator", 0)
	if err != nil {
		t.Fatalf("cannot mint an enrolment token: %v", err)
	}
	key, csrPEM := newCSR(t, hostname)
	issued, err := a.Enrol(context.Background(), EnrolRequest{
		Token:    invite.Token,
		CSRPEM:   string(csrPEM),
		Hostname: hostname,
	})
	if err != nil {
		t.Fatalf("enrolment failed: %v", err)
	}
	return &agentIdentity{
		id:      issued.AgentID,
		key:     key,
		pair:    pairFrom(t, key, issued.CertPEM),
		issued:  issued,
		certPEM: issued.CertPEM,
	}
}

func signedFor(t *testing.T, id *agentIdentity, clk *clock, body []byte) SignedHeaders {
	t.Helper()
	headers, err := SignRequest(id.id, id.issued.Serial, id.key, clk.Now(), body)
	if err != nil {
		t.Fatalf("cannot sign a request: %v", err)
	}
	return headers
}

// ============================================================
// THE DEPRECATED CHANNEL, WHILE IT LASTS
// ============================================================

func TestAStaticTokenIsStillAcceptedAndReportedAsDeprecated(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "old-node"})
	g := NewGateway(a, dir, nil)

	identity, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)})
	if err != nil {
		t.Fatalf("an agent that has not been enrolled yet was locked out: %v", err)
	}
	if identity.Channel != ChannelStaticToken || !identity.Deprecated {
		t.Fatalf("channel is %q deprecated=%v, want the deprecated static token channel",
			identity.Channel, identity.Deprecated)
	}
	if identity.ServerID != "server-1" {
		t.Fatalf("identified server %q, want server-1", identity.ServerID)
	}
	if dir.usedAt("server-1").IsZero() {
		t.Fatal("the use of the deprecated channel was not recorded, so no operator can see it")
	}
}

func TestAnUnknownStaticTokenIsRefused(t *testing.T) {
	clk := newClock()
	g := NewGateway(newAuthority(t, clk), newDirectory(&LegacyServer{ServerID: "server-1"}), nil)

	_, err := g.Authenticate(context.Background(), Credentials{StaticToken: "not-a-token-anyone-holds"})
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("Authenticate returned %v, want ErrUnknownAgent", err)
	}
}

func TestARetiredStaticTokenIsRefusedWithoutAnyLookup(t *testing.T) {
	clk := newClock()
	dir := newDirectory()
	retired := RetiredTokenPrefix + "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	dir.byToken[retired] = &LegacyServer{ServerID: "server-9"}
	g := NewGateway(newAuthority(t, clk), dir, nil)

	_, err := g.Authenticate(context.Background(), Credentials{StaticToken: retired})
	if !errors.Is(err, ErrLegacyRetired) {
		t.Fatalf("Authenticate returned %v, want ErrLegacyRetired", err)
	}
	// A server created after this release gets a retired value in that column,
	// so it must be refused even when the directory would have answered.
	if !IsRetiredToken(retired) {
		t.Fatal("IsRetiredToken does not recognise the value the panel writes for a new server")
	}
}

func TestAnOperatorRetiredTokenIsRefused(t *testing.T) {
	clk := newClock()
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "crossed-over", Retired: true})
	g := NewGateway(newAuthority(t, clk), dir, nil)

	_, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)})
	if !errors.Is(err, ErrLegacyRetired) {
		t.Fatalf("Authenticate returned %v, want ErrLegacyRetired", err)
	}
}

// ============================================================
// THE CERTIFICATE WINS
// ============================================================

func TestACertificateIsPreferredWhenBothArePresented(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "node-1"})
	g := NewGateway(a, dir, nil)
	id := enrolForServer(t, a, "server-2", "node-2")

	body := []byte(`{"hello":"panel"}`)
	identity, err := g.Authenticate(context.Background(), Credentials{
		Signed:      signedFor(t, id, clk, body),
		Body:        body,
		StaticToken: tokenFor(0),
	})
	if err != nil {
		t.Fatalf("a signed request was refused: %v", err)
	}
	if identity.Channel != ChannelMutualTLS || identity.Deprecated {
		t.Fatalf("channel is %q deprecated=%v, want the certificate channel",
			identity.Channel, identity.Deprecated)
	}
	if identity.AgentID != id.id || identity.ServerID != "server-2" {
		t.Fatalf("identified %q/%q, want %q/server-2", identity.AgentID, identity.ServerID, id.id)
	}
	// The token that was also presented belonged to a different server, and it
	// must not have been what answered.
	if !dir.usedAt("server-1").IsZero() {
		t.Fatal("the static token was consulted even though a certificate was presented")
	}
}

func TestAHandshakeCertificateIsPreferredOverAToken(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1"})
	g := NewGateway(a, dir, nil)
	id := enrolForServer(t, a, "server-2", "node-2")

	identity, err := g.Authenticate(context.Background(), Credentials{
		PeerCertificates: id.pair.Certificate,
		StaticToken:      tokenFor(0),
	})
	if err != nil {
		t.Fatalf("a handshake certificate was refused: %v", err)
	}
	if identity.Channel != ChannelMutualTLS || identity.AgentID != id.id {
		t.Fatalf("identified %q over %q, want %q over mutual TLS", identity.AgentID, identity.Channel, id.id)
	}
}

func TestAFailedCertificateIsNotAFallbackToTheToken(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "node-1"})
	g := NewGateway(a, dir, nil)
	id := enrolForServer(t, a, "server-1", "node-1")

	body := []byte(`{}`)
	headers := signedFor(t, id, clk, body)
	headers.Signature = "AAAA"

	_, err := g.Authenticate(context.Background(), Credentials{
		Signed:      headers,
		Body:        body,
		StaticToken: tokenFor(0),
	})
	if err == nil {
		t.Fatal("a request whose signature did not verify was let in on the static token instead")
	}
	if errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Authenticate returned %v, want the signature failure", err)
	}
}

func TestAStaticTokenDiesWhenItsServerEnrols(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "node-1"})
	g := NewGateway(a, dir, nil)

	// Before enrolment the token is the only thing this server has.
	if _, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)}); err != nil {
		t.Fatalf("the token was refused before the server enrolled: %v", err)
	}

	enrolForServer(t, a, "server-1", "node-1")

	// After it, the certificate is the identity and the string is not.
	_, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)})
	if !errors.Is(err, ErrLegacyRetired) {
		t.Fatalf("Authenticate returned %v, want ErrLegacyRetired for an enrolled server", err)
	}
}

func TestARevokedAgentCannotFallBackToItsStaticToken(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(&LegacyServer{ServerID: "server-1", Hostname: "node-1"})
	g := NewGateway(a, dir, nil)
	id := enrolForServer(t, a, "server-1", "node-1")

	if err := a.Revoke(context.Background(), id.id, "key suspected stolen"); err != nil {
		t.Fatalf("cannot revoke: %v", err)
	}

	// The certificate is refused, which is the existing guarantee.
	body := []byte(`{}`)
	if _, err := g.Authenticate(context.Background(), Credentials{
		Signed: signedFor(t, id, clk, body),
		Body:   body,
	}); !errors.Is(err, ErrRevoked) {
		t.Fatalf("a revoked agent's signed request returned %v, want ErrRevoked", err)
	}

	// And the old string is not a way back in. Without this, revocation would
	// be a suggestion on any installation that has not finished migrating.
	if _, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)}); err == nil {
		t.Fatal("a revoked agent was let back in on the static token it used to hold")
	}
}

func TestNoCredentialsIsItsOwnAnswer(t *testing.T) {
	clk := newClock()
	g := NewGateway(newAuthority(t, clk), newDirectory(), nil)
	if _, err := g.Authenticate(context.Background(), Credentials{}); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Authenticate returned %v, want ErrNoCredentials", err)
	}
}

func TestTheDeprecatedChannelCanBeTurnedOff(t *testing.T) {
	clk := newClock()
	g := NewGateway(newAuthority(t, clk), nil, nil)
	_, err := g.Authenticate(context.Background(), Credentials{StaticToken: tokenFor(0)})
	if !errors.Is(err, ErrLegacyUnavailable) {
		t.Fatalf("Authenticate returned %v, want ErrLegacyUnavailable", err)
	}
}

// ============================================================
// THE CENSUS
// ============================================================

func TestTheCensusCountsTheFleetByChannel(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	dir := newDirectory(
		&LegacyServer{ServerID: "server-1", Hostname: "still-old"},
		&LegacyServer{ServerID: "server-2", Hostname: "enrolled-token-not-yet-retired"},
		&LegacyServer{ServerID: "server-3", Hostname: "finished", Retired: true},
	)
	g := NewGateway(a, dir, nil)
	enrolForServer(t, a, "server-2", "enrolled-token-not-yet-retired")
	revoked := enrolForServer(t, a, "server-4", "gone")
	if err := a.Revoke(context.Background(), revoked.id, "decommissioned"); err != nil {
		t.Fatalf("cannot revoke: %v", err)
	}

	census, err := g.Census(context.Background())
	if err != nil {
		t.Fatalf("the census failed: %v", err)
	}
	if census.StaticTokenOnly != 1 {
		t.Fatalf("static_token_only=%d, want 1", census.StaticTokenOnly)
	}
	if census.StaticTokenSuperseded != 1 {
		t.Fatalf("static_token_superseded=%d, want 1", census.StaticTokenSuperseded)
	}
	if census.Retired != 1 {
		t.Fatalf("token_retired=%d, want 1", census.Retired)
	}
	if census.Enrolled != 1 || census.Revoked != 1 {
		t.Fatalf("enrolled=%d revoked=%d, want 1 and 1", census.Enrolled, census.Revoked)
	}
	names := census.PendingNames()
	if len(names) != 1 || names[0] != "still-old" {
		t.Fatalf("the census names %v as pending, want [still-old]", names)
	}
}

func TestTheCensusIsHonestWhenThereIsNothingLeftToMigrate(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	g := NewGateway(a, newDirectory(&LegacyServer{ServerID: "server-1", Retired: true}), nil)
	enrolForServer(t, a, "server-1", "node-1")

	census := g.LogCensus(context.Background())
	if census.StaticTokenOnly != 0 || len(census.Pending) != 0 {
		t.Fatalf("the census reports %d server(s) pending on a fleet that has finished", census.StaticTokenOnly)
	}
}

// ============================================================
// REVOCATION REACHES WHAT IS ALREADY OPEN
// ============================================================

func TestRevocationNotifiesWhoeverHoldsConnections(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)
	id := enrol(t, a, "node-1")

	var mu sync.Mutex
	var dropped []string
	a.OnRevoke(func(agentID string) {
		mu.Lock()
		defer mu.Unlock()
		dropped = append(dropped, agentID)
	})

	if err := a.Revoke(context.Background(), id.id, "operator"); err != nil {
		t.Fatalf("cannot revoke: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dropped) != 1 || dropped[0] != id.id {
		t.Fatalf("the revocation notified %v, want [%s]", dropped, id.id)
	}
}

func TestEnrolmentTellsThePanelThatAServerHasCrossedOver(t *testing.T) {
	clk := newClock()
	a := newAuthority(t, clk)

	var mu sync.Mutex
	crossed := map[string]string{}
	a.OnEnrol(func(serverID, agentID string) {
		mu.Lock()
		defer mu.Unlock()
		crossed[serverID] = agentID
	})

	id := enrolForServer(t, a, "server-7", "node-7")
	mu.Lock()
	defer mu.Unlock()
	if crossed["server-7"] != id.id {
		t.Fatalf("enrolment recorded %v, want server-7 -> %s", crossed, id.id)
	}
}
