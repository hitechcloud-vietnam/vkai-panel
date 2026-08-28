// This file drives the quota store against a real PostgreSQL. It is skipped
// unless VKAI_SCHEMA_DSN (or VKAI_TEST_DSN) names a database with every
// migration applied, including migrations/pending/packages.sql:
//
//	VKAI_SCHEMA_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	    go test ./internal/quota/ -run Live -v
//
// Preparing every statement proves its columns exist. That is the check the
// jobs table never had: CREATE TABLE IF NOT EXISTS hid a table that did not
// have the shape the code queried, and every /jobs endpoint failed at runtime
// on every install.

package quota_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
)

func liveDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		dsn = os.Getenv("VKAI_TEST_DSN")
	}
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live database tests")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func liveTenant(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	suffix := uuid.New().String()[:8]
	err := db.QueryRowContext(context.Background(),
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		"quota-test-"+suffix, "quota-test-"+suffix).Scan(&id)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_quota_events WHERE tenant_id = $1`, id)
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_quota_usage WHERE tenant_id = $1`, id)
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_quota_overrides WHERE tenant_id = $1`, id)
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_feature_overrides WHERE tenant_id = $1`, id)
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_packages WHERE tenant_id = $1`, id)
		_, _ = db.ExecContext(bg, `DELETE FROM tenants WHERE id = $1`, id)
	})
	return id
}

func livePackage(t *testing.T, db *sqlx.DB, st quota.Store, p quota.Package) *quota.Package {
	t.Helper()
	if p.Slug == "" {
		p.Slug = "test-" + uuid.New().String()[:8]
	}
	if p.Name == "" {
		p.Name = p.Slug
	}
	if p.OverQuotaAction == "" {
		p.OverQuotaAction = quota.ActionRefuse
	}
	if p.WarnPercent == 0 {
		p.WarnPercent = 90
	}
	if p.GraceFloorMB == 0 {
		p.GraceFloorMB = 16
	}
	if p.GracePercent == 0 {
		p.GracePercent = 2
	}
	p.IsActive = true
	if err := st.CreatePackage(context.Background(), &p); err != nil {
		t.Fatalf("create package: %v", err)
	}
	t.Cleanup(func() {
		// The assignments have to go first: tenant_packages.package_id is ON
		// DELETE RESTRICT, which is the point of the column and also the reason
		// a naive cleanup leaves test packages behind.
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_packages WHERE package_id = $1`, p.ID)
		_ = st.DeletePackage(bg, p.ID)
	})
	return &p
}

// TestLiveEveryStatementPrepares is the cheapest proof that the schema and the
// code agree. Every statement the store runs is prepared against the real
// database; a column that does not exist fails here.
func TestLiveEveryStatementPrepares(t *testing.T) {
	db := liveDB(t)
	ctx := context.Background()

	for _, s := range quota.PreparedStatements() {
		stmt, err := db.PrepareContext(ctx, s.SQL)
		if err != nil {
			t.Errorf("PREPARE %s failed: %v\n%s", s.Name, err, strings.TrimSpace(s.SQL))
			continue
		}
		_ = stmt.Close()
	}
}

func TestLivePackageLifecycleAndAssignment(t *testing.T) {
	db := liveDB(t)
	st := quota.NewPostgresStore(db)
	ctx := context.Background()

	tenantID := liveTenant(t, db)
	pkg := livePackage(t, db, st, quota.Package{
		Name:   "Starter 10G",
		Limits: quota.Limits{DiskMB: ptr(10240), BandwidthMB: ptr(20480), Websites: ptr(5), Databases: ptr(3)},
		Features: map[string]bool{
			quota.FeatureCron: true, quota.FeatureSSH: false,
		},
	})

	if err := st.AssignPackage(ctx, tenantID, pkg.ID, nil); err != nil {
		t.Fatalf("assign: %v", err)
	}

	a, err := st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	if a == nil {
		t.Fatal("the account has a package but the store reported none")
	}
	if got := a.Package.Limits.Websites; got == nil || *got != 5 {
		t.Fatalf("website limit came back as %v", got)
	}
	if a.Package.GracePercent != 2 {
		t.Fatalf("grace_percent came back as %v; NUMERIC has to be cast, not scanned raw", a.Package.GracePercent)
	}
	if allowed, _ := a.FeatureAllowed(quota.FeatureCron); !allowed {
		t.Fatal("a feature the package includes came back as not allowed")
	}
	if allowed, _ := a.FeatureAllowed(quota.FeatureDocker); allowed {
		t.Fatal("a feature the package never mentions came back as allowed")
	}

	// A package somebody is on must not be deletable: deleting it would
	// silently un-limit every account assigned to it.
	if err := st.DeletePackage(ctx, pkg.ID); err == nil {
		t.Fatal("a package with accounts on it was deleted")
	}
}

func TestLiveOverridesBeatThePackageAndExpire(t *testing.T) {
	db := liveDB(t)
	st := quota.NewPostgresStore(db)
	ctx := context.Background()

	tenantID := liveTenant(t, db)
	pkg := livePackage(t, db, st, quota.Package{Limits: quota.Limits{Websites: ptr(2)}})
	if err := st.AssignPackage(ctx, tenantID, pkg.ID, nil); err != nil {
		t.Fatal(err)
	}

	if err := st.SetOverride(ctx, tenantID, quota.ResourceWebsites, ptr(9), "negotiated", nil, nil); err != nil {
		t.Fatalf("set override: %v", err)
	}
	a, err := st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	limit, fromOverride := a.Effective(quota.ResourceWebsites)
	if limit == nil || *limit != 9 || !fromOverride {
		t.Fatalf("the override did not beat the package: limit=%v override=%v", limit, fromOverride)
	}

	// An expired override is ignored but not deleted, so the record of what was
	// granted survives the grant.
	past := time.Now().Add(-time.Hour)
	if err := st.SetOverride(ctx, tenantID, quota.ResourceWebsites, ptr(9), "expired", &past, nil); err != nil {
		t.Fatal(err)
	}
	a, err = st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	limit, fromOverride = a.Effective(quota.ResourceWebsites)
	if fromOverride || limit == nil || *limit != 2 {
		t.Fatalf("an expired override was still in force: limit=%v override=%v", limit, fromOverride)
	}
	var rows int
	if err := db.GetContext(ctx, &rows,
		`SELECT COUNT(*) FROM tenant_quota_overrides WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("the expired override row was deleted rather than ignored (%d rows)", rows)
	}

	// An override to NULL means unlimited for this account.
	if err := st.SetOverride(ctx, tenantID, quota.ResourceWebsites, nil, "unlimited", nil, nil); err != nil {
		t.Fatal(err)
	}
	a, err = st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if limit, fromOverride = a.Effective(quota.ResourceWebsites); limit != nil || !fromOverride {
		t.Fatalf("an override to unlimited did not take effect: limit=%v override=%v", limit, fromOverride)
	}
}

func TestLiveEventThrottleStopsAFlood(t *testing.T) {
	db := liveDB(t)
	st := quota.NewPostgresStore(db)
	ctx := context.Background()
	tenantID := liveTenant(t, db)

	ev := quota.Event{
		TenantID: tenantID, Resource: quota.ResourceWebsites,
		Kind: quota.EventRefuse, UsageValue: 5, Message: "at the limit",
	}
	wrote, err := st.RecordEventThrottled(ctx, ev, time.Hour)
	if err != nil || !wrote {
		t.Fatalf("first event not written: wrote=%v err=%v", wrote, err)
	}
	for i := 0; i < 20; i++ {
		if wrote, err = st.RecordEventThrottled(ctx, ev, time.Hour); err != nil {
			t.Fatal(err)
		}
		if wrote {
			t.Fatal("a retrying client was able to write a second event inside the window")
		}
	}

	events, err := st.ListEvents(ctx, tenantID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one recorded event, got %d", len(events))
	}
	if events[0].Resource != quota.ResourceWebsites || events[0].Kind != quota.EventRefuse {
		t.Fatalf("the event came back wrong: %+v", events[0])
	}
}

func TestLiveMeasuredUsageRoundTrip(t *testing.T) {
	db := liveDB(t)
	st := quota.NewPostgresStore(db)
	ctx := context.Background()
	tenantID := liveTenant(t, db)

	m, err := st.MeasuredUsage(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Present {
		t.Fatal("an account that was never sampled reported a sample")
	}

	sample := quota.DiskSample{
		UsedBytes: 3*quota.BytesPerMB + 1, // must round up to 4
		FileCount: 1234,
		Duration:  1500 * time.Millisecond,
		Partial:   true,
	}
	if err := st.SaveDiskUsage(ctx, tenantID, sample); err != nil {
		t.Fatal(err)
	}
	from, _ := quota.MonthWindow(time.Now())
	if err := st.SaveBandwidthUsage(ctx, tenantID, 77, from); err != nil {
		t.Fatal(err)
	}

	m, err = st.MeasuredUsage(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Present {
		t.Fatal("the sample did not come back")
	}
	if m.DiskUsedMB != 4 {
		t.Fatalf("disk usage came back as %d MB; bytes must round up", m.DiskUsedMB)
	}
	if m.DiskFileCount != 1234 || m.DiskMeasureMS != 1500 || !m.DiskPartial {
		t.Fatalf("the cost columns did not round trip: %+v", m)
	}
	if m.BandwidthUsedMB != 77 {
		t.Fatalf("bandwidth came back as %d", m.BandwidthUsedMB)
	}
	if m.BandwidthPeriodStart.UTC().Format("2006-01-02") != from.Format("2006-01-02") {
		t.Fatalf("the bandwidth period came back as %s, want %s", m.BandwidthPeriodStart, from)
	}
}

func TestLiveSuspensionIsAFlagAndIsReversible(t *testing.T) {
	db := liveDB(t)
	st := quota.NewPostgresStore(db)
	ctx := context.Background()

	tenantID := liveTenant(t, db)
	pkg := livePackage(t, db, st, quota.Package{})
	if err := st.AssignPackage(ctx, tenantID, pkg.ID, nil); err != nil {
		t.Fatal(err)
	}

	if err := st.SetSuspended(ctx, tenantID, true, "unpaid invoice", false); err != nil {
		t.Fatal(err)
	}
	a, err := st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Suspended || a.SuspendedAt == nil || a.SuspendedReason != "unpaid invoice" || a.SuspendedAutomatically {
		t.Fatalf("the suspension was not recorded as an operator's: %+v", a)
	}

	if err := st.SetSuspended(ctx, tenantID, false, "", false); err != nil {
		t.Fatal(err)
	}
	a, err = st.Assignment(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Suspended || a.SuspendedAt != nil {
		t.Fatalf("the suspension was not reversed: %+v", a)
	}

	// An account with no package cannot be suspended, and says so rather than
	// silently doing nothing.
	orphan := liveTenant(t, db)
	if err := st.SetSuspended(ctx, orphan, true, "x", false); err == nil {
		t.Fatal("suspending an account with no package was silently accepted")
	}
}
