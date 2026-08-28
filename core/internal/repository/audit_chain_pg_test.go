package repository

// Integration tests for the tamper-evident audit chain, against a real
// PostgreSQL 16 with every migration applied - including
// migrations/pending/audit_chain.sql.
//
// These do not run against a mock and cannot: everything worth testing here is
// enforced by the database. The append-only guard is a privilege revocation and
// a trigger; the hashes are computed in PL/pgSQL by an AFTER INSERT trigger;
// the verification pass is a SQL function. A test that stubbed any of that out
// would prove the stub works.
//
//	createdb vkai_audit_test
//	psql -d vkai_audit_test -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"'
//	for f in migrations/*.sql migrations/pending/audit_chain.sql; do psql -1 -d vkai_audit_test -f "$f"; done
//	VKAI_TEST_POSTGRES_DSN='postgres://vkai:vkai@127.0.0.1:5432/vkai_audit_test?sslmode=disable' go test ./internal/repository/ -run Chain -v
//
// The DSN must name a THROWAWAY database. These tests write audit entries that
// they then cannot delete, which is the whole point of the feature.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// chainDSNEnvs are the environment variables that can name the test database,
// in order of preference.
var chainDSNEnvs = []string{"VKAI_AUDIT_DSN", "VKAI_SCHEMA_DSN"}

// openChainTestDB connects, or skips with an instruction rather than a shrug.
func openChainTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	var dsn, from string
	for _, name := range chainDSNEnvs {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			dsn, from = v, name
			break
		}
	}
	if dsn == "" {
		t.Skipf("neither %s is set; see the comment at the top of this file for the "+
			"three commands that stand up a throwaway PostgreSQL for them",
			strings.Join(chainDSNEnvs, " nor "))
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connecting to %s: %v", from, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var present bool
	if err := db.Get(&present,
		`SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'audit_chain_verify')`); err != nil {
		t.Fatalf("checking the schema: %v", err)
	}
	if !present {
		t.Fatalf("audit_chain_verify() is missing: apply migrations/pending/audit_chain.sql to the test database")
	}

	return db
}

// newTestTenant gives each test its own chain. Tenants are never removed: the
// audit rows they own cannot be deleted, which is exactly what is being tested.
func newTestTenant(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status, plan, max_servers, max_websites)
		 VALUES ($1, $2, $3, 'active', 'enterprise', 1, 1)`,
		id, "chain-test-"+id.String()[:8], "chain-test-"+id.String()[:8])
	if err != nil {
		t.Fatalf("creating a test tenant: %v", err)
	}
	return id
}

// writeEntries appends n entries through the repository, exactly as the panel
// does. The chain is built by the database trigger, not by this code.
func writeEntries(t *testing.T, repo *AuditRepository, tenantID uuid.UUID, n int) []models.AuditLog {
	t.Helper()

	ctx := context.Background()
	written := make([]models.AuditLog, 0, n)
	for i := 1; i <= n; i++ {
		l := &models.AuditLog{
			TenantID:  tenantID,
			Action:    "act." + strconv.Itoa(i),
			Resource:  audit.ResourceSession,
			Details:   models.JSONMap{"n": i, "note": "entry " + strconv.Itoa(i)},
			IPAddress: "203.0.113." + strconv.Itoa(i%250+1),
			UserAgent: "chain-test/1.0",
			Status:    audit.StatusSuccess,
		}
		if err := repo.Create(ctx, l); err != nil {
			t.Fatalf("writing entry %d: %v", i, err)
		}
		written = append(written, *l)
	}
	return written
}

// asAttacker runs fn with the append-only guards down.
//
// It is the only way to produce a tampered log to test against, and how much
// work it takes is itself the finding: the privileges have to be granted back
// AND the trigger disabled, both of which need ownership of the table. Nothing
// the panel's own SQL surface can reach gets here.
func asAttacker(t *testing.T, db *sqlx.DB, fn func()) {
	t.Helper()

	tables := []string{"audit_logs", "audit_log_chain", "audit_chain_seal"}

	for _, table := range tables {
		if _, err := db.Exec(fmt.Sprintf(
			"GRANT UPDATE, DELETE ON %s TO CURRENT_USER", table)); err != nil {
			t.Fatalf("the test needs ownership of %s to simulate tampering: %v", table, err)
		}
		if _, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s DISABLE TRIGGER %s_append_only", table, table)); err != nil {
			t.Fatalf("disabling the guard on %s: %v", table, err)
		}
	}

	restore := func() {
		for _, table := range tables {
			_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s ENABLE ALWAYS TRIGGER %s_append_only", table, table))
			_, _ = db.Exec(fmt.Sprintf("REVOKE UPDATE, DELETE, TRUNCATE ON %s FROM CURRENT_USER", table))
		}
	}
	t.Cleanup(restore)

	fn()
	restore()
}

func verify(t *testing.T, repo *AuditRepository, tenantID uuid.UUID, deep bool) *ChainVerification {
	t.Helper()

	v, err := repo.Verify(context.Background(), tenantID, 1, 0, deep)
	if err != nil {
		t.Fatalf("running the verification pass: %v", err)
	}
	return v
}

func breakReason(v *ChainVerification) string {
	if v.BreakReason == nil {
		return ""
	}
	return *v.BreakReason
}

func breakSeq(v *ChainVerification) int64 {
	if v.BreakSeq == nil {
		return 0
	}
	return *v.BreakSeq
}

// ------------------------------------------------------------
// 1. An intact log verifies, and the database's hashes are the ones this
//    codebase's format says they should be.
// ------------------------------------------------------------

func TestChainAppendsAndVerifies(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	written := writeEntries(t, repo, tenantID, 20)

	v := verify(t, repo, tenantID, true)
	if !v.OK {
		t.Fatalf("an intact chain did not verify: seq %d, reason %q", breakSeq(v), breakReason(v))
	}
	if v.Checked != 20 {
		t.Fatalf("checked %d entries, wrote 20", v.Checked)
	}
	if !v.HeadOK {
		t.Fatal("the chain head does not match the last entry")
	}

	// Every entry is linked, in order, exactly once.
	seq, hash, err := repo.ChainEntryFor(context.Background(), written[0].ID)
	if err != nil {
		t.Fatalf("the first entry has no chain link: %v", err)
	}
	if seq != 1 {
		t.Fatalf("the first entry is at seq %d, want 1", seq)
	}
	if len(hash) != 64 {
		t.Fatalf("entry hash is %d characters, want 64", len(hash))
	}
}

// TestChainHashesMatchTheGoImplementation is the cross-check that keeps the
// PL/pgSQL and the Go halves of the format honest.
//
// The database computed these hashes. Go recomputes them from the same strings
// the database will hand an auditor. If the two ever drift, an export verifies
// against one implementation and not the other, and nobody can tell which is
// lying.
func TestChainHashesMatchTheGoImplementation(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 12)

	exp, err := repo.ExportRange(context.Background(), tenantID, 1, 0, 0)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if len(exp.Entries) != 12 {
		t.Fatalf("exported %d entries, wrote 12", len(exp.Entries))
	}

	prev := audit.GenesisHash
	for _, e := range exp.Entries {
		if got := e.Content.ContentHash(); got != e.ContentHash {
			t.Fatalf("seq %d: PostgreSQL computed content_hash %s, Go computes %s from the same fields",
				e.Seq, e.ContentHash, got)
		}
		if got := audit.EntryHash(prev, e.Content.TenantID, e.Seq, e.ContentHash); got != e.EntryHash {
			t.Fatalf("seq %d: PostgreSQL computed entry_hash %s, Go computes %s",
				e.Seq, e.EntryHash, got)
		}
		if e.PrevHash != prev {
			t.Fatalf("seq %d: prev_hash %s does not name the previous entry %s", e.Seq, e.PrevHash, prev)
		}
		prev = e.EntryHash
	}
}

// TestChainsAreIndependentPerTenant matters more than it looks.
//
// One chain shared across tenants would mean pruning one customer's history
// breaks every other customer's chain, and a tenant reading a verification
// result would learn how many entries their neighbours had written. The
// sequence number is per tenant, the head is per tenant, and interleaved writes
// must not braid the two chains together.
func TestChainsAreIndependentPerTenant(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	first := newTestTenant(t, db)
	second := newTestTenant(t, db)
	ctx := context.Background()

	// Interleaved, so a shared counter would show up immediately.
	for i := 1; i <= 6; i++ {
		for _, tenantID := range []uuid.UUID{first, second} {
			l := &models.AuditLog{
				TenantID: tenantID, Action: "act." + strconv.Itoa(i),
				Resource: audit.ResourceSession, Status: audit.StatusSuccess,
			}
			if err := repo.Create(ctx, l); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, tenantID := range []uuid.UUID{first, second} {
		v := verify(t, repo, tenantID, true)
		if !v.OK {
			t.Fatalf("tenant %s did not verify: %q at %d", tenantID, breakReason(v), breakSeq(v))
		}
		if v.Checked != 6 {
			t.Fatalf("tenant %s has %d entries, wrote 6", tenantID, v.Checked)
		}
		if v.FirstSeq == nil || *v.FirstSeq != 1 || v.LastSeq == nil || *v.LastSeq != 6 {
			t.Fatalf("tenant %s numbered its chain %v..%v, want 1..6", tenantID, v.FirstSeq, v.LastSeq)
		}
	}

	// And the two chains share no hash: the tenant id is inside both hashes, so
	// identical content in two tenants cannot produce identical links.
	var shared int
	if err := db.Get(&shared, `
		SELECT count(*) FROM audit_log_chain a
		  JOIN audit_log_chain b ON a.entry_hash = b.entry_hash
		 WHERE a.tenant_id = $1 AND b.tenant_id = $2`, first, second); err != nil {
		t.Fatal(err)
	}
	if shared != 0 {
		t.Fatalf("%d entry hashes are shared between two tenants", shared)
	}
}

// ------------------------------------------------------------
// 2. Append-only, enforced by the database.
// ------------------------------------------------------------

func TestChainAppendOnlyIsEnforcedByTheDatabase(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 3)

	// Layer 1: the privilege. The panel's own role has UPDATE, DELETE and
	// TRUNCATE revoked on all three tables.
	refused := []struct {
		what string
		stmt string
	}{
		{"UPDATE audit_logs", `UPDATE audit_logs SET action = 'rewritten' WHERE tenant_id = $1`},
		{"DELETE audit_logs", `DELETE FROM audit_logs WHERE tenant_id = $1`},
		{"UPDATE audit_log_chain", `UPDATE audit_log_chain SET entry_hash = repeat('0', 64) WHERE tenant_id = $1`},
		{"DELETE audit_log_chain", `DELETE FROM audit_log_chain WHERE tenant_id = $1`},
		{"DELETE audit_chain_seal", `DELETE FROM audit_chain_seal WHERE tenant_id = $1`},
	}
	for _, c := range refused {
		if _, err := db.Exec(c.stmt, tenantID); err == nil {
			t.Fatalf("%s succeeded; the audit log is not append-only", c.what)
		} else if !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("%s failed for the wrong reason: %v", c.what, err)
		}
	}

	for _, table := range []string{"audit_logs", "audit_log_chain", "audit_chain_seal"} {
		if _, err := db.Exec("TRUNCATE " + table); err == nil {
			t.Fatalf("TRUNCATE %s succeeded", table)
		}
	}

	// Layer 2: the trigger. The table owner can grant the privilege back to
	// itself in one statement, so the privilege alone is not the guard. With it
	// granted, the trigger still refuses.
	for _, table := range []string{"audit_logs", "audit_log_chain"} {
		if _, err := db.Exec(fmt.Sprintf("GRANT UPDATE, DELETE, TRUNCATE ON %s TO CURRENT_USER", table)); err != nil {
			t.Fatalf("re-granting on %s: %v", table, err)
		}
	}
	t.Cleanup(func() {
		for _, table := range []string{"audit_logs", "audit_log_chain"} {
			_, _ = db.Exec(fmt.Sprintf("REVOKE UPDATE, DELETE, TRUNCATE ON %s FROM CURRENT_USER", table))
		}
	})

	for _, c := range refused[:4] {
		_, err := db.Exec(c.stmt, tenantID)
		if err == nil {
			t.Fatalf("%s succeeded once the privilege was granted back; the trigger is not holding", c.what)
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s: expected the append-only trigger to refuse it, got: %v", c.what, err)
		}
	}

	// The one documented exemption is not a boolean an attacker can set: it
	// names a seal that must already cover the entry being removed.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("SET LOCAL vkai.audit_prune = 'on'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("DELETE FROM audit_log_chain WHERE tenant_id = $1", tenantID); err == nil {
		t.Fatal("setting the prune flag to an arbitrary value was enough to delete chain entries")
	}

	// And the log still verifies after all of that.
	if v := verify(t, repo, tenantID, true); !v.OK {
		t.Fatalf("the chain broke while nothing was allowed to change it: %q at %d",
			breakReason(v), breakSeq(v))
	}
}

// ------------------------------------------------------------
// 3-6. Detection.
// ------------------------------------------------------------

func TestChainDetectsAlteredEntry(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	written := writeEntries(t, repo, tenantID, 10)
	target := written[6] // seq 7

	asAttacker(t, db, func() {
		if _, err := db.Exec(
			`UPDATE audit_logs SET action = 'act.innocent', status = 'success' WHERE id = $1`,
			target.ID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("an edited entry verified as intact")
	}
	if breakReason(v) != "content_altered" {
		t.Fatalf("reason %q, want content_altered", breakReason(v))
	}
	if breakSeq(v) != 7 {
		t.Fatalf("break at seq %d, the edit is at 7", breakSeq(v))
	}
	if v.BreakAt == nil {
		t.Fatal("the break must carry the timestamp of the entry")
	}
	if v.BreakLogID == nil || *v.BreakLogID != target.ID {
		t.Fatalf("the break must name the entry: got %v, want %s", v.BreakLogID, target.ID)
	}

	// The structural pass alone cannot see this, and says so by passing. That
	// is the documented cost of the cheap mode, asserted rather than assumed.
	shallow, err := repo.Verify(context.Background(), tenantID, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !shallow.OK {
		t.Fatal("the structural pass reported a break it cannot actually detect; " +
			"if this now works, the documentation of deep vs shallow is wrong")
	}
}

func TestChainDetectsDeletedEntry(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	written := writeEntries(t, repo, tenantID, 10)

	// (a) the careless attacker removes the row and leaves the link.
	asAttacker(t, db, func() {
		if _, err := db.Exec(`DELETE FROM audit_logs WHERE id = $1`, written[3].ID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("a removed entry verified as intact")
	}
	if breakReason(v) != "entry_missing" || breakSeq(v) != 4 {
		t.Fatalf("got %q at seq %d, want entry_missing at 4", breakReason(v), breakSeq(v))
	}

	// (b) the thorough attacker removes the link too. The sequence number is
	// still gone, and nothing can put it back.
	asAttacker(t, db, func() {
		if _, err := db.Exec(`DELETE FROM audit_log_chain WHERE tenant_id = $1 AND seq = 4`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v = verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("a removed entry with its link removed verified as intact")
	}
	if breakReason(v) != "sequence_gap" || breakSeq(v) != 5 {
		t.Fatalf("got %q at seq %d, want sequence_gap at 5", breakReason(v), breakSeq(v))
	}

	// The cheap pass catches this one, which is the point of having it.
	shallow, err := repo.Verify(context.Background(), tenantID, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if shallow.OK {
		t.Fatal("the structural pass missed a sequence gap")
	}
}

func TestChainDetectsReorderedEntries(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 10)

	// Swap entries 4 and 5 whole - link, contents and all - so the log reads in
	// a different order. entry_hash binds the sequence number, so it cannot.
	asAttacker(t, db, func() {
		// Park both rows out of the way, then bring them back swapped. Two
		// statements rather than a temporary table, because sqlx hands out
		// pooled connections and a temporary table would not survive the trip.
		if _, err := db.Exec(
			`UPDATE audit_log_chain SET seq = seq + 1000000 WHERE tenant_id = $1 AND seq IN (4, 5)`,
			tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
		if _, err := db.Exec(`
			UPDATE audit_log_chain SET seq = CASE seq WHEN 1000004 THEN 5 ELSE 4 END
			 WHERE tenant_id = $1 AND seq IN (1000004, 1000005)`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("a reordered chain verified as intact")
	}
	if breakReason(v) != "chain_broken" || breakSeq(v) != 4 {
		t.Fatalf("got %q at seq %d, want chain_broken at 4", breakReason(v), breakSeq(v))
	}
}

func TestChainDetectsTruncatedTail(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 10)

	// Lop the three newest entries off both tables. What remains is a perfect
	// chain; only the head knows there should be more.
	asAttacker(t, db, func() {
		if _, err := db.Exec(`
			DELETE FROM audit_logs WHERE id IN (
				SELECT audit_log_id FROM audit_log_chain WHERE tenant_id = $1 AND seq > 7)`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM audit_log_chain WHERE tenant_id = $1 AND seq > 7`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("a chain with its newest entries removed verified as intact")
	}
	if breakReason(v) != "truncated_tail" {
		t.Fatalf("reason %q, want truncated_tail", breakReason(v))
	}
	if breakSeq(v) != 8 {
		t.Fatalf("break at seq %d, the first missing entry is 8", breakSeq(v))
	}
	if v.HeadOK {
		t.Fatal("head_ok must be false when the tail is gone")
	}
}

// ------------------------------------------------------------
// 7. Export, verified by something that is not this codebase.
// ------------------------------------------------------------

func TestChainExportVerifiesWithTheReferenceVerifier(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 15)

	exp, err := repo.ExportRange(context.Background(), tenantID, 1, 0, 0)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}

	res, err := audit.VerifyExport(exp)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("a freshly exported bundle did not verify: %+v", res.Break)
	}
	if exp.Anchor.Kind != "genesis" {
		t.Fatalf("a bundle starting at seq 1 must anchor on genesis, got %q", exp.Anchor.Kind)
	}

	// A checkpoint seal travels with the bundle, and an auditor holding it can
	// catch what the chain alone cannot. Seal the tip, re-export, and confirm
	// the seal is in there and agrees.
	headSeq, headHash, err := repo.HeadOf(context.Background(), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordCheckpoint(context.Background(), tenantID, headSeq, headHash, "test"); err != nil {
		t.Fatal(err)
	}
	exp, err = repo.ExportRange(context.Background(), tenantID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Seals) == 0 {
		t.Fatal("the bundle carries no seals; an auditor has nothing to check the panel against")
	}
	found := false
	for _, seal := range exp.Seals {
		if seal.Kind == "checkpoint" && seal.Seq == headSeq && seal.EntryHash == headHash {
			found = true
		}
	}
	if !found {
		t.Fatalf("the checkpoint seal is not in the bundle: %+v", exp.Seals)
	}
	if res, err := audit.VerifyExport(exp); err != nil || !res.OK {
		t.Fatalf("the bundle stopped verifying once the seal was in it: %v %+v", err, res)
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed; the independent check cannot run here")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "verify_audit_export.py")
	if err := os.WriteFile(script, []byte(audit.ReferenceVerifier()), 0o600); err != nil {
		t.Fatal(err)
	}

	bundle := filepath.Join(dir, "bundle.json")
	raw, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(python, script, bundle).CombinedOutput()
	if err != nil {
		t.Fatalf("the published verifier rejected a bundle this panel produced:\n%s", out)
	}
	if !strings.Contains(string(out), "INTACT: 15 entries") {
		t.Fatalf("unexpected verifier output: %s", out)
	}

	// And it must reject a doctored bundle, or it proves nothing.
	exp.Entries[9].Content.Action = "act.innocent"
	raw, _ = json.MarshalIndent(exp, "", "  ")
	doctored := filepath.Join(dir, "doctored.json")
	if err := os.WriteFile(doctored, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(python, script, doctored).CombinedOutput()
	if err == nil {
		t.Fatalf("the published verifier accepted a doctored bundle:\n%s", out)
	}
	if !strings.Contains(string(out), "BROKEN at seq 10") {
		t.Fatalf("the verifier did not name the doctored entry:\n%s", out)
	}
}

// ------------------------------------------------------------
// 8. Retention: pruning, and what it costs.
// ------------------------------------------------------------

func TestChainPruneSealsTheCutAndKeepsTheRestVerifiable(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	// Four entries dated a year ago, three recent. created_at is written
	// explicitly, so the chain hashes the timestamp the row actually has.
	old := time.Now().AddDate(0, 0, -400)
	for i := 1; i <= 4; i++ {
		if _, err := db.Exec(`
			INSERT INTO audit_logs (tenant_id, action, resource, status, created_at)
			VALUES ($1, $2, 'session', 'success', $3)`,
			tenantID, "old."+strconv.Itoa(i), old.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	writeEntries(t, repo, tenantID, 3)

	if v := verify(t, repo, tenantID, true); !v.OK {
		t.Fatalf("the fixture chain is broken before anything was pruned: %q", breakReason(v))
	}

	cutoff := time.Now().AddDate(0, 0, -365)

	preview, err := repo.PrunePreview(ctx, tenantID, cutoff)
	if err != nil {
		t.Fatalf("previewing the prune: %v", err)
	}
	if preview.Prunable != 4 {
		t.Fatalf("preview says %d entries are prunable, 4 are older than the cutoff", preview.Prunable)
	}

	// The panel cannot do it. That is the feature.
	if _, err := db.Exec(`SELECT * FROM audit_chain_prune($1, $2, 'test')`, tenantID, cutoff); err == nil {
		t.Fatal("the panel's own role pruned the audit log; DELETE has not been revoked")
	} else if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the prune failed for the wrong reason: %v", err)
	}

	// The documented operator ritual, run as an operator would.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("GRANT DELETE ON audit_logs, audit_log_chain TO CURRENT_USER"); err != nil {
		t.Fatal(err)
	}

	var pruned, firstSeq, sealSeq int64
	var sealHash string
	var sealID uuid.UUID
	if err := tx.QueryRow(
		`SELECT pruned, first_seq, seal_seq, seal_hash, seal_id FROM audit_chain_prune($1, $2, $3)`,
		tenantID, cutoff, "annual retention policy",
	).Scan(&pruned, &firstSeq, &sealSeq, &sealHash, &sealID); err != nil {
		t.Fatalf("the prune failed: %v", err)
	}
	if _, err := tx.Exec("REVOKE DELETE ON audit_logs, audit_log_chain FROM CURRENT_USER"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if pruned != 4 || sealSeq != 4 {
		t.Fatalf("pruned %d entries up to seq %d, want 4 up to 4", pruned, sealSeq)
	}

	// The surviving chain still verifies, anchored on the seal rather than on
	// genesis. That is the whole reason the seal exists.
	v := verify(t, repo, tenantID, true)
	if !v.OK {
		t.Fatalf("the chain stopped verifying after a legitimate prune: %q at %d",
			breakReason(v), breakSeq(v))
	}
	if v.FirstSeq == nil || *v.FirstSeq != 5 {
		t.Fatalf("the surviving chain should start at seq 5, got %v", v.FirstSeq)
	}

	// The seal is permanent and says what was removed.
	seals, err := repo.Seals(ctx, tenantID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(seals) != 1 {
		t.Fatalf("expected exactly one seal, got %d", len(seals))
	}
	if seals[0].Kind != "prune" || seals[0].EntryCount != 4 || seals[0].EntryHash != sealHash {
		t.Fatalf("the seal does not describe the prune: %+v", seals[0])
	}

	// The prune is itself in the log, and cannot be pruned away without
	// leaving another seal.
	var pruneAction string
	if err := db.Get(&pruneAction,
		`SELECT action FROM audit_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 1`,
		tenantID); err != nil {
		t.Fatal(err)
	}
	if pruneAction != audit.ActionPruned {
		t.Fatalf("the newest entry is %q; the prune must be recorded as %q", pruneAction, audit.ActionPruned)
	}

	// An export of the surviving range anchors on the seal and verifies.
	exp, err := repo.ExportRange(ctx, tenantID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Anchor.Kind != "seal" {
		t.Fatalf("the export should anchor on the seal, got %q", exp.Anchor.Kind)
	}
	if exp.Anchor.SealID != sealID.String() {
		t.Fatalf("the export names seal %s, the prune recorded %s", exp.Anchor.SealID, sealID)
	}
	res, err := audit.VerifyExport(exp)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("the post-prune export did not verify: %+v", res.Break)
	}
}

// ------------------------------------------------------------
// 9. Cost at scale.
// ------------------------------------------------------------

// TestChainVerificationAtScale measures the verification pass, so the claim
// about what this costs on a large log is a measurement rather than a guess.
//
// It runs at two sizes, and the difference between them is not just the number.
//
//   - By default it writes a few thousand entries through the ORDINARY write
//     path - the same AFTER INSERT trigger production uses - and measures that.
//     Safe to run alongside anything.
//
//   - With VKAI_TEST_AUDIT_SCALE set it builds the fixture set-based, which
//     means taking the chain-append trigger off the table for the duration.
//     That is a change to the whole database, not to one tenant, so entries
//     written by anything else while it runs are left unchained. It therefore
//     REQUIRES EXCLUSIVE USE of the database - which is why it is opt-in and
//     not the default. Learned the hard way: with it on by default, tests in
//     another package running concurrently wrote sign-in entries that never
//     got chained, and the failure surfaced as an unrelated test being flaky.
//
//     VKAI_TEST_AUDIT_SCALE=10000000 VKAI_AUDIT_DSN=... \
//     go test ./internal/repository/ -run AtScale -v -timeout 4h
func TestChainVerificationAtScale(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	rows := 3000
	exclusive := false
	if s := os.Getenv("VKAI_TEST_AUDIT_SCALE"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			t.Fatalf("VKAI_TEST_AUDIT_SCALE=%q is not a positive number", s)
		}
		rows = n
		exclusive = true
	}

	t.Logf("generating %d entries", rows)
	started := time.Now()
	if exclusive {
		if err := generateSyntheticChain(ctx, db, tenantID, rows); err != nil {
			t.Fatalf("generating the fixture: %v", err)
		}
	} else {
		writeEntries(t, repo, tenantID, rows)
	}
	t.Logf("generated in %s", time.Since(started).Round(time.Millisecond))

	shallow, err := repo.Verify(ctx, tenantID, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !shallow.OK {
		t.Fatalf("the synthetic chain does not verify structurally: %q at %d",
			breakReason(shallow), breakSeq(shallow))
	}
	t.Logf("STRUCTURAL pass over %d entries: %d ms (%.1f us/entry)",
		shallow.Checked, shallow.DurationMS,
		float64(shallow.DurationMS)*1000/float64(shallow.Checked))

	deep, err := repo.Verify(ctx, tenantID, 1, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !deep.OK {
		t.Fatalf("the synthetic chain does not verify deeply: %q at %d",
			breakReason(deep), breakSeq(deep))
	}
	t.Logf("DEEP pass over %d entries: %d ms (%.1f us/entry)",
		deep.Checked, deep.DurationMS,
		float64(deep.DurationMS)*1000/float64(deep.Checked))

	// The incremental case: what a scheduled run actually pays once the bulk of
	// the table has already been cleared by an earlier pass.
	if rows > 1000 {
		from := int64(rows - 1000)
		inc, err := repo.Verify(ctx, tenantID, from, 0, true)
		if err != nil {
			t.Fatal(err)
		}
		if !inc.OK {
			t.Fatalf("the incremental pass reported a break: %q", breakReason(inc))
		}
		t.Logf("INCREMENTAL deep pass over the newest %d entries: %d ms", inc.Checked, inc.DurationMS)
	}
}

// generateSyntheticChain builds a valid chain directly in SQL.
//
// It reuses the very functions the trigger and the verifier use, so the fixture
// is a real chain and not a plausible-looking one: if audit_content_hash() and
// audit_entry_hash() were wrong, this would produce a table that fails
// verification rather than a table that hides the fault.
//
// It takes the chain-append trigger off audit_logs for the duration, which
// affects the WHOLE DATABASE and not just this tenant. Only call it when the
// database is yours alone - see the note on TestChainVerificationAtScale.
func generateSyntheticChain(ctx context.Context, db *sqlx.DB, tenantID uuid.UUID, rows int) error {
	const batch = 200000

	// The append trigger is the panel's write path and is measured separately:
	// about a millisecond an entry when each entry is its own transaction, and
	// worse than linear when thousands share one, because every entry leaves
	// another version of the same head row. Neither is a way to build a ten
	// million row fixture, so the trigger comes off and the chain is built
	// set-based below - out of the same SQL functions the trigger uses, so this
	// produces a real chain rather than a plausible-looking one.
	if _, err := db.ExecContext(ctx, "ALTER TABLE audit_logs DISABLE TRIGGER audit_logs_chain_append"); err != nil {
		return err
	}
	reinstate := func() {
		_, _ = db.ExecContext(ctx, "ALTER TABLE audit_logs ENABLE ALWAYS TRIGGER audit_logs_chain_append")
	}
	defer reinstate()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, action, resource, details, ip_address, user_agent, status, created_at)
		SELECT $1, 'scale.' || (g % 20), 'session',
		       jsonb_build_object('n', g),
		       '203.0.113.' || (g % 250 + 1), 'scale-test/1.0', 'success',
		       NOW() - ((($2::bigint - g) || ' seconds')::interval)
		  FROM generate_series(1, $2::bigint) g`, tenantID, rows); err != nil {
		return fmt.Errorf("inserting rows: %w", err)
	}
	reinstate()

	// content_hash carries no ordering dependency, so the expensive half is one
	// set-based statement.
	//
	// An ordinary table rather than a temporary one: sqlx hands out pooled
	// connections and a temporary table belongs to whichever session happened
	// to create it.
	staging := "scale_content_" + strings.ReplaceAll(tenantID.String(), "-", "")
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE UNLOGGED TABLE %s AS
		SELECT row_number() OVER (ORDER BY l.created_at, l.id) AS seq, l.id, l.created_at,
		       audit_content_hash(l.id, l.tenant_id, l.user_id, l.action, l.resource,
		                          l.resource_id, l.details, l.ip_address, l.user_agent,
		                          l.status, l.created_at) AS content_hash
		  FROM audit_logs l WHERE l.tenant_id = $1`, staging), tenantID); err != nil {
		return fmt.Errorf("hashing contents: %w", err)
	}
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+staging) }()

	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("CREATE UNIQUE INDEX ON %s (seq)", staging)); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "ANALYZE "+staging); err != nil {
		return err
	}

	// The links are inherently sequential. A recursive CTE over ten million
	// rows is unusably slow, so it is done in bounded batches, each anchored on
	// the last hash of the one before.
	prev := audit.GenesisHash
	for start := int64(1); start <= int64(rows); start += batch {
		end := start + batch - 1
		if end > int64(rows) {
			end = int64(rows)
		}

		// The recursive term joins the staging TABLE, not a CTE. Joining a CTE
		// here re-scans the whole thing on every step, which turns a linear
		// walk into a quadratic one - 20 000 entries took a minute before this
		// was an indexed lookup.
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`
			WITH RECURSIVE chained AS (
				SELECT sc.seq, sc.id, sc.created_at, sc.content_hash,
				       $4::text AS prev_hash,
				       audit_entry_hash($4::text, $1::uuid, sc.seq, sc.content_hash) AS entry_hash
				  FROM %[1]s sc WHERE sc.seq = $2
				UNION ALL
				SELECT sc.seq, sc.id, sc.created_at, sc.content_hash, c.entry_hash,
				       audit_entry_hash(c.entry_hash, $1::uuid, sc.seq, sc.content_hash)
				  FROM chained c JOIN %[1]s sc ON sc.seq = c.seq + 1
				 WHERE sc.seq <= $3
			)
			INSERT INTO audit_log_chain (tenant_id, seq, audit_log_id, prev_hash, content_hash, entry_hash, created_at)
			SELECT $1, seq, id, prev_hash, content_hash, entry_hash, created_at FROM chained`, staging),
			tenantID, start, end, prev); err != nil {
			return fmt.Errorf("linking %d..%d: %w", start, end, err)
		}

		if err := db.QueryRowContext(ctx,
			"SELECT entry_hash FROM audit_log_chain WHERE tenant_id = $1 AND seq = $2",
			tenantID, end).Scan(&prev); err != nil {
			return fmt.Errorf("reading the batch head: %w", err)
		}
	}

	var headPrev string
	if err := db.QueryRowContext(ctx,
		"SELECT prev_hash FROM audit_log_chain WHERE tenant_id = $1 AND seq = $2",
		tenantID, rows).Scan(&headPrev); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO audit_chain_head (tenant_id, seq, prev_hash, head_hash) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE
		   SET seq = EXCLUDED.seq, prev_hash = EXCLUDED.prev_hash, head_hash = EXCLUDED.head_hash`,
		tenantID, rows, headPrev, prev); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, "ANALYZE audit_log_chain"); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "ANALYZE audit_logs")
	return err
}

// ------------------------------------------------------------
// 10. Incremental verification: the mode that makes running this often
//     affordable, and the mode most easily made worthless.
// ------------------------------------------------------------

// TestChainIncrementalResumeCoversNewEntries is the regression test for a bug
// that made incremental verification a no-op.
//
// An unbounded pass is asked for [1, 2^63-1]. Filing THAT as the verified range
// let the next incremental pass resume at 2^63-1, read nothing, and report the
// log intact - a check that examines nothing and passes, which is worse than no
// check at all because a schedule is relying on it. What must be recorded is
// the range the pass actually covered.
func TestChainIncrementalResumeCoversNewEntries(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	writeEntries(t, repo, tenantID, 10)

	first, err := repo.Verify(ctx, tenantID, 1, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.OK {
		t.Fatalf("the first pass found a break: %q", breakReason(first))
	}
	if first.ToSeq != 10 {
		t.Fatalf("the pass recorded to_seq %d; it verified up to 10", first.ToSeq)
	}
	if err := repo.RecordVerification(ctx, tenantID, first); err != nil {
		t.Fatal(err)
	}

	resume, err := repo.LastGoodSeq(ctx, tenantID, true)
	if err != nil {
		t.Fatal(err)
	}
	if resume != 10 {
		t.Fatalf("an incremental pass would resume at %d, want 10", resume)
	}

	// Five more entries, one of which is then altered. Resuming from the
	// recorded point has to reach it.
	written := writeEntries(t, repo, tenantID, 5)
	asAttacker(t, db, func() {
		if _, err := db.Exec(`UPDATE audit_logs SET status = 'failure' WHERE id = $1`,
			written[2].ID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	second, err := repo.Verify(ctx, tenantID, resume, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if second.OK {
		t.Fatal("the incremental pass missed an entry altered after the previous pass")
	}
	if breakSeq(second) != 13 || breakReason(second) != "content_altered" {
		t.Fatalf("got %q at seq %d, want content_altered at 13", breakReason(second), breakSeq(second))
	}
	if second.Checked >= 15 {
		t.Fatalf("the incremental pass re-read %d entries; it should only have read the new ones",
			second.Checked)
	}
}

// TestChainVerifyOnAnEmptyChainRecordsNothingVerified guards the other half of
// the same bug: a tenant that has never written an entry must not leave a
// resume point that skips its first entries when it does.
func TestChainVerifyOnAnEmptyChainRecordsNothingVerified(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	v, err := repo.Verify(ctx, tenantID, 1, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.Checked != 0 {
		t.Fatalf("an empty chain should verify with nothing checked, got ok=%v checked=%d", v.OK, v.Checked)
	}
	if v.ToSeq != 0 {
		t.Fatalf("an empty chain recorded to_seq %d; nothing was verified", v.ToSeq)
	}
	if err := repo.RecordVerification(ctx, tenantID, v); err != nil {
		t.Fatal(err)
	}

	writeEntries(t, repo, tenantID, 3)

	resume, err := repo.LastGoodSeq(ctx, tenantID, true)
	if err != nil {
		t.Fatal(err)
	}
	if resume != 0 {
		t.Fatalf("resume point is %d after verifying an empty chain; it must be 0 so the "+
			"first real entries are read", resume)
	}
}

// ------------------------------------------------------------
// 11. The one thing the head cannot do.
// ------------------------------------------------------------

// TestChainCheckpointMakesATruncatedTailUndeniable.
//
// audit_chain_head is mutable - it has to be, it advances on every entry - so
// an attacker holding the panel's database role can delete the newest entries
// AND move the head to match, leaving a chain that verifies perfectly. The head
// alone is therefore not a witness against truncation; it is only the cheap
// first check.
//
// A checkpoint seal is the witness. It lives in a table that refuses UPDATE and
// DELETE and it says the chain once reached a sequence number, so any later
// state with fewer entries is a provable deletion.
func TestChainCheckpointMakesATruncatedTailUndeniable(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	writeEntries(t, repo, tenantID, 12)

	headSeq, headHash, err := repo.HeadOf(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if headSeq != 12 {
		t.Fatalf("head is at %d, want 12", headSeq)
	}
	if _, err := repo.RecordCheckpoint(ctx, tenantID, headSeq, headHash, "test checkpoint"); err != nil {
		t.Fatalf("sealing the checkpoint: %v", err)
	}

	// Truncate the tail AND cover the tracks: fix the head to match what is
	// left. Without the seal this would verify clean.
	asAttacker(t, db, func() {
		if _, err := db.Exec(`
			DELETE FROM audit_logs WHERE id IN (
				SELECT audit_log_id FROM audit_log_chain WHERE tenant_id = $1 AND seq > 8)`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM audit_log_chain WHERE tenant_id = $1 AND seq > 8`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
		// audit_chain_head is deliberately NOT append-only, so this needs no
		// special privilege at all: it is the panel's ordinary write path.
		if _, err := db.Exec(`
			UPDATE audit_chain_head h
			   SET seq = c.seq, prev_hash = c.prev_hash, head_hash = c.entry_hash
			  FROM audit_log_chain c
			 WHERE h.tenant_id = $1 AND c.tenant_id = $1 AND c.seq = 8`, tenantID); err != nil {
			t.Fatalf("rewriting the head failed: %v", err)
		}
	})

	// The head now agrees with the truncated chain, and the chain links are
	// internally perfect. The seal is the only thing that disagrees.
	var headNow int64
	if err := db.Get(&headNow, `SELECT seq FROM audit_chain_head WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if headNow != 8 {
		t.Fatalf("the fixture did not rewrite the head: it is at %d", headNow)
	}

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("a truncated tail with a rewritten head verified as intact; " +
			"the checkpoint seal is not being consulted")
	}
	if breakReason(v) != "truncated_tail" || breakSeq(v) != 9 {
		t.Fatalf("got %q at seq %d, want truncated_tail at 9", breakReason(v), breakSeq(v))
	}
	if v.HeadOK {
		t.Fatal("head_ok must be false when a seal names entries that are gone")
	}
}

// TestChainHandlesNullsAndMultiByteText is where a hash format usually breaks.
//
// Three ways for PostgreSQL and Go to disagree while every ASCII test passes:
// a NULL column rendered as "" by one side and "null" by the other; a NULL
// details column rendered as "{}" by one and "" by the other; and a length
// prefix counting characters instead of UTF-8 bytes, which agrees on ASCII and
// diverges the moment somebody writes a hostname in Vietnamese.
func TestChainHandlesNullsAndMultiByteText(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	// A user to hang a non-NULL user_id on, so both branches are covered.
	userID := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, username, email, password_hash, status)
		VALUES ($1, $2, $3, $4, 'x', 'active')`,
		userID, tenantID, "chain-"+userID.String()[:8], userID.String()[:8]+"@example.test"); err != nil {
		t.Fatalf("creating a user: %v", err)
	}
	resourceID := uuid.New()

	cases := []struct {
		name string
		log  models.AuditLog
	}{
		{"every nullable column NULL", models.AuditLog{
			Action: "act.bare", Resource: "r", Status: audit.StatusSuccess,
		}},
		{"every column populated", models.AuditLog{
			UserID: &userID, Action: "act.full", Resource: "r", ResourceID: &resourceID,
			Details:   models.JSONMap{"who": "quản trị viên", "n": 42},
			IPAddress: "203.0.113.9", UserAgent: "Mozilla/5.0", Status: audit.StatusFailure,
		}},
		{"multi-byte text in every text column", models.AuditLog{
			Action: "hành.động", Resource: "tài_nguyên",
			Details:   models.JSONMap{"ghi chú": "máy chủ đã bị xoá", "emoji": "🔐"},
			IPAddress: "2001:db8::1", UserAgent: "trình duyệt/1.0 (Việt Nam)",
			Status: audit.StatusSuccess,
		}},
		{"empty details object", models.AuditLog{
			Action: "act.empty", Resource: "r", Details: models.JSONMap{},
			Status: audit.StatusSuccess,
		}},
	}

	for _, c := range cases {
		l := c.log
		l.TenantID = tenantID
		if err := repo.Create(ctx, &l); err != nil {
			t.Fatalf("%s: writing: %v", c.name, err)
		}
	}

	// PostgreSQL hashed these. Go must reproduce every one of them from the
	// strings the export carries.
	exp, err := repo.ExportRange(ctx, tenantID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Entries) != len(cases) {
		t.Fatalf("exported %d entries, wrote %d", len(exp.Entries), len(cases))
	}

	prev := audit.GenesisHash
	for i, e := range exp.Entries {
		if got := e.Content.ContentHash(); got != e.ContentHash {
			t.Fatalf("%s: PostgreSQL says content_hash %s, Go says %s (details %q, user_id %q)",
				cases[i].name, e.ContentHash, got, e.Content.Details, e.Content.UserID)
		}
		if got := audit.EntryHash(prev, e.Content.TenantID, e.Seq, e.ContentHash); got != e.EntryHash {
			t.Fatalf("%s: entry_hash disagrees", cases[i].name)
		}
		prev = e.EntryHash
	}

	// NULL renders as the empty string, never "null" or "<nil>"; NULL details
	// renders as "{}". Both are stated in the published format, so both are
	// asserted rather than assumed.
	bare := exp.Entries[0].Content
	if bare.UserID != "" || bare.ResourceID != "" || bare.IPAddress != "" || bare.UserAgent != "" {
		t.Fatalf("a NULL column did not render as the empty string: %+v", bare)
	}
	if bare.Details != "{}" {
		t.Fatalf("NULL details rendered as %q, the format says \"{}\"", bare.Details)
	}
	if exp.Entries[3].Content.Details != "{}" {
		t.Fatalf("an empty details object rendered as %q", exp.Entries[3].Content.Details)
	}

	if res, err := audit.VerifyExport(exp); err != nil || !res.OK {
		t.Fatalf("the bundle did not verify: %v %+v", err, res)
	}
	if v := verify(t, repo, tenantID, true); !v.OK {
		t.Fatalf("the chain did not verify in the database: %q at %d", breakReason(v), breakSeq(v))
	}
}

// TestChainDetectsAWholesaleDeletion. The obvious attack, and the one an
// "intact, nothing to check" answer would wave straight through: remove every
// entry rather than a careful few.
func TestChainDetectsAWholesaleDeletion(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)

	writeEntries(t, repo, tenantID, 6)

	asAttacker(t, db, func() {
		if _, err := db.Exec(`
			DELETE FROM audit_logs WHERE id IN (
				SELECT audit_log_id FROM audit_log_chain WHERE tenant_id = $1)`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM audit_log_chain WHERE tenant_id = $1`, tenantID); err != nil {
			t.Fatalf("the tamper itself failed: %v", err)
		}
	})

	v := verify(t, repo, tenantID, true)
	if v.OK {
		t.Fatal("an emptied chain verified as intact")
	}
	if breakReason(v) != "truncated_tail" {
		t.Fatalf("reason %q, want truncated_tail", breakReason(v))
	}
	if v.Checked != 0 {
		t.Fatalf("checked %d entries of an empty chain", v.Checked)
	}
}

// TestAuditReadsSurviveANewColumn is the regression guard for the trap that
// shaped this whole design.
//
// repository/audit.go used to read audit_logs with SELECT * and scan the result
// POSITIONALLY into eleven destinations. Adding a column put it at the end of
// the physical column order and took every audit read down with it - measured
// on PostgreSQL 16.15 before the change:
//
//	Search()  -> sql: expected 12 destination arguments in Scan, not 11
//	GetByID() -> missing destination name probe_extra in *models.AuditLog
//
// That is why the chain lives in side tables and adds nothing to audit_logs.
// The reads were converted to explicit column lists in the same change, and
// this asserts that conversion actually holds rather than assuming it: a column
// really is added here, and every read really is driven over it.
func TestAuditReadsSurviveANewColumn(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	writeEntries(t, repo, tenantID, 3)

	if _, err := db.Exec("ALTER TABLE audit_logs ADD COLUMN probe_extra TEXT"); err != nil {
		t.Fatalf("adding a column: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("ALTER TABLE audit_logs DROP COLUMN IF EXISTS probe_extra") })

	logs, total, err := repo.Search(ctx, tenantID, &models.AuditLogSearchRequest{})
	if err != nil {
		t.Fatalf("Search broke on a new column: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("Search returned %d/%d, want 3/3", len(logs), total)
	}
	if _, err := repo.GetByID(ctx, tenantID, logs[0].ID); err != nil {
		t.Fatalf("GetByID broke on a new column: %v", err)
	}
	if _, err := repo.GetStats(ctx, tenantID, 30); err != nil {
		t.Fatalf("GetStats broke on a new column: %v", err)
	}
	if exp, err := repo.ExportRange(ctx, tenantID, 1, 0, 0); err != nil || len(exp.Entries) != 3 {
		t.Fatalf("ExportRange broke on a new column: %v", err)
	}
	if v := verify(t, repo, tenantID, true); !v.OK {
		t.Fatalf("the chain broke on a new column: %q", breakReason(v))
	}
}

// TestChainExportSeriesDoesNotAccuseALongChain.
//
// One bundle is capped, so a chain longer than the cap comes out as a series.
// Each bundle in that series must verify on its own and must not be mistaken
// for a chain whose tail was deleted - accusing a customer of tampering because
// their audit log got big is the worst false positive this feature could have.
//
// Uses an explicit small limit rather than 50 000 entries, so the shape is
// tested without writing fifty thousand rows to prove it.
func TestChainExportSeriesDoesNotAccuseALongChain(t *testing.T) {
	db := openChainTestDB(t)
	repo := NewAuditRepository(db)
	tenantID := newTestTenant(t, db)
	ctx := context.Background()

	writeEntries(t, repo, tenantID, 25)

	// First bundle: starts at the beginning, cut short by the limit.
	first, err := repo.ExportRange(ctx, tenantID, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 10 {
		t.Fatalf("the first bundle holds %d entries, want the limit of 10", len(first.Entries))
	}
	if first.Complete {
		t.Fatal("a bundle cut short by the limit must not claim to hold the whole chain")
	}
	if first.Anchor.Kind != "genesis" {
		t.Fatalf("the first bundle should anchor on genesis, got %q", first.Anchor.Kind)
	}

	res, err := audit.VerifyExport(first)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("the first bundle of a series was reported as tampered with: %+v", res.Break)
	}

	// The rest of the series, each chaining to the one before it.
	prev := first
	for from := int64(11); from <= 25; from += 10 {
		next, err := repo.ExportRange(ctx, tenantID, from, 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if next.Anchor.Kind != "entry" {
			t.Fatalf("bundle from %d should anchor on an entry, got %q", from, next.Anchor.Kind)
		}
		last := prev.Entries[len(prev.Entries)-1]
		if next.Anchor.Hash != last.EntryHash {
			t.Fatalf("bundle from %d does not chain to the one before it", from)
		}

		res, err := audit.VerifyExport(next)
		if err != nil {
			t.Fatal(err)
		}
		if !res.OK {
			t.Fatalf("bundle from %d did not verify: %+v", from, res.Break)
		}
		prev = next
	}

	// The last bundle reaches the head; the whole chain in one bundle is
	// complete and says so.
	whole, err := repo.ExportRange(ctx, tenantID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !whole.Complete {
		t.Fatal("a bundle holding the entire chain must say so, or the tail checks never run")
	}
	if res, err := audit.VerifyExport(whole); err != nil || !res.OK || !res.HeadOK {
		t.Fatalf("the complete bundle did not verify: %v %+v", err, res)
	}
}
