package repository_test

// The API key and panel session repositories, against a real PostgreSQL 16.
//
// This file exists because of what it found. Before this change,
// models.APIKey.Scopes was a plain []string mapped onto a text[] column, and
// under the pgx driver the panel actually runs on, every read of that column
// failed at runtime:
//
//	sql: Scan error on column index 6, name "scopes": unsupported Scan,
//	storing driver.Value type string into type *[]string
//
// - for a NULL column as well as a populated one. Nothing in the unit tests
// could see it, because a unit test does not have a PostgreSQL array on the
// other end of the wire. So: real cluster, real migrations, real driver.
//
// The driver here is deliberately pgx and not lib/pq. internal/database opens
// the panel's pool with "pgx", and the two drivers disagree about exactly this
// - lib/pq refuses a []string as a parameter, pgx accepts it and then cannot
// scan it back. A test on the wrong driver would have proved the wrong thing.
//
// Set VKAI_SCHEMA_DSN to a database with every migration applied, including
// migrations/pending:
//
//	VKAI_SCHEMA_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	    go test ./internal/repository/ -run TestAccessControl -v

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// openAccessDB opens the live database with the driver the panel uses.
func openAccessDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live database tests")
	}
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open VKAI_SCHEMA_DSN with the pgx driver: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping VKAI_SCHEMA_DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedAccount creates a tenant and a user, and removes them afterwards.
func seedAccount(t *testing.T, db *sqlx.DB) (tenantID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	if err := db.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		"access-test-"+suffix, "access-test-"+suffix,
	).Scan(&tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	if err := db.QueryRowContext(ctx,
		`INSERT INTO users (tenant_id, username, email, password_hash)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, "access-"+suffix, "access-"+suffix+"@example.test", "not-a-real-hash",
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM panel_sessions WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(bg, `UPDATE api_keys SET rotated_from = NULL WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(bg, `DELETE FROM api_keys WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(bg, `DELETE FROM users WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(bg, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	return tenantID, userID
}

func TestAccessControlAPIKeyRepositoryAgainstLiveSchema(t *testing.T) {
	db := openAccessDB(t)
	ctx := context.Background()
	tenantID, userID := seedAccount(t, db)

	repo := repository.NewAPIKeyRepository(db)
	expiry := time.Now().Add(90 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	key := &models.APIKey{
		ID:           uuid.New(),
		TenantID:     tenantID,
		UserID:       userID,
		Name:         "live schema key",
		KeyHash:      "hmac-sha256$" + uuid.New().String(),
		KeyPrefix:    "vk_live_aaaa",
		Scopes:       []string{"website:read", "database:write"},
		ExpiresAt:    &expiry,
		Status:       "active",
		AllowedCIDRs: []string{"203.0.113.0/24", "198.51.100.7"},
	}

	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if key.CreatedAt.IsZero() {
		t.Fatal("Create did not return created_at")
	}

	// The read that used to fail. Both array columns come back, populated.
	read, err := repo.GetByID(ctx, tenantID, key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(read.Scopes) != 2 || read.Scopes[0] != "website:read" || read.Scopes[1] != "database:write" {
		t.Fatalf("scopes did not round-trip: %#v", read.Scopes)
	}
	if len(read.AllowedCIDRs) != 2 || read.AllowedCIDRs[0] != "203.0.113.0/24" {
		t.Fatalf("allowed_cidrs did not round-trip: %#v", read.AllowedCIDRs)
	}
	if read.Status != "active" || read.RevokedAt != nil || read.RotationDeadline != nil {
		t.Fatalf("unexpected state on a fresh key: %+v", read)
	}

	// A key with no arrays at all: NULL text[] is the case that also failed.
	bare := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		Name:      "no arrays",
		KeyHash:   "hmac-sha256$" + uuid.New().String(),
		KeyPrefix: "vk_live_bbbb",
		Status:    "active",
	}
	if err := repo.Create(ctx, bare); err != nil {
		t.Fatalf("Create with NULL arrays: %v", err)
	}
	if _, err := repo.GetByID(ctx, tenantID, bare.ID); err != nil {
		t.Fatalf("reading a row with NULL text[] columns: %v", err)
	}

	// The authentication lookup: both prefix conventions, several candidates.
	candidates, err := repo.ListByPrefixes(ctx, auth.APIKeyLookupPrefixes("vk_live_aaaabbbbccccdddd"))
	if err != nil {
		t.Fatalf("ListByPrefixes: %v", err)
	}
	found := false
	for _, candidate := range candidates {
		if candidate.ID == key.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("the authentication lookup did not find the key by its prefix")
	}
	if empty, err := repo.ListByPrefixes(ctx, nil); err != nil || empty != nil {
		t.Fatalf("ListByPrefixes(nil) = %v, %v; want no rows and no error", empty, err)
	}

	// Listing and paging.
	listed, total, err := repo.ListByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListByTenant: %v", err)
	}
	if total != 2 || len(listed) != 2 {
		t.Fatalf("ListByTenant returned %d of %d, want 2 of 2", len(listed), total)
	}
	if mine, err := repo.ListByUser(ctx, tenantID, userID); err != nil || len(mine) != 2 {
		t.Fatalf("ListByUser = %d rows, %v", len(mine), err)
	}

	// Updating the grant.
	read.Scopes = []string{"*:read"}
	read.AllowedCIDRs = nil
	read.Name = "renamed"
	if err := repo.Update(ctx, read); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, err := repo.GetByID(ctx, tenantID, key.ID)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	if len(after.Scopes) != 1 || after.Scopes[0] != "*:read" || len(after.AllowedCIDRs) != 0 || after.Name != "renamed" {
		t.Fatalf("Update did not take: %+v", after)
	}

	// Last used, in time and place.
	used := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.MarkUsed(ctx, key.ID, used, "203.0.113.9"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	after, _ = repo.GetByID(ctx, tenantID, key.ID)
	if after.LastUsed == nil || after.LastUsedIP == nil || *after.LastUsedIP != "203.0.113.9" {
		t.Fatalf("MarkUsed did not record where the key was used: %+v", after)
	}
	if err := repo.UpdateLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}

	// The digest upgrade is conditional on the digest that was read.
	if err := repo.UpgradeHash(ctx, key.ID, "a digest this row does not have", "hmac-sha256$new"); err != nil {
		t.Fatalf("UpgradeHash: %v", err)
	}
	after, _ = repo.GetByID(ctx, tenantID, key.ID)
	if after.KeyHash == "hmac-sha256$new" {
		t.Fatal("UpgradeHash rewrote a digest that had already changed")
	}
	if err := repo.UpgradeHash(ctx, key.ID, after.KeyHash, "hmac-sha256$upgraded"); err != nil {
		t.Fatalf("UpgradeHash: %v", err)
	}
	after, _ = repo.GetByID(ctx, tenantID, key.ID)
	if after.KeyHash != "hmac-sha256$upgraded" {
		t.Fatalf("UpgradeHash did not take: %q", after.KeyHash)
	}

	// Rotation: the replacement points at what it replaces, and the original
	// gets a deadline.
	deadline := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	replacement := &models.APIKey{
		ID:          uuid.New(),
		TenantID:    tenantID,
		UserID:      userID,
		Name:        "replacement",
		KeyHash:     "hmac-sha256$" + uuid.New().String(),
		KeyPrefix:   "vk_live_cccc",
		Scopes:      []string{"*:read"},
		ExpiresAt:   &expiry,
		Status:      "active",
		RotatedFrom: &key.ID,
	}
	if err := repo.Create(ctx, replacement); err != nil {
		t.Fatalf("Create replacement: %v", err)
	}
	changed, err := repo.MarkSuperseded(ctx, tenantID, key.ID, deadline)
	if err != nil || changed != 1 {
		t.Fatalf("MarkSuperseded = %d, %v", changed, err)
	}
	after, _ = repo.GetByID(ctx, tenantID, key.ID)
	if after.Status != "superseded" || after.RotationDeadline == nil {
		t.Fatalf("MarkSuperseded did not take: %+v", after)
	}
	replacementRead, _ := repo.GetByID(ctx, tenantID, replacement.ID)
	if replacementRead.RotatedFrom == nil || *replacementRead.RotatedFrom != key.ID {
		t.Fatalf("the replacement does not name the key it replaces: %+v", replacementRead)
	}

	// Keys whose overlap or expiry is close.
	expiring, err := repo.ListExpiring(ctx, tenantID, 24*time.Hour)
	if err != nil {
		t.Fatalf("ListExpiring: %v", err)
	}
	if len(expiring) != 1 || expiring[0].ID != key.ID {
		t.Fatalf("ListExpiring returned %d rows, want the key whose overlap ends in two hours", len(expiring))
	}

	// Revocation, and the fact that revoking twice is not an error.
	changed, err = repo.Revoke(ctx, tenantID, key.ID, "test", time.Now())
	if err != nil || changed != 1 {
		t.Fatalf("Revoke = %d, %v", changed, err)
	}
	changed, err = repo.Revoke(ctx, tenantID, key.ID, "test", time.Now())
	if err != nil || changed != 0 {
		t.Fatalf("a second Revoke = %d, %v; want no rows changed and no error", changed, err)
	}
	after, _ = repo.GetByID(ctx, tenantID, key.ID)
	if after.RevokedAt == nil || after.RevokedReason == nil || *after.RevokedReason != "test" {
		t.Fatalf("Revoke did not record itself: %+v", after)
	}

	// Tenant scoping: another tenant cannot read or change this key.
	otherTenant, _ := seedAccount(t, db)
	if _, err := repo.GetByID(ctx, otherTenant, key.ID); err == nil {
		t.Fatal("a key was readable from another tenant")
	}
	if changed, _ := repo.Revoke(ctx, otherTenant, replacement.ID, "x", time.Now()); changed != 0 {
		t.Fatal("a key was revocable from another tenant")
	}

	if err := repo.Delete(ctx, tenantID, bare.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, tenantID, bare.ID); err == nil {
		t.Fatal("a deleted key was still readable")
	}
}

// TestAccessControlAPIKeySelectStarStillWorks guards the other reader of this
// table.
//
// internal/repository/multi_user.go reads api_keys with `SELECT *` into
// models.APIKey. Every column added by migrations/pending/apikey_scopes.sql
// therefore has to exist as a field on that struct, or that query fails with
// "missing destination name" - which is exactly the shape of the jobs table
// bug this codebase already shipped. This asserts the two are in step.
func TestAccessControlAPIKeySelectStarStillWorks(t *testing.T) {
	db := openAccessDB(t)
	ctx := context.Background()
	tenantID, userID := seedAccount(t, db)

	repo := repository.NewAPIKeyRepository(db)
	key := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		Name:      "select star",
		KeyHash:   "hmac-sha256$" + uuid.New().String(),
		KeyPrefix: "vk_live_dddd",
		Scopes:    []string{"website:read"},
		Status:    "active",
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var rows []models.APIKey
	if err := db.SelectContext(ctx, &rows,
		`SELECT * FROM api_keys WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatalf("SELECT * FROM api_keys into models.APIKey: %v\n"+
			"a column was added to api_keys without a field on models.APIKey; "+
			"repository/multi_user.go reads this table with SELECT *", err)
	}
	if len(rows) != 1 {
		t.Fatalf("SELECT * returned %d rows", len(rows))
	}
}

func TestAccessControlPanelSessionRepositoryAgainstLiveSchema(t *testing.T) {
	db := openAccessDB(t)
	ctx := context.Background()
	tenantID, userID := seedAccount(t, db)

	repo := repository.NewPanelSessionRepository(db)
	tokenID := uuid.New().String()
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	candidate := &models.PanelSession{
		ID:                uuid.New(),
		TokenID:           tokenID,
		UserID:            userID,
		TenantID:          tenantID,
		OriginIP:          "203.0.113.10",
		OriginNetwork:     auth.NetworkOf("203.0.113.10"),
		DeviceFingerprint: auth.DeviceFingerprint("Mozilla/5.0 Chrome/126.0.0.0"),
		UserAgent:         "Mozilla/5.0 Chrome/126.0.0.0",
		LastSeenAt:        time.Now().UTC().Truncate(time.Microsecond),
		ExpiresAt:         expires,
	}

	established, err := repo.Establish(ctx, candidate)
	if err != nil {
		t.Fatalf("Establish: %v", err)
	}
	if established.ID != candidate.ID {
		t.Fatalf("Establish returned a different row: %v", established.ID)
	}

	// Establishing twice for the same token must produce one session, not two
	// and not an error: two requests carrying a brand new token race here.
	second := *candidate
	second.ID = uuid.New()
	second.OriginIP = "198.51.100.4"
	again, err := repo.Establish(ctx, &second)
	if err != nil {
		t.Fatalf("Establish twice: %v", err)
	}
	if again.ID != established.ID {
		t.Fatal("a second Establish for the same token created a second session")
	}
	if again.OriginIP != "203.0.113.10" {
		t.Fatalf("the second caller's values overwrote the binding: %q", again.OriginIP)
	}

	// Activity and a move.
	touched := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Touch(ctx, established.ID, "198.51.100.4", touched, true, true); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	read, err := repo.GetByTokenID(ctx, tokenID)
	if err != nil {
		t.Fatalf("GetByTokenID: %v", err)
	}
	if read.OriginChanges != 1 || !read.ReauthRequired {
		t.Fatalf("Touch did not record the move: %+v", read)
	}
	if read.LastSeenIP == nil || *read.LastSeenIP != "198.51.100.4" {
		t.Fatalf("Touch did not record where from: %+v", read.LastSeenIP)
	}

	// Proving the password moves the binding and clears the flag.
	if err := repo.Rebind(ctx, established.ID, "198.51.100.4", auth.NetworkOf("198.51.100.4"), time.Now()); err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	read, _ = repo.GetByTokenID(ctx, tokenID)
	if read.ReauthRequired || read.OriginIP != "198.51.100.4" || read.OriginNetwork != "198.51.100.0/24" {
		t.Fatalf("Rebind did not take: %+v", read)
	}

	// The operator's own list.
	live, err := repo.ListForUser(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("ListForUser returned %d rows, want 1", len(live))
	}

	if _, err := repo.GetByID(ctx, tenantID, established.ID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// Ending it, and the row surviving so the token stays refused.
	changed, err := repo.Revoke(ctx, tenantID, userID, established.ID, "terminated_by_user", time.Now())
	if err != nil || changed != 1 {
		t.Fatalf("Revoke = %d, %v", changed, err)
	}
	read, err = repo.GetByTokenID(ctx, tokenID)
	if err != nil {
		t.Fatalf("a revoked session row disappeared; the same token could establish a new session: %v", err)
	}
	if read.RevokedAt == nil {
		t.Fatal("Revoke did not mark the row")
	}
	if live, _ := repo.ListForUser(ctx, tenantID, userID); len(live) != 0 {
		t.Fatal("a revoked session is still listed as live")
	}
	if changed, _ := repo.Revoke(ctx, tenantID, userID, established.ID, "again", time.Now()); changed != 0 {
		t.Fatal("revoking twice reported a change")
	}

	// The other two ways a session ends.
	byToken := &models.PanelSession{
		ID: uuid.New(), TokenID: uuid.New().String(), UserID: userID, TenantID: tenantID,
		OriginIP: "203.0.113.11", OriginNetwork: "203.0.113.0/24",
		LastSeenAt: time.Now(), ExpiresAt: expires,
	}
	if _, err := repo.Establish(ctx, byToken); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	if changed, err := repo.RevokeByTokenID(ctx, byToken.TokenID, "device_changed", time.Now()); err != nil || changed != 1 {
		t.Fatalf("RevokeByTokenID = %d, %v", changed, err)
	}

	forAll := &models.PanelSession{
		ID: uuid.New(), TokenID: uuid.New().String(), UserID: userID, TenantID: tenantID,
		OriginIP: "203.0.113.12", OriginNetwork: "203.0.113.0/24",
		LastSeenAt: time.Now(), ExpiresAt: expires,
	}
	if _, err := repo.Establish(ctx, forAll); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	count, err := repo.RevokeAllForUser(ctx, tenantID, userID, "password_changed", time.Now())
	if err != nil || count != 1 {
		t.Fatalf("RevokeAllForUser = %d, %v; want the one session still live", count, err)
	}

	// The janitor removes only rows whose token has expired.
	expired := &models.PanelSession{
		ID: uuid.New(), TokenID: uuid.New().String(), UserID: userID, TenantID: tenantID,
		OriginIP: "203.0.113.13", OriginNetwork: "203.0.113.0/24",
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(-time.Hour),
	}
	if _, err := repo.Establish(ctx, expired); err != nil {
		t.Fatalf("Establish an expired session: %v", err)
	}
	removed, err := repo.PurgeExpired(ctx, time.Now())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if removed != 1 {
		t.Fatalf("PurgeExpired removed %d rows, want exactly the expired one", removed)
	}
	if _, err := repo.GetByTokenID(ctx, byToken.TokenID); err != nil {
		t.Fatal("PurgeExpired removed a revoked row whose token has not expired; that token could establish a new session")
	}
}
