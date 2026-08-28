package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// auditLogColumns is the explicit column list every read of audit_logs uses.
//
// This file used to read the table with SELECT * and scan the result
// POSITIONALLY into eleven destinations. That is a trap: a column added to
// audit_logs lands at the end of the physical column order and takes every one
// of these reads down with it. Measured on PostgreSQL 16.15 with one extra
// column present:
//
//	Search()  -> sql: expected 12 destination arguments in Scan, not 11
//	GetByID() -> missing destination name probe_extra in *models.AuditLog
//
// Loud rather than silent, but loud on every audit query on every install. The
// tamper-evident chain therefore adds no column to audit_logs at all - it lives
// in audit_log_chain, keyed by audit_logs.id - and the reads below name their
// columns so that the next person to add one has a fighting chance.
const auditLogColumns = `id, tenant_id, user_id, action, resource, resource_id, ` +
	`details, ip_address, user_agent, status, created_at`

// scanAuditLog reads one row selected with auditLogColumns, in that order.
func scanAuditLog(rows *sql.Rows, l *models.AuditLog) error {
	return rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.Action, &l.Resource,
		&l.ResourceID, &l.Details, &l.IPAddress, &l.UserAgent, &l.Status, &l.CreatedAt)
}

type AuditRepository struct {
	db *sqlx.DB
}

func NewAuditRepository(db *sqlx.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := `
		INSERT INTO audit_logs (tenant_id, user_id, action, resource, resource_id, details, ip_address, user_agent, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		log.TenantID, log.UserID, log.Action, log.Resource, log.ResourceID,
		log.Details, log.IPAddress, log.UserAgent, log.Status,
	).Scan(&log.ID, &log.CreatedAt)
}

func (r *AuditRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.AuditLog, error) {
	var log models.AuditLog
	err := r.db.GetContext(ctx, &log,
		"SELECT "+auditLogColumns+" FROM audit_logs WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditRepository) Search(ctx context.Context, tenantID uuid.UUID, req *models.AuditLogSearchRequest) ([]models.AuditLog, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if req.UserID != nil {
		where += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, *req.UserID)
		argIdx++
	}
	if req.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, req.Action)
		argIdx++
	}
	if req.Resource != "" {
		where += fmt.Sprintf(" AND resource = $%d", argIdx)
		args = append(args, req.Resource)
		argIdx++
	}
	if req.ResourceID != nil {
		where += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, *req.ResourceID)
		argIdx++
	}
	if req.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, req.Status)
		argIdx++
	}
	if req.Start != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *req.Start)
		argIdx++
	}
	if req.End != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *req.End)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM audit_logs " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get logs
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT %s FROM audit_logs %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, auditLogColumns, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := scanAuditLog(rows, &l); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *AuditRepository) GetStats(ctx context.Context, tenantID uuid.UUID, days int) (*models.AuditLogStats, error) {
	stats := &models.AuditLogStats{
		ByAction:   make(map[string]int),
		ByResource: make(map[string]int),
		ByStatus:   make(map[string]int),
		ByUser:     make(map[string]int),
	}

	since := time.Now().AddDate(0, 0, -days)

	// Total logs
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2",
		tenantID, since,
	).Scan(&stats.TotalLogs)
	if err != nil {
		return nil, err
	}

	// By action
	rows, err := r.db.QueryContext(ctx,
		"SELECT action, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY action",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var count int
		if err := rows.Scan(&action, &count); err != nil {
			return nil, err
		}
		stats.ByAction[action] = count
	}

	// By resource
	rows, err = r.db.QueryContext(ctx,
		"SELECT resource, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY resource",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var resource string
		var count int
		if err := rows.Scan(&resource, &count); err != nil {
			return nil, err
		}
		stats.ByResource[resource] = count
	}

	// By status
	rows, err = r.db.QueryContext(ctx,
		"SELECT status, COUNT(*) FROM audit_logs WHERE tenant_id = $1 AND created_at >= $2 GROUP BY status",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.ByStatus[status] = count
	}

	// By user
	rows, err = r.db.QueryContext(ctx,
		"SELECT COALESCE(u.username, 'system'), COUNT(*) FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id WHERE a.tenant_id = $1 AND a.created_at >= $2 GROUP BY u.username",
		tenantID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		var count int
		if err := rows.Scan(&username, &count); err != nil {
			return nil, err
		}
		stats.ByUser[username] = count
	}

	// Recent logs
	rows, err = r.db.QueryContext(ctx,
		"SELECT "+auditLogColumns+" FROM audit_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 10",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l models.AuditLog
		if err := scanAuditLog(rows, &l); err != nil {
			return nil, err
		}
		stats.RecentLogs = append(stats.RecentLogs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// ============================================================
// The tamper-evident chain.
//
// Nothing below writes a hash. The chain is appended by an AFTER INSERT trigger
// on audit_logs (see migrations/pending/audit_chain.sql), so an entry written
// by a future service that never heard of this repository is chained too. A
// chain that only covers the rows one Go function wrote is not a chain.
// ============================================================

// ErrChainAppendOnly is returned when the database refuses a write to the audit
// tables. It is not a bug report: it is the guard working.
var ErrChainAppendOnly = errors.New("the audit log is append-only; entries cannot be changed or removed")

// ChainStatus is the shape of a tenant's chain right now.
type ChainStatus struct {
	TenantID   uuid.UUID          `json:"tenant_id"`
	Entries    int64              `json:"entries"`
	FirstSeq   int64              `json:"first_seq"`
	LastSeq    int64              `json:"last_seq"`
	HeadSeq    int64              `json:"head_seq"`
	HeadHash   string             `json:"head_hash"`
	OldestAt   *time.Time         `json:"oldest_at,omitempty"`
	NewestAt   *time.Time         `json:"newest_at,omitempty"`
	Seals      int64              `json:"seals"`
	LastVerify *ChainVerification `json:"last_verification,omitempty"`
}

// ChainVerification is one verification pass, as audit_chain_verify() reports
// it and as audit_chain_verification records it.
type ChainVerification struct {
	Mode        string     `json:"mode"`
	OK          bool       `json:"ok"`
	Checked     int64      `json:"checked"`
	FromSeq     int64      `json:"from_seq"`
	ToSeq       int64      `json:"to_seq"`
	FirstSeq    *int64     `json:"first_seq,omitempty"`
	LastSeq     *int64     `json:"last_seq,omitempty"`
	BreakSeq    *int64     `json:"break_seq,omitempty"`
	BreakAt     *time.Time `json:"break_at,omitempty"`
	BreakReason *string    `json:"break_reason,omitempty"`
	BreakLogID  *uuid.UUID `json:"break_log_id,omitempty"`
	HeadSeq     *int64     `json:"head_seq,omitempty"`
	HeadOK      bool       `json:"head_ok"`
	DurationMS  int64      `json:"duration_ms"`
	RanAt       time.Time  `json:"ran_at"`
}

// Seal is a permanent statement that the chain had a particular hash at a
// particular sequence number. Pruning writes one; so does an export.
type Seal struct {
	ID         uuid.UUID  `json:"id"`
	Kind       string     `json:"kind"`
	Seq        int64      `json:"seq"`
	EntryHash  string     `json:"entry_hash"`
	FirstSeq   *int64     `json:"first_seq,omitempty"`
	LastSeq    *int64     `json:"last_seq,omitempty"`
	EntryCount int64      `json:"entry_count"`
	Cutoff     *time.Time `json:"cutoff,omitempty"`
	Note       string     `json:"note"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PrunePreview answers "what would a retention run remove?" without removing
// anything. The panel can run this; it cannot run the prune itself, on purpose.
type PrunePreview struct {
	Prunable  int64      `json:"prunable"`
	FirstSeq  *int64     `json:"first_seq,omitempty"`
	SealSeq   *int64     `json:"seal_seq,omitempty"`
	SealHash  *string    `json:"seal_hash,omitempty"`
	OldestAt  *time.Time `json:"oldest_at,omitempty"`
	NewestAt  *time.Time `json:"newest_at,omitempty"`
	Retention int        `json:"retention_days"`
	Cutoff    time.Time  `json:"cutoff"`
}

// maxSeq is the open right-hand end of a range, matching the SQL default.
const maxSeq = int64(9223372036854775807)

// Verify walks [fromSeq, toSeq] and returns the first break.
//
// deep = false reads audit_log_chain only and catches removal, reordering,
// relinking and a rewritten chain row. deep = true additionally re-reads
// audit_logs and recomputes each entry's content hash, which is what catches an
// edited entry; it costs a join, and is the half worth budgeting for.
func (r *AuditRepository) Verify(ctx context.Context, tenantID uuid.UUID, fromSeq, toSeq int64, deep bool) (*ChainVerification, error) {
	if fromSeq < 1 {
		fromSeq = 1
	}
	if toSeq <= 0 {
		toSeq = maxSeq
	}

	mode := "links"
	if deep {
		mode = "deep"
	}

	v := &ChainVerification{Mode: mode, FromSeq: fromSeq, ToSeq: toSeq, RanAt: time.Now().UTC()}

	started := time.Now()
	err := r.db.QueryRowContext(ctx,
		`SELECT ok, checked, first_seq, last_seq, break_seq, break_at, break_reason,
		        break_log_id, head_seq, head_ok
		   FROM audit_chain_verify($1, $2, $3, $4)`,
		tenantID, fromSeq, toSeq, deep,
	).Scan(&v.OK, &v.Checked, &v.FirstSeq, &v.LastSeq, &v.BreakSeq, &v.BreakAt,
		&v.BreakReason, &v.BreakLogID, &v.HeadSeq, &v.HeadOK)
	if err != nil {
		return nil, fmt.Errorf("verify audit chain: %w", err)
	}
	v.DurationMS = time.Since(started).Milliseconds()

	// Record the range this pass ACTUALLY covered, not the range it asked for.
	//
	// An unbounded request asks for [1, 2^63-1]. Filing that as the verified
	// range would let the next incremental pass resume at 2^63-1, verify no
	// entries at all, and report the log intact - a verification that checks
	// nothing and passes is worse than no verification, because somebody is
	// relying on it. Storing what was covered means a resume starts one entry
	// before the last one this pass actually read.
	if v.FirstSeq != nil {
		v.FromSeq = *v.FirstSeq
	}
	switch {
	case v.LastSeq != nil:
		v.ToSeq = *v.LastSeq
	case toSeq == maxSeq:
		v.ToSeq = 0
	}

	return v, nil
}

// RecordVerification files the result of a pass so the next incremental run
// knows where the last one got to. It is a cache of work already done, not
// evidence, and a failure to record it must not fail the verification.
func (r *AuditRepository) RecordVerification(ctx context.Context, tenantID uuid.UUID, v *ChainVerification) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_chain_verification
		     (tenant_id, mode, from_seq, to_seq, checked, ok, break_seq, break_at,
		      break_reason, break_log_id, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		tenantID, v.Mode, v.FromSeq, v.ToSeq, v.Checked, v.OK, v.BreakSeq,
		v.BreakAt, v.BreakReason, v.BreakLogID, v.DurationMS)
	return err
}

// LastGoodSeq is the highest sequence number a previous pass verified and
// found intact, or 0 if there has never been one.
//
// An incremental pass starts there. That is a real assumption and it is stated
// plainly: it trusts that the prefix was intact when it was checked, and it
// will not notice a change made to the prefix afterwards. A full pass is the
// ground truth and belongs on a schedule.
func (r *AuditRepository) LastGoodSeq(ctx context.Context, tenantID uuid.UUID, deep bool) (int64, error) {
	mode := "links"
	if deep {
		mode = "deep"
	}

	var seq sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT max(to_seq) FROM audit_chain_verification
		  WHERE tenant_id = $1 AND ok AND mode = $2`,
		tenantID, mode,
	).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

// Status is the cheap answer for a dashboard: no hashing, no scan.
func (r *AuditRepository) Status(ctx context.Context, tenantID uuid.UUID) (*ChainStatus, error) {
	st := &ChainStatus{TenantID: tenantID}

	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(count(*), 0), COALESCE(min(seq), 0), COALESCE(max(seq), 0),
		        min(created_at), max(created_at)
		   FROM audit_log_chain WHERE tenant_id = $1`,
		tenantID,
	).Scan(&st.Entries, &st.FirstSeq, &st.LastSeq, &st.OldestAt, &st.NewestAt)
	if err != nil {
		return nil, err
	}

	var headSeq sql.NullInt64
	var headHash sql.NullString
	if err := r.db.QueryRowContext(ctx,
		"SELECT seq, head_hash FROM audit_chain_head WHERE tenant_id = $1", tenantID,
	).Scan(&headSeq, &headHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	st.HeadSeq = headSeq.Int64
	st.HeadHash = headHash.String

	if err := r.db.QueryRowContext(ctx,
		"SELECT count(*) FROM audit_chain_seal WHERE tenant_id = $1", tenantID,
	).Scan(&st.Seals); err != nil {
		return nil, err
	}

	last := &ChainVerification{}
	err = r.db.QueryRowContext(ctx,
		`SELECT mode, ok, checked, from_seq, to_seq, break_seq, break_at, break_reason,
		        break_log_id, duration_ms, ran_at
		   FROM audit_chain_verification WHERE tenant_id = $1
		  ORDER BY ran_at DESC LIMIT 1`,
		tenantID,
	).Scan(&last.Mode, &last.OK, &last.Checked, &last.FromSeq, &last.ToSeq,
		&last.BreakSeq, &last.BreakAt, &last.BreakReason, &last.BreakLogID,
		&last.DurationMS, &last.RanAt)
	switch {
	case err == nil:
		st.LastVerify = last
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, err
	}

	return st, nil
}

// Seals lists the tenant's seals, newest first.
func (r *AuditRepository) Seals(ctx context.Context, tenantID uuid.UUID, limit int) ([]Seal, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, kind, seq, entry_hash, first_seq, last_seq, entry_count, cutoff, note, created_at
		   FROM audit_chain_seal WHERE tenant_id = $1
		  ORDER BY created_at DESC LIMIT $2`,
		tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seals := make([]Seal, 0, 8)
	for rows.Next() {
		var s Seal
		if err := rows.Scan(&s.ID, &s.Kind, &s.Seq, &s.EntryHash, &s.FirstSeq,
			&s.LastSeq, &s.EntryCount, &s.Cutoff, &s.Note, &s.CreatedAt); err != nil {
			return nil, err
		}
		seals = append(seals, s)
	}
	return seals, rows.Err()
}

// PrunePreview reports what a retention run would remove. Read-only, and needs
// no privilege the panel has been denied.
func (r *AuditRepository) PrunePreview(ctx context.Context, tenantID uuid.UUID, before time.Time) (*PrunePreview, error) {
	p := &PrunePreview{Cutoff: before.UTC()}
	err := r.db.QueryRowContext(ctx,
		`SELECT prunable, first_seq, seal_seq, seal_hash, oldest_at, newest_at
		   FROM audit_chain_prune_preview($1, $2)`,
		tenantID, before,
	).Scan(&p.Prunable, &p.FirstSeq, &p.SealSeq, &p.SealHash, &p.OldestAt, &p.NewestAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// exportColumns names the strings that go into the hash, rendered by the
// database in exactly the form the hash was taken over. Nothing here is
// re-serialised in Go: details comes out as PostgreSQL's own jsonb text and
// created_at through the same audit_chain_timestamp() the hash used, so an
// exported bundle carries the hash preimage byte for byte.
const exportColumns = `c.seq, c.prev_hash, c.content_hash, c.entry_hash,
	l.id::text, l.tenant_id::text, COALESCE(l.user_id::text, ''), l.action, l.resource,
	COALESCE(l.resource_id::text, ''), COALESCE(l.details, '{}'::jsonb)::text,
	COALESCE(l.ip_address, ''), COALESCE(l.user_agent, ''), l.status,
	audit_chain_timestamp(l.created_at)`

// ExportRange builds a bundle for [fromSeq, toSeq], capped at limit entries.
//
// The join is deliberately an inner join. An entry whose row has been removed
// simply does not appear, which shows up in the bundle as a sequence gap and
// fails verification - which is the truth. Papering over it with a placeholder
// would produce a bundle that verifies and lies.
func (r *AuditRepository) ExportRange(ctx context.Context, tenantID uuid.UUID, fromSeq, toSeq int64, limit int) (*audit.Export, error) {
	if fromSeq < 1 {
		fromSeq = 1
	}
	if toSeq <= 0 {
		toSeq = maxSeq
	}
	if limit <= 0 || limit > exportHardCap {
		limit = exportHardCap
	}

	exp := audit.NewExport(tenantID.String(), time.Now())

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+exportColumns+`
		   FROM audit_log_chain c
		   JOIN audit_logs l ON l.id = c.audit_log_id
		  WHERE c.tenant_id = $1 AND c.seq BETWEEN $2 AND $3
		  ORDER BY c.seq
		  LIMIT $4`,
		tenantID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var e audit.ExportEntry
		if err := rows.Scan(&e.Seq, &e.PrevHash, &e.ContentHash, &e.EntryHash,
			&e.Content.ID, &e.Content.TenantID, &e.Content.UserID, &e.Content.Action,
			&e.Content.Resource, &e.Content.ResourceID, &e.Content.Details,
			&e.Content.IPAddress, &e.Content.UserAgent, &e.Content.Status,
			&e.Content.CreatedAt); err != nil {
			return nil, err
		}
		exp.Entries = append(exp.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	exp.EntryCount = len(exp.Entries)

	if len(exp.Entries) > 0 {
		anchor, err := r.anchorFor(ctx, tenantID, exp.Entries[0].Seq)
		if err != nil {
			return nil, err
		}
		exp.Anchor = anchor
	}

	// The head goes in so a verifier can tell a complete bundle from one whose
	// newest entries were thrown away.
	//
	// It is deliberately omitted for a bundle that came back empty from an
	// explicit slice request: "you asked for entries 500 to 600 and this chain
	// has fifty" is a bad request, not evidence of tampering, and a verifier
	// that reported truncated_tail for it would be crying wolf. An empty bundle
	// for a range that starts at the beginning is a different matter - a chain
	// with a head and no entries at all is exactly what a wholesale deletion
	// leaves - so there the head stays and the verifier says so.
	if len(exp.Entries) > 0 || fromSeq <= 1 {
		var headSeq sql.NullInt64
		var headHash sql.NullString
		if err := r.db.QueryRowContext(ctx,
			"SELECT seq, head_hash FROM audit_chain_head WHERE tenant_id = $1", tenantID,
		).Scan(&headSeq, &headHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if headSeq.Valid {
			exp.Head = &audit.Head{Seq: headSeq.Int64, Hash: headHash.String}
		}
	}

	// Does this bundle contain the whole chain? Only if it started at the
	// beginning, was not cut short by the cap, and the requested range reached
	// the tip. Anything else is a slice, and a slice must say so - a verifier
	// treats a short complete bundle as a deleted tail, which for a chain
	// longer than exportHardCap would be a false accusation on every export.
	exp.Complete = fromSeq <= 1 &&
		len(exp.Entries) < limit &&
		(exp.Head == nil || toSeq >= exp.Head.Seq)

	// The seals travel with the bundle. They are the part an auditor can check
	// the panel AGAINST rather than merely take from it: a seal cannot be
	// rewritten, so it catches both an entry edited after it was sealed and a
	// tail removed with the head tidied up afterwards.
	seals, err := r.Seals(ctx, tenantID, 500)
	if err != nil {
		return nil, err
	}
	for _, seal := range seals {
		exp.Seals = append(exp.Seals, audit.ExportSeal{
			ID:         seal.ID.String(),
			Kind:       seal.Kind,
			Seq:        seal.Seq,
			EntryHash:  seal.EntryHash,
			EntryCount: seal.EntryCount,
			Note:       seal.Note,
			CreatedAt:  audit.FormatTime(seal.CreatedAt),
		})
	}

	return exp, nil
}

// exportHardCap bounds one bundle. Bundles compose: the anchor of the next one
// is the last entry hash of this one, so a long history is exported as a series
// and verified as a series, without any single request having to hold ten
// million entries in memory.
const exportHardCap = 50000

// anchorFor resolves what the entry at firstSeq chains back to: the entry
// before it, a seal left where a prune cut the chain, or the genesis hash.
func (r *AuditRepository) anchorFor(ctx context.Context, tenantID uuid.UUID, firstSeq int64) (audit.Anchor, error) {
	if firstSeq <= 1 {
		return audit.Anchor{Kind: "genesis", Seq: 0, Hash: audit.GenesisHash}, nil
	}

	var hash string
	err := r.db.QueryRowContext(ctx,
		"SELECT entry_hash FROM audit_log_chain WHERE tenant_id = $1 AND seq = $2",
		tenantID, firstSeq-1).Scan(&hash)
	if err == nil {
		return audit.Anchor{Kind: "entry", Seq: firstSeq - 1, Hash: hash}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return audit.Anchor{}, err
	}

	var sealID uuid.UUID
	var sealedAt time.Time
	err = r.db.QueryRowContext(ctx,
		`SELECT id, entry_hash, created_at FROM audit_chain_seal
		  WHERE tenant_id = $1 AND seq = $2 ORDER BY created_at DESC LIMIT 1`,
		tenantID, firstSeq-1).Scan(&sealID, &hash, &sealedAt)
	if err == nil {
		return audit.Anchor{
			Kind:     "seal",
			Seq:      firstSeq - 1,
			Hash:     hash,
			SealID:   sealID.String(),
			SealedAt: audit.FormatTime(sealedAt),
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return audit.Anchor{}, err
	}

	// Nothing to chain back to. Saying so is the honest answer; a verifier
	// reports it as missing_anchor rather than silently accepting the range.
	return audit.Anchor{Kind: "none", Seq: firstSeq - 1}, nil
}

// SealExport records that a bundle was taken, so an auditor holding the bundle
// can later ask the operator to produce the seal that matches it.
func (r *AuditRepository) SealExport(ctx context.Context, tenantID uuid.UUID, exp *audit.Export, note string) (*Seal, error) {
	if exp == nil || len(exp.Entries) == 0 {
		return nil, nil
	}

	last := exp.Entries[len(exp.Entries)-1]
	s := &Seal{}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO audit_chain_seal (tenant_id, kind, seq, entry_hash, first_seq,
		                               last_seq, entry_count, note)
		 VALUES ($1, 'export', $2, $3, $4, $5, $6, $7)
		 RETURNING id, kind, seq, entry_hash, first_seq, last_seq, entry_count, cutoff, note, created_at`,
		tenantID, last.Seq, last.EntryHash, exp.Entries[0].Seq, last.Seq,
		len(exp.Entries), note,
	).Scan(&s.ID, &s.Kind, &s.Seq, &s.EntryHash, &s.FirstSeq, &s.LastSeq,
		&s.EntryCount, &s.Cutoff, &s.Note, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// RecordCheckpoint seals the tip of the chain in the append-only seal table.
//
// This is the answer to the one thing audit_chain_head cannot do. The head is
// mutable - it advances on every entry - so an attacker holding the panel's
// database role can delete the newest entries and move the head to match,
// leaving a chain that verifies perfectly. A checkpoint says "this chain
// reached sequence N with hash H at time T" somewhere that refuses UPDATE and
// DELETE, and from then on any state with fewer entries is a provable deletion.
//
// Written by a clean full verification pass, which is the moment the panel has
// just satisfied itself that everything up to N is intact.
func (r *AuditRepository) RecordCheckpoint(ctx context.Context, tenantID uuid.UUID, seq int64, entryHash, note string) (*Seal, error) {
	s := &Seal{}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO audit_chain_seal (tenant_id, kind, seq, entry_hash, first_seq, last_seq, entry_count, note)
		 VALUES ($1, 'checkpoint', $2, $3, NULL, $2, 0, $4)
		 RETURNING id, kind, seq, entry_hash, first_seq, last_seq, entry_count, cutoff, note, created_at`,
		tenantID, seq, entryHash, note,
	).Scan(&s.ID, &s.Kind, &s.Seq, &s.EntryHash, &s.FirstSeq, &s.LastSeq,
		&s.EntryCount, &s.Cutoff, &s.Note, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// LastCheckpointSeq is the highest sequence number any seal has committed to.
// Nothing at or below it can be removed without the removal being provable.
func (r *AuditRepository) LastCheckpointSeq(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var seq sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT max(seq) FROM audit_chain_seal
		  WHERE tenant_id = $1 AND kind IN ('checkpoint', 'export')`, tenantID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	return seq.Int64, nil
}

// EntryHashAt returns the entry hash at one sequence number, or "" if there is
// no entry there.
func (r *AuditRepository) EntryHashAt(ctx context.Context, tenantID uuid.UUID, seq int64) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx,
		"SELECT entry_hash FROM audit_log_chain WHERE tenant_id = $1 AND seq = $2",
		tenantID, seq).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return hash, nil
}

// HeadOf returns the tenant's current chain tip.
func (r *AuditRepository) HeadOf(ctx context.Context, tenantID uuid.UUID) (int64, string, error) {
	var seq int64
	var hash string
	err := r.db.QueryRowContext(ctx,
		"SELECT seq, head_hash FROM audit_chain_head WHERE tenant_id = $1", tenantID).Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	return seq, hash, nil
}

// ChainEntryFor returns the chain link for one audit entry: the caller usually
// wants it right after writing, to hand back a receipt the writer can keep.
func (r *AuditRepository) ChainEntryFor(ctx context.Context, logID uuid.UUID) (int64, string, error) {
	var seq int64
	var hash string
	err := r.db.QueryRowContext(ctx,
		"SELECT seq, entry_hash FROM audit_log_chain WHERE audit_log_id = $1", logID,
	).Scan(&seq, &hash)
	if err != nil {
		return 0, "", err
	}
	return seq, hash, nil
}
