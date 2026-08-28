package handler

// The audit endpoints driven as HTTP, against a real PostgreSQL.
//
// Separate from audit_routes_test.go on purpose: that file asks whether the
// routes are mounted, this one asks whether they answer. Both questions have
// had "no" for an answer in this repository, for different reasons, and a test
// that only asked one of them would have missed the other.
//
// What this file caught: every handler in audit.go read the caller's tenant
// with c.MustGet("tenant_id").(uuid.UUID), and middleware.AuthRequired stores
// tenant_id as a STRING. The assertion could never succeed, so the audit
// endpoints panicked on every real request and gin's recovery middleware turned
// each one into a 500. A test that called the handler with its own context, or
// that only checked the route table, would not have seen it. Driving a request
// through the guard chain does.
//
//	VKAI_AUDIT_DSN='postgres://vkai:vkai@127.0.0.1:5432/vkai_audit_test?sslmode=disable' \
//	    go test ./internal/handler/ -run AuditHTTP -v

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

func openAuditHTTPDB(t *testing.T) *sqlx.DB {
	t.Helper()

	var dsn string
	for _, name := range []string{"VKAI_AUDIT_DSN", "VKAI_SCHEMA_DSN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		t.Skip("neither VKAI_AUDIT_DSN nor VKAI_SCHEMA_DSN is set; " +
			"these tests need a throwaway PostgreSQL with every migration applied")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// buildAuditEngine mounts the audit routes exactly as router.go will, behind a
// stand-in for AuthRequired that sets the same context values in the same
// shapes the real middleware sets them - tenant_id and user_id as STRINGS,
// claims as *auth.TokenClaims. Getting those shapes right is the whole point:
// a test that set tenant_id as a uuid.UUID would have passed while production
// panicked.
func buildAuditEngine(t *testing.T, db *sqlx.DB, tenantID uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	logger := zap.NewNop()
	repo := repository.NewAuditRepository(db)
	handler := NewAuditHandler(service.NewAuditService(repo, logger), logger)

	engine := gin.New()
	engine.Use(gin.Recovery())

	userID := uuid.New()
	v1 := engine.Group("/api/v1")
	v1.Use(func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Set("tenant_id", tenantID.String())
		c.Set("claims", &auth.TokenClaims{
			UserID:      userID,
			TenantID:    tenantID,
			Username:    "audit-http-test",
			RoleIDs:     []string{"super_admin"},
			Permissions: []string{"audit.read", "audit.write"},
		})
		c.Next()
	})

	// The line router.go needs, used here verbatim.
	RegisterAuditRoutes(v1, handler)

	// What router.go already mounts, so the two are exercised together.
	existing := v1.Group("/audit")
	existing.GET("/search", handler.Search)
	existing.GET("/stats", handler.GetStats)
	existing.GET("/:id", handler.Get)
	existing.POST("/cleanup", handler.CleanupOld)

	return engine
}

func seedAuditTenant(t *testing.T, db *sqlx.DB, entries int) (uuid.UUID, *repository.AuditRepository) {
	t.Helper()

	tenantID := uuid.New()
	slug := "audit-http-" + tenantID.String()[:8]
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status, plan, max_servers, max_websites)
		 VALUES ($1, $2, $3, 'active', 'enterprise', 1, 1)`, tenantID, slug, slug); err != nil {
		t.Fatalf("creating a tenant: %v", err)
	}

	repo := repository.NewAuditRepository(db)
	for i := 1; i <= entries; i++ {
		l := &models.AuditLog{
			TenantID:  tenantID,
			Action:    audit.ActionSignIn,
			Resource:  audit.ResourceSession,
			Details:   models.JSONMap{"n": i},
			IPAddress: "203.0.113.5",
			UserAgent: "audit-http-test/1.0",
			Status:    audit.StatusSuccess,
		}
		if err := repo.Create(context.Background(), l); err != nil {
			t.Fatalf("writing entry %d: %v", i, err)
		}
	}
	return tenantID, repo
}

func doAudit(t *testing.T, engine *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

// TestAuditHTTPEndpointsAnswer is the regression test for the tenant_id panic.
// Every route in audit.go, driven through the guard chain, must produce an
// answer rather than a 500 from the recovery middleware.
func TestAuditHTTPEndpointsAnswer(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, _ := seedAuditTenant(t, db, 6)
	engine := buildAuditEngine(t, db, tenantID)

	cases := []struct {
		method string
		path   string
		want   int
		why    string
	}{
		{"GET", "/api/v1/audit/search", 200, "the pre-existing search endpoint"},
		{"GET", "/api/v1/audit/stats", 200, "the pre-existing stats endpoint"},
		{"GET", "/api/v1/audit/chain/status", 200, "chain status"},
		{"GET", "/api/v1/audit/chain/verify", 200, "an intact chain verifies"},
		{"POST", "/api/v1/audit/chain/verify", 200, "the same over POST"},
		{"GET", "/api/v1/audit/chain/export", 200, "the auditor's bundle"},
		{"GET", "/api/v1/audit/chain/seals", 200, "the seal list"},
		{"GET", "/api/v1/audit/chain/retention", 200, "the retention plan"},
		{"GET", "/api/v1/audit/chain/procedure", 200, "the published procedure"},
		{"GET", "/api/v1/audit/chain/verifier", 200, "the reference verifier"},
		{"POST", "/api/v1/audit/cleanup", 409, "retention is refused, with the reason"},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			w := doAudit(t, engine, c.method, c.path)
			if w.Code == http.StatusInternalServerError {
				t.Fatalf("%s (%s) answered 500 - a panic in the handler chain: %s",
					c.path, c.why, w.Body.String())
			}
			if w.Code != c.want {
				t.Fatalf("%s (%s): got %d, want %d: %s", c.path, c.why, w.Code, c.want, w.Body.String())
			}
		})
	}
}

func TestAuditHTTPVerifyReports409OnATamperedLog(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, _ := seedAuditTenant(t, db, 8)
	engine := buildAuditEngine(t, db, tenantID)

	if w := doAudit(t, engine, "GET", "/api/v1/audit/chain/verify?full=true"); w.Code != 200 {
		t.Fatalf("an intact chain did not answer 200: %d %s", w.Code, w.Body.String())
	}

	// Tamper, with the guards down - see the repository tests for how much that
	// takes.
	var logID uuid.UUID
	if err := db.Get(&logID,
		`SELECT audit_log_id FROM audit_log_chain WHERE tenant_id = $1 AND seq = 5`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("GRANT UPDATE ON audit_logs TO CURRENT_USER"); err != nil {
		t.Skipf("this test needs ownership of audit_logs to simulate tampering: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE audit_logs DISABLE TRIGGER audit_logs_append_only"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("ALTER TABLE audit_logs ENABLE ALWAYS TRIGGER audit_logs_append_only")
		_, _ = db.Exec("REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs FROM CURRENT_USER")
	})
	if _, err := db.Exec(`UPDATE audit_logs SET status = 'failure' WHERE id = $1`, logID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE audit_logs ENABLE ALWAYS TRIGGER audit_logs_append_only"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs FROM CURRENT_USER"); err != nil {
		t.Fatal(err)
	}

	// A monitoring system that only reads status codes must not see this as
	// healthy, so a break is 409 and not 200-with-a-flag.
	w := doAudit(t, engine, "GET", "/api/v1/audit/chain/verify?full=true&deep=true")
	if w.Code != http.StatusConflict {
		t.Fatalf("a tampered log answered %d, want 409: %s", w.Code, w.Body.String())
	}

	var report struct {
		OK          bool   `json:"ok"`
		BreakSeq    int64  `json:"break_seq"`
		BreakReason string `json:"break_reason"`
		Note        string `json:"note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("could not read the report: %v (%s)", err, w.Body.String())
	}
	if report.OK {
		t.Fatal("the report says the log is fine")
	}
	if report.BreakSeq != 5 || report.BreakReason != "content_altered" {
		t.Fatalf("got seq %d %q, want 5 content_altered", report.BreakSeq, report.BreakReason)
	}
	if !strings.Contains(report.Note, "TAMPERED") {
		t.Fatalf("the note should say plainly what happened: %q", report.Note)
	}

	// The export of a range that does not verify comes back with the reason and
	// a 409, rather than being withheld - an investigator needs the evidence.
	w = doAudit(t, engine, "GET", "/api/v1/audit/chain/export")
	if w.Code != http.StatusConflict {
		t.Fatalf("exporting a tampered range answered %d, want 409", w.Code)
	}
}

func TestAuditHTTPExportIsVerifiableByTheReferenceVerifier(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, _ := seedAuditTenant(t, db, 12)
	engine := buildAuditEngine(t, db, tenantID)

	w := doAudit(t, engine, "GET", "/api/v1/audit/chain/export?note=quarterly")
	if w.Code != 200 {
		t.Fatalf("export answered %d: %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("the bundle should download, not render: Content-Disposition %q", cd)
	}

	var exp audit.Export
	if err := json.Unmarshal(w.Body.Bytes(), &exp); err != nil {
		t.Fatalf("the bundle is not readable JSON: %v", err)
	}
	if exp.Format != audit.ExportFormat || len(exp.Entries) != 12 {
		t.Fatalf("unexpected bundle: format %q, %d entries", exp.Format, len(exp.Entries))
	}
	if exp.Procedure == "" {
		t.Fatal("the bundle must point at the procedure it is meant to satisfy")
	}

	// The endpoint serves the specification and an implementation of it.
	if w := doAudit(t, engine, "GET", "/api/v1/audit/chain/procedure"); !strings.Contains(w.Body.String(), "F(s)") {
		t.Fatal("/audit/chain/procedure does not serve VERIFYING.md")
	}
	verifier := doAudit(t, engine, "GET", "/api/v1/audit/chain/verifier").Body.String()
	if !strings.Contains(verifier, "def entry_hash(") {
		t.Fatal("/audit/chain/verifier does not serve the reference implementation")
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed; the independent check cannot run here")
	}

	// Everything from here uses ONLY what the two endpoints served: the script
	// the panel handed out, run over the bundle the panel handed out. No part
	// of this package is in the loop.
	dir := t.TempDir()
	script := filepath.Join(dir, "verify_audit_export.py")
	bundle := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(script, []byte(verifier), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, w.Body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(python, script, bundle).CombinedOutput()
	if err != nil {
		t.Fatalf("the verifier the panel served rejected the bundle the panel served:\n%s", out)
	}
	if !strings.Contains(string(out), "INTACT: 12 entries") {
		t.Fatalf("unexpected verifier output:\n%s", out)
	}
}

func TestAuditHTTPRetentionRefusesAndExplains(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, _ := seedAuditTenant(t, db, 4)
	engine := buildAuditEngine(t, db, tenantID)

	w := doAudit(t, engine, "GET", "/api/v1/audit/chain/retention?days=30")
	if w.Code != 200 {
		t.Fatalf("retention answered %d: %s", w.Code, w.Body.String())
	}

	var plan struct {
		Refused     bool   `json:"refused"`
		Prunable    int64  `json:"prunable"`
		Command     string `json:"command"`
		Explanation string `json:"explanation"`
		Warning     string `json:"warning"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatalf("could not read the plan: %v (%s)", err, w.Body.String())
	}
	if !plan.Refused {
		t.Fatal("the panel must not claim it can prune the audit log")
	}
	if plan.Prunable != 0 {
		t.Fatalf("nothing is 30 days old yet, but the preview says %d", plan.Prunable)
	}
	for _, want := range []string{"audit_chain_prune(", "GRANT DELETE", "REVOKE DELETE", tenantID.String()} {
		if !strings.Contains(plan.Command, want) {
			t.Fatalf("the operator command does not contain %q:\n%s", want, plan.Command)
		}
	}
	if plan.Warning == "" || plan.Explanation == "" {
		t.Fatal("a refusal that does not say what is lost is not an answer")
	}

	// POST /audit/cleanup, the endpoint that used to delete, now refuses with
	// the same plan rather than reporting a cheerful "0 deleted".
	w = doAudit(t, engine, "POST", "/api/v1/audit/cleanup?days=30")
	if w.Code != http.StatusConflict {
		t.Fatalf("cleanup answered %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "append-only") {
		t.Fatalf("the refusal does not say why: %s", w.Body.String())
	}
}

// TestAuditHTTPRefusesWithoutATenant asserts the fail-closed path that replaced
// the panic: no tenant in the context is a 401, not a 500 and not a query that
// silently reads another tenant's rows.
func TestAuditHTTPRefusesWithoutATenant(t *testing.T) {
	db := openAuditHTTPDB(t)
	seedAuditTenant(t, db, 2)

	logger := zap.NewNop()
	handler := NewAuditHandler(service.NewAuditService(repository.NewAuditRepository(db), logger), logger)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	v1 := engine.Group("/api/v1")
	v1.Use(func(c *gin.Context) {
		// Authenticated enough for the permission gate, but with no tenant -
		// the shape a misconfigured or partially-migrated token produces.
		c.Set("claims", &auth.TokenClaims{RoleIDs: []string{"super_admin"}})
		c.Next()
	})
	RegisterAuditRoutes(v1, handler)

	for _, path := range []string{"/api/v1/audit/chain/status", "/api/v1/audit/chain/verify"} {
		w := doAudit(t, engine, "GET", path)
		if w.Code == http.StatusInternalServerError {
			t.Fatalf("%s panicked without a tenant", path)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s answered %d without a tenant, want 401: %s", path, w.Code, w.Body.String())
		}
	}

}

// TestAuditHTTPFullPassSealsACheckpoint drives the whole checkpoint mechanism
// through HTTP, which is the only way it will ever be exercised in production.
//
// The point of a checkpoint is that audit_chain_head is mutable and a seal is
// not, so a clean full pass leaves behind evidence that the chain reached a
// sequence number - evidence a later truncation cannot talk its way out of.
func TestAuditHTTPFullPassSealsACheckpoint(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, _ := seedAuditTenant(t, db, 5)
	engine := buildAuditEngine(t, db, tenantID)

	// An incremental pass must NOT seal: it has not re-read the prefix and has
	// no business vouching for it.
	w := doAudit(t, engine, "GET", "/api/v1/audit/chain/verify?deep=true")
	if w.Code != 200 {
		t.Fatalf("verify answered %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"checkpoint"`) {
		t.Fatal("an incremental pass sealed a checkpoint; it has not verified the prefix")
	}

	w = doAudit(t, engine, "GET", "/api/v1/audit/chain/verify?full=true&deep=true")
	if w.Code != 200 {
		t.Fatalf("a full pass answered %d: %s", w.Code, w.Body.String())
	}

	var report struct {
		OK         bool `json:"ok"`
		Checkpoint *struct {
			Kind      string `json:"kind"`
			Seq       int64  `json:"seq"`
			EntryHash string `json:"entry_hash"`
		} `json:"checkpoint"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("could not read the report: %v", err)
	}
	if report.Checkpoint == nil {
		t.Fatal("a clean full pass left no checkpoint; a later truncation would be deniable")
	}
	if report.Checkpoint.Kind != "checkpoint" || report.Checkpoint.Seq != 5 {
		t.Fatalf("unexpected checkpoint: %+v", report.Checkpoint)
	}

	// The seal must name the entry the pass actually verified.
	var hashAt5 string
	if err := db.Get(&hashAt5,
		`SELECT entry_hash FROM audit_log_chain WHERE tenant_id = $1 AND seq = 5`, tenantID); err != nil {
		t.Fatal(err)
	}
	if report.Checkpoint.EntryHash != hashAt5 {
		t.Fatalf("the checkpoint names %s; entry 5 is %s", report.Checkpoint.EntryHash, hashAt5)
	}

	// A second full pass with nothing new must not add another seal: a row per
	// poll is noise, not evidence.
	w = doAudit(t, engine, "GET", "/api/v1/audit/chain/verify?full=true&deep=true")
	if strings.Contains(w.Body.String(), `"checkpoint"`) {
		t.Fatal("a full pass over an unchanged chain sealed a second checkpoint")
	}

	var seals int
	if err := db.Get(&seals,
		`SELECT count(*) FROM audit_chain_seal WHERE tenant_id = $1 AND kind = 'checkpoint'`,
		tenantID); err != nil {
		t.Fatal(err)
	}
	if seals != 1 {
		t.Fatalf("%d checkpoint seals after two passes over the same chain, want 1", seals)
	}

	// And the seal shows up where an operator and an auditor both look.
	if w := doAudit(t, engine, "GET", "/api/v1/audit/chain/seals"); !strings.Contains(w.Body.String(), "checkpoint") {
		t.Fatalf("the seal list does not show the checkpoint: %s", w.Body.String())
	}
	w = doAudit(t, engine, "GET", "/api/v1/audit/chain/export")
	if !strings.Contains(w.Body.String(), `"kind": "checkpoint"`) &&
		!strings.Contains(w.Body.String(), `"kind":"checkpoint"`) {
		t.Fatal("the exported bundle does not carry the checkpoint seal")
	}
}

// TestPermissionChangesAreAuditedIntoTheChain.
//
// A permission change is the action that makes every other action possible, so
// a trail that records what an account did but not how it came to be allowed to
// answers only half the question an investigation asks.
//
// This drives the real MultiUserHandler over a real database and then verifies
// the chain, rather than asserting that a capturing fake was called - which
// would prove the handler calls something, not that the entry is in the log an
// auditor reads.
func TestPermissionChangesAreAuditedIntoTheChain(t *testing.T) {
	db := openAuditHTTPDB(t)
	tenantID, auditRepo := seedAuditTenant(t, db, 0)

	logger := zap.NewNop()
	auditService := service.NewAuditService(auditRepo, logger)

	multiUser := NewMultiUserHandler(
		service.NewMultiUserService(repository.NewMultiUserRepository(db, logger), logger), logger)
	multiUser.SetAudit(auditService)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// The actor has to be a real user: audit_logs.user_id has a foreign key to
	// users(id), so an entry naming somebody who does not exist is refused by
	// the database and dropped (loudly, at error level, by AuditService.Record
	// - an audit write must never fail the action it is recording). In
	// production the actor is always a real user; in a test it is easy to
	// forget, and the symptom is an empty trail rather than an error.
	actor := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, username, email, password_hash, status)
		VALUES ($1, $2, $3, $4, 'x', 'active')`,
		actor, tenantID, "actor-"+actor.String()[:8], actor.String()[:8]+"@example.test"); err != nil {
		t.Fatalf("creating the acting user: %v", err)
	}

	v1 := engine.Group("/api/v1")
	v1.Use(func(c *gin.Context) {
		c.Set("user_id", actor.String())
		c.Set("tenant_id", tenantID.String())
		c.Set("claims", &auth.TokenClaims{
			UserID: actor, TenantID: tenantID, RoleIDs: []string{"super_admin"},
		})
		c.Next()
	})
	v1.POST("/users/:id/roles", multiUser.AssignUserRole)
	v1.DELETE("/users/:id/roles/:roleId", multiUser.RemoveUserRole)
	v1.POST("/roles", multiUser.CreateRole)

	target := uuid.New()
	roleID := uuid.New()

	// The grant is refused - there is no such user - which is precisely the
	// case worth recording: an attempt to widen somebody's privileges that did
	// not work is still an attempt, and a trail that only holds successes is a
	// trail that hides the reconnaissance.
	body := strings.NewReader(`{"role_id":"` + roleID.String() + `"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/users/"+target.String()+"/roles", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "permission-test/1.0")
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError && strings.Contains(w.Body.String(), "panic") {
		t.Fatalf("the handler panicked: %s", w.Body.String())
	}

	type row struct {
		Action    string `db:"action"`
		Resource  string `db:"resource"`
		Status    string `db:"status"`
		UserAgent string `db:"user_agent"`
	}
	var rows []row
	if err := db.Select(&rows,
		`SELECT action, resource, status, COALESCE(user_agent, '') AS user_agent
		   FROM audit_logs WHERE tenant_id = $1 ORDER BY created_at`, tenantID); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("a role assignment left %d audit entries, want 1: %+v", len(rows), rows)
	}
	if rows[0].Action != audit.ActionRoleAssigned || rows[0].Resource != audit.ResourceUser {
		t.Fatalf("unexpected entry: %+v", rows[0])
	}
	if rows[0].UserAgent != "permission-test/1.0" {
		t.Fatalf("the entry did not capture the caller's user agent: %q", rows[0].UserAgent)
	}

	// The actor, the target and the role are all recoverable - "who granted
	// what to whom" is the whole question.
	var details struct {
		RoleID       string
		TargetUserID string
	}
	if err := db.QueryRow(
		`SELECT details->>'role_id', details->>'target_user_id'
		   FROM audit_logs WHERE tenant_id = $1`, tenantID).
		Scan(&details.RoleID, &details.TargetUserID); err != nil {
		t.Fatal(err)
	}
	if details.RoleID != roleID.String() || details.TargetUserID != target.String() {
		t.Fatalf("the entry does not say who was granted what: %+v", details)
	}

	var actorID string
	if err := db.Get(&actorID,
		`SELECT user_id::text FROM audit_logs WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if actorID != actor.String() {
		t.Fatalf("the entry names %s as the actor, want %s", actorID, actor)
	}

	// Chained, like everything else.
	v, err := auditRepo.Verify(context.Background(), tenantID, 1, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.Checked != 1 {
		t.Fatalf("the permission-change entry is not in the chain: ok=%v checked=%d", v.OK, v.Checked)
	}
}
