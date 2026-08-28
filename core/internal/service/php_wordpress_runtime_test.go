package service_test

// End-to-end tests for multi-version PHP and the WordPress runtime identity,
// driven through the SERVICES against a real PostgreSQL and a real filesystem.
//
// The package-level tests in internal/phpfpm and internal/wpcli prove the
// mechanisms. These prove they are wired to the database rows the panel
// actually stores - which is the join that was missing everywhere the audit
// looked. A pool file that renders correctly from a hand-built PoolSpec says
// nothing about whether the values in php_pool_settings ever reach one.
//
// Set VKAI_SCHEMA_DSN to a database with every migration applied, including
// migrations/pending/php_wordpress.sql:
//
//	VKAI_SCHEMA_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	    go test ./internal/service/ -run TestPHPWordPress -v

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/phpfpm"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/wpcli"
)

func liveDB(t *testing.T) *sqlx.DB {
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

// scriptedRunner answers external commands from a script, and records them.
type scriptedRunner struct {
	calls  []string
	fail   map[string]error
	active string
}

func (s *scriptedRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, call)
	for marker, err := range s.fail {
		if strings.Contains(call, marker) {
			return []byte("simulated"), err
		}
	}
	if strings.Contains(call, "is-active") {
		state := s.active
		if state == "" {
			state = "active"
		}
		return []byte(state), nil
	}
	return []byte("ok"), nil
}

func (s *scriptedRunner) called(marker string) bool {
	for _, call := range s.calls {
		if strings.Contains(call, marker) {
			return true
		}
	}
	return false
}

// fixture is a tenant with a server, a website, two PHP versions and a pool
// bound to the older one - the shape a customer has just before they click
// "switch this site to PHP 8.3".
type fixture struct {
	db        *sqlx.DB
	tenantID  uuid.UUID
	serverID  uuid.UUID
	websiteID uuid.UUID
	poolID    string
	php81     string
	php83     string
	service   *service.PHPService
	runner    *scriptedRunner
	root      string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := liveDB(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.New().String()[:8], "-", "")

	f := &fixture{db: db, root: t.TempDir()}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.QueryRowContext(ctx, `INSERT INTO tenants (name, slug) VALUES ($1,$1) RETURNING id`,
		"f2-"+suffix).Scan(&f.tenantID))
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM php_pool_settings WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM php_pools WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM php_versions WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM wordpress_site_runtime WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM wordpress_staging WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM wordpress_sites WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM websites WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM servers WHERE tenant_id = $1`, f.tenantID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = $1`, f.tenantID)
	})

	must(db.QueryRowContext(ctx,
		`INSERT INTO servers (tenant_id, hostname, ip_address, agent_token)
		 VALUES ($1,$2,'10.0.0.1',$3) RETURNING id`,
		f.tenantID, "host-"+suffix, "tok-"+suffix).Scan(&f.serverID))

	domain := "f2-" + suffix + ".example.com"
	must(db.QueryRowContext(ctx,
		`INSERT INTO websites (tenant_id, server_id, domain, root_dir, web_server_type, php_version)
		 VALUES ($1,$2,$3,$4,'nginx','8.1') RETURNING id`,
		f.tenantID, f.serverID, domain, "/vkai-panel/www/domains/"+domain).Scan(&f.websiteID))

	phpRepo := repository.NewPHPRepository(db)
	logger := zap.NewNop()

	// Two versions, recorded through the repository the panel really uses -
	// which is also the assertion that the TEXT[] extensions bug is fixed,
	// because this call used to fail on every install.
	for _, spec := range []struct {
		id      *string
		version string
		exts    []string
	}{
		{&f.php81, "8.1", []string{}},
		{&f.php83, "8.3", []string{"redis"}},
	} {
		version := &models.PHPVersion{
			ID: uuid.New().String(), Version: spec.version,
			Path: "/usr/bin/php" + spec.version, FPMPath: "/usr/sbin/php-fpm" + spec.version,
			Extensions: spec.exts, IsActive: true,
			ServerID: f.serverID.String(), TenantID: f.tenantID.String(),
		}
		if err := phpRepo.CreatePHPVersion(ctx, version); err != nil {
			t.Fatalf("recording PHP %s failed: %v\\n"+
				"If this is `malformed array literal`, the php_versions.extensions column is "+
				"TEXT[] and the repository is marshalling JSON into it again.", spec.version, err)
		}
		*spec.id = version.ID
	}

	pool := &models.PHPPool{
		ID: uuid.New().String(), Name: "site-" + suffix, PHPVersionID: f.php81,
		User: "site-" + suffix, Group: "site-" + suffix,
		Listen:      "/run/php/site-" + suffix + ".sock",
		ListenOwner: "www-data", ListenGroup: "www-data", ListenMode: "0660",
		PM: "dynamic", PMMaxChildren: 10, PMStartServers: 2,
		PMMinSpareServers: 1, PMMaxSpareServers: 3, PMMaxRequests: 500,
		IsActive: true, WebsiteID: f.websiteID.String(),
		ServerID: f.serverID.String(), TenantID: f.tenantID.String(),
	}
	if err := phpRepo.CreatePHPPool(ctx, pool); err != nil {
		t.Fatalf("recording the pool failed: %v", err)
	}
	f.poolID = pool.ID

	// The host-side half, pointed at a temporary root with both versions' pool
	// directories and php-fpm binaries in place.
	f.runner = &scriptedRunner{fail: map[string]error{}}
	distro := phpfpm.DetectedForTest("ubuntu", "24.04", "Ubuntu 24.04 LTS", "debian")
	manager, err := phpfpm.New(phpfpm.Options{
		Distro: &distro, Runner: f.runner, Logger: logger, RootDir: f.root,
	})
	must(err)
	for _, version := range []string{"8.1", "8.3"} {
		layout, err := manager.Layout(version)
		must(err)
		must(os.MkdirAll(filepath.Join(f.root, layout.PoolDir), 0o755))
		must(os.MkdirAll(filepath.Join(f.root, filepath.Dir(layout.Binary)), 0o755))
		must(os.WriteFile(filepath.Join(f.root, layout.Binary), []byte("#!/bin/sh\\n"), 0o755))
	}

	f.service = service.NewPHPService(phpRepo, logger)
	f.service.SetRuntimeForTest(manager, repository.NewPHPRuntimeRepository(db))
	return f
}

func (f *fixture) poolPath(version, name string) string {
	return filepath.Join(f.root, "/etc/php", version, "fpm/pool.d", name+".conf")
}

// TestPHPWordPressPoolSettingsReachThePoolFile drives the settings the panel
// stores all the way onto disk, and then reads both the file and the row back.
func TestPHPWordPressPoolSettingsReachThePoolFile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	result, err := f.service.ApplyPoolSettings(ctx, f.poolID, f.tenantID.String(),
		&service.PoolSettingsRequest{
			MemoryLimit:       "768M",
			MaxExecutionTime:  240,
			UploadMaxFilesize: "256M",
			Extensions:        []string{"redis", "imagick", "Redis "},
			PostMaxSize:       "256M",
			Timezone:          "Asia/Ho_Chi_Minh",
		})
	if err != nil {
		t.Fatalf("applying the settings failed: %v", err)
	}

	// On disk, at the path the service told the operator it wrote to. Reading
	// the reported path rather than a computed one is the point: a service
	// that reports one path and writes another is exactly the kind of untruth
	// this whole task is about.
	if want := f.poolPath("8.1", poolNameOf(t, f)); result.PoolFile != want {
		t.Fatalf("the service reported writing %q, want %q", result.PoolFile, want)
	}
	content, err := os.ReadFile(result.PoolFile)
	if err != nil {
		t.Fatalf("the pool file the service reported does not exist: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"php_admin_value[memory_limit] = 768M",
		"php_admin_value[max_execution_time] = 240",
		"php_admin_value[upload_max_filesize] = 256M",
		"php_admin_value[date.timezone] = Asia/Ho_Chi_Minh",
		";   extension = imagick",
		";   extension = redis",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the pool file on disk does not carry %q.\n%s", want, text)
		}
	}
	// "Redis " and "redis" are one extension and one package.
	if strings.Count(text, "extension = redis") != 1 {
		t.Errorf("the extension list was not normalised:\n%s", text)
	}

	// In the database, and separated into intent and reality.
	stored, err := f.service.GetPoolSettings(ctx, f.poolID, f.tenantID.String())
	if err != nil {
		t.Fatal(err)
	}
	if stored.MemoryLimit != "768M" || stored.MaxExecutionTime != 240 ||
		stored.UploadMaxFilesize != "256M" {
		t.Fatalf("the stored settings do not match what was applied: %+v", stored)
	}
	applied := stored.Applied()
	if !applied.InForce {
		t.Fatal("the settings are on disk but the row does not record that anything was applied")
	}
	if applied.PHPVersion != "8.1" {
		t.Fatalf("applied_php_version is %q, want 8.1", applied.PHPVersion)
	}
	if applied.PoolFile != result.PoolFile {
		t.Fatalf("the recorded pool file %q is not the one that was written %q",
			applied.PoolFile, result.PoolFile)
	}
	if !f.runner.called("systemctl reload php8.1-fpm") {
		t.Fatal("php-fpm was never reloaded, so the settings are on disk and not in force")
	}
}

// poolNameOf reads the pool's name back out of the database.
func poolNameOf(t *testing.T, f *fixture) string {
	t.Helper()
	var name string
	if err := f.db.Get(&name, `SELECT name FROM php_pools WHERE id = $1`, f.poolID); err != nil {
		t.Fatal(err)
	}
	return name
}

// TestPHPWordPressSiteVersionSwitchMovesEverythingTogether is the multi-version
// feature end to end: the pool file moves between the two version directories,
// both services reload, php_pools points at the new version and - the one that
// is easy to forget - websites.php_version says the new version too, because
// that is the column a vhost rewrite reads.
func TestPHPWordPressSiteVersionSwitchMovesEverythingTogether(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tenant := f.tenantID.String()
	name := poolNameOf(t, f)

	if _, err := f.service.ApplyPoolSettings(ctx, f.poolID, tenant,
		&service.PoolSettingsRequest{MemoryLimit: "256M"}); err != nil {
		t.Fatalf("the initial apply failed: %v", err)
	}
	if _, err := os.Stat(f.poolPath("8.1", name)); err != nil {
		t.Fatalf("the site is not on PHP 8.1: %v", err)
	}

	f.runner.calls = nil
	result, err := f.service.SetSiteVersion(ctx, f.websiteID.String(), tenant,
		&service.SwitchVersionRequest{Version: "8.3", ServerID: f.serverID.String()})
	if err != nil {
		t.Fatalf("the version switch failed: %v", err)
	}
	if result.PreviousVersion != "8.1" || result.Version != "8.3" {
		t.Fatalf("the switch reports %s -> %s", result.PreviousVersion, result.Version)
	}

	if _, err := os.Stat(f.poolPath("8.1", name)); !os.IsNotExist(err) {
		t.Fatal("the pool file is still in the PHP 8.1 directory; two FPM masters would both " +
			"try to listen on this site's socket")
	}
	moved, err := os.ReadFile(f.poolPath("8.3", name))
	if err != nil {
		t.Fatalf("the pool file is not in the PHP 8.3 directory: %v", err)
	}
	if !strings.Contains(string(moved), "php_admin_value[memory_limit] = 256M") {
		t.Fatal("the site's settings did not survive the version switch")
	}

	for _, unit := range []string{"systemctl reload php8.3-fpm", "systemctl reload php8.1-fpm"} {
		if !f.runner.called(unit) {
			t.Errorf("%q was never run", unit)
		}
	}

	// The pool row.
	var versionID string
	if err := f.db.Get(&versionID, `SELECT php_version_id FROM php_pools WHERE id = $1`, f.poolID); err != nil {
		t.Fatal(err)
	}
	if versionID != f.php83 {
		t.Fatal("php_pools still points at PHP 8.1")
	}

	// The website row - the one a vhost rewrite reads.
	var sitePHP string
	if err := f.db.Get(&sitePHP, `SELECT php_version FROM websites WHERE id = $1`, f.websiteID); err != nil {
		t.Fatal(err)
	}
	if sitePHP != "8.3" {
		t.Fatalf("websites.php_version is %q; the next vhost rewrite would point this site at "+
			"the PHP 8.1 socket, which no longer has a pool", sitePHP)
	}

	// And reading it back through the service agrees.
	current, err := f.service.GetSiteVersion(ctx, f.websiteID.String(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != "8.3" {
		t.Fatalf("GetSiteVersion says %q", current.Version)
	}
}

// TestPHPWordPressAFailedSwitchLeavesTheSiteOnTheOldVersionEverywhere is the
// rollback, asserted across BOTH the filesystem and the database. A rollback
// that restores the file but leaves the database saying 8.3 is worse than no
// rollback: the next vhost rewrite then points at a socket that does not exist.
func TestPHPWordPressAFailedSwitchLeavesTheSiteOnTheOldVersionEverywhere(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tenant := f.tenantID.String()
	name := poolNameOf(t, f)

	if _, err := f.service.ApplyPoolSettings(ctx, f.poolID, tenant,
		&service.PoolSettingsRequest{MemoryLimit: "256M"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(f.poolPath("8.1", name))
	if err != nil {
		t.Fatal(err)
	}

	f.runner.fail["php-fpm8.3 --test"] = fmt.Errorf("[ERROR] unknown entry")

	if _, err := f.service.SetSiteVersion(ctx, f.websiteID.String(), tenant,
		&service.SwitchVersionRequest{Version: "8.3", ServerID: f.serverID.String()}); err == nil {
		t.Fatal("a switch to a PHP that rejected the pool was reported as success")
	}

	after, err := os.ReadFile(f.poolPath("8.1", name))
	if err != nil {
		t.Fatalf("the site's pool file was not put back on PHP 8.1; the site is down: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("the restored pool file differs from the one that was serving")
	}
	if _, err := os.Stat(f.poolPath("8.3", name)); !os.IsNotExist(err) {
		t.Fatal("a pool file was left in the PHP 8.3 directory")
	}

	var versionID, sitePHP string
	if err := f.db.Get(&versionID, `SELECT php_version_id FROM php_pools WHERE id = $1`, f.poolID); err != nil {
		t.Fatal(err)
	}
	if versionID != f.php81 {
		t.Fatal("php_pools was moved to PHP 8.3 even though the switch failed")
	}
	if err := f.db.Get(&sitePHP, `SELECT php_version FROM websites WHERE id = $1`, f.websiteID); err != nil {
		t.Fatal(err)
	}
	if sitePHP != "8.1" {
		t.Fatalf("websites.php_version is %q after a failed switch; the site is serving 8.1 and "+
			"the panel believes 8.3", sitePHP)
	}

	// applied_php_version must still say 8.1: it describes the host, not the
	// intent, and after a rollback the host is unchanged.
	stored, err := f.service.GetPoolSettings(ctx, f.poolID, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if applied := stored.Applied(); applied.PHPVersion != "8.1" {
		t.Fatalf("applied_php_version is %q after a rolled-back switch, want 8.1", applied.PHPVersion)
	} else if applied.LastError == "" {
		t.Error("nothing recorded WHY the switch failed, so an operator has only a log file")
	}
}

// TestPHPWordPressInvalidSettingsNeverReachTheDisk. A memory limit of "512MB"
// is a typo an operator makes; it must be refused before anything is written,
// not after php-fpm has been asked to parse it.
func TestPHPWordPressInvalidSettingsNeverReachTheDisk(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tenant := f.tenantID.String()
	name := poolNameOf(t, f)

	if _, err := f.service.ApplyPoolSettings(ctx, f.poolID, tenant,
		&service.PoolSettingsRequest{MemoryLimit: "512MB"}); err == nil {
		t.Fatal("a memory limit of 512MB was accepted")
	}
	if _, err := os.Stat(f.poolPath("8.1", name)); !os.IsNotExist(err) {
		t.Fatal("a pool file was written from settings that could not be rendered")
	}
	if f.runner.called("systemctl reload") {
		t.Fatal("php-fpm was reloaded for settings that were never valid")
	}
}

// TestPHPWordPressSystemReportNamesTheHostAndItsLimits proves the capability
// report is served from the detected host rather than from a constant.
func TestPHPWordPressSystemReportNamesTheHostAndItsLimits(t *testing.T) {
	f := newFixture(t)
	report, err := f.service.SystemReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Distribution != "Ubuntu 24.04 LTS" || report.Family != "debian" {
		t.Fatalf("the report describes %+v", report)
	}
	if !report.MultiVersionSupported {
		t.Fatal("Ubuntu is reported as unable to run several PHP versions")
	}
	if len(report.SupportMatrix) != 9 {
		t.Fatalf("the support matrix has %d rows, want one per supported operating system",
			len(report.SupportMatrix))
	}
	if len(report.InstalledVersions) != 2 {
		t.Fatalf("InstalledVersions is %v; it must read the filesystem, which has 8.1 and 8.3",
			report.InstalledVersions)
	}
}

// TestPHPWordPressRuntimeIdentityRefusesRoot is the database's own half of the
// "never run WP-CLI as root" guarantee: a CHECK constraint, so a bug in the
// service layer becomes a failed write rather than a root WP-CLI run.
func TestPHPWordPressRuntimeIdentityRefusesRoot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	var siteID uuid.UUID
	err := f.db.QueryRowContext(ctx,
		`INSERT INTO wordpress_sites (tenant_id, server_id, website_id, name, domain, path,
			db_name, db_user, db_password, admin_user, admin_password, admin_email)
		 VALUES ($1,$2,$3,$4,$5,$6,'d','u','p','admin','p','a@example.com') RETURNING id`,
		f.tenantID, f.serverID, f.websiteID, "wp-"+suffix, "wp-"+suffix+".example.com",
		"/vkai-panel/www/domains/wp-"+suffix+".example.com").Scan(&siteID)
	if err != nil {
		t.Fatal(err)
	}

	repo := repository.NewWordPressRuntimeRepository(f.db)

	if err := repo.UpsertRuntime(ctx, &models.WordPressRuntime{
		SiteID: siteID, TenantID: f.tenantID, RunAsUser: "root", RunAsGroup: "root",
	}); err == nil {
		t.Fatal("a WordPress site was recorded as running its commands as root; the database " +
			"CHECK constraint wordpress_site_runtime_not_root is missing")
	}

	// A real user is accepted, and reads back.
	if err := repo.UpsertRuntime(ctx, &models.WordPressRuntime{
		SiteID: siteID, TenantID: f.tenantID,
		RunAsUser: "site-" + suffix, RunAsGroup: "site-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetRuntime(ctx, siteID, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RunAsUser != "site-"+suffix {
		t.Fatalf("the runtime identity read back as %q", stored.RunAsUser)
	}

	// A site with no runtime row must produce the named error, not a nil row:
	// the service refuses to run anything rather than falling back to root.
	if _, err := repo.GetRuntime(ctx, uuid.New(), f.tenantID); err == nil {
		t.Fatal("a site with no runtime identity returned one")
	}
}

// ---------------------------------------------------------------------------
// The WordPress half, end to end through the service
// ---------------------------------------------------------------------------

// wpFixture is a WordPress site with a runtime identity, wired to a WP-CLI
// client whose process launcher is a stub. WP-CLI itself is not installed in
// this environment, so the stub is what makes the join between a
// wordpress_sites row and the argv that would run testable at all - and that
// join is the thing that was missing: the previous service inserted rows and
// ran nothing.
type wpFixture struct {
	*fixture
	siteID     uuid.UUID
	wp         *service.WordPressService
	commands   []string
	launched   []credentialRecord
	stagingDir string
}

type credentialRecord struct {
	uid, gid uint32
	dir      string
}

func newWPFixture(t *testing.T) *wpFixture {
	t.Helper()
	base := newFixture(t)
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	f := &wpFixture{fixture: base}

	// Both site trees live under a temporary web root, so the service's
	// "inside the panel web root" check passes without touching /vkai-panel.
	webRoot := filepath.Join(base.root, "www")
	t.Setenv("VKAI_WEB_ROOT", webRoot)
	siteDir := filepath.Join(webRoot, "wp-"+suffix+".example.com")
	f.stagingDir = filepath.Join(webRoot, "staging.wp-"+suffix+".example.com")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := base.db.QueryRowContext(ctx,
		`INSERT INTO wordpress_sites (tenant_id, server_id, website_id, name, domain, path,
			db_name, db_user, db_password, db_host, db_prefix,
			admin_user, admin_password, admin_email)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'dbpass','localhost','wp_','admin','adminpass',
			'owner@example.com') RETURNING id`,
		base.tenantID, base.serverID, base.websiteID, "wp-"+suffix,
		"wp-"+suffix+".example.com", siteDir, "wpdb_"+suffix, "wpuser_"+suffix).Scan(&f.siteID)
	if err != nil {
		t.Fatal(err)
	}

	// The site's unix identity. LookupIdentity is a package variable precisely
	// so a test does not have to create users on the machine; the substitute
	// still refuses root, which the assertions below rely on.
	realLookup := wpcli.LookupIdentity
	t.Cleanup(func() { wpcli.LookupIdentity = realLookup })
	wpcli.LookupIdentity = func(name string) (wpcli.Identity, error) {
		if name == "root" {
			return wpcli.Identity{}, &wpcli.ErrWouldRunAsRoot{Requested: name}
		}
		if _, err := wpcli.UnixName("site user", name); err != nil {
			return wpcli.Identity{}, err
		}
		return wpcli.Identity{Name: name, Group: name, UID: 4201, GID: 4201}, nil
	}

	runner := wpcli.NewRunnerForTest(func(_ context.Context, cmd *exec.Cmd) (*wpcli.Result, error) {
		f.commands = append(f.commands, strings.Join(cmd.Args[1:], " "))
		record := credentialRecord{dir: cmd.Dir}
		if cmd.SysProcAttr != nil && cmd.SysProcAttr.Credential != nil {
			record.uid = cmd.SysProcAttr.Credential.Uid
			record.gid = cmd.SysProcAttr.Credential.Gid
		}
		f.launched = append(f.launched, record)
		return &wpcli.Result{Stdout: "[]"}, nil
	})
	client := wpcli.NewClient(runner, zap.NewNop())
	staging := wpcli.NewStaging(client, zap.NewNop())
	staging.SetCopierForTest(func(_ context.Context, site wpcli.Site, from, to string) error {
		f.commands = append(f.commands, "COPY "+from+" -> "+to)
		f.launched = append(f.launched, credentialRecord{
			uid: site.Identity.UID, gid: site.Identity.GID, dir: to,
		})
		return os.MkdirAll(to, 0o750)
	})

	f.wp = service.NewWordPressService(repository.NewWordPressRepository(base.db), zap.NewNop())
	f.wp.SetRuntimeForTest(client, staging, repository.NewWordPressRuntimeRepository(base.db))

	if _, err := f.wp.SetRuntime(ctx, base.tenantID, f.siteID, &service.SetRuntimeRequest{
		RunAsUser: "site-" + suffix,
	}); err != nil {
		t.Fatalf("recording the site's runtime identity failed: %v", err)
	}
	return f
}

func (f *wpFixture) ran(marker string) bool {
	for _, command := range f.commands {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

// TestPHPWordPressEveryCommandRunsAsTheSitesOwnUser is the answer to "state the
// user each command runs as and how you enforce it", asserted rather than
// documented: every process the service would launch carries the site's
// credential, and none of them is root.
func TestPHPWordPressEveryCommandRunsAsTheSitesOwnUser(t *testing.T) {
	f := newWPFixture(t)
	ctx := context.Background()

	if _, _, err := f.wp.LivePlugins(ctx, f.tenantID, f.siteID); err != nil {
		t.Fatalf("listing plugins failed: %v", err)
	}
	if _, err := f.wp.UpdatePluginsLive(ctx, f.tenantID, f.siteID, &service.UpdateRequest{}); err != nil {
		t.Fatalf("updating plugins failed: %v", err)
	}
	if _, _, err := f.wp.LiveThemes(ctx, f.tenantID, f.siteID); err != nil {
		t.Fatalf("listing themes failed: %v", err)
	}
	if _, err := f.wp.ResetUserPasswordLive(ctx, f.tenantID, f.siteID,
		&service.ResetPasswordRequest{Login: "admin"}); err != nil {
		t.Fatalf("resetting the password failed: %v", err)
	}

	if len(f.launched) == 0 {
		t.Fatal("the service ran no commands at all; it is still only writing rows")
	}
	for i, record := range f.launched {
		if record.uid == 0 || record.gid == 0 {
			t.Fatalf("command %d (%q) would run as uid %d gid %d",
				i, f.commands[i], record.uid, record.gid)
		}
		if record.uid != 4201 {
			t.Fatalf("command %d ran as uid %d, want the site's own 4201", i, record.uid)
		}
	}

	// The identity is recorded in the database, so "which user did that run
	// as" is answerable after a log rotation.
	runtime, err := f.wp.GetRuntime(ctx, f.tenantID, f.siteID)
	if err != nil {
		t.Fatal(err)
	}
	view := runtime.View()
	if !strings.Contains(view.LastRanAs, "uid 4201") {
		t.Fatalf("last_ran_as is %q; it must name the identity the command ran under",
			view.LastRanAs)
	}
	if view.LastCommand == "" {
		t.Fatal("last_command is empty; there is no record of what ran")
	}
}

// TestPHPWordPressRootIsRefusedAtTheServiceBoundary.
func TestPHPWordPressRootIsRefusedAtTheServiceBoundary(t *testing.T) {
	f := newWPFixture(t)
	ctx := context.Background()

	if _, err := f.wp.SetRuntime(ctx, f.tenantID, f.siteID,
		&service.SetRuntimeRequest{RunAsUser: "root"}); err == nil {
		t.Fatal("a site was set to run its WP-CLI as root")
	}
	if _, err := f.wp.InstallSite(ctx, f.tenantID, f.siteID,
		&service.InstallSiteRequest{RunAsUser: "root"}); err == nil {
		t.Fatal("a WordPress install was accepted with root as the site user")
	}
	if len(f.commands) != 0 {
		t.Fatalf("commands ran despite the refusal: %v", f.commands)
	}
}

// TestPHPWordPressSearchReplaceDefaultsToADryRun. The API default is the safe
// one: this rewrites every row of a customer's database.
func TestPHPWordPressSearchReplaceDefaultsToADryRun(t *testing.T) {
	f := newWPFixture(t)
	ctx := context.Background()

	if _, _, err := f.wp.SearchReplaceLive(ctx, f.tenantID, f.siteID,
		&service.SearchReplaceRequest{From: "https://old.example.com", To: "https://new.example.com"}); err != nil {
		t.Fatal(err)
	}
	if !f.ran("--dry-run") {
		t.Fatalf("a search-replace with dry_run omitted ran for real: %v", f.commands)
	}

	live := false
	f.commands = nil
	if _, _, err := f.wp.SearchReplaceLive(ctx, f.tenantID, f.siteID,
		&service.SearchReplaceRequest{
			From: "https://old.example.com", To: "https://new.example.com", DryRun: &live,
		}); err != nil {
		t.Fatal(err)
	}
	if f.ran("--dry-run") {
		t.Fatal("an explicit dry_run=false still ran as a dry run")
	}
	if !f.ran("--precise") {
		t.Fatal("the live search-replace was not serialisation-safe")
	}
}

// TestPHPWordPressStagingPushRecordsTheDatabaseDecision is the staging half end
// to end: the clone writes a row, a push with no decision is refused before
// anything runs, and a push that IS made records which decision it was and
// where production was backed up first.
func TestPHPWordPressStagingPushRecordsTheDatabaseDecision(t *testing.T) {
	f := newWPFixture(t)
	ctx := context.Background()

	view, err := f.wp.CreateStaging(ctx, f.tenantID, f.siteID, &service.CreateStagingRequest{
		DBName: "wpstg", DBUser: "wpstguser", DBPassword: "stgpass",
	})
	if err != nil {
		t.Fatalf("cloning to staging failed: %v", err)
	}
	if view.Path != f.stagingDir {
		t.Fatalf("staging was created at %q, want %q", view.Path, f.stagingDir)
	}
	if !f.ran("option update blog_public 0") {
		t.Fatal("the staging copy was not hidden from search engines")
	}

	// A push with no decision is refused, and nothing runs.
	f.commands = nil
	if _, err := f.wp.PushStaging(ctx, f.tenantID, f.siteID,
		&service.PushStagingRequest{}); err == nil {
		t.Fatal("a push with no database decision was accepted")
	} else if !strings.Contains(err.Error(), "week of orders") {
		t.Fatalf("the refusal does not say what is at stake: %v", err)
	}
	if len(f.commands) != 0 {
		t.Fatalf("a push with no decision still ran commands: %v", f.commands)
	}

	// The database still records no push.
	var recorded *string
	if err := f.db.Get(&recorded,
		`SELECT last_push_database FROM wordpress_staging WHERE production_site_id = $1`,
		f.siteID); err != nil {
		t.Fatal(err)
	}
	if recorded != nil {
		t.Fatalf("last_push_database is %q after a refused push", *recorded)
	}

	// A decided push runs, and is recorded with the decision and the backups.
	pushed, err := f.wp.PushStaging(ctx, f.tenantID, f.siteID,
		&service.PushStagingRequest{Database: "keep_production"})
	if err != nil {
		t.Fatalf("a decided push failed: %v", err)
	}
	if pushed.Push == nil || pushed.Push.DatabaseCopied {
		t.Fatal("a keep_production push copied the database")
	}
	if pushed.Push.BackupPath == "" || pushed.Push.DatabaseBackup == "" {
		t.Fatal("the push did not report where production was backed up")
	}

	var action, filesBackup, dbBackup string
	if err := f.db.QueryRow(
		`SELECT last_push_database, last_push_backup, last_push_db_backup
		   FROM wordpress_staging WHERE production_site_id = $1`,
		f.siteID).Scan(&action, &filesBackup, &dbBackup); err != nil {
		t.Fatal(err)
	}
	if action != "keep_production" {
		t.Fatalf("the recorded decision is %q", action)
	}
	if filesBackup != pushed.Push.BackupPath || dbBackup != pushed.Push.DatabaseBackup {
		t.Fatal("the recorded backup paths are not the ones the push reported")
	}

	// The database itself refuses a decision that is not one of the three.
	if _, err := f.db.Exec(
		`UPDATE wordpress_staging SET last_push_database = 'whatever'
		   WHERE production_site_id = $1`, f.siteID); err == nil {
		t.Fatal("the database accepted a push decision outside the three choices; the CHECK " +
			"constraint wordpress_staging_push_choice is missing")
	}
}

// TestPHPWordPressInstallReallyInstalls is the first WordPress requirement end
// to end. The previous service inserted a wordpress_sites row and returned it;
// nothing was downloaded, no wp-config.php was written, no salts were
// generated and no installer ran. This asserts the whole sequence, in order,
// as the site's own user.
func TestPHPWordPressInstallReallyInstalls(t *testing.T) {
	f := newWPFixture(t)
	ctx := context.Background()
	f.commands = nil
	f.launched = nil

	result, err := f.wp.InstallSite(ctx, f.tenantID, f.siteID, &service.InstallSiteRequest{
		RunAsUser: "site-installer", SiteTitle: "Shop", CoreVersion: "6.5.2",
	})
	if err != nil {
		t.Fatalf("the install failed: %v", err)
	}

	// The sequence, in order. Each step is the one whose absence produces a
	// specific broken site.
	steps := []struct {
		marker string
		why    string
	}{
		{"core download --locale=en_US --force --version=6.5.2",
			"without this there are no WordPress files at all"},
		{"config create",
			"without this there is no wp-config.php and the site cannot reach its database"},
		{"--skip-salts",
			"wp config create fetches salts from WordPress.org; on a host with no outbound " +
				"access it silently writes the placeholders from wp-config-sample.php, and " +
				"anyone who knows those constants can forge this site's auth cookies"},
		{"config set AUTH_KEY", "the salts have to be written locally instead"},
		{"config set NONCE_SALT", "all eight of them"},
		{"core install --url=https://", "without this there are no database tables"},
		{"--skip-email", "a panel install must not email the customer a password it generated"},
	}
	for _, step := range steps {
		if !f.ran(step.marker) {
			t.Errorf("the install never ran %q: %s\n  commands: %v", step.marker, step.why, f.commands)
		}
	}

	// The salts are generated locally, eight of them, and they differ.
	salts := map[string]bool{}
	for _, command := range f.commands {
		if !strings.HasPrefix(command, "config set ") || !strings.Contains(command, "_KEY ") &&
			!strings.Contains(command, "_SALT ") {
			continue
		}
		fields := strings.Fields(command)
		if len(fields) >= 4 {
			salts[fields[3]] = true
		}
	}
	if len(salts) != 8 {
		t.Fatalf("%d distinct salts were written, want 8 - one per WordPress auth constant", len(salts))
	}

	// Every step ran as the site's user, never as the panel.
	for i, record := range f.launched {
		if record.uid == 0 || record.gid == 0 {
			t.Fatalf("install step %d (%q) ran as root", i, f.commands[i])
		}
	}
	if !strings.Contains(result.RanAs, "uid 4201") {
		t.Fatalf("the install reports it ran as %q", result.RanAs)
	}

	// The file modes the web server needs, and no more.
	for _, want := range []string{"site-installer", "wp-config.php 600", "uploads 770"} {
		if !strings.Contains(result.Ownership, want) {
			t.Errorf("the reported ownership %q does not mention %q", result.Ownership, want)
		}
	}

	// On disk: the tree belongs to the site user and wp-config.php is not
	// readable by the web server's group.
	info, err := os.Stat(filepath.Join(result.Path, "wp-content", "uploads"))
	if err != nil {
		t.Fatalf("wp-content/uploads was not created, so WordPress cannot accept an upload: %v", err)
	}
	if info.Mode().Perm() != 0o770 {
		t.Fatalf("wp-content/uploads is %v, want 0770: PHP has to be able to write it", info.Mode().Perm())
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Fatal("wp-content/uploads is world writable")
	}
}
