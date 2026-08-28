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
	"github.com/lib/pq"

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
		server.Location, pq.StringArray(server.Tags), server.Role, server.Status,
	).Scan(&server.CreatedAt, &server.UpdatedAt)
}

// serverColumns is what every read of a server selects.
//
// It replaced `SELECT *` because the panel host writes NULL into the columns it
// could not measure - a container with no /etc/os-release, a statfs that
// failed - and models.Server holds os, kernel, cpu_cores, ram_total,
// disk_total, ipv6_address, location and ssh_port as plain strings and ints. A
// NULL in any of them fails the scan with "converting NULL to string is
// unsupported", which would take out every server query on the installation,
// not only the row that has one.
//
// The honest storage is the NULL; the COALESCE is the read. A caller cannot
// tell an unmeasured os from an empty one, but nothing in models.Server ever
// could, and the alternative - writing 'linux' and 0 cores so the scan
// succeeds - is the panel asserting facts it never measured. The distinction
// stays intact where it matters: in the database, and in every query that reads
// the columns directly.
const serverColumns = `id, tenant_id, hostname, ip_address,
	COALESCE(ipv6_address, '') AS ipv6_address,
	COALESCE(ssh_port, 0) AS ssh_port,
	COALESCE(agent_status, '') AS agent_status,
	agent_token,
	COALESCE(os, '') AS os,
	COALESCE(kernel, '') AS kernel,
	COALESCE(cpu_cores, 0) AS cpu_cores,
	COALESCE(ram_total, 0) AS ram_total,
	COALESCE(disk_total, 0) AS disk_total,
	COALESCE(location, '') AS location,
	tags,
	COALESCE(role, '') AS role,
	COALESCE(status, '') AS status,
	last_seen_at, created_at, updated_at, deleted_at`

// serverRow is what a server is scanned into.
//
// It exists because servers.tags is a text[], and a text[] reaches
// database/sql through the pgx driver as the raw array literal. Scanning that
// into models.Server.Tags ([]string) fails - "unsupported Scan, storing
// driver.Value type string into type *[]string" - and so does NULL, which fails
// with the same message and a nil value. Every read of a server was broken by
// it, on every row, which is why nothing here could be made to work until it
// was fixed.
//
// pq.StringArray is the type that knows the wire format. It shadows the
// embedded field of the same database name - sqlx resolves the shallowest field
// for a column - so the fix stays inside this file and models.Server is
// untouched. The same type is used on the way in, where []string cannot be
// encoded either.
type serverRow struct {
	models.Server
	Tags pq.StringArray `db:"tags"`
}

func (row serverRow) server() *models.Server {
	server := row.Server
	server.Tags = row.Tags
	return &server
}

func (r *ServerRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Server, error) {
	var row serverRow
	query := `SELECT ` + serverColumns + ` FROM servers WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &row, query, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return row.server(), nil
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
	var row serverRow
	query := `SELECT ` + serverColumns + ` FROM servers WHERE agent_token = $1 AND deleted_at IS NULL`
	err := r.db.GetContext(ctx, &row, query, token)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return row.server(), nil
}

func (r *ServerRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Server, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL`
	if err := r.db.GetContext(ctx, &total, countQuery, tenantID); err != nil {
		return nil, 0, err
	}

	var rows []serverRow
	offset := (page - 1) * perPage
	query := `SELECT ` + serverColumns + ` FROM servers WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &rows, query, tenantID, perPage, offset); err != nil {
		return nil, 0, err
	}

	servers := make([]models.Server, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, *row.server())
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
		server.RAMTotal, server.DiskTotal, server.Location, pq.StringArray(server.Tags),
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

// ============================================================
// THE PANEL HOST AS A MANAGED NODE
//
// The machine the panel is installed on is a node like any other: it has a row
// in `servers`. What makes it different is a row in `server_local_node`, which
// records that some panel process claims to be running on it and carries the
// evidence for the claim. Operations on a node listed there need no agent
// transport, so the marker has to be as trustworthy as the transport it
// replaces - which is why it is a table of its own rather than a value in
// servers.role, where the operator-facing PUT /servers/:id could set it.
//
// Everything here degrades the same way the agent-channel bookkeeping above
// does: an installation that has not run migrations/pending/local_node.sql
// reports "there is no local node" rather than failing, and every operation
// then takes the remote path it takes today.
// ============================================================

// ErrLocalNodeBookkeepingUnavailable means server_local_node does not exist
// yet. On the read path it is indistinguishable from "no local node", which is
// the safe reading; on the registration path it is an error, because there is
// nowhere to record the claim.
var ErrLocalNodeBookkeepingUnavailable = errors.New("repository: server_local_node is missing; run migrations/pending/local_node.sql")

// LocalNodeRole is written into servers.role when the panel host's row is
// created.
//
// servers.role is a free-form VARCHAR(50) with no values fixed anywhere in the
// code; docs/API.md shows 'web_server' as an example of what an operator might
// type. None of them describe this node, whose distinguishing property is that
// it runs the control plane, so a value is introduced here and only as a
// default: it is set when the row is created and never overwritten afterwards,
// and nothing branches on it. What makes a node local is its row in
// server_local_node, never this string.
const LocalNodeRole = "panel"

// LocalNodeFacts is what was measured on the panel host.
//
// Every field the panel might fail to measure is a pointer, and a nil pointer
// is written as SQL NULL. cpu_cores = 0 or os = 'linux' would be the panel
// asserting something it never read.
type LocalNodeFacts struct {
	// Hostname and ip_address are NOT NULL in the schema, so these two are
	// required rather than nullable.
	Hostname  string
	IPAddress string

	IPv6Address *string
	OS          *string
	Kernel      *string
	CPUCores    *int
	RAMTotal    *int64
	DiskTotal   *int64
}

// LocalNodeRegistration is one idempotent registration of the panel host.
type LocalNodeRegistration struct {
	// NodeID is the stable identity: a UUID generated once and persisted at
	// <EtcRoot>/node.json. It is this node's servers.id, which is what makes
	// registration an upsert on the primary key rather than a hostname match.
	NodeID uuid.UUID

	TenantID uuid.UUID
	Facts    LocalNodeFacts

	// Fingerprint is the salted hash of the host machine id, nil on a host
	// that has none.
	Fingerprint       *string
	FingerprintSource *string

	// Role and Status are only applied when the row is created, or when it is
	// being restored after a soft delete. A later registration never overwrites
	// what an operator chose.
	Role   string
	Status string
}

// LocalNodeRecord is the marker row plus the parts of the server row that say
// whether the claim is current.
type LocalNodeRecord struct {
	ServerID           uuid.UUID
	Hostname           string
	Fingerprint        string
	FingerprintSource  string
	RegisteredAt       time.Time
	LastVerifiedAt     *time.Time
	LastMismatchAt     *time.Time
	LastMismatchReason string
	AgentStatus        string
	LastSeenAt         *time.Time
}

// localNodeRow is the scan target. The columns are listed rather than selected
// with *, because this query joins two tables and a * would break the moment
// either grows a column.
type localNodeRow struct {
	ServerID           uuid.UUID      `db:"server_id"`
	Hostname           string         `db:"hostname"`
	Fingerprint        sql.NullString `db:"machine_fingerprint"`
	FingerprintSource  sql.NullString `db:"fingerprint_source"`
	RegisteredAt       time.Time      `db:"registered_at"`
	LastVerifiedAt     *time.Time     `db:"last_verified_at"`
	LastMismatchAt     *time.Time     `db:"last_mismatch_at"`
	LastMismatchReason sql.NullString `db:"last_mismatch_reason"`
	AgentStatus        sql.NullString `db:"agent_status"`
	LastSeenAt         *time.Time     `db:"last_seen_at"`
}

func (row localNodeRow) toRecord() *LocalNodeRecord {
	return &LocalNodeRecord{
		ServerID:           row.ServerID,
		Hostname:           row.Hostname,
		Fingerprint:        row.Fingerprint.String,
		FingerprintSource:  row.FingerprintSource.String,
		RegisteredAt:       row.RegisteredAt,
		LastVerifiedAt:     row.LastVerifiedAt,
		LastMismatchAt:     row.LastMismatchAt,
		LastMismatchReason: row.LastMismatchReason.String,
		AgentStatus:        row.AgentStatus.String,
		LastSeenAt:         row.LastSeenAt,
	}
}

const localNodeSelect = `
	SELECT l.server_id, s.hostname, l.machine_fingerprint, l.fingerprint_source,
	       l.registered_at, l.last_verified_at, l.last_mismatch_at, l.last_mismatch_reason,
	       s.agent_status, s.last_seen_at
	FROM server_local_node l
	JOIN servers s ON s.id = l.server_id
	WHERE s.deleted_at IS NULL
`

// RegisterLocalNode creates or refreshes the panel host's node, and returns the
// row along with whether this call created it.
//
// It is idempotent because it is keyed on the node id, which is the row's
// primary key: called twice on one machine it produces one row, and a machine
// that was renamed or readdressed updates that row rather than growing a second
// one. A row that had been soft-deleted is restored, because a panel process
// running on a machine cannot coherently report that the machine is not
// managed.
//
// The two writes share one transaction on purpose. Without the marker in
// server_local_node the server row is just another remote node with nobody
// running an agent on it, so a partial success is worse than a failure - and on
// an installation that has not run the migration, the transaction aborts and
// nothing at all is written.
func (r *ServerRepository) RegisterLocalNode(ctx context.Context, reg LocalNodeRegistration) (*models.Server, bool, error) {
	switch {
	case reg.NodeID == uuid.Nil:
		return nil, false, errors.New("repository: local node registration has no node id")
	case reg.TenantID == uuid.Nil:
		return nil, false, errors.New("repository: local node registration has no tenant")
	case strings.TrimSpace(reg.Facts.Hostname) == "":
		return nil, false, errors.New("repository: local node registration has no hostname")
	case strings.TrimSpace(reg.Facts.IPAddress) == "":
		return nil, false, errors.New("repository: local node registration has no IP address")
	}

	role := reg.Role
	if role == "" {
		role = LocalNodeRole
	}
	status := reg.Status
	if status == "" {
		status = "active"
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var existing int
	if err := tx.GetContext(ctx, &existing, `SELECT COUNT(*) FROM servers WHERE id = $1`, reg.NodeID); err != nil {
		return nil, false, err
	}
	created := existing == 0

	// Only the measured facts are refreshed on a repeat registration. ssh_port,
	// location, tags and - once the row exists - role and status belong to the
	// operator, and a machine measuring itself has nothing to say about them.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO servers (id, tenant_id, hostname, ip_address, ipv6_address, ssh_port,
			agent_status, agent_token, os, kernel, cpu_cores, ram_total, disk_total,
			role, status, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 22, 'online', $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			hostname     = EXCLUDED.hostname,
			ip_address   = EXCLUDED.ip_address,
			ipv6_address = EXCLUDED.ipv6_address,
			os           = EXCLUDED.os,
			kernel       = EXCLUDED.kernel,
			cpu_cores    = EXCLUDED.cpu_cores,
			ram_total    = EXCLUDED.ram_total,
			disk_total   = EXCLUDED.disk_total,
			agent_status = 'online',
			last_seen_at = NOW(),
			role         = COALESCE(NULLIF(servers.role, ''), EXCLUDED.role),
			status       = CASE WHEN servers.deleted_at IS NOT NULL THEN EXCLUDED.status ELSE servers.status END,
			deleted_at   = NULL,
			updated_at   = NOW()
	`,
		reg.NodeID, reg.TenantID, reg.Facts.Hostname, reg.Facts.IPAddress, reg.Facts.IPv6Address,
		NewAgentToken(), reg.Facts.OS, reg.Facts.Kernel, reg.Facts.CPUCores,
		reg.Facts.RAMTotal, reg.Facts.DiskTotal, role, status,
	)
	if err != nil {
		return nil, false, fmt.Errorf("repository: cannot register the panel host as a node: %w", err)
	}

	// Registration is the moment the claim is made afresh, on the machine
	// itself, so any recorded mismatch is cleared with it.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO server_local_node (server_id, machine_fingerprint, fingerprint_source,
			registered_at, last_verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW(), NOW(), NOW())
		ON CONFLICT (server_id) DO UPDATE SET
			machine_fingerprint  = EXCLUDED.machine_fingerprint,
			fingerprint_source   = EXCLUDED.fingerprint_source,
			last_verified_at     = NOW(),
			last_mismatch_at     = NULL,
			last_mismatch_reason = NULL,
			updated_at           = NOW()
	`, reg.NodeID, reg.Fingerprint, reg.FingerprintSource)
	if isMissingLocalNodeTable(err) {
		return nil, false, ErrLocalNodeBookkeepingUnavailable
	}
	if err != nil {
		return nil, false, fmt.Errorf("repository: cannot mark the panel host as the local node: %w", err)
	}

	var row serverRow
	if err := tx.GetContext(ctx, &row, `SELECT `+serverColumns+` FROM servers WHERE id = $1`, reg.NodeID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return row.server(), created, nil
}

// GetLocalNode returns the marker for one node, or sql.ErrNoRows when that node
// is not marked local. A missing bookkeeping table is reported separately so
// the caller can tell "this node is remote" from "this installation cannot
// answer the question".
func (r *ServerRepository) GetLocalNode(ctx context.Context, serverID uuid.UUID) (*LocalNodeRecord, error) {
	var row localNodeRow
	err := r.db.GetContext(ctx, &row, localNodeSelect+` AND l.server_id = $1`, serverID)
	if isMissingLocalNodeTable(err) {
		return nil, ErrLocalNodeBookkeepingUnavailable
	}
	if err != nil {
		return nil, err
	}
	return row.toRecord(), nil
}

// FindLocalNodeByFingerprint finds the node some machine registered, by the
// witness it registered with.
//
// It exists for one recovery: node.json was lost - a rebuilt etc directory, a
// restore that skipped it - and generating a fresh identity would fork this
// machine into a second row. Registration asks this first, and adopts the row
// it finds rather than creating a duplicate. It is keyed on the machine
// witness, so it can only ever adopt a row that this same machine registered.
func (r *ServerRepository) FindLocalNodeByFingerprint(ctx context.Context, fingerprint string) (*LocalNodeRecord, error) {
	if strings.TrimSpace(fingerprint) == "" {
		return nil, sql.ErrNoRows
	}
	var row localNodeRow
	err := r.db.GetContext(ctx, &row, localNodeSelect+` AND l.machine_fingerprint = $1`, fingerprint)
	if isMissingLocalNodeTable(err) {
		return nil, ErrLocalNodeBookkeepingUnavailable
	}
	if err != nil {
		return nil, err
	}
	return row.toRecord(), nil
}

// TouchLocalNode records that a panel process stood on this machine and proved
// it, now. It is what keeps servers.last_seen_at for the local node from being
// a claim written once at install time and true forever.
func (r *ServerRepository) TouchLocalNode(ctx context.Context, serverID uuid.UUID, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE server_local_node
		SET last_verified_at = $2, last_mismatch_at = NULL, last_mismatch_reason = NULL, updated_at = NOW()
		WHERE server_id = $1
	`, serverID, at)
	if isMissingLocalNodeTable(err) {
		return ErrLocalNodeBookkeepingUnavailable
	}
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
		UPDATE servers SET last_seen_at = $2, agent_status = 'online', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, serverID, at)
	return err
}

// NoteLocalNodeMismatch records that verification failed and marks the node
// offline.
//
// last_seen_at is deliberately not advanced: the panel has just discovered that
// it is not standing on this machine, so it has no grounds to report having
// seen it. The marker row is kept rather than deleted, because the row is the
// evidence an operator needs to understand what happened, and deleting it would
// let the next registration quietly recreate the same wrong claim.
func (r *ServerRepository) NoteLocalNodeMismatch(ctx context.Context, serverID uuid.UUID, reason string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE server_local_node
		SET last_mismatch_at = $2, last_mismatch_reason = $3, updated_at = NOW()
		WHERE server_id = $1
	`, serverID, at, reason)
	if isMissingLocalNodeTable(err) {
		return ErrLocalNodeBookkeepingUnavailable
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE servers SET agent_status = 'offline', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, serverID)
	return err
}

// DefaultTenantID is the tenant a self-registering panel host belongs to.
//
// The seeded installation has exactly one tenant, slug 'default', and that is
// what a single-VPS install will always have; it is preferred by name rather
// than by a UUID literal copied out of 001_initial_schema.sql. The oldest live
// tenant is the fallback for an installation that renamed or replaced it.
func (r *ServerRepository) DefaultTenantID(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.GetContext(ctx, &id, `
		SELECT id FROM tenants
		WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY (slug = 'default') DESC, created_at ASC, id ASC
		LIMIT 1
	`)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// isMissingLocalNodeTable reports whether an error is PostgreSQL saying
// server_local_node has not been created yet. It is matched on the message for
// the same reason as isMissingChannelTable: so this file keeps its single
// database dependency.
func isMissingLocalNodeTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "server_local_node") && strings.Contains(msg, "does not exist")
}
