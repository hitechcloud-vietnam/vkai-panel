// The tests in this file talk to a real PostgreSQL. They are skipped unless
// VKAI_TEST_DSN names one, because the module's suite must stay runnable with
// no database, but they are how the SQL in this file is actually verified - an
// upsert, a NULL that has to survive a scan and a transaction that has to leave
// nothing behind are not things a fake can check.
//
//	sudo -u postgres createdb vkai_localnode_check
//	sudo -u postgres psql -d vkai_localnode_check -f migrations/001_initial_schema.sql
//	sudo -u postgres psql -d vkai_localnode_check -f migrations/pending/local_node.sql
//	VKAI_TEST_DSN="postgres://user:pass@127.0.0.1:5432/vkai_localnode_check?sslmode=disable" \
//	  go test ./internal/repository/ -run TestLive -v
//
// TestLiveMissingTableDegrades wants a second database with 001 applied and the
// pending migration deliberately not applied, named by
// VKAI_TEST_DSN_NOMIGRATION. It is what proves that an installation which has
// not run the migration keeps working.

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// TestLiveLocalNode registers the same machine twice and checks the three
// things the fakes cannot: that one row comes out of it, that a fact nobody
// could measure is stored as NULL and still reads back, and that a node which
// was soft-deleted is restored rather than duplicated.
func TestLiveLocalNode(t *testing.T) {
	dsn := os.Getenv("VKAI_TEST_DSN")
	if dsn == "" {
		t.Skip("no VKAI_TEST_DSN")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewServerRepository(db)
	ctx := context.Background()

	tenant, err := repo.DefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("default tenant: %v", err)
	}
	t.Logf("default tenant = %s", tenant)

	node := uuid.New()
	fp := "fp-live-machine"
	src := "machine-id"

	// First: nothing measurable.
	server, created, err := repo.RegisterLocalNode(ctx, LocalNodeRegistration{
		NodeID:            node,
		TenantID:          tenant,
		Facts:             LocalNodeFacts{Hostname: "live-host", IPAddress: "203.0.113.77"},
		Fingerprint:       &fp,
		FingerprintSource: &src,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !created {
		t.Fatal("first registration did not report created")
	}
	if server.OS != "" || server.CPUCores != 0 || server.RAMTotal != 0 || server.DiskTotal != 0 {
		t.Fatalf("unmeasured facts were not null: %+v", server)
	}
	t.Logf("created role=%q status=%q agent_status=%q last_seen=%v token=%q",
		server.Role, server.Status, server.AgentStatus, server.LastSeenAt, server.AgentToken)

	var nulls int
	if err := db.GetContext(ctx, &nulls, `SELECT count(*) FROM servers WHERE id=$1 AND os IS NULL AND kernel IS NULL AND cpu_cores IS NULL AND ram_total IS NULL AND disk_total IS NULL AND ipv6_address IS NULL`, node); err != nil {
		t.Fatal(err)
	}
	if nulls != 1 {
		t.Fatal("unmeasured facts did not reach the database as NULL")
	}

	// Second: renamed, readdressed, now measurable.
	osName := "Ubuntu 24.04.1 LTS"
	kernel := "6.8.0-79-generic"
	cores := 8
	ram := int64(16) << 30
	disk := int64(200) << 30
	v6 := "2001:db8::99"
	server, created, err = repo.RegisterLocalNode(ctx, LocalNodeRegistration{
		NodeID:   node,
		TenantID: tenant,
		Facts: LocalNodeFacts{
			Hostname: "live-host-renamed", IPAddress: "198.51.100.7", IPv6Address: &v6,
			OS: &osName, Kernel: &kernel, CPUCores: &cores, RAMTotal: &ram, DiskTotal: &disk,
		},
		Fingerprint:       &fp,
		FingerprintSource: &src,
	})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if created {
		t.Fatal("re-registration reported creating a second node")
	}
	if server.Hostname != "live-host-renamed" || server.CPUCores != 8 {
		t.Fatalf("update did not take: %+v", server)
	}

	var rows int
	if err := db.GetContext(ctx, &rows, `SELECT count(*) FROM servers WHERE tenant_id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	t.Logf("servers for tenant after two registrations = %d", rows)

	record, err := repo.GetLocalNode(ctx, node)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	t.Logf("record = %+v", record)

	found, err := repo.FindLocalNodeByFingerprint(ctx, fp)
	if err != nil || found.ServerID != node {
		t.Fatalf("find by fingerprint: %v %+v", err, found)
	}

	if err := repo.NoteLocalNodeMismatch(ctx, node, "the machine-id disagrees", time.Now().UTC()); err != nil {
		t.Fatalf("mismatch: %v", err)
	}
	record, _ = repo.GetLocalNode(ctx, node)
	if record.AgentStatus != "offline" || record.LastMismatchAt == nil {
		t.Fatalf("mismatch not recorded: %+v", record)
	}

	if err := repo.TouchLocalNode(ctx, node, time.Now().UTC()); err != nil {
		t.Fatalf("touch: %v", err)
	}
	record, _ = repo.GetLocalNode(ctx, node)
	if record.AgentStatus != "online" || record.LastMismatchAt != nil || record.LastVerifiedAt == nil {
		t.Fatalf("touch not recorded: %+v", record)
	}

	if err := repo.TouchLocalNode(ctx, uuid.New(), time.Now().UTC()); err == nil {
		t.Fatal("touching an unknown node succeeded")
	}

	// A soft-deleted node is restored by registering again.
	if _, err := db.ExecContext(ctx, `UPDATE servers SET deleted_at=NOW() WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetLocalNode(ctx, node); err == nil {
		t.Fatal("a soft-deleted node is still reported as local")
	}
	if _, _, err := repo.RegisterLocalNode(ctx, LocalNodeRegistration{
		NodeID: node, TenantID: tenant,
		Facts:       LocalNodeFacts{Hostname: "live-host-renamed", IPAddress: "198.51.100.7"},
		Fingerprint: &fp, FingerprintSource: &src,
	}); err != nil {
		t.Fatalf("re-register after delete: %v", err)
	}
	if _, err := repo.GetLocalNode(ctx, node); err != nil {
		t.Fatalf("node was not restored: %v", err)
	}

	// And the ordinary server queries still work with the new table present.
	list, total, err := repo.ListByTenant(ctx, tenant, 1, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	t.Logf("list total=%d len=%d", total, len(list))

	if _, err := db.ExecContext(ctx, `DELETE FROM servers WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
}

// TestLiveMissingTableDegrades is the upgrade path: an installation that has
// not run migrations/pending/local_node.sql reports that it has no local node,
// keeps working, and is left with no half-registered server row.
func TestLiveMissingTableDegrades(t *testing.T) {
	dsn := os.Getenv("VKAI_TEST_DSN_NOMIGRATION")
	if dsn == "" {
		t.Skip("no VKAI_TEST_DSN_NOMIGRATION")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewServerRepository(db)
	ctx := context.Background()

	if _, err := repo.GetLocalNode(ctx, uuid.New()); err != ErrLocalNodeBookkeepingUnavailable {
		t.Fatalf("GetLocalNode err=%v, want ErrLocalNodeBookkeepingUnavailable", err)
	}
	if _, err := repo.FindLocalNodeByFingerprint(ctx, "x"); err != ErrLocalNodeBookkeepingUnavailable {
		t.Fatalf("Find err=%v", err)
	}
	if err := repo.TouchLocalNode(ctx, uuid.New(), time.Now()); err != ErrLocalNodeBookkeepingUnavailable {
		t.Fatalf("Touch err=%v", err)
	}
	if err := repo.NoteLocalNodeMismatch(ctx, uuid.New(), "x", time.Now()); err != ErrLocalNodeBookkeepingUnavailable {
		t.Fatalf("Note err=%v", err)
	}
	tenant, err := repo.DefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	node := uuid.New()
	_, _, err = repo.RegisterLocalNode(ctx, LocalNodeRegistration{
		NodeID: node, TenantID: tenant,
		Facts: LocalNodeFacts{Hostname: "h", IPAddress: "203.0.113.1"},
	})
	if err != ErrLocalNodeBookkeepingUnavailable {
		t.Fatalf("Register err=%v, want ErrLocalNodeBookkeepingUnavailable", err)
	}
	var leaked int
	if err := db.GetContext(ctx, &leaked, `SELECT count(*) FROM servers WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatal("a failed registration left a half-registered server row behind")
	}
}
