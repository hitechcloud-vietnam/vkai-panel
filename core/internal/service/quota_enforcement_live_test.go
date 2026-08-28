// This file drives hosting package enforcement end to end against a real
// PostgreSQL, through the SAME service objects cmd/api/main.go builds. It is
// skipped unless VKAI_SCHEMA_DSN (or VKAI_TEST_DSN) names a database with every
// migration applied, including migrations/pending/packages.sql:
//
//	VKAI_SCHEMA_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	    go test ./internal/service/ -run Quota -v
//
// Why it is written this way: a test that registers its own routes on a
// throwaway engine, or that calls quota.Enforcer.Check directly, proves the
// code COULD run. These tests call WebsiteService.Create, DatabaseService.
// CreateDatabase, MailServerService.CreateAccount, CronService.Create and
// WebsiteService.AddDomain - the real functions the HTTP handlers call - so
// they fail if anybody removes a check from one of those paths.

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	_ "github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
)

type quotaFixture struct {
	db       *sqlx.DB
	tenantID uuid.UUID
	serverID uuid.UUID
	dbServer uuid.UUID
	mailZone uuid.UUID
	webRoot  string

	store    quota.Store
	enforcer *quota.Enforcer
	sampler  *quota.Sampler

	websites  *WebsiteService
	databases *DatabaseService
	mail      *MailServerService
	crons     *CronService
	packages  *PackageService

	pkg *quota.Package
}

func newQuotaFixture(t *testing.T, limits quota.Limits, action quota.Action) *quotaFixture {
	t.Helper()

	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		dsn = os.Getenv("VKAI_TEST_DSN")
	}
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live quota tests")
	}

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	f := &quotaFixture{db: db}

	suffix := uuid.New().String()[:8]
	if err := db.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		"quota-svc-"+suffix, "quota-svc-"+suffix).Scan(&f.tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO servers (tenant_id, hostname, ip_address, agent_token)
		 VALUES ($1, $2, '203.0.113.10', $3) RETURNING id`,
		f.tenantID, "host-"+suffix, uuid.New().String()).Scan(&f.serverID); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO database_servers (tenant_id, server_id, type, version, port)
		 VALUES ($1, $2, 'mysql', '8.0', 3306) RETURNING id`,
		f.tenantID, f.serverID).Scan(&f.dbServer); err != nil {
		t.Fatalf("seed database server: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO mail_domains (tenant_id, domain) VALUES ($1, $2) RETURNING id`,
		f.tenantID, "mail-"+suffix+".example.com").Scan(&f.mailZone); err != nil {
		t.Fatalf("seed mail domain: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM tenant_quota_events WHERE tenant_id = $1`,
			`DELETE FROM tenant_quota_usage WHERE tenant_id = $1`,
			`DELETE FROM tenant_quota_overrides WHERE tenant_id = $1`,
			`DELETE FROM tenant_feature_overrides WHERE tenant_id = $1`,
			`DELETE FROM tenant_packages WHERE tenant_id = $1`,
			`DELETE FROM mail_accounts WHERE tenant_id = $1`,
			`DELETE FROM mail_domains WHERE tenant_id = $1`,
			`DELETE FROM cron_jobs WHERE tenant_id = $1`,
			`DELETE FROM domains WHERE tenant_id = $1`,
			`DELETE FROM database_entries WHERE tenant_id = $1`,
			`DELETE FROM database_servers WHERE tenant_id = $1`,
			`DELETE FROM websites WHERE tenant_id = $1`,
			`DELETE FROM servers WHERE tenant_id = $1`,
			`DELETE FROM tenants WHERE id = $1`,
		} {
			_, _ = db.ExecContext(bg, stmt, f.tenantID)
		}
	})

	// Sites are created under a temporary root so the test never writes into
	// the real /vkai-panel/www tree.
	f.webRoot = t.TempDir()
	t.Setenv("VKAI_WEB_ROOT", f.webRoot)

	f.store = quota.NewPostgresStore(db)
	f.enforcer = quota.New(f.store, zap.NewNop())

	pkg := quota.Package{
		Name:            "Live test package " + suffix,
		Slug:            "live-test-" + suffix,
		Limits:          limits,
		Features:        map[string]bool{quota.FeatureCron: true},
		OverQuotaAction: action,
		WarnPercent:     90,
		GracePercent:    2,
		GraceFloorMB:    16,
		IsActive:        true,
	}
	if err := f.store.CreatePackage(ctx, &pkg); err != nil {
		t.Fatalf("create package: %v", err)
	}
	f.pkg = &pkg
	t.Cleanup(func() {
		// Assignments first: tenant_packages.package_id is ON DELETE RESTRICT.
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM tenant_packages WHERE package_id = $1`, pkg.ID)
		_ = f.store.DeletePackage(bg, pkg.ID)
	})

	if err := f.store.AssignPackage(ctx, f.tenantID, pkg.ID, nil); err != nil {
		t.Fatalf("assign package: %v", err)
	}

	registry := webserver.NewRegistry()
	websiteRepo := repository.NewWebsiteRepository(db)
	serverRepo := repository.NewServerRepository(db)

	// Exactly the wiring cmd/api/main.go performs.
	f.websites = NewWebsiteService(websiteRepo, serverRepo, registry, f.enforcer)
	f.databases = NewDatabaseService(repository.NewDatabaseRepository(db), serverRepo, f.enforcer)
	f.mail = NewMailServerService(repository.NewMailServerRepository(db, zap.NewNop()), zap.NewNop(), f.enforcer)
	f.crons = NewCronService(repository.NewCronRepository(db), serverRepo, f.enforcer)
	f.enforcer.SetSiteController(f.websites)

	f.sampler = quota.NewSampler(quota.SamplerOptions{
		Store: f.store, Enforcer: f.enforcer, Logger: zap.NewNop(), TenantPause: 0,
	})
	f.packages = NewPackageService(f.enforcer, f.sampler, zap.NewNop())

	return f
}

func (f *quotaFixture) createWebsite(t *testing.T) (*models.Website, error) {
	t.Helper()
	return f.websites.Create(context.Background(), &models.CreateWebsiteRequest{
		Domain:        "site-" + uuid.New().String()[:8] + ".example.com",
		ServerID:      f.serverID,
		WebServerType: "nginx",
	}, f.tenantID)
}

func mb(v int64) *int64 { return &v }

// ---------------------------------------------------------------------------
// EVERY CREATION PATH IS GATED BY THE SAME FUNCTION
//
// One package, one limit of one on every counted resource. Every path is
// driven to its limit and then once more. If somebody removes a check from any
// one of these services, exactly one subtest goes red and names it.
// ---------------------------------------------------------------------------

func TestQuotaEveryCreationPathRefusesAtItsLimit(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{
		Websites:   mb(1),
		Databases:  mb(1),
		Mailboxes:  mb(1),
		Subdomains: mb(1),
		CronJobs:   mb(1),
	}, quota.ActionRefuse)
	ctx := context.Background()

	// A website is needed before a subdomain can hang off one, and it is the
	// account's one allowed website.
	site, err := f.createWebsite(t)
	if err != nil {
		t.Fatalf("the first website was refused below the limit: %v", err)
	}

	paths := []struct {
		name string
		// fill brings the resource to its limit of one.
		fill func(t *testing.T)
		// attempt tries to create one more.
		attempt func(t *testing.T) error
		// resource names the limit that must appear in the refusal.
		resource string
	}{
		{
			name:     "WebsiteService.Create",
			fill:     func(*testing.T) {},
			attempt:  func(t *testing.T) error { _, err := f.createWebsite(t); return err },
			resource: "websites",
		},
		{
			name: "WebsiteService.AddDomain",
			fill: func(t *testing.T) {
				if _, err := f.websites.AddDomain(ctx, f.tenantID, site.ID,
					"sub-"+uuid.New().String()[:8]+".example.com", "subdomain"); err != nil {
					t.Fatalf("the first subdomain was refused below the limit: %v", err)
				}
			},
			attempt: func(t *testing.T) error {
				_, err := f.websites.AddDomain(ctx, f.tenantID, site.ID,
					"sub-"+uuid.New().String()[:8]+".example.com", "subdomain")
				return err
			},
			resource: "subdomains",
		},
		{
			name: "DatabaseService.CreateDatabase",
			fill: func(t *testing.T) {
				// Seeded directly: creating one for real would need a live
				// MySQL server, and what is under test here is the gate, not
				// the DDL behind it.
				if _, err := f.db.ExecContext(ctx,
					`INSERT INTO database_entries (tenant_id, database_server_id, name, username, password)
					 VALUES ($1, $2, $3, $4, 'x')`,
					f.tenantID, f.dbServer, "db"+uuid.New().String()[:8], "u"+uuid.New().String()[:8]); err != nil {
					t.Fatalf("seed database entry: %v", err)
				}
			},
			attempt: func(t *testing.T) error {
				_, err := f.databases.CreateDatabase(ctx, &models.CreateDBEntryRequest{
					DatabaseServerID: f.dbServer,
					Name:             "db" + uuid.New().String()[:8],
					Username:         "u" + uuid.New().String()[:8],
					Password:         "correct horse battery staple",
				}, f.tenantID)
				return err
			},
			resource: "databases",
		},
		{
			name: "MailServerService.CreateAccount",
			fill: func(t *testing.T) {
				if _, err := f.mail.CreateAccount(ctx, f.tenantID, models.CreateAccountRequest{
					DomainID: f.mailZone,
					Email:    "box-" + uuid.New().String()[:8] + "@example.com",
					Password: "secret-secret",
				}); err != nil {
					t.Fatalf("the first mailbox was refused below the limit: %v", err)
				}
			},
			attempt: func(t *testing.T) error {
				_, err := f.mail.CreateAccount(ctx, f.tenantID, models.CreateAccountRequest{
					DomainID: f.mailZone,
					Email:    "box-" + uuid.New().String()[:8] + "@example.com",
					Password: "secret-secret",
				})
				return err
			},
			resource: "mailboxes",
		},
		{
			name: "CronService.Create",
			fill: func(t *testing.T) {
				job, err := f.crons.Create(ctx, &models.CreateCronJobRequest{
					ServerID: f.serverID,
					Name:     "nightly",
					Command:  "/bin/true",
					Schedule: "0 3 * * *",
				}, f.tenantID)
				if err != nil {
					t.Fatalf("the first cron job was refused below the limit: %v", err)
				}
				// Creating a cron job writes /etc/cron.d/vkai-<id8>. Remove it.
				t.Cleanup(func() {
					_ = os.Remove("/etc/cron.d/vkai-" + job.ID.String()[:8])
				})
			},
			attempt: func(t *testing.T) error {
				_, err := f.crons.Create(ctx, &models.CreateCronJobRequest{
					ServerID: f.serverID,
					Name:     "second",
					Command:  "/bin/true",
					Schedule: "0 4 * * *",
				}, f.tenantID)
				return err
			},
			resource: "cron jobs",
		},
	}

	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			p.fill(t)

			err := p.attempt(t)
			if err == nil {
				t.Fatalf("%s created a resource past the package limit of 1", p.name)
			}
			if !quota.IsExceeded(err) {
				t.Fatalf("%s failed for some other reason than quota: %v", p.name, err)
			}
			// The refusal has to name the limit and the usage, or the customer
			// cannot tell what to do about it.
			msg := err.Error()
			if !strings.Contains(msg, p.resource) {
				t.Fatalf("the refusal does not name the %s limit: %s", p.resource, msg)
			}
			if !strings.Contains(msg, "1") {
				t.Fatalf("the refusal does not carry the limit or the usage: %s", msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BELOW THE LIMIT, AND AFTER A DELETION
// ---------------------------------------------------------------------------

func TestQuotaUsageIsRecomputedAfterDeletion(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{Websites: mb(2)}, quota.ActionRefuse)
	ctx := context.Background()

	first, err := f.createWebsite(t)
	if err != nil {
		t.Fatalf("website 1 of 2: %v", err)
	}
	if _, err := f.createWebsite(t); err != nil {
		t.Fatalf("website 2 of 2: %v", err)
	}
	if _, err := f.createWebsite(t); !quota.IsExceeded(err) {
		t.Fatalf("website 3 of 2 was allowed: %v", err)
	}

	// Deleting one frees the allowance immediately, because the count comes
	// from the websites table and is never cached anywhere.
	if err := f.websites.Delete(ctx, f.tenantID, first.ID); err != nil {
		t.Fatalf("delete website: %v", err)
	}
	if _, err := f.createWebsite(t); err != nil {
		t.Fatalf("a website was still refused after one was deleted: %v", err)
	}
}

func TestQuotaOverrideBeatsThePackageThroughTheRealCreationPath(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{Websites: mb(1)}, quota.ActionRefuse)
	ctx := context.Background()

	if _, err := f.createWebsite(t); err != nil {
		t.Fatalf("website 1 of 1: %v", err)
	}
	if _, err := f.createWebsite(t); !quota.IsExceeded(err) {
		t.Fatalf("website 2 of 1 was allowed: %v", err)
	}

	// The exception always arrives.
	if err := f.packages.SetOverride(ctx, f.tenantID, "websites",
		&OverrideRequest{LimitValue: mb(3), Reason: "negotiated"}, nil); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if _, err := f.createWebsite(t); err != nil {
		t.Fatalf("the override did not take effect on the real creation path: %v", err)
	}

	// Withdrawing it puts the package limit back.
	if err := f.packages.DeleteOverride(ctx, f.tenantID, "websites"); err != nil {
		t.Fatalf("delete override: %v", err)
	}
	if _, err := f.createWebsite(t); !quota.IsExceeded(err) {
		t.Fatalf("the package limit did not come back after the override was removed: %v", err)
	}

	// And the status report agrees with the enforcement, so the panel cannot
	// show green while creations are being refused.
	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Enforced {
		t.Fatal("the status says nothing is enforced for an account that has a package")
	}
	var seen bool
	for _, rs := range status.Resources {
		if rs.Resource != quota.ResourceWebsites {
			continue
		}
		seen = true
		if rs.Usage != 2 || rs.Limit == nil || *rs.Limit != 1 {
			t.Fatalf("the report disagrees with the count: %+v", rs)
		}
		if rs.State != quota.StateOver {
			t.Fatalf("the report says %q while creations are refused", rs.State)
		}
	}
	if !seen {
		t.Fatal("the status report has no line for websites")
	}
}

// ---------------------------------------------------------------------------
// SUSPENSION AND ITS REVERSAL
// ---------------------------------------------------------------------------

func TestQuotaSuspensionIsReversibleAndDeletesNothing(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{Websites: mb(5)}, quota.ActionSuspend)
	ctx := context.Background()

	site, err := f.createWebsite(t)
	if err != nil {
		t.Fatalf("create website: %v", err)
	}
	// Put a file in the document root: nothing about a suspension may remove it.
	payload := filepath.Join(site.RootDir, "index.html")
	if err := os.MkdirAll(site.RootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte("customer data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.packages.Suspend(ctx, f.tenantID, "unpaid invoice"); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	// Every creation path is closed, and the reason is carried.
	if _, err := f.createWebsite(t); !quota.IsSuspended(err) {
		t.Fatalf("a suspended account created a website: %v", err)
	} else if !strings.Contains(err.Error(), "unpaid invoice") {
		t.Fatalf("the refusal lost the reason: %v", err)
	}

	// The website is marked, not deleted.
	reloaded, err := f.websites.GetByID(ctx, f.tenantID, site.ID)
	if err != nil {
		t.Fatalf("the website disappeared when the account was suspended: %v", err)
	}
	if reloaded.Status != StatusSuspended {
		t.Fatalf("the website status is %q, want %q", reloaded.Status, StatusSuspended)
	}
	if _, err := os.Stat(payload); err != nil {
		t.Fatalf("a suspension removed customer data: %v", err)
	}

	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Suspended || status.SuspendedAutomatically {
		t.Fatalf("the suspension is not reported as an operator's: %+v", status)
	}

	// And it comes back.
	if err := f.packages.Resume(ctx, f.tenantID, "invoice paid"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	reloaded, err = f.websites.GetByID(ctx, f.tenantID, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "active" {
		t.Fatalf("the website was not put back: status %q", reloaded.Status)
	}
	if _, err := f.createWebsite(t); err != nil {
		t.Fatalf("creations were still refused after the suspension was lifted: %v", err)
	}
	if _, err := os.Stat(payload); err != nil {
		t.Fatalf("customer data did not survive the round trip: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MEASURED USAGE: SAMPLED, NOT COUNTED
// ---------------------------------------------------------------------------

func TestQuotaDiskIsMeasuredAndRecomputedAfterDeletion(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{DiskMB: mb(1), Websites: mb(10)}, quota.ActionRefuse)
	ctx := context.Background()

	site, err := f.createWebsite(t)
	if err != nil {
		t.Fatalf("create website: %v", err)
	}
	if err := os.MkdirAll(site.RootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 8MB of files against a 1MB package: past the limit and past its grace
	// band, which for a 1MB limit is the 16MB floor... so write enough to clear
	// that too.
	big := make([]byte, 4*1024*1024)
	for _, name := range []string{"a.bin", "b.bin", "c.bin", "d.bin", "e.bin", "f.bin"} {
		if err := os.WriteFile(filepath.Join(site.RootDir, name), big, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatalf("sample: %v", err)
	}
	usage, err := f.store.MeasuredUsage(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.DiskUsedMB < 24 {
		t.Fatalf("the walk found %d MB of a 24 MB tree", usage.DiskUsedMB)
	}
	if usage.DiskFileCount == 0 || usage.DiskMeasuredAt == nil {
		t.Fatalf("the measurement cost was not recorded: %+v", usage)
	}

	// Over disk, so new resources of EVERY kind are refused - and the refusal
	// names disk, not the thing that was asked for.
	err = f.enforcer.Check(ctx, f.tenantID, quota.ResourceWebsites)
	if !quota.IsExceeded(err) {
		t.Fatalf("an account 24x over its disk quota was allowed to create a website: %v", err)
	}
	if !strings.Contains(err.Error(), "disk space") {
		t.Fatalf("the refusal does not name disk as the limit that was hit: %v", err)
	}

	// Delete the files and measure again: the number has to come back down.
	for _, name := range []string{"a.bin", "b.bin", "c.bin", "d.bin", "e.bin", "f.bin"} {
		if err := os.Remove(filepath.Join(site.RootDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatalf("re-sample: %v", err)
	}
	usage, err = f.store.MeasuredUsage(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.DiskUsedMB > 1 {
		t.Fatalf("after deleting 24 MB the account still reports %d MB", usage.DiskUsedMB)
	}
	if err := f.enforcer.Check(ctx, f.tenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("the account was still refused after freeing the disk: %v", err)
	}
}

func TestQuotaSamplerSuspendsAndUnsuspendsOnlyWhenConfiguredTo(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{DiskMB: mb(1), Websites: mb(10)}, quota.ActionSuspend)
	ctx := context.Background()

	site, err := f.createWebsite(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(site.RootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 4*1024*1024)
	for _, name := range []string{"a.bin", "b.bin", "c.bin", "d.bin", "e.bin", "f.bin"} {
		if err := os.WriteFile(filepath.Join(site.RootDir, name), big, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatalf("sample: %v", err)
	}

	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Suspended || !status.SuspendedAutomatically {
		t.Fatalf("a package that suspends on overage did not suspend: %+v", status)
	}
	reloaded, err := f.websites.GetByID(ctx, f.tenantID, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusSuspended {
		t.Fatalf("the sites were not taken offline: status %q", reloaded.Status)
	}

	// Free the disk: the next sweep lifts the suspension it made.
	for _, name := range []string{"a.bin", "b.bin", "c.bin", "d.bin", "e.bin", "f.bin"} {
		if err := os.Remove(filepath.Join(site.RootDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatal(err)
	}
	status, err = f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Suspended {
		t.Fatalf("the automatic suspension was not lifted when usage dropped: %+v", status)
	}
	reloaded, err = f.websites.GetByID(ctx, f.tenantID, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "active" {
		t.Fatalf("the sites were not put back: status %q", reloaded.Status)
	}
}

func TestQuotaOperatorSuspensionIsNotLiftedByUsageDropping(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{DiskMB: mb(10240), Websites: mb(10)}, quota.ActionSuspend)
	ctx := context.Background()

	if _, err := f.createWebsite(t); err != nil {
		t.Fatal(err)
	}
	if err := f.packages.Suspend(ctx, f.tenantID, "fraud investigation"); err != nil {
		t.Fatal(err)
	}

	// Usage is nowhere near the limit, so a sweep would "resume" the account if
	// it did not distinguish an operator's decision from its own.
	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatal(err)
	}
	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Suspended {
		t.Fatal("the sampler overruled an operator's suspension")
	}
}

// ---------------------------------------------------------------------------
// THE POLICY IS A POLICY
// ---------------------------------------------------------------------------

func TestQuotaWarnPolicyRefusesNothingButRecordsIt(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{DiskMB: mb(1), Websites: mb(10)}, quota.ActionWarn)
	ctx := context.Background()

	site, err := f.createWebsite(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(site.RootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site.RootDir, "big.bin"), make([]byte, 32*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.sampler.SampleTenant(ctx, f.tenantID); err != nil {
		t.Fatal(err)
	}

	if err := f.enforcer.Check(ctx, f.tenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("a warn-only package refused a creation: %v", err)
	}
	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Suspended {
		t.Fatal("a warn-only package suspended the account")
	}

	events, err := f.packages.ListEvents(ctx, f.tenantID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var warned bool
	for _, ev := range events {
		if ev.Resource == quota.ResourceDisk && ev.Kind == quota.EventWarn {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("nothing was recorded for an account past its disk quota: %+v", events)
	}
}

// TestQuotaUnmanagedAccountIsUnlimitedButVisible pins the one permissive case,
// so that "no package" cannot quietly become "no enforcement anywhere".
func TestQuotaUnmanagedAccountIsUnlimitedButVisible(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{Websites: mb(1)}, quota.ActionRefuse)
	ctx := context.Background()

	if _, err := f.db.ExecContext(ctx, `DELETE FROM tenant_packages WHERE tenant_id = $1`, f.tenantID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := f.createWebsite(t); err != nil {
			t.Fatalf("an account with no package was refused: %v", err)
		}
	}
	status, err := f.packages.Status(ctx, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enforced {
		t.Fatal("an account with no package reports that limits are being enforced")
	}
}

// TestQuotaEnforcerFailsClosedInTheRealServices is the disconnection test: a
// website service built without an enforcer must refuse, not allow.
func TestQuotaEnforcerFailsClosedInTheRealServices(t *testing.T) {
	f := newQuotaFixture(t, quota.Limits{Websites: mb(10)}, quota.ActionRefuse)

	registry := webserver.NewRegistry()
	unwired := NewWebsiteService(
		repository.NewWebsiteRepository(f.db),
		repository.NewServerRepository(f.db),
		registry,
		nil, // somebody removed the enforcer
	)

	_, err := unwired.Create(context.Background(), &models.CreateWebsiteRequest{
		Domain:        "unwired-" + uuid.New().String()[:8] + ".example.com",
		ServerID:      f.serverID,
		WebServerType: "nginx",
	}, f.tenantID)
	if err == nil {
		t.Fatal("a website service with no quota enforcer created a website; a wiring mistake must never read as 'no limits'")
	}
	if !errors.Is(err, quota.ErrNotWired) {
		t.Fatalf("the failure does not name the cause: %v", err)
	}
	_ = time.Now
}
