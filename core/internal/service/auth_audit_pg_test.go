package service

// Sign-in events, end to end, into the real tamper-evident chain.
//
// Requirement: "every security-relevant action lands here". A test with a
// capturing fake would prove the auth service CALLS an audit sink, which is not
// the claim. The claim is that a granted or refused sign-in ends up in the
// chained, append-only table an auditor reads - so this drives the real
// AuditService over a real PostgreSQL and then verifies the chain.
//
//	VKAI_AUDIT_DSN='postgres://vkai:vkai@127.0.0.1:5432/vkai_audit_test?sslmode=disable' \
//	    go test ./internal/service/ -run SignInIsAudited -v

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

func openSignInAuditDB(t *testing.T) *sqlx.DB {
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
			"this test needs a throwaway PostgreSQL with every migration applied")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestSignInIsAuditedIntoTheChain drives the four outcomes that matter and
// checks each one landed, chained, in the table an auditor reads.
func TestSignInIsAuditedIntoTheChain(t *testing.T) {
	db := openSignInAuditDB(t)
	ctx := context.Background()

	tenantID := uuid.New()
	slug := "signin-" + tenantID.String()[:8]
	if _, err := db.Exec(`
		INSERT INTO tenants (id, name, slug, status, plan, max_servers, max_websites)
		VALUES ($1, $2, $3, 'active', 'enterprise', 1, 1)`, tenantID, slug, slug); err != nil {
		t.Fatalf("creating a tenant: %v", err)
	}

	hash, err := utils.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{
		ID: uuid.New(), TenantID: tenantID, Username: "signin-" + tenantID.String()[:8],
		Email: slug + "@example.test", PasswordHash: hash, Status: "active",
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, username, email, password_hash, status)
		VALUES ($1, $2, $3, $4, $5, 'active')`,
		user.ID, tenantID, user.Username, user.Email, hash); err != nil {
		t.Fatalf("creating a user: %v", err)
	}

	repo := newFakeUserRepo()
	repo.add(user)

	auditRepo := repository.NewAuditRepository(db)
	auditService := NewAuditService(auditRepo, zap.NewNop())

	svc := &AuthService{
		userRepo:   repo,
		jwtManager: auth.NewJWTManager("signin-audit-test", 15*time.Minute, 24*time.Hour, "vkai-audit-test"),
		logger:     zap.NewNop(),
		failures:   newLoginFailureTracker(),
	}
	svc.SetAudit(auditService)

	// 1. A wrong password.
	if _, err := svc.Login(ctx, models.LoginRequest{Username: user.Username, Password: "wrong"}, "203.0.113.11"); err == nil {
		t.Fatal("a wrong password was accepted")
	}
	// 2. A username nobody recognises - the password-spray shape, with no
	//    tenant and no user id to attribute it to. Unique per run: these land
	//    on the shared default tenant by design, so counting "how many entries
	//    are there" would count every previous run of this test too.
	unknown := "nobody-" + uuid.NewString()
	if _, err := svc.Login(ctx, models.LoginRequest{Username: unknown, Password: "wrong"}, "203.0.113.12"); err == nil {
		t.Fatal("an unknown user was accepted")
	}
	// 3. A granted sign-in.
	if _, err := svc.Login(ctx, models.LoginRequest{Username: user.Username, Password: testPassword}, "203.0.113.13"); err != nil {
		t.Fatalf("a correct password was refused: %v", err)
	}
	// 4. A sign-out.
	svc.Logout(&auth.TokenClaims{UserID: user.ID, TenantID: tenantID, Username: user.Username}, "")

	// The tenant's own three entries.
	type row struct {
		Action    string `db:"action"`
		Status    string `db:"status"`
		IPAddress string `db:"ip_address"`
	}
	var rows []row
	if err := db.Select(&rows,
		`SELECT action, status, COALESCE(ip_address, '') AS ip_address
		   FROM audit_logs WHERE tenant_id = $1 ORDER BY created_at`, tenantID); err != nil {
		t.Fatal(err)
	}

	want := []row{
		{audit.ActionSignInFailed, audit.StatusFailure, "203.0.113.11"},
		{audit.ActionSignIn, audit.StatusSuccess, "203.0.113.13"},
		{audit.ActionSignOut, audit.StatusSuccess, ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("the trail has %d entries for this tenant, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Fatalf("entry %d is %+v, want %+v", i, rows[i], w)
		}
	}

	// The failure records WHY internally, which the caller was never told.
	var reason string
	if err := db.Get(&reason,
		`SELECT details->>'reason' FROM audit_logs
		  WHERE tenant_id = $1 AND action = $2`, tenantID, audit.ActionSignInFailed); err != nil {
		t.Fatal(err)
	}
	if reason != "bad_password" {
		t.Fatalf("the trail records the failure reason as %q, want bad_password", reason)
	}

	// The spray against an invented username is NOT dropped. It has no tenant
	// of its own, so it lands on the default tenant with the attempted name -
	// silence here would make the most recognisable attack pattern invisible.
	var sprayed int
	if err := db.Get(&sprayed,
		`SELECT count(*) FROM audit_logs
		  WHERE tenant_id = $1 AND action = $2 AND details->>'username' = $3`,
		audit.DefaultTenantID, audit.ActionSignInFailed, unknown); err != nil {
		t.Fatal(err)
	}
	if sprayed != 1 {
		t.Fatalf("a sign-in attempt for an unknown username left %d entries, want 1", sprayed)
	}

	// And all of it is chained. This is the part a capturing fake cannot say.
	v, err := auditRepo.Verify(ctx, tenantID, 1, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK {
		t.Fatalf("the sign-in entries did not chain: %v at %v", v.BreakReason, v.BreakSeq)
	}
	if v.Checked != 3 {
		t.Fatalf("the chain covers %d entries, the tenant wrote 3", v.Checked)
	}
}

// TestSetAuditRefusesAnEmptyService guards the wiring mistake that would make
// all of the above silently stop happening: a service handed a nil repository
// looks fine and records nothing.
func TestSetAuditRefusesAnEmptyService(t *testing.T) {
	svc := &AuthService{logger: zap.NewNop(), failures: newLoginFailureTracker()}

	svc.SetAudit(NewAuditService(nil, zap.NewNop()))
	if svc.audit != nil {
		t.Fatal("an audit service with no repository was accepted; sign-ins would be recorded nowhere")
	}

	svc.SetAudit(nil)
	if svc.audit != nil {
		t.Fatal("a nil audit service was accepted")
	}
}
