package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// censusTimeout bounds the one query this service makes at startup. A panel
// whose database is slow to answer must still finish booting.
const censusTimeout = 5 * time.Second

type ServerService struct {
	serverRepo *repository.ServerRepository
	logger     *zap.Logger
}

func NewServerService(serverRepo *repository.ServerRepository, logger *zap.Logger) *ServerService {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &ServerService{
		serverRepo: serverRepo,
		logger:     logger,
	}
	// The deprecation is announced at every boot rather than left as a comment
	// in a file nobody opens. An operator who upgraded six months ago and still
	// has four servers on the old channel is told so, by name, every time the
	// panel starts.
	ctx, cancel := context.WithTimeout(context.Background(), censusTimeout)
	defer cancel()
	s.LogAgentChannelCensus(ctx)
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
