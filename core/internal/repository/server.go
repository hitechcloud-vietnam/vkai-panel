package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type ServerRepository struct {
	db *sqlx.DB
}

func NewServerRepository(db *sqlx.DB) *ServerRepository {
	return &ServerRepository{db: db}
}

func (r *ServerRepository) Create(ctx context.Context, server *models.Server) error {
	// servers.agent_token is NOT NULL UNIQUE until the migration that drops it,
	// so something has to go in. What goes in is a retired placeholder: a value
	// that is refused before any lookup happens. A server created today joins
	// by enrolling in the PKI, never by holding a string.
	if strings.TrimSpace(server.AgentToken) == "" {
		server.AgentToken = NewAgentToken()
	}
	query := `
		INSERT INTO servers (id, tenant_id, hostname, ip_address, ipv6_address, ssh_port,
			agent_status, agent_token, os, kernel, cpu_cores, ram_total, disk_total,
			location, tags, role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		server.ID, server.TenantID, server.Hostname, server.IPAddress,
		server.IPv6Address, server.SSHPort, server.AgentStatus, server.AgentToken,
		server.OS, server.Kernel, server.CPUCores, server.RAMTotal, server.DiskTotal,
		server.Location, server.Tags, server.Role, server.Status,
	).Scan(&server.CreatedAt, &server.UpdatedAt)
}

func (r *ServerRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Server, error) {
	var server models.Server
	query := `SELECT * FROM servers WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &server, query, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return &server, nil
}

// GetByAgentToken looks a server up by the DEPRECATED static agent token.
//
// Deprecated: the static token is the pre-PKI channel - it never expires, it is
// stored in the clear, and it is compared by a database equality match rather
// than in constant time. It is kept only so that agents in the field keep
// reporting while an operator enrols them. New code authenticates an agent
// through agentpki.Gateway, which prefers the certificate channel and refuses a
// token whose server has already enrolled. See LookupStaticTokenChannel.
func (r *ServerRepository) GetByAgentToken(ctx context.Context, token string) (*models.Server, error) {
	if isRetiredAgentToken(token) {
		return nil, ErrAgentTokenRetired
	}
	var server models.Server
	query := `SELECT * FROM servers WHERE agent_token = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &server, query, token)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return &server, nil
}

func (r *ServerRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Server, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	var servers []models.Server
	offset := (page - 1) * perPage
	query := `SELECT * FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &servers, query, tenantID, perPage, offset); err != nil {
		return nil, 0, err
	}

	return servers, total, nil
}

func (r *ServerRepository) Update(ctx context.Context, server *models.Server) error {
	query := `
		UPDATE servers SET hostname=$1, ip_address=$2, ipv6_address=$3, ssh_port=$4,
		agent_status=$5, os=$6, kernel=$7, cpu_cores=$8, ram_total=$9, disk_total=$10,
		location=$11, tags=$12, role=$13, status=$14, updated_at=NOW()
		WHERE id=$15 AND tenant_id=$16 AND deleted_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query,
		server.Hostname, server.IPAddress, server.IPv6Address, server.SSHPort,
		server.AgentStatus, server.OS, server.Kernel, server.CPUCores,
		server.RAMTotal, server.DiskTotal, server.Location, server.Tags,
		server.Role, server.Status, server.ID, server.TenantID,
	)
	return err
}

func (r *ServerRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, metrics *models.ServerMetric) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update last seen
	_, err = tx.ExecContext(ctx, `UPDATE servers SET last_seen_at=NOW(), agent_status='online' WHERE id=$1`, id)
	if err != nil {
		return err
	}

	// Insert metrics
	_, err = tx.ExecContext(ctx, `
		INSERT INTO server_metrics (id, server_id, cpu_percent, ram_used, ram_total, disk_used, disk_total, net_in, net_out, load1, load5, load15, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
	`, uuid.New(), id, metrics.CPUPercent, metrics.RAMUsed, metrics.RAMTotal,
		metrics.DiskUsed, metrics.DiskTotal, metrics.NetIn, metrics.NetOut,
		metrics.Load1, metrics.Load5, metrics.Load15)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ServerRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `UPDATE servers SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *ServerRepository) GetLatestMetrics(ctx context.Context, serverID uuid.UUID) (*models.ServerMetric, error) {
	var metric models.ServerMetric
	query := `SELECT * FROM server_metrics WHERE server_id = $1 ORDER BY timestamp DESC LIMIT 1`
	err := r.db.GetContext(ctx, &metric, query, serverID)
	if err != nil {
		return nil, err
	}
	return &metric, nil
}

// ============================================================
// THE DEPRECATED STATIC AGENT TOKEN, AND ITS RETIREMENT
//
// Everything below exists to move a fleet off servers.agent_token and to make
// how far that has got visible. The bookkeeping lives in the separate table
// server_agent_channel (migrations/pending/agent_pki.sql) rather than in
// columns on `servers`, because this file reads servers with SELECT * into
// models.Server and a new column there breaks every server query.
//
// Every method here degrades rather than fails when that table has not been
// created yet: an installation that has not run the migration keeps working,
// with no bookkeeping, and the security-relevant half - refusing a retired
// token - does not depend on the table at all.
// ============================================================

// RetiredAgentTokenPrefix marks a static token that must never authenticate.
// The column is NOT NULL UNIQUE, so retiring cannot blank it; it is replaced
// with a unique value that is refused on sight. It must stay equal to
// agentpki.RetiredTokenPrefix, which is where the refusal is enforced.
const RetiredAgentTokenPrefix = "retired-"

// ErrAgentTokenRetired is returned when a caller presents a token that has been
// retired, or asks about one.
var ErrAgentTokenRetired = errors.New("repository: this server's static agent token has been retired")

// ErrChannelBookkeepingUnavailable means server_agent_channel does not exist
// yet, so the panel can authenticate and retire but cannot record.
var ErrChannelBookkeepingUnavailable = errors.New("repository: server_agent_channel is missing; run migrations/pending/agent_pki.sql")

// NewAgentToken is the value written into servers.agent_token for a newly
// created server. It is deliberately not a credential: it carries the retired
// prefix, so it is refused before any lookup, and it exists only because the
// column is NOT NULL UNIQUE until the migration that drops it.
func NewAgentToken() string {
	return RetiredAgentTokenPrefix + uuid.New().String()
}

func isRetiredAgentToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), RetiredAgentTokenPrefix)
}

// AgentChannelRow is one server's position in the migration.
type AgentChannelRow struct {
	ServerID string
	Hostname string
	Retired  bool
	AgentID  string
	Channel  string
}

// LookupStaticTokenChannel finds the server holding a static token. It returns
// sql.ErrNoRows when no server holds it, so the caller can tell "unknown token"
// from "database is down".
func (r *ServerRepository) LookupStaticTokenChannel(ctx context.Context, token string) (*AgentChannelRow, error) {
	if isRetiredAgentToken(token) {
		return nil, ErrAgentTokenRetired
	}
	var row struct {
		ID       string         `db:"id"`
		Hostname string         `db:"hostname"`
		Retired  bool           `db:"retired"`
		AgentID  sql.NullString `db:"agent_id"`
		Channel  sql.NullString `db:"channel"`
	}
	query := `
		SELECT s.id, s.hostname,
		       (c.token_retired_at IS NOT NULL) AS retired,
		       c.agent_id, c.channel
		FROM servers s
		LEFT JOIN server_agent_channel c ON c.server_id = s.id
		WHERE s.agent_token = $1 AND s.deleted_at IS NULL
	`
	err := r.db.GetContext(ctx, &row, query, token)
	if isMissingChannelTable(err) {
		// No bookkeeping table: fall back to the servers table alone. The token
		// still authenticates, which is the point of the migration path.
		var bare struct {
			ID       string `db:"id"`
			Hostname string `db:"hostname"`
		}
		bareQuery := `SELECT id, hostname FROM servers WHERE agent_token = $1 AND deleted_at IS NULL`
		if err := r.db.GetContext(ctx, &bare, bareQuery, token); err != nil {
			return nil, err
		}
		return &AgentChannelRow{ServerID: bare.ID, Hostname: bare.Hostname}, nil
	}
	if err != nil {
		return nil, err
	}
	return &AgentChannelRow{
		ServerID: row.ID,
		Hostname: row.Hostname,
		Retired:  row.Retired,
		AgentID:  row.AgentID.String,
		Channel:  row.Channel.String,
	}, nil
}

// ListAgentChannels returns every live server and whether its static token has
// been retired. It is what the startup census counts.
func (r *ServerRepository) ListAgentChannels(ctx context.Context) ([]AgentChannelRow, error) {
	var rows []struct {
		ID       string         `db:"id"`
		Hostname string         `db:"hostname"`
		Retired  bool           `db:"retired"`
		AgentID  sql.NullString `db:"agent_id"`
		Channel  sql.NullString `db:"channel"`
	}
	query := `
		SELECT s.id, s.hostname,
		       (s.agent_token LIKE $1 || '%' OR c.token_retired_at IS NOT NULL) AS retired,
		       c.agent_id, c.channel
		FROM servers s
		LEFT JOIN server_agent_channel c ON c.server_id = s.id
		WHERE s.deleted_at IS NULL
		ORDER BY s.hostname
	`
	err := r.db.SelectContext(ctx, &rows, query, RetiredAgentTokenPrefix)
	if isMissingChannelTable(err) {
		bareQuery := `
			SELECT s.id, s.hostname, (s.agent_token LIKE $1 || '%') AS retired
			FROM servers s WHERE s.deleted_at IS NULL ORDER BY s.hostname
		`
		var bare []struct {
			ID       string `db:"id"`
			Hostname string `db:"hostname"`
			Retired  bool   `db:"retired"`
		}
		if err := r.db.SelectContext(ctx, &bare, bareQuery, RetiredAgentTokenPrefix); err != nil {
			return nil, err
		}
		out := make([]AgentChannelRow, 0, len(bare))
		for _, row := range bare {
			out = append(out, AgentChannelRow{ServerID: row.ID, Hostname: row.Hostname, Retired: row.Retired})
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]AgentChannelRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, AgentChannelRow{
			ServerID: row.ID,
			Hostname: row.Hostname,
			Retired:  row.Retired,
			AgentID:  row.AgentID.String,
			Channel:  row.Channel.String,
		})
	}
	return out, nil
}

// NoteStaticTokenUse records that the deprecated channel was used, so an
// operator can see which servers are still alive on it.
func (r *ServerRepository) NoteStaticTokenUse(ctx context.Context, serverID uuid.UUID, at time.Time) error {
	query := `
		INSERT INTO server_agent_channel (server_id, channel, token_last_used_at, created_at, updated_at)
		VALUES ($1, 'static-token', $2, NOW(), NOW())
		ON CONFLICT (server_id) DO UPDATE
		SET token_last_used_at = EXCLUDED.token_last_used_at,
		    channel = CASE WHEN server_agent_channel.agent_id IS NULL THEN 'static-token'
		                   ELSE server_agent_channel.channel END,
		    updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, serverID, at)
	if isMissingChannelTable(err) {
		return ErrChannelBookkeepingUnavailable
	}
	return err
}

// MarkEnrolled records that a server has crossed to the certificate channel.
func (r *ServerRepository) MarkEnrolled(ctx context.Context, serverID uuid.UUID, agentID string, at time.Time) error {
	query := `
		INSERT INTO server_agent_channel (server_id, channel, agent_id, enrolled_at, created_at, updated_at)
		VALUES ($1, 'mutual-tls', $2, $3, NOW(), NOW())
		ON CONFLICT (server_id) DO UPDATE
		SET channel = 'mutual-tls',
		    agent_id = EXCLUDED.agent_id,
		    enrolled_at = COALESCE(server_agent_channel.enrolled_at, EXCLUDED.enrolled_at),
		    updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, serverID, agentID, at)
	if isMissingChannelTable(err) {
		return ErrChannelBookkeepingUnavailable
	}
	return err
}

// RetireStaticToken replaces a server's static token with a value that cannot
// authenticate, and records who did it.
//
// The token is replaced rather than deleted because the column is NOT NULL
// UNIQUE. The replacement carries RetiredAgentTokenPrefix, which is refused
// before any lookup, so this is a one-way door: the old value is gone from the
// database and the new one is not a credential.
//
// The two statements are deliberately not one transaction. In PostgreSQL a
// failed statement aborts the transaction it is in, so a missing bookkeeping
// table would take the retirement down with it and leave an operator believing
// a token was retired when it was not. The security-relevant half runs first
// and stands on its own; the record of who did it is best effort after it.
func (r *ServerRepository) RetireStaticToken(ctx context.Context, tenantID, serverID uuid.UUID, by string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE servers SET agent_token = $1, updated_at = NOW()
		 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		NewAgentToken(), serverID, tenantID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO server_agent_channel (server_id, channel, token_retired_at, token_retired_by, created_at, updated_at)
		VALUES ($1, 'mutual-tls', NOW(), $2, NOW(), NOW())
		ON CONFLICT (server_id) DO UPDATE
		SET token_retired_at = COALESCE(server_agent_channel.token_retired_at, NOW()),
		    token_retired_by = EXCLUDED.token_retired_by,
		    updated_at = NOW()
	`, serverID, by)
	if isMissingChannelTable(err) {
		return ErrChannelBookkeepingUnavailable
	}
	return err
}

// isMissingChannelTable reports whether an error is PostgreSQL saying the
// bookkeeping table has not been created yet. It is matched on the message
// rather than on a driver error code so that this file keeps its single
// database dependency.
func isMissingChannelTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "server_agent_channel") && strings.Contains(msg, "does not exist")
}
