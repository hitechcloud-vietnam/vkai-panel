package agentpki

// The retirement of the static agent token.
//
// # What the old channel is
//
// Every row in `servers` carries `agent_token`: one uuid.New().String() minted
// when the server record was created, stored in the clear, sent by the agent on
// every call, compared for equality, and valid until the row is deleted. It is
// not a credential in any useful sense - it never expires, it cannot be
// rotated, it is readable by anything that can read the database or a request
// log, and possession of it is full authority over a machine the panel manages
// as root.
//
// # What replaces it
//
// An operator mints a single-use, time-limited enrolment token; the agent
// exchanges it once for a certificate; from then on the certificate - and the
// private key behind it, which never leaves the managed server - is the
// identity. That is the rest of this package.
//
// # Why this file exists at all
//
// An installation that upgrades has agents in the field holding static tokens.
// Deleting the old channel in the same release that adds the new one strands
// every one of them: they stop reporting, and the panel cannot reach them to
// tell them how to enrol. So the old channel stays, with three properties that
// make it a migration path rather than a permanent second door:
//
//   - It is deprecated in code and loud in the log. Every acceptance is a
//     warning naming the server, throttled to once an hour per server so a
//     fleet reporting every thirty seconds does not drown the log.
//   - It dies the moment the server it belongs to is enrolled. A server with a
//     certificate cannot fall back to its token, which is what stops an
//     attacker who holds a stolen token from downgrading around a revocation.
//   - A newly created server never gets a live one: Create writes a value
//     carrying RetiredTokenPrefix, which this file refuses before it reaches
//     any lookup.
//
// What cannot be fixed while the old channel exists is that the token is
// compared by a database equality match rather than in constant time, and that
// it is at rest in the clear. Both are properties of the value itself. The
// answer to them is to finish the migration and drop the column; see
// migrations/pending/agent_pki.sql.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Channel names, as reported to an operator and written to the log.
const (
	// ChannelMutualTLS is the certificate channel: the agent proved possession
	// of the private key behind a certificate this panel issued it.
	ChannelMutualTLS = "mutual-tls"

	// ChannelStaticToken is the deprecated channel: the agent presented the
	// bare string in servers.agent_token.
	ChannelStaticToken = "static-token"
)

// RetiredTokenPrefix marks a static token that must never authenticate
// anything. The column is NOT NULL UNIQUE, so a retired token cannot simply be
// blanked; it is replaced by a unique value that this package refuses on sight,
// before any database lookup happens.
const RetiredTokenPrefix = "retired-"

// legacyWarnEvery is how often one server's use of the deprecated channel is
// worth another log line. The point is that an operator sees it, not that every
// heartbeat is recorded.
const legacyWarnEvery = time.Hour

var (
	// ErrNoCredentials means the caller presented neither a certificate, nor a
	// signature, nor a token.
	ErrNoCredentials = errors.New("agentpki: the request carried no agent credentials")

	// ErrLegacyRetired means a static token was presented for a server that no
	// longer accepts one - because it has been enrolled, or because an operator
	// retired the token explicitly.
	ErrLegacyRetired = errors.New("agentpki: the static agent token for this server has been retired")

	// ErrLegacyUnavailable means this panel has no way to look a static token
	// up, so the deprecated channel cannot answer at all.
	ErrLegacyUnavailable = errors.New("agentpki: the deprecated static-token channel is not available on this panel")

	// ErrPKIUnavailable means the certificate authority could not be opened, so
	// no certificate can be checked. It is separate from a refusal: nothing was
	// wrong with the caller.
	ErrPKIUnavailable = errors.New("agentpki: the certificate authority is not available on this panel")
)

// LegacyServer is the little a directory has to know about a server that is
// still on the old channel.
type LegacyServer struct {
	ServerID string
	Hostname string

	// Retired is true when the stored token can no longer authenticate: it
	// carries RetiredTokenPrefix, or an operator retired it.
	Retired bool
}

// LegacyDirectory is the seam between this package and the panel database. It
// is deliberately three methods wide: this package must not learn SQL, and the
// repository must not learn what a certificate is.
type LegacyDirectory interface {
	// LookupByStaticToken finds the server a token belongs to. It returns
	// ErrNotFound when no server holds it.
	LookupByStaticToken(ctx context.Context, token string) (*LegacyServer, error)

	// ListStaticTokenServers returns every server that still holds a token,
	// retired ones included, so the census can tell the two apart.
	ListStaticTokenServers(ctx context.Context) ([]LegacyServer, error)

	// NoteStaticTokenUse records that the deprecated channel was used. A
	// failure here is bookkeeping lost, never an authentication refused.
	NoteStaticTokenUse(ctx context.Context, serverID string, at time.Time) error
}

// Credentials is everything a caller may have presented. A request carries at
// most one of the first two and, on an installation mid-migration, possibly the
// third as well.
type Credentials struct {
	// PeerCertificates is the raw chain from a completed TLS handshake, when
	// the agent connected to a listener that asks for a client certificate.
	PeerCertificates [][]byte

	// Signed and Body are the headers and the exact bytes of a request signed
	// with the agent's certificate key. This is how an agent authenticates to
	// the panel's ordinary HTTPS listener, which cannot demand a client
	// certificate from a browser.
	Signed SignedHeaders
	Body   []byte

	// StaticToken is the deprecated channel.
	StaticToken string
}

// Identity is who the caller turned out to be, and over which channel.
type Identity struct {
	AgentID  string
	ServerID string
	Hostname string
	Channel  string

	// Deprecated is true when this identity was established by the static
	// token. A caller that wants to refuse the old channel for a particular
	// operation checks this one field.
	Deprecated bool

	// Agent is the PKI record, present only on the certificate channel.
	Agent *AgentRecord
}

// Gateway is the one place that decides who an agent request is from. It exists
// so that the precedence between the two channels is written down once, in a
// package that can be tested without a database or a router.
type Gateway struct {
	authority *Authority
	legacy    LegacyDirectory
	logger    *zap.Logger
	now       func() time.Time

	mu     sync.Mutex
	warned map[string]time.Time
}

// NewGateway builds the gateway. legacy may be nil on an installation that has
// no database-backed servers, in which case the deprecated channel simply
// cannot answer.
func NewGateway(authority *Authority, legacy LegacyDirectory, logger *zap.Logger) *Gateway {
	if logger == nil {
		logger = zap.NewNop()
	}
	now := time.Now
	if authority != nil {
		now = authority.Now
	}
	return &Gateway{
		authority: authority,
		legacy:    legacy,
		logger:    logger,
		now:       now,
		warned:    make(map[string]time.Time),
	}
}

// Authority exposes the certificate authority behind the gateway.
func (g *Gateway) Authority() *Authority { return g.authority }

// IsRetiredToken reports whether a token value is one this panel wrote in place
// of a real one. It is checked before any lookup, so a retired token is refused
// even if a copy of the old value is presented.
func IsRetiredToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), RetiredTokenPrefix)
}

// Present reports whether a signed request's headers were supplied at all.
func (s SignedHeaders) Present() bool {
	return s.AgentID != "" || s.Signature != ""
}

// Authenticate decides who the caller is.
//
// The order is the whole point of this method: a certificate, in either of its
// two forms, is preferred over a token whenever one is presented, and a failed
// certificate is a refusal rather than a fall through to the token. Without
// that second rule, an agent whose certificate had just been revoked could
// present its old static token and be let back in, which would make revocation
// a suggestion.
func (g *Gateway) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	switch {
	case len(creds.PeerCertificates) > 0:
		return g.authenticateHandshake(ctx, creds)
	case creds.Signed.Present():
		return g.authenticateSignature(ctx, creds)
	case strings.TrimSpace(creds.StaticToken) != "":
		return g.authenticateStaticToken(ctx, creds.StaticToken)
	default:
		return nil, ErrNoCredentials
	}
}

func (g *Gateway) authenticateHandshake(ctx context.Context, creds Credentials) (*Identity, error) {
	if g.authority == nil {
		return nil, ErrPKIUnavailable
	}
	rec, err := g.authority.VerifyAgentPeer(ctx, creds.PeerCertificates, "")
	if err != nil {
		return nil, err
	}
	return identityFrom(rec), nil
}

func (g *Gateway) authenticateSignature(ctx context.Context, creds Credentials) (*Identity, error) {
	if g.authority == nil {
		return nil, ErrPKIUnavailable
	}
	rec, err := g.authority.VerifySignedRequest(ctx, creds.Signed, creds.Body)
	if err != nil {
		return nil, err
	}
	return identityFrom(rec), nil
}

func identityFrom(rec *AgentRecord) *Identity {
	return &Identity{
		AgentID:  rec.AgentID,
		ServerID: rec.ServerID,
		Hostname: rec.Hostname,
		Channel:  ChannelMutualTLS,
		Agent:    rec,
	}
}

// authenticateStaticToken is the deprecated path, kept working for exactly as
// long as an operator needs to enrol the agents that still use it.
func (g *Gateway) authenticateStaticToken(ctx context.Context, token string) (*Identity, error) {
	token = strings.TrimSpace(token)
	if IsRetiredToken(token) {
		return nil, ErrLegacyRetired
	}
	if g.legacy == nil {
		return nil, ErrLegacyUnavailable
	}
	server, err := g.legacy.LookupByStaticToken(ctx, token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnknownAgent
		}
		return nil, err
	}
	if server.Retired {
		return nil, ErrLegacyRetired
	}
	// A server that has enrolled has a certificate, and a certificate is the
	// only thing that speaks for it from that moment on. This is what makes
	// enrolment a one-way door and revocation final.
	if enrolled, lookupErr := g.enrolledFor(ctx, server.ServerID); lookupErr != nil {
		return nil, lookupErr
	} else if enrolled != nil {
		g.logger.Warn("Agent channel: a static token was refused for a server that is already enrolled",
			zap.String("server_id", server.ServerID),
			zap.String("hostname", server.Hostname),
			zap.String("agent_id", enrolled.AgentID),
			zap.Bool("agent_revoked", enrolled.Revoked))
		return nil, ErrLegacyRetired
	}

	now := g.now()
	g.warnDeprecated(server, now)
	if noteErr := g.legacy.NoteStaticTokenUse(ctx, server.ServerID, now); noteErr != nil {
		// Bookkeeping only. Refusing a request because the panel could not
		// write down that it happened would turn a reporting problem into an
		// outage on every server still mid-migration.
		g.logger.Debug("Agent channel: cannot record the use of a deprecated static token",
			zap.String("server_id", server.ServerID), zap.Error(noteErr))
	}
	return &Identity{
		ServerID:   server.ServerID,
		Hostname:   server.Hostname,
		Channel:    ChannelStaticToken,
		Deprecated: true,
	}, nil
}

// enrolledFor returns the agent record belonging to a server, or nil.
func (g *Gateway) enrolledFor(ctx context.Context, serverID string) (*AgentRecord, error) {
	if g.authority == nil || serverID == "" {
		return nil, nil
	}
	rec, err := g.authority.AgentForServer(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// warnDeprecated logs one server's continued use of the old channel, at most
// once an hour for that server.
func (g *Gateway) warnDeprecated(server *LegacyServer, now time.Time) {
	g.mu.Lock()
	last, seen := g.warned[server.ServerID]
	quiet := seen && now.Sub(last) < legacyWarnEvery
	if !quiet {
		g.warned[server.ServerID] = now
	}
	g.mu.Unlock()
	if quiet {
		return
	}
	g.logger.Warn("Agent channel: DEPRECATED static agent token accepted. "+
		"Enrol this server so it uses mutual TLS, then retire the token",
		zap.String("server_id", server.ServerID),
		zap.String("hostname", server.Hostname),
		zap.String("channel", ChannelStaticToken))
}

// ============================================================
// THE CENSUS
// ============================================================

// ChannelCensus is the answer to "how much of the fleet is still on the old
// channel", which is the number that decides when the column can be dropped.
type ChannelCensus struct {
	// Enrolled is the number of agents holding a certificate, revoked ones
	// excluded.
	Enrolled int

	// Revoked is the number of agent records whose certificates are denied.
	Revoked int

	// StaticTokenOnly is servers that still authenticate with a token and have
	// no certificate. This is the number that must reach zero.
	StaticTokenOnly int

	// StaticTokenSuperseded is servers that hold a live token but have already
	// enrolled. Their token no longer authenticates anything; retiring it is
	// tidying, not a fix.
	StaticTokenSuperseded int

	// Retired is servers whose token has been retired.
	Retired int

	// Pending names the servers behind StaticTokenOnly, so the log line and the
	// operator UI can say which ones.
	Pending []LegacyServer
}

// Census counts the fleet by channel.
func (g *Gateway) Census(ctx context.Context) (ChannelCensus, error) {
	var census ChannelCensus
	byServer := make(map[string]*AgentRecord)
	if g.authority != nil {
		records, err := g.authority.Store().ListAgents(ctx)
		if err != nil {
			return census, err
		}
		for _, rec := range records {
			if rec.Role != RoleAgent {
				continue
			}
			if rec.Revoked {
				census.Revoked++
			} else {
				census.Enrolled++
			}
			if rec.ServerID != "" {
				byServer[rec.ServerID] = rec
			}
		}
	}
	if g.legacy == nil {
		return census, nil
	}
	servers, err := g.legacy.ListStaticTokenServers(ctx)
	if err != nil {
		return census, err
	}
	for _, server := range servers {
		switch {
		case server.Retired:
			census.Retired++
		case byServer[server.ServerID] != nil:
			census.StaticTokenSuperseded++
		default:
			census.StaticTokenOnly++
			census.Pending = append(census.Pending, server)
		}
	}
	return census, nil
}

// LogCensus writes the startup line. A comment saying the old channel is
// deprecated is worth nothing to an operator; a line at every boot saying how
// many of their servers are still on it is what gets it finished.
func (g *Gateway) LogCensus(ctx context.Context) ChannelCensus {
	census, err := g.Census(ctx)
	if err != nil {
		g.logger.Warn("Agent channel: cannot count how many servers still use the deprecated static token",
			zap.Error(err))
		return census
	}
	fields := []zap.Field{
		zap.Int("enrolled_mutual_tls", census.Enrolled),
		zap.Int("revoked", census.Revoked),
		zap.Int("static_token_only", census.StaticTokenOnly),
		zap.Int("static_token_superseded", census.StaticTokenSuperseded),
		zap.Int("token_retired", census.Retired),
	}
	if census.StaticTokenOnly == 0 {
		g.logger.Info("Agent channel: every managed server is on mutual TLS", fields...)
		return census
	}
	fields = append(fields, zap.Strings("pending", census.PendingNames()))
	g.logger.Warn(fmt.Sprintf(
		"Agent channel: %d server(s) still authenticate with the DEPRECATED static agent token. "+
			"Enrol each one (Servers -> Add agent -> mint an enrolment token) and then retire its token",
		census.StaticTokenOnly), fields...)
	return census
}

// PendingNames lists the servers still on the old channel, hostname first and
// the identifier when there is no hostname, capped so one log line stays one
// log line.
func (c ChannelCensus) PendingNames() []string {
	const maxNames = 20
	names := make([]string, 0, len(c.Pending))
	for i, server := range c.Pending {
		if i == maxNames {
			names = append(names, fmt.Sprintf("... and %d more", len(c.Pending)-maxNames))
			break
		}
		if server.Hostname != "" {
			names = append(names, server.Hostname)
			continue
		}
		names = append(names, server.ServerID)
	}
	return names
}
