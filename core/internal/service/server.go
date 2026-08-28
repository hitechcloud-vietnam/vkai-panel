package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/localnode"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// censusTimeout bounds the one query this service makes at startup. A panel
// whose database is slow to answer must still finish booting.
const censusTimeout = 5 * time.Second

type ServerService struct {
	serverRepo *repository.ServerRepository
	logger     *zap.Logger

	// The panel host as a managed node. localNodes is the repository seam and
	// probe is the machine; both are interfaces so the identity guard can be
	// tested without a database and without being on the machine in question.
	localNodes localNodeStore
	probe      localnode.Probe

	heartbeatOnce sync.Once
	heartbeatMu   sync.Mutex
	heartbeatStop chan struct{}
}

func NewServerService(serverRepo *repository.ServerRepository, logger *zap.Logger) *ServerService {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &ServerService{
		serverRepo: serverRepo,
		logger:     logger,
		probe:      localnode.NewSystemProbe(),
	}
	// A typed nil pointer in an interface is not a nil interface, and every
	// local-node method tests the interface for nil, so the assignment is
	// guarded rather than unconditional.
	if serverRepo != nil {
		s.localNodes = serverRepo
	}
	// The deprecation is announced at every boot rather than left as a comment
	// in a file nobody opens. An operator who upgraded six months ago and still
	// has four servers on the old channel is told so, by name, every time the
	// panel starts.
	ctx, cancel := context.WithTimeout(context.Background(), censusTimeout)
	defer cancel()
	s.LogAgentChannelCensus(ctx)

	// The panel registers the machine it is running on, at every start, so that
	// installing the panel is all it takes to have a node to work with. It is
	// done here rather than in the installer because the facts it writes have
	// to be measured on the running system, and because a panel that is moved
	// or renamed then corrects itself on the next boot instead of drifting.
	s.bootstrapLocalNode()
	return s
}

func (s *ServerService) Create(ctx context.Context, tenantID uuid.UUID, req models.CreateServerRequest) (*models.Server, error) {
	server := &models.Server{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Hostname:    req.Hostname,
		IPAddress:   req.IPAddress,
		SSHPort:     req.SSHPort,
		AgentStatus: "offline",
		// Not a credential. servers.agent_token is NOT NULL UNIQUE until the
		// migration that drops it, so a value has to go in; this one carries
		// the retired prefix and is refused before any lookup happens. A server
		// created today joins by enrolling in the PKI, never by holding a
		// string. See migrations/pending/agent_pki.sql.
		AgentToken: repository.NewAgentToken(),
		Location:   req.Location,
		Tags:       req.Tags,
		Role:       req.Role,
		Status:     "active",
	}

	if server.SSHPort == 0 {
		server.SSHPort = 22
	}

	if err := s.serverRepo.Create(ctx, server); err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return server, nil
}

func (s *ServerService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Server, error) {
	return s.serverRepo.GetByID(ctx, tenantID, id)
}

func (s *ServerService) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Server, int64, error) {
	return s.serverRepo.ListByTenant(ctx, tenantID, page, perPage)
}

func (s *ServerService) Update(ctx context.Context, server *models.Server) error {
	return s.serverRepo.Update(ctx, server)
}

func (s *ServerService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.serverRepo.Delete(ctx, tenantID, id)
}

func (s *ServerService) Heartbeat(ctx context.Context, serverID uuid.UUID, metrics *models.ServerMetric) error {
	return s.serverRepo.UpdateHeartbeat(ctx, serverID, metrics)
}

func (s *ServerService) GetMetrics(ctx context.Context, serverID uuid.UUID) (*models.ServerMetric, error) {
	return s.serverRepo.GetLatestMetrics(ctx, serverID)
}

// ============================================================
// THE DEPRECATED STATIC AGENT TOKEN
//
// servers.agent_token is the channel that protects every managed server on an
// installation that has not yet enrolled its agents: one uuid per server,
// stored in the clear, sent on every request, compared for equality, never
// expiring. Everything below is the crossing to the certificate channel in
// internal/agentpki - what still uses the old one, how one server is moved,
// and how the old one is closed behind it.
// ============================================================

// LegacyAgentDirectory adapts the server repository to
// agentpki.LegacyDirectory, so the package that decides which channel a request
// came in on does not have to know any SQL.
type LegacyAgentDirectory struct {
	repo   *repository.ServerRepository
	logger *zap.Logger
}

// The adapter is what agentpki.NewGateway is handed, so the compiler is asked
// to keep the two in step.
var _ agentpki.LegacyDirectory = (*LegacyAgentDirectory)(nil)

// LegacyAgentDirectory returns the adapter to hand to agentpki.NewGateway.
func (s *ServerService) LegacyAgentDirectory() *LegacyAgentDirectory {
	return &LegacyAgentDirectory{repo: s.serverRepo, logger: s.logger}
}

// LookupByStaticToken implements agentpki.LegacyDirectory.
func (d *LegacyAgentDirectory) LookupByStaticToken(ctx context.Context, token string) (*agentpki.LegacyServer, error) {
	if d.repo == nil {
		return nil, agentpki.ErrLegacyUnavailable
	}
	row, err := d.repo.LookupStaticTokenChannel(ctx, token)
	switch {
	case errors.Is(err, repository.ErrAgentTokenRetired):
		return nil, agentpki.ErrLegacyRetired
	case errors.Is(err, sql.ErrNoRows):
		return nil, agentpki.ErrNotFound
	case err != nil:
		return nil, err
	}
	return &agentpki.LegacyServer{
		ServerID: row.ServerID,
		Hostname: row.Hostname,
		Retired:  row.Retired,
	}, nil
}

// ListStaticTokenServers implements agentpki.LegacyDirectory.
func (d *LegacyAgentDirectory) ListStaticTokenServers(ctx context.Context) ([]agentpki.LegacyServer, error) {
	if d.repo == nil {
		return nil, agentpki.ErrLegacyUnavailable
	}
	rows, err := d.repo.ListAgentChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]agentpki.LegacyServer, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentpki.LegacyServer{
			ServerID: row.ServerID,
			Hostname: row.Hostname,
			Retired:  row.Retired,
		})
	}
	return out, nil
}

// NoteStaticTokenUse implements agentpki.LegacyDirectory. A failure is
// bookkeeping lost, never an agent refused, so it is reported and swallowed by
// the caller.
func (d *LegacyAgentDirectory) NoteStaticTokenUse(ctx context.Context, serverID string, at time.Time) error {
	if d.repo == nil {
		return agentpki.ErrLegacyUnavailable
	}
	id, err := uuid.Parse(serverID)
	if err != nil {
		return fmt.Errorf("%q is not a server id: %w", serverID, err)
	}
	return d.repo.NoteStaticTokenUse(ctx, id, at)
}

// MarkEnrolled records that a server has crossed to mutual TLS. It is called
// once, when an agent trades its enrolment token for a certificate.
func (s *ServerService) MarkEnrolled(ctx context.Context, serverID uuid.UUID, agentID string) error {
	if err := s.serverRepo.MarkEnrolled(ctx, serverID, agentID, time.Now().UTC()); err != nil {
		if errors.Is(err, repository.ErrChannelBookkeepingUnavailable) {
			s.logger.Warn("Agent channel: enrolment could not be recorded; run migrations/pending/agent_pki.sql",
				zap.String("server_id", serverID.String()), zap.String("agent_id", agentID))
			return nil
		}
		return err
	}
	s.logger.Info("Agent channel: server moved to mutual TLS",
		zap.String("server_id", serverID.String()),
		zap.String("agent_id", agentID),
		zap.String("channel", agentpki.ChannelMutualTLS))
	return nil
}

// RetireStaticToken closes the old channel for one server. It is the last step
// an operator takes after that server's agent has enrolled, and it is not
// reversible: the stored token is replaced by a value that authenticates
// nothing, so the old string is gone from the database.
func (s *ServerService) RetireStaticToken(ctx context.Context, tenantID, serverID uuid.UUID, by string) error {
	err := s.serverRepo.RetireStaticToken(ctx, tenantID, serverID, by)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("server %s not found", serverID)
	case errors.Is(err, repository.ErrChannelBookkeepingUnavailable):
		s.logger.Warn("Agent channel: static token retired, but the retirement could not be recorded; "+
			"run migrations/pending/agent_pki.sql",
			zap.String("server_id", serverID.String()))
	case err != nil:
		return err
	}
	s.logger.Warn("Agent channel: DEPRECATED static agent token retired for a server",
		zap.String("server_id", serverID.String()),
		zap.String("by", by))
	return nil
}

// AgentChannelSummary is what the startup line reports and what an operator
// screen can show: how much of the fleet has crossed.
type AgentChannelSummary struct {
	Total       int      `json:"total"`
	MutualTLS   int      `json:"mutual_tls"`
	StaticToken int      `json:"static_token"`
	Retired     int      `json:"token_retired"`
	Pending     []string `json:"pending"`
}

// AgentChannelCensus counts the fleet by channel, from the database alone.
//
// It is deliberately the pessimistic count: a server is reported as still on
// the old channel unless its token has been retired or its enrolment has been
// recorded. agentpki.Gateway.Census is the richer answer, because it can also
// see the certificates; this one exists so that the panel can say something
// true at startup without the CA having been opened.
func (s *ServerService) AgentChannelCensus(ctx context.Context) (AgentChannelSummary, error) {
	var summary AgentChannelSummary
	if s.serverRepo == nil {
		return summary, errors.New("no server repository")
	}
	rows, err := s.serverRepo.ListAgentChannels(ctx)
	if err != nil {
		return summary, err
	}
	return summariseAgentChannels(rows), nil
}

// summariseAgentChannels is the classification, kept apart from the query so
// that what counts as "still on the old channel" can be tested without a
// database.
//
// A retired token counts as crossed even when no enrolment has been recorded:
// the string in that row cannot authenticate anything any more, which is the
// property the count is about.
func summariseAgentChannels(rows []repository.AgentChannelRow) AgentChannelSummary {
	summary := AgentChannelSummary{Total: len(rows)}
	for _, row := range rows {
		switch {
		case row.Retired:
			summary.Retired++
			summary.MutualTLS++
		case row.Channel == agentpki.ChannelMutualTLS:
			summary.MutualTLS++
		default:
			summary.StaticToken++
			name := row.Hostname
			if name == "" {
				name = row.ServerID
			}
			summary.Pending = append(summary.Pending, name)
		}
	}
	return summary
}

// LogAgentChannelCensus writes the startup line.
func (s *ServerService) LogAgentChannelCensus(ctx context.Context) AgentChannelSummary {
	summary, err := s.AgentChannelCensus(ctx)
	if err != nil {
		// A panel that cannot reach its database at this moment must still
		// start; the census is a report, not a dependency.
		s.logger.Debug("Agent channel: cannot count how many servers still use the deprecated static token",
			zap.Error(err))
		return summary
	}
	if summary.StaticToken == 0 {
		s.logger.Info("Agent channel: no server uses the deprecated static agent token",
			zap.Int("servers", summary.Total),
			zap.Int("mutual_tls", summary.MutualTLS))
		return summary
	}
	s.logger.Warn(fmt.Sprintf(
		"Agent channel: %d of %d managed server(s) still authenticate with the DEPRECATED static agent token. "+
			"Enrol each one (Servers -> Add agent -> mint an enrolment token), then retire its token",
		summary.StaticToken, summary.Total),
		zap.Int("static_token", summary.StaticToken),
		zap.Int("mutual_tls", summary.MutualTLS),
		zap.Int("token_retired", summary.Retired),
		zap.Strings("pending", summary.Pending))
	return summary
}

// ============================================================
// THE PANEL HOST AS A MANAGED NODE
//
// The panel was built as a control plane and installed as one: the panel here,
// agents there, every operation carried over mutual TLS. The consequence nobody
// handled is that the machine the panel is installed on manages nothing - not
// even itself - so a fresh install has zero servers, zero websites, and needs a
// second machine before it can do anything at all.
//
// Everything below makes that machine the first managed node. It is not a
// replacement for the fleet model: additional machines still enrol through
// agentpki, and clustering is still an optional layer an operator adds. It is
// the missing bottom of the ladder.
//
// The rule that everything here serves: a node is treated as local only when
// this process can prove it is standing on it. Not because a column says so.
// ============================================================

// LocalNodeHeartbeatInterval is how often the running panel re-proves that it
// is standing on its node and refreshes servers.last_seen_at.
//
// It matters that this is done by the running panel rather than by the
// installer. A row written once at install time saying 'online' would go on
// saying it after the machine had been off for a month.
const LocalNodeHeartbeatInterval = time.Minute

// localNodeStaleAfter is how long servers.last_seen_at may go unrefreshed
// before the local node is reported unhealthy. Three intervals is two missed
// beats: enough that a slow database or a restart is not an outage.
const localNodeStaleAfter = 3 * LocalNodeHeartbeatInterval

// localNodeBootstrapTimeout bounds the registration the panel does at startup.
// A panel whose database is slow must still finish booting.
const localNodeBootstrapTimeout = 10 * time.Second

var (
	// ErrLocalNodeUnavailable means this service was built without the pieces
	// needed to manage a local node - no repository, or no way to read the
	// machine. It is a wiring fault, not an operator one.
	ErrLocalNodeUnavailable = errors.New("service: this panel is not wired to manage a local node")

	// ErrNoLocalNode means this panel host has not been registered as a node.
	ErrNoLocalNode = errors.New("service: this panel host is not registered as a managed node")
)

// localNodeStore is the slice of the repository this half of the service uses.
// It is an interface so the identity guard - the part that decides whether a
// local command may run - can be tested without a PostgreSQL.
type localNodeStore interface {
	RegisterLocalNode(ctx context.Context, reg repository.LocalNodeRegistration) (*models.Server, bool, error)
	GetLocalNode(ctx context.Context, serverID uuid.UUID) (*repository.LocalNodeRecord, error)
	FindLocalNodeByFingerprint(ctx context.Context, fingerprint string) (*repository.LocalNodeRecord, error)
	TouchLocalNode(ctx context.Context, serverID uuid.UUID, at time.Time) error
	NoteLocalNodeMismatch(ctx context.Context, serverID uuid.UUID, reason string, at time.Time) error
	DefaultTenantID(ctx context.Context) (uuid.UUID, error)
}

// The repository is what the service is handed, so the compiler is asked to
// keep the two in step.
var _ localNodeStore = (*repository.ServerRepository)(nil)

// LocalNodeOptions are the choices a caller of the registration path may make.
// All of them have defaults, because the installer calls this with none.
type LocalNodeOptions struct {
	// TenantID owns the node. Zero means the installation's default tenant.
	TenantID uuid.UUID

	// Role and Status are applied only when the row is created. A later
	// registration never overwrites what an operator chose.
	Role   string
	Status string

	// Rebind re-records the machine witness for an identity that no longer
	// matches this machine. It is the remedy for the legitimate half of a
	// mismatch - the panel's etc directory restored onto rebuilt hardware - and
	// it is off by default, so an unattended restart can never rebind. Only an
	// operator running registration on the machine asks for it.
	Rebind bool
}

// LocalNodeRegistrationResult reports what the idempotent registration did.
type LocalNodeRegistrationResult struct {
	Server        *models.Server `json:"server"`
	NodeID        uuid.UUID      `json:"node_id"`
	Created       bool           `json:"created"`
	Adopted       bool           `json:"adopted"`
	Rebound       bool           `json:"rebound"`
	Verified      bool           `json:"verified"`
	WitnessSource string         `json:"witness_source,omitempty"`
}

// NodeRoute is the answer to "does an operation on this node need the agent
// transport". It is what lets the layer above take the short path, and the
// reason it may not.
type NodeRoute struct {
	ServerID uuid.UUID `json:"server_id"`

	// Local is true only when this process has proved it is standing on the
	// node. Everything else - including every case where the evidence could not
	// be gathered - is false, so a failure sends the operation over the agent
	// transport rather than running a command here on a guess.
	Local bool `json:"local"`

	// Verified reports that the machine witness agreed. A local route with
	// Verified false is a node whose id matches but whose host has no machine
	// id to witness with; the claim rests on the node identity file alone.
	Verified bool `json:"verified"`

	// Mismatch is the dangerous case: this node's row claims to be the machine
	// this process is running on, and the evidence says otherwise. It is what a
	// database restored onto a different machine looks like.
	Mismatch bool `json:"mismatch"`

	// Reason explains a route that is not local, in words for an operator.
	Reason string `json:"reason,omitempty"`
}

// LocalNodeStatus is the health of the panel host as a node, computed now
// rather than read out of a column.
type LocalNodeStatus struct {
	// Registered reports whether this panel host has a node at all.
	Registered bool `json:"registered"`

	NodeID   uuid.UUID `json:"node_id,omitempty"`
	Hostname string    `json:"hostname,omitempty"`

	Local    bool `json:"local"`
	Verified bool `json:"verified"`
	Mismatch bool `json:"mismatch"`

	WitnessSource string `json:"witness_source,omitempty"`

	RegisteredAt       *time.Time `json:"registered_at,omitempty"`
	LastVerifiedAt     *time.Time `json:"last_verified_at,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	LastMismatchAt     *time.Time `json:"last_mismatch_at,omitempty"`
	LastMismatchReason string     `json:"last_mismatch_reason,omitempty"`

	// Healthy is the live verdict: this process is standing on the node and has
	// said so recently. A node whose panel stopped a week ago is not healthy,
	// however much the stored agent_status still says 'online'.
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason,omitempty"`
}

// RegisterLocalNode is the idempotent registration path. The installer, the
// CLI and the panel's own startup all call it, and calling it twice on one
// machine produces one row.
//
// What makes it idempotent is the key: the node id from <EtcRoot>/node.json,
// which is the row's primary key. A machine that was renamed or readdressed
// between two calls updates its row; it does not grow a second one, which is
// what keying on hostname or IP would have done.
func (s *ServerService) RegisterLocalNode(ctx context.Context, opts LocalNodeOptions) (*LocalNodeRegistrationResult, error) {
	if s.localNodes == nil || s.probe == nil {
		return nil, ErrLocalNodeUnavailable
	}

	identity, err := s.probe.EnsureIdentity()
	if err != nil {
		return nil, err
	}
	witness := s.probe.Witness()
	result := &LocalNodeRegistrationResult{WitnessSource: witness.Source}

	// node.json was just generated. Before accepting a brand new node id, ask
	// whether the database already holds a node this same machine registered:
	// an etc directory that was rebuilt or restored without it would otherwise
	// fork one machine into two rows.
	if identity.Created && witness.Available() {
		record, findErr := s.localNodes.FindLocalNodeByFingerprint(ctx, witness.Fingerprint)
		switch {
		case findErr == nil && record != nil:
			adopted, saveErr := s.probe.SaveIdentity(record.ServerID)
			if saveErr != nil {
				return nil, saveErr
			}
			identity = adopted
			result.Adopted = true
			s.logger.Warn("Local node: this machine had no node identity file, but the database holds a node it registered before; adopting it rather than creating a second one",
				zap.String("node_id", record.ServerID.String()),
				zap.String("hostname", record.Hostname))
		case findErr != nil && !errors.Is(findErr, sql.ErrNoRows) && !errors.Is(findErr, repository.ErrLocalNodeBookkeepingUnavailable):
			return nil, findErr
		}
	}

	verified, verifyErr := identity.Verify(witness)
	if verifyErr != nil {
		if !opts.Rebind {
			return nil, fmt.Errorf("%w. If this panel's configuration was restored onto rebuilt hardware, "+
				"re-run registration on this machine asking for a rebind; if it was not, this database belongs to a different machine",
				verifyErr)
		}
		rebound, saveErr := s.probe.SaveIdentity(identity.NodeID)
		if saveErr != nil {
			return nil, saveErr
		}
		identity = rebound
		result.Rebound = true
		if verified, verifyErr = identity.Verify(witness); verifyErr != nil {
			return nil, verifyErr
		}
		s.logger.Warn("Local node: the machine witness for this node was re-bound to the machine this panel is running on",
			zap.String("node_id", identity.NodeID.String()))
	}
	result.NodeID = identity.NodeID
	result.Verified = verified

	tenantID := opts.TenantID
	if tenantID == uuid.Nil {
		tenantID, err = s.localNodes.DefaultTenantID(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot decide which tenant owns the panel host: %w", err)
		}
	}

	facts, err := s.probe.Facts()
	if err != nil {
		return nil, err
	}

	server, created, err := s.localNodes.RegisterLocalNode(ctx, repository.LocalNodeRegistration{
		NodeID:            identity.NodeID,
		TenantID:          tenantID,
		Facts:             repositoryFacts(facts),
		Fingerprint:       nilIfEmpty(witness.Fingerprint),
		FingerprintSource: nilIfEmpty(witness.Source),
		Role:              opts.Role,
		Status:            opts.Status,
	})
	if err != nil {
		return nil, err
	}
	result.Server = server
	result.Created = created
	return result, nil
}

// ResolveNodeRoute decides whether an operation on one node can be run here.
//
// It is the seam the layers above use to choose the short path, and it fails
// closed in every direction: a node with no marker, an unreadable identity, a
// database that cannot answer, and a witness that disagrees all come back as
// "not local", which means the operation goes over the agent transport exactly
// as it does today.
//
// The one case that also writes to the database is the dangerous one: a row
// that claims to be this machine when it is not. That is recorded, and the node
// is marked offline, because an operator restoring a database onto new hardware
// needs to find out from the panel rather than from a command that ran on the
// wrong box.
func (s *ServerService) ResolveNodeRoute(ctx context.Context, serverID uuid.UUID) NodeRoute {
	route := NodeRoute{ServerID: serverID}
	if s.localNodes == nil || s.probe == nil {
		route.Reason = "this panel is not wired to manage a local node"
		return route
	}
	if serverID == uuid.Nil {
		route.Reason = "no node was named"
		return route
	}

	record, err := s.localNodes.GetLocalNode(ctx, serverID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		route.Reason = "this node is not the machine any panel is running on"
		return route
	case errors.Is(err, repository.ErrLocalNodeBookkeepingUnavailable):
		route.Reason = "this installation has no local-node bookkeeping; run migrations/pending/local_node.sql"
		return route
	case err != nil:
		route.Reason = fmt.Sprintf("cannot tell whether this node is local: %v", err)
		return route
	}

	identity, err := s.probe.Identity()
	if err != nil {
		if errors.Is(err, localnode.ErrNoIdentity) {
			route.Reason = "this panel has no node identity, so it cannot be standing on any node"
		} else {
			route.Reason = fmt.Sprintf("cannot read this panel's node identity: %v", err)
		}
		return route
	}

	route = evaluateLocalClaim(serverID, identity, record, s.probe.Witness())
	if route.Mismatch {
		s.noteMismatch(ctx, serverID, record, route.Reason)
	}
	return route
}

// noteMismatch reports and records a refused claim, at most once per heartbeat
// interval.
//
// Route resolution happens on the path of every operation, and a mismatch does
// not fix itself: without the interval, one restored database would write a row
// and an error line for every request the panel served. Once a minute is often
// enough for an operator to see it and rare enough not to be the reason the
// disk filled up.
func (s *ServerService) noteMismatch(ctx context.Context, serverID uuid.UUID, record *repository.LocalNodeRecord, reason string) {
	if record != nil && record.LastMismatchAt != nil && time.Since(*record.LastMismatchAt) < LocalNodeHeartbeatInterval {
		return
	}
	s.logger.Error("Local node: a node is recorded as the machine this panel runs on, and it is not. "+
		"Operations on it will go over the agent transport, not run here",
		zap.String("node_id", serverID.String()),
		zap.String("reason", reason))
	if err := s.localNodes.NoteLocalNodeMismatch(ctx, serverID, reason, time.Now().UTC()); err != nil &&
		!errors.Is(err, repository.ErrLocalNodeBookkeepingUnavailable) {
		s.logger.Warn("Local node: the mismatch could not be recorded",
			zap.String("node_id", serverID.String()), zap.Error(err))
	}
}

// evaluateLocalClaim is the guard itself, with the database and the filesystem
// taken out of it so that what it refuses can be fixed by a test.
//
// The witness is checked twice, against the identity file and against the
// database row, because the two can be moved independently: copying the etc
// directory to another machine carries the file, restoring a dump carries the
// row, and only a machine that satisfies both is the machine that was
// registered.
func evaluateLocalClaim(serverID uuid.UUID, identity *localnode.Identity, record *repository.LocalNodeRecord, witness localnode.MachineWitness) NodeRoute {
	route := NodeRoute{ServerID: serverID}
	switch {
	case record == nil:
		route.Reason = "this node is not the machine any panel is running on"
		return route
	case identity == nil:
		route.Reason = "this panel has no node identity, so it cannot be standing on any node"
		return route
	case identity.NodeID != serverID:
		// Not a mismatch: on a clustered installation this is simply another
		// panel's host, and nothing is wrong with it.
		route.Reason = fmt.Sprintf("this panel runs on node %s, not on node %s", identity.NodeID, serverID)
		return route
	}

	fileVerified, err := localnode.CompareWitness(identity.Fingerprint, identity.FingerprintSource, witness)
	if err != nil {
		where := "the node identity file"
		if identity.Path != "" {
			where = identity.Path
		}
		route.Mismatch = true
		route.Reason = fmt.Sprintf("the node identity in %s does not describe this machine: %v", where, err)
		return route
	}
	rowVerified, err := localnode.CompareWitness(record.Fingerprint, record.FingerprintSource, witness)
	if err != nil {
		route.Mismatch = true
		route.Reason = fmt.Sprintf("the stored record for node %s does not describe this machine: %v", serverID, err)
		return route
	}

	route.Local = true
	route.Verified = fileVerified && rowVerified
	if !route.Verified {
		route.Reason = "this machine has no machine id to witness with, so the claim rests on the node identity file alone"
	}
	return route
}

// LocalNodeID is the node this panel is running on, or ErrNoLocalNode.
func (s *ServerService) LocalNodeID(ctx context.Context) (uuid.UUID, error) {
	if s.probe == nil {
		return uuid.Nil, ErrLocalNodeUnavailable
	}
	identity, err := s.probe.Identity()
	if errors.Is(err, localnode.ErrNoIdentity) {
		return uuid.Nil, ErrNoLocalNode
	}
	if err != nil {
		return uuid.Nil, err
	}
	return identity.NodeID, nil
}

// LocalNodeStatus reports the health of the panel host as a node.
//
// Every part of the verdict is computed at the moment it is asked for: the
// identity is re-read, the machine is re-witnessed, and the freshness of
// last_seen_at is measured. Nothing here trusts servers.agent_status, which is
// a value some earlier process wrote.
func (s *ServerService) LocalNodeStatus(ctx context.Context) (*LocalNodeStatus, error) {
	status := &LocalNodeStatus{}
	if s.localNodes == nil || s.probe == nil {
		status.Reason = "this panel is not wired to manage a local node"
		return status, nil
	}

	identity, err := s.probe.Identity()
	if errors.Is(err, localnode.ErrNoIdentity) {
		status.Reason = "this panel host has not been registered as a managed node"
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	status.NodeID = identity.NodeID

	record, err := s.localNodes.GetLocalNode(ctx, identity.NodeID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		status.Reason = "this panel has a node identity, but no node in the database carries it"
		return status, nil
	case errors.Is(err, repository.ErrLocalNodeBookkeepingUnavailable):
		status.Reason = "this installation has no local-node bookkeeping; run migrations/pending/local_node.sql"
		return status, nil
	case err != nil:
		return nil, err
	}

	witness := s.probe.Witness()
	route := evaluateLocalClaim(identity.NodeID, identity, record, witness)

	status.Registered = true
	status.Hostname = record.Hostname
	status.Local = route.Local
	status.Verified = route.Verified
	status.Mismatch = route.Mismatch
	status.WitnessSource = witness.Source
	status.RegisteredAt = &record.RegisteredAt
	status.LastVerifiedAt = record.LastVerifiedAt
	status.LastSeenAt = record.LastSeenAt
	status.LastMismatchAt = record.LastMismatchAt
	status.LastMismatchReason = record.LastMismatchReason
	status.Reason = route.Reason

	switch {
	case !route.Local:
		status.Healthy = false
	case record.LastVerifiedAt == nil:
		status.Healthy = false
		status.Reason = "no panel process has proved it is standing on this machine since it was registered"
	case time.Since(*record.LastVerifiedAt) > localNodeStaleAfter:
		status.Healthy = false
		status.Reason = fmt.Sprintf("no panel process has proved it is standing on this machine for %s",
			time.Since(*record.LastVerifiedAt).Round(time.Second))
	default:
		status.Healthy = true
	}
	return status, nil
}

// HeartbeatLocalNode re-proves the claim and refreshes last_seen_at. It is one
// beat of what StartLocalNodeHeartbeat runs.
func (s *ServerService) HeartbeatLocalNode(ctx context.Context) error {
	if s.localNodes == nil || s.probe == nil {
		return ErrLocalNodeUnavailable
	}
	nodeID, err := s.LocalNodeID(ctx)
	if err != nil {
		return err
	}
	route := s.ResolveNodeRoute(ctx, nodeID)
	if !route.Local {
		return fmt.Errorf("%w: %s", ErrNoLocalNode, route.Reason)
	}
	return s.localNodes.TouchLocalNode(ctx, nodeID, time.Now().UTC())
}

// StartLocalNodeHeartbeat runs the heartbeat until the returned stop function
// is called or ctx is done. Calling it more than once on one service is a no-op
// after the first.
func (s *ServerService) StartLocalNodeHeartbeat(ctx context.Context, interval time.Duration) {
	if s.localNodes == nil || s.probe == nil {
		return
	}
	if interval <= 0 {
		interval = LocalNodeHeartbeatInterval
	}
	s.heartbeatOnce.Do(func() {
		stop := make(chan struct{})
		s.heartbeatMu.Lock()
		s.heartbeatStop = stop
		s.heartbeatMu.Unlock()
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-stop:
					return
				case <-ticker.C:
					beat, cancel := context.WithTimeout(ctx, interval)
					if err := s.HeartbeatLocalNode(beat); err != nil {
						// Debug, not warn: on a panel that manages only remote
						// nodes this fires every interval and says nothing an
						// operator needs. A mismatch is reported separately, at
						// error level, by ResolveNodeRoute.
						s.logger.Debug("Local node: heartbeat did not land", zap.Error(err))
					}
					cancel()
				}
			}
		}()
	})
}

// StopLocalNodeHeartbeat ends the heartbeat goroutine.
func (s *ServerService) StopLocalNodeHeartbeat() {
	s.heartbeatMu.Lock()
	defer s.heartbeatMu.Unlock()
	if s.heartbeatStop != nil {
		close(s.heartbeatStop)
		s.heartbeatStop = nil
	}
}

// bootstrapLocalNode is what makes an install of the panel an install of a
// usable node: the machine registers itself the first time the API starts, so
// an administrator who has just signed in can create a website on the box they
// installed on without finding a second machine first.
//
// Nothing here is fatal. A panel whose database is not migrated yet, whose etc
// directory is read-only, or whose node was moved onto other hardware still
// starts and still manages whatever remote nodes it has; it says what went
// wrong and offers no local node.
func (s *ServerService) bootstrapLocalNode() {
	if s.localNodes == nil || s.probe == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), localNodeBootstrapTimeout)
	defer cancel()

	result, err := s.RegisterLocalNode(ctx, LocalNodeOptions{})
	if err != nil {
		if errors.Is(err, repository.ErrLocalNodeBookkeepingUnavailable) {
			s.logger.Warn("Local node: this panel host cannot be managed until the migration is applied; " +
				"run migrations/pending/local_node.sql")
			return
		}
		s.logger.Warn("Local node: this panel host was not registered as a managed node", zap.Error(err))
		return
	}
	switch {
	case result.Created:
		s.logger.Info("Local node: the machine this panel is installed on is now its first managed node",
			zap.String("node_id", result.NodeID.String()),
			zap.String("hostname", result.Server.Hostname),
			zap.Bool("witnessed", result.Verified))
	default:
		s.logger.Info("Local node: the machine this panel is installed on is registered",
			zap.String("node_id", result.NodeID.String()),
			zap.String("hostname", result.Server.Hostname),
			zap.Bool("witnessed", result.Verified))
	}
	s.StartLocalNodeHeartbeat(context.Background(), LocalNodeHeartbeatInterval)
}

// repositoryFacts moves the measured facts across the layer boundary. The nil
// pointers survive the crossing, which is the whole point: a fact that could
// not be read reaches PostgreSQL as NULL rather than as a zero.
func repositoryFacts(facts localnode.Facts) repository.LocalNodeFacts {
	return repository.LocalNodeFacts{
		Hostname:    facts.Hostname,
		IPAddress:   facts.IPAddress,
		IPv6Address: nilIfEmpty(facts.IPv6Address),
		OS:          facts.OS,
		Kernel:      facts.Kernel,
		CPUCores:    facts.CPUCores,
		RAMTotal:    facts.RAMTotal,
		DiskTotal:   facts.DiskTotal,
	}
}

func nilIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
