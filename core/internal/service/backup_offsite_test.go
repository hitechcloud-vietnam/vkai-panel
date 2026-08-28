package service

// The end-to-end test for offsite backup, driven against a real PostgreSQL.
//
// It exists because the alternative - unit tests around a mocked repository -
// is what let four features ship connected to nothing. Everything here goes
// through the real repository, the real schema from
// migrations/pending/backup.sql, the real archive format and a real
// destination on disk. A backup is taken, an artifact row is written, a dry
// run reports what it would overwrite, a restore puts the files back, a
// verification pass proves the archive restores, a corrupted object is caught,
// and retention deletes the right generations.
//
// It needs a database. Point VKAI_TEST_DATABASE_URL at one with the migrations
// applied and it runs; without it the test skips and says so, because a test
// that silently passes when it did not run is worse than one that is missing.

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/backup"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// EnvTestDatabaseURL points the database-backed tests at a PostgreSQL that has
// the migrations applied.
const EnvTestDatabaseURL = "VKAI_TEST_DATABASE_URL"

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(EnvTestDatabaseURL))
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL with the migrations applied to run this test", EnvTestDatabaseURL)
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("cannot connect to %s: %v", EnvTestDatabaseURL, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Fail loudly rather than skipping if the schema is not there: the point
	// of this test is the schema.
	var exists bool
	if err := db.Get(&exists, `SELECT to_regclass('public.backup_artifacts') IS NOT NULL`); err != nil {
		t.Fatalf("cannot inspect the schema: %v", err)
	}
	if !exists {
		t.Fatalf("the database at %s has not had migrations/pending/backup.sql applied", EnvTestDatabaseURL)
	}
	return db
}

// testTenant creates a tenant to hang everything off, and removes it after.
func testTenant(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	name := "backup-test-" + id.String()[:8]
	_, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active')`,
		id, name, name)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		// The backup tables cascade from tenants, so one delete is enough.
		_, _ = db.Exec(`DELETE FROM backup_artifacts WHERE tenant_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM backup_job_settings WHERE tenant_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM backup_destinations WHERE tenant_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM backup_records WHERE tenant_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM backup_jobs WHERE tenant_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = $1`, id)
	})
	return id
}

// backupTestRig is a panel root laid out the way an installation is, a service
// wired to a real repository, and a key the operator holds.
type backupTestRig struct {
	svc       *BackupService
	repo      *repository.BackupRepository
	db        *sqlx.DB
	tenantID  uuid.UUID
	panelRoot string
	keyDir    string
	key       *backup.Key
}

func newBackupTestRig(t *testing.T) *backupTestRig {
	t.Helper()
	db := openTestDB(t)

	panelRoot := t.TempDir()
	// The key lives outside the panel root, which is what LoadKey insists on:
	// a key stored beside the archives protects nothing.
	keyDir := t.TempDir()

	t.Setenv(config.EnvPanelRoot, panelRoot)
	for _, dir := range []string{"www/domains", "www/backup", "tmp", "etc"} {
		if err := os.MkdirAll(filepath.Join(panelRoot, dir), 0o750); err != nil {
			t.Fatalf("lay out the panel root: %v", err)
		}
	}

	// The key material is fixed rather than random so that a failure is
	// reproducible; the package does not export a key's bytes, which is the
	// right design, so the test builds the key from the bytes it writes.
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i * 7)
	}
	key, err := backup.NewKey(raw)
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	keyFile := filepath.Join(keyDir, "backup.key")
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(raw)), 0o400); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	t.Setenv(backup.EnvKeyFile, keyFile)
	t.Setenv(backup.EnvKey, "")

	repo := repository.NewBackupRepository(db)
	svc := NewBackupService(repo)
	// No PostgreSQL superuser in a test, so the scratch importer is replaced
	// by one that reads the dump it is handed. The real importer is
	// postgresScratchImporter and is what runs on a panel.
	svc.SetDatabaseImporter(func(ctx context.Context, path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "CREATE TABLE") {
			return fmt.Errorf("the dump does not look like a dump")
		}
		return nil
	})

	return &backupTestRig{
		svc:       svc,
		repo:      repo,
		db:        db,
		tenantID:  testTenant(t, db),
		panelRoot: panelRoot,
		keyDir:    keyDir,
		key:       key,
	}
}

// site creates a website tree under the web root and returns its resource id.
func (r *backupTestRig) site(t *testing.T, files map[string]string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	root := filepath.Join(config.WebRoot(), id.String())
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return id
}

// createJob inserts the backup_jobs row directly rather than going through
// BackupService.CreateJob.
//
// Not for convenience: backup_jobs.destination is VARCHAR(50) in migration
// 001, and CreateJob stores the validated absolute backup root in it. Under a
// test panel root - or under any installation whose VKAI_BACKUP_ROOT is more
// than about forty characters, or that uses config.SiteBackupDir's per-domain
// subdirectory - that value does not fit and the insert fails. That is a real
// latent bug in the pre-existing local backup path, reported rather than
// worked around in production code, and it is not what this test is about; the
// offsite path never reads that column.
func (r *backupTestRig) createJob(t *testing.T, name, jobType string, resourceID uuid.UUID) *models.BackupJob {
	t.Helper()
	job := &models.BackupJob{
		ID:          uuid.New(),
		TenantID:    r.tenantID,
		Name:        name,
		Type:        jobType,
		ResourceID:  resourceID,
		Destination: "/tmp/legacy",
		Retention:   30,
		Status:      "active",
	}
	// schedule is written as '' rather than left NULL. models.BackupJob.Schedule
	// is a plain string and repository.GetJobByID reads the row with SELECT *,
	// so a NULL in that column makes every read of the job fail with "converting
	// NULL to string is unsupported". The panel's own CreateJob always writes ''
	// so it does not hit this today; it is the second latent SELECT * hazard
	// this test found and it is reported rather than fixed here.
	_, err := r.db.Exec(`
		INSERT INTO backup_jobs (id, tenant_id, name, type, resource_id, destination, schedule, retention, encrypted, status)
		VALUES ($1, $2, $3, $4, $5, $6, '', $7, TRUE, 'active')`,
		job.ID, job.TenantID, job.Name, job.Type, job.ResourceID, job.Destination, job.Retention)
	if err != nil {
		t.Fatalf("insert backup job: %v", err)
	}
	return job
}

func (r *backupTestRig) localDestination(t *testing.T, name string) *backup.DestinationRecord {
	t.Helper()
	dest, err := r.svc.CreateDestination(context.Background(), r.tenantID, &CreateDestinationRequest{
		Name:      name,
		Kind:      backup.DestinationLocal,
		LocalRoot: filepath.Join(config.BackupRoot(), name),
	})
	if err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	return dest
}

// waitForOperation blocks until an operation finishes, and returns its final
// state. A backup that never finishes is a test that hangs, so it gives up.
func (r *backupTestRig) waitForOperation(t *testing.T, id uuid.UUID) Operation {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		op, err := r.svc.GetOperation(r.tenantID, id)
		if err != nil {
			t.Fatalf("the operation vanished before it finished: %v", err)
		}
		switch op.Progress.Phase {
		case backup.PhaseDone, backup.PhaseFailed, backup.PhaseCancelled:
			return *op
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the operation did not finish within a minute")
	return Operation{}
}

func TestOffsiteBackupRestoreAndVerifyEndToEnd(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	resourceID := rig.site(t, map[string]string{
		"index.php":            "<?php echo 'live site';",
		"wp-config.php":        "define('DB_NAME','shop');",
		"uploads/photo.jpg":    strings.Repeat("\xff\xd8\xff", 4000),
		"cache/nested/tmp.txt": "cache",
	})
	job := rig.createJob(t, "shop website", "website", resourceID)
	dest := rig.localDestination(t, "primary")

	// ---- configure ------------------------------------------------------
	settings, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{
		DestinationID: dest.ID,
	})
	if err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}
	if !settings.Encrypt {
		t.Fatal("backups default to unencrypted")
	}
	if settings.EncryptionKeyID != rig.key.ID() {
		t.Fatalf("the job recorded key id %q, the operator holds %q", settings.EncryptionKeyID, rig.key.ID())
	}
	if settings.RetentionClass != backup.KindWebsite {
		t.Fatalf("retention class is %q", settings.RetentionClass)
	}

	// ---- probe the destination ------------------------------------------
	if err := rig.svc.ProbeDestination(ctx, rig.tenantID, dest.ID); err != nil {
		t.Fatalf("ProbeDestination: %v", err)
	}
	probed, err := rig.svc.GetDestination(ctx, rig.tenantID, dest.ID)
	if err != nil {
		t.Fatalf("GetDestination: %v", err)
	}
	if probed.LastProbeOK == nil || !*probed.LastProbeOK {
		t.Fatalf("the probe result was not recorded: %+v", probed)
	}

	// ---- take the backup -------------------------------------------------
	op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("RunOffsiteBackup: %v", err)
	}
	final := rig.waitForOperation(t, op.ID)
	if final.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the backup did not succeed: phase=%s error=%s", final.Progress.Phase, final.Error)
	}

	artifacts, err := rig.svc.ListArtifacts(ctx, rig.tenantID, 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("the backup produced %d artifacts, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if !artifact.Encrypted || artifact.EncryptionKeyID != rig.key.ID() {
		t.Fatalf("the artifact is not encrypted under the operator's key: %+v", artifact)
	}
	if artifact.FileCount != 4 {
		t.Fatalf("the artifact records %d files, the site has 4", artifact.FileCount)
	}

	// The object is really on disk, and it really is ciphertext.
	objectPath := filepath.Join(config.BackupRoot(), "primary", filepath.FromSlash(artifact.ObjectKey))
	raw, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatalf("the archive is not where the panel says it is: %v", err)
	}
	if int64(len(raw)) != artifact.SizeBytes {
		t.Fatalf("the archive is %d bytes, the record says %d", len(raw), artifact.SizeBytes)
	}
	if strings.Contains(string(raw), "DB_NAME") {
		t.Fatal("the archive on disk contains site content in the clear")
	}

	// ---- dry run ---------------------------------------------------------
	// Change the live site first, so the plan has something to report.
	livePath := filepath.Join(config.WebRoot(), resourceID.String(), "index.php")
	if err := os.WriteFile(livePath, []byte("<?php echo 'edited since the backup';"), 0o644); err != nil {
		t.Fatalf("edit the live site: %v", err)
	}

	planned, _, err := rig.svc.Restore(ctx, rig.tenantID, &RestoreRequest{
		ArtifactID: artifact.ID,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !planned.DryRun || planned.Status != backup.RestorePlanned {
		t.Fatalf("the dry run recorded status %q dry_run=%v", planned.Status, planned.DryRun)
	}
	if planned.OverwritesChanged != 1 {
		t.Fatalf("the dry run reports %d changed overwrites, want exactly index.php", planned.OverwritesChanged)
	}
	if planned.Overwrites != 4 {
		t.Fatalf("the dry run reports %d overwrites in total, want 4", planned.Overwrites)
	}
	if !strings.Contains(string(planned.Plan), "index.php") {
		t.Fatal("the stored plan does not name the file that would be overwritten")
	}
	if data, _ := os.ReadFile(livePath); !strings.Contains(string(data), "edited since the backup") {
		t.Fatal("the dry run modified the live site")
	}

	// ---- a real restore that would overwrite is refused without consent ---
	if _, _, err := rig.svc.Restore(ctx, rig.tenantID, &RestoreRequest{
		ArtifactID: artifact.ID,
		DryRun:     false,
	}); err == nil {
		// The refusal happens inside the operation, so the call itself
		// succeeds; the operation must then fail.
		restores, listErr := rig.svc.ListRestores(ctx, rig.tenantID, 10)
		if listErr != nil {
			t.Fatalf("ListRestores: %v", listErr)
		}
		waitFor(t, func() bool {
			refreshed, err := rig.svc.GetRestore(ctx, rig.tenantID, restores[0].ID)
			return err == nil && refreshed.Status == backup.RestoreFailed
		}, "the restore that would overwrite a live file was not refused")
	}
	if data, _ := os.ReadFile(livePath); !strings.Contains(string(data), "edited since the backup") {
		t.Fatal("a restore that should have been refused changed the live site")
	}

	// ---- the real restore ------------------------------------------------
	record, restoreOp, err := rig.svc.Restore(ctx, rig.tenantID, &RestoreRequest{
		ArtifactID:     artifact.ID,
		DryRun:         false,
		AllowOverwrite: true,
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restoreOp == nil {
		t.Fatal("a real restore did not return an operation to watch")
	}
	restoreFinal := rig.waitForOperation(t, restoreOp.ID)
	if restoreFinal.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the restore did not succeed: %s %s", restoreFinal.Progress.Phase, restoreFinal.Error)
	}
	if data, _ := os.ReadFile(livePath); !strings.Contains(string(data), "live site") {
		t.Fatal("the restore did not put the original file back")
	}

	waitFor(t, func() bool {
		refreshed, err := rig.svc.GetRestore(ctx, rig.tenantID, record.ID)
		return err == nil && refreshed.Status == backup.RestoreCompleted && refreshed.FilesWritten == 4
	}, "the completed restore was not recorded with what it wrote")

	// ---- verification ----------------------------------------------------
	verification, err := rig.svc.VerifyArtifact(ctx, rig.tenantID, artifact.ID)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if verification.Status != backup.VerifyPassed {
		t.Fatalf("a good archive failed verification: %s", string(verification.Details))
	}
	if verification.ChecksumsChecked != 4 || verification.FilesRestored != 4 {
		t.Fatalf("the pass checked %d checksums over %d files", verification.ChecksumsChecked, verification.FilesRestored)
	}
	if verification.ArchiveSHA256 != artifact.SHA256 {
		t.Fatalf("the pass verified a different object than the one recorded")
	}

	// The verdict must be stamped where retention and the operator can see it.
	stamped, err := rig.svc.GetArtifact(ctx, rig.tenantID, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if stamped.LastVerifyStatus != backup.VerifyPassed || stamped.LastVerifiedAt == nil {
		t.Fatalf("the artifact was not stamped with its verification: %+v", stamped)
	}
	health, err := rig.svc.BackupHealth(ctx, rig.tenantID)
	if err != nil {
		t.Fatalf("BackupHealth: %v", err)
	}
	if health.Artifacts != 1 || health.Verified != 1 || health.NeverVerified != 0 {
		t.Fatalf("backup health is %+v", health)
	}

	// ---- and the pass has to catch a corrupted archive -------------------
	corrupt := append([]byte(nil), raw...)
	corrupt[len(corrupt)/2] ^= 0x80
	if err := os.WriteFile(objectPath, corrupt, 0o600); err != nil {
		t.Fatalf("corrupt the archive: %v", err)
	}

	failed, err := rig.svc.VerifyArtifact(ctx, rig.tenantID, artifact.ID)
	if err != nil {
		t.Fatalf("VerifyArtifact on a corrupted archive: %v", err)
	}
	if failed.Status != backup.VerifyFailed {
		t.Fatal("a corrupted archive passed the restorability check")
	}
	afterFailure, err := rig.svc.GetArtifact(ctx, rig.tenantID, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if afterFailure.LastVerifyStatus != backup.VerifyFailed {
		t.Fatal("the failed verification was not recorded against the artifact")
	}
}

func TestOffsiteBackupRetentionKeepsTheNewestAndTheNewestVerified(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	resourceID := rig.site(t, map[string]string{"index.php": "site"})
	job := rig.createJob(t, "retention site", "website", resourceID)
	dest := rig.localDestination(t, "retention")

	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{
		DestinationID:   dest.ID,
		KeepGenerations: intPtr(2),
		KeepDays:        intPtr(0),
		MinKeep:         intPtr(1),
	}); err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}

	// Five generations, a second apart so their object keys differ.
	var artifacts []backup.Artifact
	for i := 0; i < 5; i++ {
		rig.svc.offsite().now = fixedClock(time.Now().Add(time.Duration(i) * time.Second))
		op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		final := rig.waitForOperation(t, op.ID)
		if final.Progress.Phase != backup.PhaseDone {
			t.Fatalf("backup %d failed: %s", i, final.Error)
		}
		listed, err := rig.svc.ListArtifacts(ctx, rig.tenantID, 10)
		if err != nil {
			t.Fatalf("ListArtifacts: %v", err)
		}
		artifacts = listed
	}

	// Retention ran after each backup, so only the newest two survive.
	if len(artifacts) != 2 {
		t.Fatalf("retention left %d generations, the policy keeps 2", len(artifacts))
	}
	// ListArtifacts is newest first.
	newest := artifacts[0]

	// The objects retention deleted must be gone from the destination too, not
	// just from the database.
	entries, err := os.ReadDir(filepath.Join(config.BackupRoot(), "retention", rig.tenantID.String(), backup.KindWebsite, resourceID.String()))
	if err != nil {
		t.Fatalf("read the destination: %v", err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the destination still holds %d archives: %v", len(entries), names)
	}

	// And the newest is one of the survivors, which is the rule that holds
	// whatever the policy says.
	found := false
	for _, e := range entries {
		if strings.HasSuffix(newest.ObjectKey, e.Name()) {
			found = true
		}
	}
	if !found {
		t.Fatal("the newest generation is not among the archives left at the destination")
	}
}

func TestConfigureJobRefusesWhenNoEncryptionKeyIsAvailable(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	// Take the key away, the way an operator who has not set one up yet has it.
	t.Setenv(backup.EnvKeyFile, "")
	t.Setenv(backup.EnvKey, "")
	rig.svc.offsite().keyOptions = backup.LoadKeyOptions{ForbiddenRoot: config.BackupRoot()}

	resourceID := rig.site(t, map[string]string{"index.php": "site"})
	job := rig.createJob(t, "no key", "website", resourceID)
	dest := rig.localDestination(t, "nokey")

	_, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{DestinationID: dest.ID})
	if err == nil {
		t.Fatal("a job was configured to encrypt with no key available")
	}
	if !strings.Contains(err.Error(), "no encryption key configured") {
		t.Fatalf("the error does not tell the operator what to do: %v", err)
	}

	// Explicitly unencrypted is allowed - it just has to be chosen.
	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{
		DestinationID: dest.ID,
		Encrypt:       boolPtr(false),
	}); err != nil {
		t.Fatalf("an explicitly unencrypted job was refused: %v", err)
	}
}

func TestRestoreWithTheWrongKeyIsRefusedByName(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	resourceID := rig.site(t, map[string]string{"index.php": "site"})
	job := rig.createJob(t, "wrong key", "website", resourceID)
	dest := rig.localDestination(t, "wrongkey")
	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{DestinationID: dest.ID}); err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}

	op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("RunOffsiteBackup: %v", err)
	}
	if final := rig.waitForOperation(t, op.ID); final.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the backup failed: %s", final.Error)
	}
	artifacts, err := rig.svc.ListArtifacts(ctx, rig.tenantID, 1)
	if err != nil || len(artifacts) == 0 {
		t.Fatalf("ListArtifacts: %v", err)
	}

	// The operator now holds a different key.
	otherKeyFile := filepath.Join(rig.keyDir, "other.key")
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(200 - i)
	}
	if err := os.WriteFile(otherKeyFile, []byte(hex.EncodeToString(other)), 0o400); err != nil {
		t.Fatalf("write the other key: %v", err)
	}
	t.Setenv(backup.EnvKeyFile, otherKeyFile)

	_, _, err = rig.svc.Restore(ctx, rig.tenantID, &RestoreRequest{ArtifactID: artifacts[0].ID, DryRun: true})
	if err == nil {
		t.Fatal("a restore proceeded with the wrong key")
	}
	if !strings.Contains(err.Error(), artifacts[0].EncryptionKeyID) {
		t.Fatalf("the error does not name the key the archive needs: %v", err)
	}
	if !strings.Contains(err.Error(), "no way to recover it without that key") {
		t.Fatalf("the error does not tell the operator the truth about their position: %v", err)
	}

	// Verification of an archive whose key is gone must be recorded as a
	// failure, not swallowed: an archive nobody can decrypt is not a backup.
	verification, err := rig.svc.VerifyArtifact(ctx, rig.tenantID, artifacts[0].ID)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if verification.Status != backup.VerifyFailed {
		t.Fatal("an archive that cannot be decrypted was verified as good")
	}
}

func TestS3DestinationStoresItsSecretEncrypted(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	secretKey := make([]byte, 32)
	for i := range secretKey {
		secretKey[i] = byte(i)
	}
	t.Setenv("VKAI_SECRET_KEY", hex.EncodeToString(secretKey))

	const plaintextSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	dest, err := rig.svc.CreateDestination(ctx, rig.tenantID, &CreateDestinationRequest{
		Name:              "offsite bucket",
		Kind:              backup.DestinationS3,
		S3Endpoint:        "https://s3.eu-west-1.amazonaws.com",
		S3Region:          "eu-west-1",
		S3Bucket:          "vkai-backups",
		S3AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		S3SecretAccessKey: plaintextSecret,
	})
	if err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	if !dest.Offsite() {
		t.Fatal("an S3 destination does not report itself as offsite")
	}

	// The secret must not be in the column as written, and must not be on the
	// struct the API returns.
	var stored string
	if err := rig.db.Get(&stored, `SELECT s3_secret_key_enc FROM backup_destinations WHERE id = $1`, dest.ID); err != nil {
		t.Fatalf("read the stored secret: %v", err)
	}
	if strings.Contains(stored, plaintextSecret) || stored == "" {
		t.Fatal("the S3 secret access key was stored in the clear")
	}

	listed, err := rig.svc.ListDestinations(ctx, rig.tenantID)
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	for _, d := range listed {
		encoded := fmt.Sprintf("%+v", d)
		if strings.Contains(encoded, plaintextSecret) {
			t.Fatal("a destination returned by the API carries its secret access key")
		}
	}

	// And it round-trips: opening the destination decrypts it.
	if _, err := rig.svc.openDestination(ctx, rig.tenantID, dest.ID); err != nil {
		t.Fatalf("the stored destination could not be opened: %v", err)
	}
}

func TestOffsiteBackupCanBeCancelled(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	// Enough data that the backup is still running when the cancel arrives.
	files := map[string]string{}
	for i := 0; i < 40; i++ {
		files[fmt.Sprintf("file-%02d", i)] = strings.Repeat("x", 400_000)
	}
	resourceID := rig.site(t, files)
	job := rig.createJob(t, "big site", "website", resourceID)
	dest := rig.localDestination(t, "cancel")
	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{DestinationID: dest.ID}); err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}

	op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("RunOffsiteBackup: %v", err)
	}
	if err := rig.svc.CancelOperation(rig.tenantID, op.ID); err != nil {
		t.Fatalf("CancelOperation: %v", err)
	}

	final := rig.waitForOperation(t, op.ID)
	if final.Progress.Phase != backup.PhaseCancelled {
		t.Fatalf("a cancelled backup finished as %q", final.Progress.Phase)
	}

	// A cancelled backup must leave no artifact: a half-uploaded object that
	// the panel counts as a generation is how retention deletes a good backup
	// to make room for a broken one.
	artifacts, err := rig.svc.ListArtifacts(ctx, rig.tenantID, 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("a cancelled backup recorded %d artifacts", len(artifacts))
	}
}

func TestVerifyDuePicksUpJobsThatHaveNeverBeenChecked(t *testing.T) {
	rig := newBackupTestRig(t)
	ctx := context.Background()

	resourceID := rig.site(t, map[string]string{"index.php": "site"})
	job := rig.createJob(t, "due site", "website", resourceID)
	dest := rig.localDestination(t, "due")
	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{
		DestinationID:       dest.ID,
		VerifyIntervalHours: intPtr(24),
	}); err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}

	op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("RunOffsiteBackup: %v", err)
	}
	if final := rig.waitForOperation(t, op.ID); final.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the backup failed: %s", final.Error)
	}

	// A job that has never been verified is due immediately.
	results, err := rig.svc.VerifyDue(ctx, 50)
	if err != nil {
		t.Fatalf("VerifyDue: %v", err)
	}
	mine := 0
	for _, r := range results {
		if r.TenantID == rig.tenantID {
			mine++
			if r.Status != backup.VerifyPassed {
				t.Fatalf("the scheduled pass failed: %s", string(r.Details))
			}
		}
	}
	if mine != 1 {
		t.Fatalf("the scheduled pass verified %d of this tenant's jobs, want 1", mine)
	}

	// Having just been checked, it is no longer due.
	settings, err := rig.svc.GetJobSettings(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("GetJobSettings: %v", err)
	}
	if settings.LastVerifiedAt == nil || settings.LastVerifyStatus != backup.VerifyPassed {
		t.Fatalf("the job was not stamped: %+v", settings)
	}

	again, err := rig.svc.VerifyDue(ctx, 50)
	if err != nil {
		t.Fatalf("VerifyDue: %v", err)
	}
	for _, r := range again {
		if r.TenantID == rig.tenantID {
			t.Fatal("a job verified moments ago is still reported as due")
		}
	}
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(message)
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

// ============================================================
// The database path, against a real PostgreSQL
// ============================================================
//
// Everything above stubs the scratch-database importer, because a unit test
// should not need a superuser. These two do not stub it. They run
// postgresScratchImporter - the function a panel actually runs - against the
// system cluster through sudo -u postgres, which is how every other database
// operation in this panel reaches PostgreSQL.
//
// That matters because "the database imports" is the headline claim of the
// verification pass for a database backup, and a claim tested only against a
// function literal that returns nil is not tested at all.

// requireSuperuserPostgres skips when there is no local cluster this process
// can administer.
func requireSuperuserPostgres(t *testing.T) {
	t.Helper()
	cmd := exec.Command("sudo", "-n", "-u", "postgres", "psql", "-tAc", "SELECT 1")
	if err := cmd.Run(); err != nil {
		t.Skip("no local PostgreSQL reachable as sudo -u postgres; skipping the real database path")
	}
}

func TestPostgresScratchImporterImportsAndCleansUp(t *testing.T) {
	requireSuperuserPostgres(t)

	dir := t.TempDir()
	good := filepath.Join(dir, "good.sql")
	if err := os.WriteFile(good, []byte("CREATE TABLE orders (id int primary key, total numeric);\nINSERT INTO orders VALUES (1, 9.99);\n"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	before := scratchDatabaseCount(t)

	if err := postgresScratchImporter(context.Background(), good); err != nil {
		t.Fatalf("a valid dump did not import: %v", err)
	}

	bad := filepath.Join(dir, "bad.sql")
	if err := os.WriteFile(bad, []byte("CREATE TABLE orders (id int);\nCREAT TABLE broken (id int);\n"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	err := postgresScratchImporter(context.Background(), bad)
	if err == nil {
		t.Fatal("a dump with a syntax error in it imported cleanly")
	}
	if !strings.Contains(err.Error(), "did not import") {
		t.Fatalf("the failure does not say what happened: %v", err)
	}

	// Both runs must have taken their scratch database with them, including
	// the one that failed half way through.
	if after := scratchDatabaseCount(t); after != before {
		t.Fatalf("the importer left %d scratch database(s) behind", after-before)
	}
}

func scratchDatabaseCount(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("sudo", "-n", "-u", "postgres", "psql", "-tAc",
		"SELECT count(*) FROM pg_database WHERE datname LIKE 'vkai_verify_%'").Output()
	if err != nil {
		t.Fatalf("count scratch databases: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("count scratch databases: %q: %v", string(out), err)
	}
	return count
}

func TestDatabaseBackupIsDumpedEncryptedAndProvedToImport(t *testing.T) {
	requireSuperuserPostgres(t)
	rig := newBackupTestRig(t)
	ctx := context.Background()

	// A real database, named the way a backup job addresses one: by the
	// resource id of the managed database.
	resourceID := uuid.New()
	dbName := resourceID.String()
	if out, err := exec.Command("sudo", "-n", "-u", "postgres", "createdb", "--", dbName).CombinedOutput(); err != nil {
		t.Fatalf("createdb: %s: %v", string(out), err)
	}
	t.Cleanup(func() {
		_ = exec.Command("sudo", "-n", "-u", "postgres", "dropdb", "--if-exists", "--", dbName).Run()
	})

	seed := "CREATE TABLE orders (id int primary key, customer text);\nINSERT INTO orders VALUES (1, 'a customer whose data must survive');\n"
	seedCmd := exec.Command("sudo", "-n", "-u", "postgres", "psql", "-v", "ON_ERROR_STOP=1", "-q", "-d", dbName)
	seedCmd.Stdin = strings.NewReader(seed)
	if out, err := seedCmd.CombinedOutput(); err != nil {
		t.Fatalf("seed the database: %s: %v", string(out), err)
	}

	// The real importer, not the stub the other tests use.
	rig.svc.SetDatabaseImporter(postgresScratchImporter)

	job := rig.createJob(t, "orders database", "database", resourceID)
	dest := rig.localDestination(t, "dbdest")
	if _, err := rig.svc.ConfigureJob(ctx, rig.tenantID, job.ID, &ConfigureJobRequest{
		DestinationID:  dest.ID,
		RetentionClass: backup.KindDatabase,
	}); err != nil {
		t.Fatalf("ConfigureJob: %v", err)
	}

	op, err := rig.svc.RunOffsiteBackup(ctx, rig.tenantID, job.ID)
	if err != nil {
		t.Fatalf("RunOffsiteBackup: %v", err)
	}
	if final := rig.waitForOperation(t, op.ID); final.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the database backup failed: %s", final.Error)
	}

	artifacts, err := rig.svc.ListArtifacts(ctx, rig.tenantID, 5)
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("ListArtifacts: %v %+v", err, artifacts)
	}
	artifact := artifacts[0]
	if artifact.RetentionClass != backup.KindDatabase || artifact.FileCount != 1 {
		t.Fatalf("the artifact is not a one-file database backup: %+v", artifact)
	}

	// The dump must not be readable at the destination.
	objectPath := filepath.Join(config.BackupRoot(), "dbdest", filepath.FromSlash(artifact.ObjectKey))
	raw, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatalf("read the archive: %v", err)
	}
	if strings.Contains(string(raw), "a customer whose data must survive") {
		t.Fatal("the database dump is readable at the destination")
	}

	// The verification pass restores it and imports it into a scratch
	// database. This is the assertion the whole task is about.
	verification, err := rig.svc.VerifyArtifact(ctx, rig.tenantID, artifact.ID)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if verification.Status != backup.VerifyPassed {
		t.Fatalf("the database backup failed verification: %s", string(verification.Details))
	}
	if !verification.DatabaseChecked || !verification.DatabaseImported {
		t.Fatalf("the dump was not actually imported: checked=%v imported=%v error=%q",
			verification.DatabaseChecked, verification.DatabaseImported, verification.DatabaseError)
	}

	// And a restore puts a usable dump back on disk, containing the data.
	target := filepath.Join(t.TempDir(), "restored-dump")
	record, restoreOp, err := rig.svc.Restore(ctx, rig.tenantID, &RestoreRequest{
		ArtifactID:     artifact.ID,
		TargetPath:     target,
		DryRun:         false,
		AllowOverwrite: true,
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if final := rig.waitForOperation(t, restoreOp.ID); final.Progress.Phase != backup.PhaseDone {
		t.Fatalf("the restore failed: %s", final.Error)
	}
	restored, err := os.ReadFile(filepath.Join(target, dbName+".sql"))
	if err != nil {
		t.Fatalf("the restored dump is not where the restore said it went: %v", err)
	}
	if !strings.Contains(string(restored), "a customer whose data must survive") {
		t.Fatal("the restored dump does not contain the data that was backed up")
	}
	_ = record
}
