// This file drives the whole local-node path against a real PostgreSQL and a
// real machine: the identity file is written into a temporary directory, the
// facts are measured off the host running the tests, and the rows go into a
// live database. It is skipped unless VKAI_TEST_DSN names one; see
// internal/repository/local_node_live_test.go for how to prepare it.

package service

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/localnode"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// TestLiveEndToEndLocalNode is the product claim end to end: installing the
// panel makes the machine it is installed on a managed node, that node is
// visible through the ordinary server queries, and a database moved to another
// machine loses the short path rather than keeping it.
func TestLiveEndToEndLocalNode(t *testing.T) {
	dsn := os.Getenv("VKAI_TEST_DSN")
	if dsn == "" {
		t.Skip("no VKAI_TEST_DSN")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	etc := t.TempDir()
	repo := repository.NewServerRepository(db)
	svc := &ServerService{
		serverRepo: repo,
		logger:     zap.NewNop(),
		localNodes: repo,
		probe:      &localnode.SystemProbe{EtcRoot: etc},
	}
	ctx := context.Background()

	first, err := svc.RegisterLocalNode(ctx, LocalNodeOptions{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Logf("node=%s created=%v verified=%v witness=%q", first.NodeID, first.Created, first.Verified, first.WitnessSource)
	t.Logf("server hostname=%q ip=%q ipv6=%q os=%q kernel=%q cores=%d ram=%d disk=%d role=%q status=%q agent=%q",
		first.Server.Hostname, first.Server.IPAddress, first.Server.IPv6Address, first.Server.OS,
		first.Server.Kernel, first.Server.CPUCores, first.Server.RAMTotal, first.Server.DiskTotal,
		first.Server.Role, first.Server.Status, first.Server.AgentStatus)

	second, err := svc.RegisterLocalNode(ctx, LocalNodeOptions{})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if second.Created || second.NodeID != first.NodeID {
		t.Fatalf("second registration made a new node: %+v", second)
	}

	route := svc.ResolveNodeRoute(ctx, first.NodeID)
	t.Logf("route=%+v", route)
	if !route.Local {
		t.Fatalf("the machine that registered the node is not offered the short path: %+v", route)
	}

	status, err := svc.LocalNodeStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status=%+v", status)
	if !status.Registered || !status.Local || !status.Healthy {
		t.Fatalf("status=%+v, want a registered healthy local node", status)
	}

	if err := svc.HeartbeatLocalNode(ctx); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	// The node is readable through the ordinary server queries.
	fetched, err := svc.GetByID(ctx, first.Server.TenantID, first.NodeID)
	if err != nil {
		t.Fatalf("GetByID on the local node: %v", err)
	}
	list, total, err := svc.ListByTenant(ctx, first.Server.TenantID, 1, 20)
	if err != nil {
		t.Fatalf("ListByTenant: %v", err)
	}
	t.Logf("fetched %s, list total=%d len=%d", fetched.Hostname, total, len(list))

	// A database moved to another machine: node.json is gone and the machine
	// witness of this host does not match the row.
	if err := os.Remove(localnode.IdentityPath(etc)); err != nil {
		t.Fatal(err)
	}
	elsewhere := &ServerService{
		serverRepo: repo, logger: zap.NewNop(), localNodes: repo,
		probe: &fakeProbe{
			identity: &localnode.Identity{NodeID: first.NodeID, Fingerprint: "some-other-machine", FingerprintSource: "machine-id"},
			witness:  localnode.MachineWitness{Fingerprint: "this-machine", Source: "machine-id"},
		},
	}
	route = elsewhere.ResolveNodeRoute(ctx, first.NodeID)
	if route.Local || !route.Mismatch {
		t.Fatalf("a restored database was offered the local short path: %+v", route)
	}
	record, err := repo.GetLocalNode(ctx, first.NodeID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if record.LastMismatchAt == nil || record.AgentStatus != "offline" {
		t.Fatalf("the mismatch was not recorded: %+v", record)
	}
	t.Logf("recorded mismatch: %s", record.LastMismatchReason)

	if _, err := db.ExecContext(ctx, `DELETE FROM servers WHERE id=$1`, first.NodeID); err != nil {
		t.Fatal(err)
	}
}
