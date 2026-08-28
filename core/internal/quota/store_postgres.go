package quota

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// PostgresStore is the production Store, on the panel database.
//
// Every statement it runs is a constant in this file and every one of them is
// listed in preparedStatements, which the live-schema test PREPAREs against a
// real PostgreSQL. Preparing a statement proves its columns exist - which is
// precisely what was never true of the jobs table.
type PostgresStore struct {
	db *sqlx.DB
}

// NewPostgresStore wraps the panel database.
func NewPostgresStore(db *sqlx.DB) *PostgresStore { return &PostgresStore{db: db} }

const (
	qAssignment = `
SELECT tp.tenant_id, tp.assigned_at, tp.suspended, tp.suspended_at,
       tp.suspended_reason, tp.suspended_automatically,
       p.id, p.owner_tenant_id, p.name, p.slug, p.description,
       p.disk_mb, p.bandwidth_mb, p.max_websites, p.max_databases,
       p.max_mailboxes, p.max_subdomains, p.max_cron_jobs,
       p.features, p.over_quota_action, p.warn_percent,
       p.grace_percent::double precision, p.grace_floor_mb,
       p.is_active, p.created_at, p.updated_at
  FROM tenant_packages tp
  JOIN hosting_packages p ON p.id = tp.package_id
 WHERE tp.tenant_id = $1`

	// Expired overrides are filtered here rather than deleted, so the record of
	// what was granted survives the grant.
	qOverrides = `
SELECT resource, limit_value, reason, expires_at, created_at
  FROM tenant_quota_overrides
 WHERE tenant_id = $1
   AND (expires_at IS NULL OR expires_at > NOW())
 ORDER BY resource`

	qFeatureOverrides = `
SELECT feature, allowed
  FROM tenant_feature_overrides
 WHERE tenant_id = $1
   AND (expires_at IS NULL OR expires_at > NOW())`

	// One round trip for every counted resource. Each subquery hits the
	// tenant_id index of its own table.
	//
	// Subdomains counts every hostname attached to the account that is not the
	// site's primary domain. Counting only type = 'subdomain' would leave
	// 'addon' and 'alias' rows outside every limit, which is a loophole rather
	// than a policy.
	qCounts = `
SELECT
  (SELECT COUNT(*) FROM websites         WHERE tenant_id = $1 AND deleted_at IS NULL) AS websites,
  (SELECT COUNT(*) FROM database_entries WHERE tenant_id = $1)                        AS databases,
  (SELECT COUNT(*) FROM mail_accounts    WHERE tenant_id = $1)                        AS mailboxes,
  (SELECT COUNT(*) FROM domains          WHERE tenant_id = $1 AND type <> 'primary')  AS subdomains,
  (SELECT COUNT(*) FROM cron_jobs        WHERE tenant_id = $1)                        AS cron_jobs`

	qMeasured = `
SELECT disk_used_mb, disk_file_count, disk_measured_at, disk_measure_ms, disk_partial,
       bandwidth_used_mb, bandwidth_period_start, bandwidth_measured_at
  FROM tenant_quota_usage
 WHERE tenant_id = $1`

	qInsertEvent = `
INSERT INTO tenant_quota_events (tenant_id, resource, event, limit_value, usage_value, message)
VALUES ($1, $2, $3, $4, $5, $6)`

	// The casts are not decoration. Without them PostgreSQL deduces one type
	// for $2 from the INSERT target column (varchar(32)) and another from the
	// comparison in the WHERE clause, and refuses to prepare the statement at
	// all: "inconsistent types deduced for parameter $2". Spelling the types
	// out makes both uses agree.
	qInsertEventThrottled = `
INSERT INTO tenant_quota_events (tenant_id, resource, event, limit_value, usage_value, message)
SELECT $1::uuid, $2::text, $3::text, $4::bigint, $5::bigint, $6::text
 WHERE NOT EXISTS (
       SELECT 1 FROM tenant_quota_events
        WHERE tenant_id = $1::uuid AND resource = $2::text AND event = $3::text
          AND created_at > NOW() - make_interval(secs => $7))`

	qSetSuspended = `
UPDATE tenant_packages
   SET suspended               = $2,
       suspended_at            = CASE WHEN $2 THEN COALESCE(suspended_at, NOW()) ELSE NULL END,
       suspended_reason        = CASE WHEN $2 THEN $3 ELSE '' END,
       suspended_automatically = CASE WHEN $2 THEN $4 ELSE FALSE END,
       updated_at              = NOW()
 WHERE tenant_id = $1`

	qManagedTenants = `
SELECT tp.tenant_id
  FROM tenant_packages tp
  JOIN tenants t ON t.id = tp.tenant_id AND t.deleted_at IS NULL
 ORDER BY tp.tenant_id`

	qSiteRoots = `
SELECT DISTINCT COALESCE(root_dir, '')
  FROM websites
 WHERE tenant_id = $1
   AND deleted_at IS NULL
   AND COALESCE(root_dir, '') <> ''`

	qDatabaseBytes = `
SELECT COALESCE(SUM(size), 0) FROM database_entries WHERE tenant_id = $1`

	qBandwidthBytes = `
SELECT COALESCE(SUM(total_bandwidth), 0)
  FROM website_stats
 WHERE tenant_id = $1 AND date >= $2 AND date < $3`

	qSaveDisk = `
INSERT INTO tenant_quota_usage
       (tenant_id, disk_used_mb, disk_file_count, disk_measured_at, disk_measure_ms, disk_partial, updated_at)
VALUES ($1, $2, $3, NOW(), $4, $5, NOW())
ON CONFLICT (tenant_id) DO UPDATE
   SET disk_used_mb    = EXCLUDED.disk_used_mb,
       disk_file_count = EXCLUDED.disk_file_count,
       disk_measured_at= EXCLUDED.disk_measured_at,
       disk_measure_ms = EXCLUDED.disk_measure_ms,
       disk_partial    = EXCLUDED.disk_partial,
       updated_at      = NOW()`

	qSaveBandwidth = `
INSERT INTO tenant_quota_usage
       (tenant_id, bandwidth_used_mb, bandwidth_period_start, bandwidth_measured_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
ON CONFLICT (tenant_id) DO UPDATE
   SET bandwidth_used_mb      = EXCLUDED.bandwidth_used_mb,
       bandwidth_period_start = EXCLUDED.bandwidth_period_start,
       bandwidth_measured_at  = EXCLUDED.bandwidth_measured_at,
       updated_at             = NOW()`

	packageColumns = `
       id, owner_tenant_id, name, slug, description,
       disk_mb, bandwidth_mb, max_websites, max_databases,
       max_mailboxes, max_subdomains, max_cron_jobs,
       features, over_quota_action, warn_percent,
       grace_percent::double precision, grace_floor_mb,
       is_active, created_at, updated_at`

	qListPackages = `SELECT ` + packageColumns + ` FROM hosting_packages ORDER BY name`

	qGetPackage = `SELECT ` + packageColumns + ` FROM hosting_packages WHERE id = $1`

	qGetPackageBySlug = `SELECT ` + packageColumns + ` FROM hosting_packages WHERE slug = $1`

	qCreatePackage = `
INSERT INTO hosting_packages
       (owner_tenant_id, name, slug, description,
        disk_mb, bandwidth_mb, max_websites, max_databases,
        max_mailboxes, max_subdomains, max_cron_jobs,
        features, over_quota_action, warn_percent, grace_percent, grace_floor_mb, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING id, created_at, updated_at`

	qUpdatePackage = `
UPDATE hosting_packages
   SET owner_tenant_id   = $1,
       name              = $2,
       slug              = $3,
       description       = $4,
       disk_mb           = $5,
       bandwidth_mb      = $6,
       max_websites      = $7,
       max_databases     = $8,
       max_mailboxes     = $9,
       max_subdomains    = $10,
       max_cron_jobs     = $11,
       features          = $12,
       over_quota_action = $13,
       warn_percent      = $14,
       grace_percent     = $15,
       grace_floor_mb    = $16,
       is_active         = $17,
       updated_at        = NOW()
 WHERE id = $18
RETURNING updated_at`

	qDeletePackage = `DELETE FROM hosting_packages WHERE id = $1`

	qAssignPackage = `
INSERT INTO tenant_packages (tenant_id, package_id, assigned_by)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id) DO UPDATE
   SET package_id  = EXCLUDED.package_id,
       assigned_by = EXCLUDED.assigned_by,
       assigned_at = NOW(),
       updated_at  = NOW()`

	qSetOverride = `
INSERT INTO tenant_quota_overrides (tenant_id, resource, limit_value, reason, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, resource) DO UPDATE
   SET limit_value = EXCLUDED.limit_value,
       reason      = EXCLUDED.reason,
       expires_at  = EXCLUDED.expires_at,
       created_by  = EXCLUDED.created_by,
       created_at  = NOW()`

	qDeleteOverride = `DELETE FROM tenant_quota_overrides WHERE tenant_id = $1 AND resource = $2`

	qSetFeatureOverride = `
INSERT INTO tenant_feature_overrides (tenant_id, feature, allowed, reason, expires_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, feature) DO UPDATE
   SET allowed    = EXCLUDED.allowed,
       reason     = EXCLUDED.reason,
       expires_at = EXCLUDED.expires_at,
       created_at = NOW()`

	qDeleteFeatureOverride = `DELETE FROM tenant_feature_overrides WHERE tenant_id = $1 AND feature = $2`

	qListEvents = `
SELECT id, tenant_id, resource, event, limit_value, usage_value, message, created_at
  FROM tenant_quota_events
 WHERE tenant_id = $1
 ORDER BY created_at DESC
 LIMIT $2`
)

// preparedStatements is every statement this store runs. The live-schema test
// PREPAREs each one; a column that does not exist fails there instead of at
// three in the morning on a customer's VPS.
var preparedStatements = []struct{ Name, SQL string }{
	{"assignment", qAssignment},
	{"overrides", qOverrides},
	{"feature_overrides", qFeatureOverrides},
	{"counts", qCounts},
	{"measured", qMeasured},
	{"insert_event", qInsertEvent},
	{"insert_event_throttled", qInsertEventThrottled},
	{"set_suspended", qSetSuspended},
	{"managed_tenants", qManagedTenants},
	{"site_roots", qSiteRoots},
	{"database_bytes", qDatabaseBytes},
	{"bandwidth_bytes", qBandwidthBytes},
	{"save_disk", qSaveDisk},
	{"save_bandwidth", qSaveBandwidth},
	{"list_packages", qListPackages},
	{"get_package", qGetPackage},
	{"get_package_by_slug", qGetPackageBySlug},
	{"create_package", qCreatePackage},
	{"update_package", qUpdatePackage},
	{"delete_package", qDeletePackage},
	{"assign_package", qAssignPackage},
	{"set_override", qSetOverride},
	{"delete_override", qDeleteOverride},
	{"set_feature_override", qSetFeatureOverride},
	{"delete_feature_override", qDeleteFeatureOverride},
	{"list_events", qListEvents},
}

// PreparedStatements exposes the statement list to the live-schema test in this
// package's external test binary.
func PreparedStatements() []struct{ Name, SQL string } { return preparedStatements }

// ErrPackageInUse is returned when a package cannot be deleted because accounts
// are still on it. The database enforces this with ON DELETE RESTRICT; deleting
// the package would otherwise silently un-limit every customer on it.
var ErrPackageInUse = errors.New("this package cannot be deleted while accounts are assigned to it")

// ErrPackageNotFound is returned when no package has the given id or slug.
var ErrPackageNotFound = errors.New("hosting package not found")

func (s *PostgresStore) Assignment(ctx context.Context, tenantID uuid.UUID) (*Assignment, error) {
	var (
		a        Assignment
		p        Package
		features []byte
		action   string
	)

	row := s.db.QueryRowContext(ctx, qAssignment, tenantID)
	err := row.Scan(
		&a.TenantID, &a.AssignedAt, &a.Suspended, &a.SuspendedAt,
		&a.SuspendedReason, &a.SuspendedAutomatically,
		&p.ID, &p.OwnerTenantID, &p.Name, &p.Slug, &p.Description,
		&p.Limits.DiskMB, &p.Limits.BandwidthMB, &p.Limits.Websites, &p.Limits.Databases,
		&p.Limits.Mailboxes, &p.Limits.Subdomains, &p.Limits.CronJobs,
		&features, &action, &p.WarnPercent,
		&p.GracePercent, &p.GraceFloorMB,
		&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// No package: unmanaged, not an error. See doc.go.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read hosting package assignment: %w", err)
	}

	p.OverQuotaAction = Action(action)
	p.Features = decodeFeatures(features)
	a.Package = p

	a.Overrides = map[Resource]*int64{}
	rows, err := s.db.QueryContext(ctx, qOverrides, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read quota overrides: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			res       string
			limit     *int64
			reason    string
			expiresAt *time.Time
			createdAt time.Time
		)
		if err := rows.Scan(&res, &limit, &reason, &expiresAt, &createdAt); err != nil {
			return nil, fmt.Errorf("read quota overrides: %w", err)
		}
		a.Overrides[Resource(res)] = limit
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read quota overrides: %w", err)
	}

	a.FeatureOverrides = map[string]bool{}
	frows, err := s.db.QueryContext(ctx, qFeatureOverrides, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read feature overrides: %w", err)
	}
	defer frows.Close()
	for frows.Next() {
		var (
			feature string
			allowed bool
		)
		if err := frows.Scan(&feature, &allowed); err != nil {
			return nil, fmt.Errorf("read feature overrides: %w", err)
		}
		a.FeatureOverrides[feature] = allowed
	}
	if err := frows.Err(); err != nil {
		return nil, fmt.Errorf("read feature overrides: %w", err)
	}

	return &a, nil
}

func (s *PostgresStore) Counts(ctx context.Context, tenantID uuid.UUID) (map[Resource]int64, error) {
	var websites, databases, mailboxes, subdomains, cronJobs int64
	err := s.db.QueryRowContext(ctx, qCounts, tenantID).
		Scan(&websites, &databases, &mailboxes, &subdomains, &cronJobs)
	if err != nil {
		return nil, fmt.Errorf("count quota resources: %w", err)
	}
	return map[Resource]int64{
		ResourceWebsites:   websites,
		ResourceDatabases:  databases,
		ResourceMailboxes:  mailboxes,
		ResourceSubdomains: subdomains,
		ResourceCronJobs:   cronJobs,
	}, nil
}

func (s *PostgresStore) MeasuredUsage(ctx context.Context, tenantID uuid.UUID) (Measured, error) {
	var m Measured
	err := s.db.QueryRowContext(ctx, qMeasured, tenantID).Scan(
		&m.DiskUsedMB, &m.DiskFileCount, &m.DiskMeasuredAt, &m.DiskMeasureMS, &m.DiskPartial,
		&m.BandwidthUsedMB, &m.BandwidthPeriodStart, &m.BandwidthMeasuredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Never sampled. Zero usage, and Present stays false so the caller can
		// tell "no data yet" from "measured at zero".
		return Measured{}, nil
	}
	if err != nil {
		return Measured{}, fmt.Errorf("read measured quota usage: %w", err)
	}
	m.Present = true
	return m, nil
}

func (s *PostgresStore) RecordEvent(ctx context.Context, ev Event) error {
	_, err := s.db.ExecContext(ctx, qInsertEvent,
		ev.TenantID, string(ev.Resource), ev.Kind, ev.LimitValue, ev.UsageValue, ev.Message)
	if err != nil {
		return fmt.Errorf("record quota event: %w", err)
	}
	return nil
}

func (s *PostgresStore) RecordEventThrottled(ctx context.Context, ev Event, within time.Duration) (bool, error) {
	res, err := s.db.ExecContext(ctx, qInsertEventThrottled,
		ev.TenantID, string(ev.Resource), ev.Kind, ev.LimitValue, ev.UsageValue, ev.Message,
		within.Seconds())
	if err != nil {
		return false, fmt.Errorf("record quota event: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, nil
	}
	return n > 0, nil
}

func (s *PostgresStore) SetSuspended(ctx context.Context, tenantID uuid.UUID, suspended bool, reason string, automatic bool) error {
	res, err := s.db.ExecContext(ctx, qSetSuspended, tenantID, suspended, reason, automatic)
	if err != nil {
		return fmt.Errorf("set account suspension: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("account %s has no hosting package, so it cannot be suspended", tenantID)
	}
	return nil
}

func (s *PostgresStore) ManagedTenants(ctx context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := s.db.SelectContext(ctx, &ids, qManagedTenants); err != nil {
		return nil, fmt.Errorf("list accounts with a hosting package: %w", err)
	}
	return ids, nil
}

func (s *PostgresStore) SiteRoots(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	var roots []string
	if err := s.db.SelectContext(ctx, &roots, qSiteRoots, tenantID); err != nil {
		return nil, fmt.Errorf("list site roots: %w", err)
	}
	return roots, nil
}

func (s *PostgresStore) DatabaseBytes(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var b int64
	if err := s.db.GetContext(ctx, &b, qDatabaseBytes, tenantID); err != nil {
		return 0, fmt.Errorf("sum database sizes: %w", err)
	}
	return b, nil
}

func (s *PostgresStore) BandwidthBytes(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (int64, error) {
	var b int64
	if err := s.db.GetContext(ctx, &b, qBandwidthBytes, tenantID, from, to); err != nil {
		return 0, fmt.Errorf("sum bandwidth: %w", err)
	}
	return b, nil
}

func (s *PostgresStore) SaveDiskUsage(ctx context.Context, tenantID uuid.UUID, sample DiskSample) error {
	_, err := s.db.ExecContext(ctx, qSaveDisk,
		tenantID, MBFromBytes(sample.UsedBytes), sample.FileCount,
		sample.Duration.Milliseconds(), sample.Partial)
	if err != nil {
		return fmt.Errorf("save disk usage: %w", err)
	}
	return nil
}

func (s *PostgresStore) SaveBandwidthUsage(ctx context.Context, tenantID uuid.UUID, usedMB int64, periodStart time.Time) error {
	_, err := s.db.ExecContext(ctx, qSaveBandwidth, tenantID, usedMB, periodStart)
	if err != nil {
		return fmt.Errorf("save bandwidth usage: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListPackages(ctx context.Context) ([]Package, error) {
	rows, err := s.db.QueryContext(ctx, qListPackages)
	if err != nil {
		return nil, fmt.Errorf("list hosting packages: %w", err)
	}
	defer rows.Close()

	out := []Package{}
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetPackage(ctx context.Context, id uuid.UUID) (*Package, error) {
	return s.getPackage(ctx, qGetPackage, id)
}

func (s *PostgresStore) GetPackageBySlug(ctx context.Context, slug string) (*Package, error) {
	return s.getPackage(ctx, qGetPackageBySlug, slug)
}

func (s *PostgresStore) getPackage(ctx context.Context, query string, arg any) (*Package, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("read hosting package: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("read hosting package: %w", err)
		}
		return nil, ErrPackageNotFound
	}
	return scanPackage(rows)
}

func (s *PostgresStore) CreatePackage(ctx context.Context, p *Package) error {
	features, err := encodeFeatures(p.Features)
	if err != nil {
		return err
	}
	err = s.db.QueryRowContext(ctx, qCreatePackage,
		p.OwnerTenantID, p.Name, p.Slug, p.Description,
		p.Limits.DiskMB, p.Limits.BandwidthMB, p.Limits.Websites, p.Limits.Databases,
		p.Limits.Mailboxes, p.Limits.Subdomains, p.Limits.CronJobs,
		features, string(p.OverQuotaAction), p.WarnPercent, p.GracePercent, p.GraceFloorMB, p.IsActive,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create hosting package: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdatePackage(ctx context.Context, p *Package) error {
	features, err := encodeFeatures(p.Features)
	if err != nil {
		return err
	}
	err = s.db.QueryRowContext(ctx, qUpdatePackage,
		p.OwnerTenantID, p.Name, p.Slug, p.Description,
		p.Limits.DiskMB, p.Limits.BandwidthMB, p.Limits.Websites, p.Limits.Databases,
		p.Limits.Mailboxes, p.Limits.Subdomains, p.Limits.CronJobs,
		features, string(p.OverQuotaAction), p.WarnPercent, p.GracePercent, p.GraceFloorMB, p.IsActive,
		p.ID,
	).Scan(&p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPackageNotFound
	}
	if err != nil {
		return fmt.Errorf("update hosting package: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeletePackage(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, qDeletePackage, id)
	if err != nil {
		// ON DELETE RESTRICT on tenant_packages.package_id. The message the
		// driver returns is a constraint name; this one is an answer.
		return ErrPackageInUse
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrPackageNotFound
	}
	return nil
}

func (s *PostgresStore) AssignPackage(ctx context.Context, tenantID, packageID uuid.UUID, assignedBy *uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, qAssignPackage, tenantID, packageID, assignedBy); err != nil {
		return fmt.Errorf("assign hosting package: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetOverride(ctx context.Context, tenantID uuid.UUID, r Resource, limit *int64, reason string, expiresAt *time.Time, createdBy *uuid.UUID) error {
	if !r.Valid() {
		return fmt.Errorf("unknown quota resource %q", r)
	}
	if _, err := s.db.ExecContext(ctx, qSetOverride, tenantID, string(r), limit, reason, expiresAt, createdBy); err != nil {
		return fmt.Errorf("set quota override: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteOverride(ctx context.Context, tenantID uuid.UUID, r Resource) error {
	if _, err := s.db.ExecContext(ctx, qDeleteOverride, tenantID, string(r)); err != nil {
		return fmt.Errorf("delete quota override: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]Override, error) {
	rows, err := s.db.QueryContext(ctx, qOverrides, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list quota overrides: %w", err)
	}
	defer rows.Close()

	out := []Override{}
	for rows.Next() {
		var (
			o   Override
			res string
		)
		if err := rows.Scan(&res, &o.LimitValue, &o.Reason, &o.ExpiresAt, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("list quota overrides: %w", err)
		}
		o.Resource = Resource(res)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SetFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string, allowed bool, reason string, expiresAt *time.Time) error {
	if _, err := s.db.ExecContext(ctx, qSetFeatureOverride, tenantID, feature, allowed, reason, expiresAt); err != nil {
		return fmt.Errorf("set feature override: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteFeatureOverride(ctx context.Context, tenantID uuid.UUID, feature string) error {
	if _, err := s.db.ExecContext(ctx, qDeleteFeatureOverride, tenantID, feature); err != nil {
		return fmt.Errorf("delete feature override: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, qListEvents, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list quota events: %w", err)
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var (
			ev  Event
			res string
		)
		if err := rows.Scan(&ev.ID, &ev.TenantID, &res, &ev.Kind,
			&ev.LimitValue, &ev.UsageValue, &ev.Message, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("list quota events: %w", err)
		}
		ev.Resource = Resource(res)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// scanRow is the shared shape of *sql.Rows and *sql.Row for scanPackage.
type scanRow interface{ Scan(dest ...any) error }

func scanPackage(row scanRow) (*Package, error) {
	var (
		p        Package
		features []byte
		action   string
	)
	err := row.Scan(
		&p.ID, &p.OwnerTenantID, &p.Name, &p.Slug, &p.Description,
		&p.Limits.DiskMB, &p.Limits.BandwidthMB, &p.Limits.Websites, &p.Limits.Databases,
		&p.Limits.Mailboxes, &p.Limits.Subdomains, &p.Limits.CronJobs,
		&features, &action, &p.WarnPercent,
		&p.GracePercent, &p.GraceFloorMB,
		&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("read hosting package: %w", err)
	}
	p.OverQuotaAction = Action(action)
	p.Features = decodeFeatures(features)
	return &p, nil
}

// decodeFeatures turns the JSONB column into a map. A column that cannot be
// parsed yields an empty map, which denies every feature: an unreadable feature
// set must not read as "everything included".
func decodeFeatures(raw []byte) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func encodeFeatures(m map[string]bool) ([]byte, error) {
	if m == nil {
		m = map[string]bool{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode package features: %w", err)
	}
	return b, nil
}
