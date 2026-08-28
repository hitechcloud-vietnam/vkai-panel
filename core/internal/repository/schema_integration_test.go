package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// These tests drive the repositories that this change repaired against a real
// PostgreSQL, because that is the only thing that proves the repair.
//
// Preparing a statement proves its columns exist. It does not prove sqlx can
// map the result into the struct: a row carrying a column the struct does not
// declare fails at scan time with "missing destination name", which is a
// different failure and was the second half of the same bug. Running the real
// methods and reading the rows back covers both.
//
// Set VKAI_SCHEMA_DSN to a database with every migration applied:
//
//	VKAI_SCHEMA_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	    go test ./internal/repository/ -run TestRepositoriesAgainstLiveSchema -v

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live database tests")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open VKAI_SCHEMA_DSN: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping VKAI_SCHEMA_DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedTenant creates a tenant and two servers, and removes them (and everything
// that cascades off them) when the test finishes.
func seedTenant(t *testing.T, db *sqlx.DB) (tenantID, serverA, serverB uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	err := db.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		"schema-test-"+suffix, "schema-test-"+suffix,
	).Scan(&tenantID)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		// Best effort: the rows are tenant scoped and every table that hangs
		// off a tenant cascades, but a role without DELETE on one of them
		// leaves the tenant behind. Nothing in these tests depends on the
		// cleanup succeeding, only on the rows being tenant scoped.
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM jobs WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(bg, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	for _, target := range []*uuid.UUID{&serverA, &serverB} {
		err = db.QueryRowContext(ctx,
			`INSERT INTO servers (tenant_id, hostname, ip_address, agent_token)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			tenantID, "host-"+uuid.New().String()[:8], "203.0.113.10", uuid.New().String(),
		).Scan(target)
		if err != nil {
			t.Fatalf("seed server: %v", err)
		}
	}
	return tenantID, serverA, serverB
}

func TestWAFRepositoryAgainstLiveSchema(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tenantID, serverID, _ := seedTenant(t, db)
	repo := repository.NewWAFRepository(db)

	// A rule: create, read back, toggle, then soft delete.
	rule := &models.WAFRule{
		TenantID: tenantID, Name: "block-sqli", Description: "test rule",
		RuleType: "sql_injection", Severity: "high", Action: "block",
		Pattern: "union.*select", Enabled: true,
	}
	if err := repo.CreateRule(ctx, rule); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if _, err := repo.GetRule(ctx, rule.ID); err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if err := repo.ToggleRule(ctx, rule.ID, false); err != nil {
		t.Fatalf("ToggleRule: %v", err)
	}
	rule.Enabled = false
	if err := repo.UpdateRule(ctx, rule); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	rules, err := repo.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("ListRules returned %d rules, want 1", len(rules))
	}
	if err := repo.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	rules, err = repo.ListRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRules after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("a soft deleted rule is still listed: %d rules remain", len(rules))
	}
	if _, err := repo.GetRule(ctx, rule.ID); err != sql.ErrNoRows {
		t.Fatalf("GetRule on a deleted rule returned %v, want sql.ErrNoRows", err)
	}

	// A policy: same soft delete path.
	policy := &models.WAFPolicy{
		TenantID: tenantID, Name: "strict", Description: "test policy",
		Mode: "prevention", ParanoiaLevel: 2, AnomalyThreshold: 5, Enabled: true,
	}
	if err := repo.CreatePolicy(ctx, policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if err := repo.UpdatePolicy(ctx, policy); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}
	if err := repo.DeletePolicy(ctx, policy.ID); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	policies, err := repo.ListPolicies(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("a soft deleted policy is still listed: %d remain", len(policies))
	}

	// An event carrying the context the schema was missing.
	var websiteID uuid.UUID
	err = db.QueryRowContext(ctx,
		`INSERT INTO websites (tenant_id, server_id, domain, web_server_type)
		 VALUES ($1, $2, $3, 'nginx') RETURNING id`,
		tenantID, serverID, "waf-"+uuid.New().String()[:8]+".example",
	).Scan(&websiteID)
	if err != nil {
		t.Fatalf("seed website: %v", err)
	}

	event := &models.WAFEvent{
		TenantID: tenantID, WebsiteID: &websiteID, SourceIP: "198.51.100.7",
		Method: "POST", Path: "/login", UserAgent: "curl/8", AttackType: "sql_injection",
		Severity: "critical", Action: "block", Details: "matched union select",
		Blocked: true,
	}
	if err := repo.CreateEvent(ctx, event); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	events, err := repo.ListEvents(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListEvents returned %d events, want 1", len(events))
	}
	got := events[0]
	if got.WebsiteID == nil || *got.WebsiteID != websiteID {
		t.Errorf("website_id was not persisted: got %v, want %v", got.WebsiteID, websiteID)
	}
	if got.Severity != "critical" || got.Action != "block" || got.Details != "matched union select" {
		t.Errorf("event context was not persisted: severity=%q action=%q details=%q",
			got.Severity, got.Action, got.Details)
	}

	stats, err := repo.GetStats(ctx, tenantID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalRequests != 1 || stats.BlockedRequests != 1 {
		t.Errorf("GetStats: total=%d blocked=%d, want 1 and 1",
			stats.TotalRequests, stats.BlockedRequests)
	}
	if len(stats.RecentEvents) != 1 {
		t.Errorf("GetStats returned %d recent events, want 1", len(stats.RecentEvents))
	}
}

func TestClusterRepositoryAgainstLiveSchema(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tenantID, serverA, serverB := seedTenant(t, db)
	repo := repository.NewClusterRepository(db)

	cluster := &models.Cluster{
		TenantID: tenantID, Name: "web-tier", Description: "test cluster",
		Type: "active-passive", Status: "active", Config: models.JSONMap{"k": "v"},
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("Create cluster: %v", err)
	}
	if _, err := repo.GetByID(ctx, tenantID, cluster.ID); err != nil {
		t.Fatalf("GetByID cluster: %v", err)
	}
	if _, err := repo.List(ctx, tenantID); err != nil {
		t.Fatalf("List clusters: %v", err)
	}
	newName := "web-tier-renamed"
	if _, err := repo.Update(ctx, tenantID, cluster.ID, &models.UpdateClusterRequest{Name: &newName}); err != nil {
		t.Fatalf("Update cluster: %v", err)
	}

	// A node is inserted without an ip_address, so reading it back exercises
	// the NULL that used to be scanned into a plain string.
	node := &models.ClusterNode{
		ClusterID: cluster.ID, ServerID: serverA, Role: "master",
		Status: "active", Weight: 100, Metadata: models.JSONMap{},
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if node.Port != 8080 {
		t.Errorf("AddNode did not read back the port default: got %d, want 8080", node.Port)
	}
	fetched, err := repo.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if fetched.IPAddress != nil {
		t.Errorf("ip_address should be NULL for a node added without one, got %q", *fetched.IPAddress)
	}
	if _, err := repo.ListNodes(ctx, cluster.ID); err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	role := "worker"
	if _, err := repo.UpdateNode(ctx, node.ID, &models.UpdateClusterNodeRequest{Role: &role}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if err := repo.UpdateNodeHeartbeat(ctx, node.ID); err != nil {
		t.Fatalf("UpdateNodeHeartbeat: %v", err)
	}

	lb := &models.LoadBalancer{
		TenantID: tenantID, ClusterID: &cluster.ID, Name: "edge", Type: "nginx",
		Algorithm: "round-robin", Status: "active", ListenPort: 80, SSLPort: 443,
		Config: models.JSONMap{},
	}
	if err := repo.CreateLoadBalancer(ctx, lb); err != nil {
		t.Fatalf("CreateLoadBalancer: %v", err)
	}
	if lb.HealthCheck == nil {
		t.Error("CreateLoadBalancer did not read back the health_check default")
	}
	if _, err := repo.GetLoadBalancerByID(ctx, tenantID, lb.ID); err != nil {
		t.Fatalf("GetLoadBalancerByID: %v", err)
	}
	if _, err := repo.ListLoadBalancers(ctx, tenantID, &cluster.ID); err != nil {
		t.Fatalf("ListLoadBalancers by cluster: %v", err)
	}
	if _, err := repo.ListLoadBalancers(ctx, tenantID, nil); err != nil {
		t.Fatalf("ListLoadBalancers: %v", err)
	}
	inactive := "inactive"
	updated, err := repo.UpdateLoadBalancer(ctx, tenantID, lb.ID, &models.UpdateLoadBalancerRequest{Status: &inactive})
	if err != nil {
		t.Fatalf("UpdateLoadBalancer: %v", err)
	}
	if updated.Status != "inactive" {
		t.Errorf("UpdateLoadBalancer: status is %q, want inactive", updated.Status)
	}

	ha := &models.HAPair{
		TenantID: tenantID, Name: "db-pair",
		PrimaryServerID: serverA, SecondaryServerID: serverB,
		VirtualIP: "203.0.113.99", Status: "active", FailoverMode: "automatic",
		Config: models.JSONMap{},
	}
	if err := repo.CreateHAPair(ctx, ha); err != nil {
		t.Fatalf("CreateHAPair: %v", err)
	}
	if _, err := repo.ListHAPairs(ctx, tenantID); err != nil {
		t.Fatalf("ListHAPairs: %v", err)
	}
	if err := repo.TriggerFailover(ctx, tenantID, ha.ID); err != nil {
		t.Fatalf("TriggerFailover: %v", err)
	}
	after, err := repo.GetHAPairByID(ctx, tenantID, ha.ID)
	if err != nil {
		t.Fatalf("GetHAPairByID after failover: %v", err)
	}
	if after.PrimaryServerID != serverB || after.SecondaryServerID != serverA {
		t.Errorf("failover did not swap the servers: primary=%v secondary=%v", after.PrimaryServerID, after.SecondaryServerID)
	}
	if after.LastFailover == nil {
		t.Error("failover did not stamp last_failover")
	}

	if err := repo.DeleteHAPair(ctx, tenantID, ha.ID); err != nil {
		t.Fatalf("DeleteHAPair: %v", err)
	}
	if err := repo.DeleteLoadBalancer(ctx, tenantID, lb.ID); err != nil {
		t.Fatalf("DeleteLoadBalancer: %v", err)
	}
	if err := repo.RemoveNode(ctx, node.ID); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}
	if err := repo.Delete(ctx, tenantID, cluster.ID); err != nil {
		t.Fatalf("Delete cluster: %v", err)
	}
}

func TestJobTaskIDIsPersisted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tenantID, serverID, _ := seedTenant(t, db)
	repo := repository.NewJobRepository(db)

	record := &job.JobRecord{
		ID: uuid.New(), TaskType: "backup", Status: "pending", Queue: "default",
		MaxRetries: 3, TenantID: tenantID, ServerID: &serverID,
		Payload: []byte(`{"server_id":"x"}`),
	}
	if err := repo.CreateJob(ctx, record); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// The row is written before the queue hands back a task id, so it starts
	// empty. This is the state cancel and retry used to be stuck in forever.
	stored, err := repo.GetJob(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if stored.TaskID != "" {
		t.Fatalf("a new job row should have an empty task_id, got %q", stored.TaskID)
	}

	// Unique per run: task_id carries a partial unique index, so a row left
	// behind by an earlier run must not be able to collide with this one.
	taskID := "asynq:task:" + uuid.New().String()
	if err := repo.UpdateJobTaskID(ctx, record.ID, taskID); err != nil {
		t.Fatalf("UpdateJobTaskID: %v", err)
	}
	stored, err = repo.GetJob(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetJob after UpdateJobTaskID: %v", err)
	}
	if stored.TaskID != taskID {
		t.Fatalf("task_id was not persisted: got %q, want %q", stored.TaskID, taskID)
	}

	// The id has to be findable, because that is the whole point of storing it.
	byTask, err := repo.GetJobByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetJobByTaskID: %v", err)
	}
	if byTask.ID != record.ID {
		t.Fatalf("GetJobByTaskID returned job %v, want %v", byTask.ID, record.ID)
	}

	// UpdateJobStatus must not disturb it: it is the method that used to be
	// asked to save the task id and silently did not.
	if err := repo.UpdateJobStatus(ctx, record.ID, "active", nil, ""); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}
	stored, err = repo.GetJob(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetJob after UpdateJobStatus: %v", err)
	}
	if stored.TaskID != taskID {
		t.Fatalf("UpdateJobStatus cleared task_id: got %q", stored.TaskID)
	}

	if err := repo.DeleteJob(ctx, record.ID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
}
