package service

// The offsite half of the backup service: destinations, encrypted archives,
// one-action restore with a dry run, and the pass that proves a backup is
// restorable.
//
// It is a separate file from backup.go because backup.go is the original
// local-tar implementation that the existing /backups routes drive, and this
// is additive: an installation that has not applied
// migrations/pending/backup.sql keeps working exactly as before, and every
// method here fails with a clear message rather than a SQL error.
//
// The service holds no long-lived state except the registry of running
// operations. Everything else is read back from the database on each call, so
// two panel processes behind a load balancer see the same thing.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/backup"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// JobRecorder is the slice of the job queue's repository this service uses.
//
// It is an interface, and it is optional, for two reasons. The job queue's
// rows are owned by internal/repository/job.go and duplicating its INSERT here
// is exactly the kind of second definition that this project has already paid
// for once. And a backup must not stop working because the queue is not wired:
// with no recorder attached, operations still run and still report progress
// through this service, they simply do not appear in GET /jobs.
type JobRecorder interface {
	CreateJob(ctx context.Context, record *job.JobRecord) error
	UpdateJobStarted(ctx context.Context, id uuid.UUID) error
	UpdateJobCompleted(ctx context.Context, id uuid.UUID, result []byte) error
	UpdateJobFailed(ctx context.Context, id uuid.UUID, errMsg string) error
	UpdateJobStatus(ctx context.Context, id uuid.UUID, status string, result []byte, errMsg string) error
}

// Operation is a running backup, restore or verification.
type Operation struct {
	ID       uuid.UUID       `json:"id"`
	TenantID uuid.UUID       `json:"tenant_id"`
	Kind     string          `json:"kind"`
	JobID    *uuid.UUID      `json:"job_id,omitempty"`
	Target   string          `json:"target,omitempty"`
	Progress backup.Progress `json:"progress"`
	Error    string          `json:"error,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// Operation kinds.
const (
	OperationBackup  = "backup"
	OperationRestore = "restore"
	OperationVerify  = "verify"
)

type runningOperation struct {
	meta    Operation
	tracker *backup.Tracker
}

// operationRegistry holds the operations this process is running.
//
// Progress lives in memory because that is where it is generated, tens of
// times a second, and writing every update to Postgres would turn a backup
// into a write amplifier. What reaches the database is the lifecycle: the row
// in jobs when the operation starts and finishes, and the durable result in
// backup_artifacts, backup_verifications or backup_restores. If the process
// dies mid-operation, the progress is gone and the job row says the operation
// never completed - which is the truth.
type operationRegistry struct {
	mu  sync.RWMutex
	ops map[uuid.UUID]*runningOperation
}

func newOperationRegistry() *operationRegistry {
	return &operationRegistry{ops: map[uuid.UUID]*runningOperation{}}
}

func (r *operationRegistry) add(op *runningOperation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops[op.meta.ID] = op
}

func (r *operationRegistry) get(tenantID, id uuid.UUID) (*runningOperation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.ops[id]
	if !ok || op.meta.TenantID != tenantID {
		return nil, false
	}
	return op, true
}

func (r *operationRegistry) list(tenantID uuid.UUID) []Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Operation{}
	for _, op := range r.ops {
		if op.meta.TenantID != tenantID {
			continue
		}
		snapshot := op.meta
		snapshot.Progress = op.tracker.Snapshot()
		out = append(out, snapshot)
	}
	return out
}

func (r *operationRegistry) update(id uuid.UUID, fn func(*Operation)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if op, ok := r.ops[id]; ok {
		fn(&op.meta)
	}
}

// forget drops an operation once nothing will ask about it again. Finished
// operations are kept for a grace period so that a UI polling every few
// seconds still sees the final state.
func (r *operationRegistry) forget(id uuid.UUID, after time.Duration) {
	time.AfterFunc(after, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.ops, id)
	})
}

// finishedOperationGrace is how long a completed operation stays visible.
const finishedOperationGrace = 10 * time.Minute

// ============================================================
// Wiring
// ============================================================

// offsite is the state this file adds to BackupService. It is a pointer on the
// struct in backup.go so that NewBackupService keeps its one-argument
// signature and cmd/api/main.go does not have to change to get any of this.
type offsiteState struct {
	logger     *zap.Logger
	ops        *operationRegistry
	jobs       JobRecorder
	keyOptions backup.LoadKeyOptions
	now        func() time.Time

	// importer runs a restored dump into a throwaway database during
	// verification. It is a field so a test can supply one that does not need
	// a PostgreSQL server.
	importer backup.DatabaseImporter
}

func newOffsiteState() *offsiteState {
	return &offsiteState{
		logger: zap.NewNop(),
		ops:    newOperationRegistry(),
		now:    time.Now,
		keyOptions: backup.LoadKeyOptions{
			ForbiddenRoot: config.BackupRoot(),
		},
		importer: postgresScratchImporter,
	}
}

func (s *BackupService) offsite() *offsiteState {
	s.offsiteOnce.Do(func() {
		if s.offsiteState == nil {
			s.offsiteState = newOffsiteState()
		}
	})
	return s.offsiteState
}

// SetLogger gives the service a logger. Without it, it logs nowhere.
func (s *BackupService) SetLogger(logger *zap.Logger) {
	if logger == nil {
		return
	}
	s.offsite().logger = logger
}

// AttachJobQueue makes long-running backup operations visible in the shared
// jobs table, so they appear in GET /api/v1/jobs alongside everything else.
//
// It is one line in cmd/api/main.go and the feature works without it.
func (s *BackupService) AttachJobQueue(jobs JobRecorder) {
	s.offsite().jobs = jobs
}

// SetDatabaseImporter replaces the importer the verification pass uses.
func (s *BackupService) SetDatabaseImporter(importer backup.DatabaseImporter) {
	s.offsite().importer = importer
}

// ============================================================
// Destinations
// ============================================================

// CreateDestinationRequest is the API shape for adding a destination.
type CreateDestinationRequest struct {
	Name string `json:"name" binding:"required,max=128"`
	Kind string `json:"kind" binding:"required,oneof=local s3"`

	LocalRoot string `json:"local_root" binding:"omitempty,max=512"`

	S3Endpoint        string `json:"s3_endpoint" binding:"omitempty,max=512"`
	S3Region          string `json:"s3_region" binding:"omitempty,max=64"`
	S3Bucket          string `json:"s3_bucket" binding:"omitempty,max=255"`
	S3Prefix          string `json:"s3_prefix" binding:"omitempty,max=512"`
	S3AccessKeyID     string `json:"s3_access_key_id" binding:"omitempty,max=255"`
	S3SecretAccessKey string `json:"s3_secret_access_key" binding:"omitempty,max=512"`
	S3PathStyle       bool   `json:"s3_path_style"`
}

// CreateDestination validates a destination, encrypts its secret and stores it.
//
// The configuration is turned into a live Destination before it is written, so
// a bucket with no region or a local root outside the backup tree is refused
// here rather than at 03:00 on the night it is first used.
func (s *BackupService) CreateDestination(ctx context.Context, tenantID uuid.UUID, req *CreateDestinationRequest) (*backup.DestinationRecord, error) {
	record := &backup.DestinationRecord{
		TenantID:      tenantID,
		Name:          strings.TrimSpace(req.Name),
		Kind:          req.Kind,
		S3Endpoint:    strings.TrimSpace(req.S3Endpoint),
		S3Region:      strings.TrimSpace(req.S3Region),
		S3Bucket:      strings.TrimSpace(req.S3Bucket),
		S3Prefix:      strings.Trim(strings.TrimSpace(req.S3Prefix), "/"),
		S3AccessKeyID: strings.TrimSpace(req.S3AccessKeyID),
		S3PathStyle:   req.S3PathStyle,
	}
	if record.Name == "" {
		return nil, errors.New("a destination needs a name")
	}

	var secretEnc string
	switch req.Kind {
	case backup.DestinationLocal:
		root, err := validateDestination(req.LocalRoot)
		if err != nil {
			return nil, err
		}
		record.LocalRoot = root

	case backup.DestinationS3:
		if strings.TrimSpace(req.S3SecretAccessKey) == "" {
			return nil, errors.New("an S3 destination needs a secret access key")
		}
		key, err := utils.DatabaseEncryptionKey()
		if err != nil {
			return nil, fmt.Errorf("cannot store the S3 secret access key safely: %w", err)
		}
		secretEnc, err = utils.EncryptSecret(strings.TrimSpace(req.S3SecretAccessKey), key)
		if err != nil {
			return nil, fmt.Errorf("cannot encrypt the S3 secret access key: %w", err)
		}

	default:
		return nil, fmt.Errorf("unknown destination kind %q", req.Kind)
	}

	// Prove the configuration builds before it is stored.
	if _, err := s.buildDestinationFrom(record, strings.TrimSpace(req.S3SecretAccessKey)); err != nil {
		return nil, err
	}

	if err := s.backupRepo.CreateDestination(ctx, record, secretEnc); err != nil {
		return nil, fmt.Errorf("could not store the destination: %w", err)
	}
	return record, nil
}

func (s *BackupService) ListDestinations(ctx context.Context, tenantID uuid.UUID) ([]backup.DestinationRecord, error) {
	return s.backupRepo.ListDestinations(ctx, tenantID)
}

func (s *BackupService) GetDestination(ctx context.Context, tenantID, id uuid.UUID) (*backup.DestinationRecord, error) {
	return s.backupRepo.GetDestination(ctx, tenantID, id)
}

func (s *BackupService) DeleteDestination(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.backupRepo.DeleteDestination(ctx, tenantID, id)
}

// ProbeDestination writes an object, reads it back, compares it and deletes it,
// then records the outcome.
func (s *BackupService) ProbeDestination(ctx context.Context, tenantID, id uuid.UUID) error {
	dest, err := s.openDestination(ctx, tenantID, id)
	if err != nil {
		return err
	}

	probeErr := backup.Probe(ctx, dest, tenantID.String())
	message := ""
	if probeErr != nil {
		message = probeErr.Error()
	}
	if err := s.backupRepo.RecordProbe(ctx, tenantID, id, probeErr == nil, message); err != nil {
		s.offsite().logger.Warn("could not record the destination probe", zap.Error(err))
	}
	return probeErr
}

// openDestination loads a destination row and turns it into a live Destination,
// decrypting the S3 secret on the way.
func (s *BackupService) openDestination(ctx context.Context, tenantID, id uuid.UUID) (backup.Destination, error) {
	record, err := s.backupRepo.GetDestination(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	secret := ""
	if record.Kind == backup.DestinationS3 {
		encrypted, err := s.backupRepo.DestinationSecret(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		key, err := utils.DatabaseEncryptionKey()
		if err != nil {
			return nil, fmt.Errorf("cannot read the S3 secret access key: %w", err)
		}
		secret, err = utils.DecryptSecret(encrypted, key)
		if err != nil {
			return nil, fmt.Errorf("cannot decrypt the S3 secret access key for %q; was VKAI_SECRET_KEY changed?: %w", record.Name, err)
		}
	}
	return s.buildDestinationFrom(record, secret)
}

func (s *BackupService) buildDestinationFrom(record *backup.DestinationRecord, secret string) (backup.Destination, error) {
	switch record.Kind {
	case backup.DestinationLocal:
		return backup.NewLocalDestination(record.LocalRoot)
	case backup.DestinationS3:
		return backup.NewS3Destination(backup.S3Config{
			Endpoint:        record.S3Endpoint,
			Region:          record.S3Region,
			Bucket:          record.S3Bucket,
			Prefix:          record.S3Prefix,
			AccessKeyID:     record.S3AccessKeyID,
			SecretAccessKey: secret,
			PathStyle:       record.S3PathStyle,
			SpoolDir:        config.TmpRoot(),
		})
	default:
		return nil, fmt.Errorf("unknown destination kind %q", record.Kind)
	}
}

// ============================================================
// Job settings
// ============================================================

// ConfigureJobRequest is the API shape for pointing a backup job at a
// destination and setting its retention.
//
// RetentionClass is what decides WHAT is archived as well as how long it is
// kept: "website" and "files" archive the site tree under the web root,
// "database" runs pg_dump first, and "config" archives the panel's own
// configuration tree - the .env, config.yaml and panel_access.json that a
// rebuilt machine needs before it can serve anything.
//
// A config backup therefore needs a job whose retention_class is "config".
// models.CreateBackupJobRequest currently constrains `type` to
// oneof=website database files, so such a job is created with any type and
// then configured with retention_class "config". Adding "config" to that
// oneof - one word, in internal/models/models.go, which this task did not own
// - would make it a first-class choice in the UI.
type ConfigureJobRequest struct {
	DestinationID       uuid.UUID `json:"destination_id" binding:"required"`
	RetentionClass      string    `json:"retention_class" binding:"omitempty,oneof=website database files config"`
	KeepGenerations     *int      `json:"keep_generations" binding:"omitempty,min=0,max=10000"`
	KeepDays            *int      `json:"keep_days" binding:"omitempty,min=0,max=36500"`
	MinKeep             *int      `json:"min_keep" binding:"omitempty,min=1,max=10000"`
	Encrypt             *bool     `json:"encrypt"`
	VerifyIntervalHours *int      `json:"verify_interval_hours" binding:"omitempty,min=0,max=8760"`
}

// ConfigureJob attaches offsite settings to an existing backup job.
func (s *BackupService) ConfigureJob(ctx context.Context, tenantID, jobID uuid.UUID, req *ConfigureJobRequest) (*backup.JobSettings, error) {
	backupJob, err := s.backupRepo.GetJobByID(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}
	if _, err := s.backupRepo.GetDestination(ctx, tenantID, req.DestinationID); err != nil {
		return nil, err
	}

	class := req.RetentionClass
	if class == "" {
		class = retentionClassForType(backupJob.Type)
	}
	defaults := backup.DefaultRetention(class)

	settings := &backup.JobSettings{
		JobID:               jobID,
		TenantID:            tenantID,
		DestinationID:       req.DestinationID,
		RetentionClass:      class,
		KeepGenerations:     defaults.KeepGenerations,
		KeepDays:            defaults.KeepDays,
		MinKeep:             defaults.MinKeep,
		Encrypt:             true,
		VerifyIntervalHours: 168,
	}
	if req.KeepGenerations != nil {
		settings.KeepGenerations = *req.KeepGenerations
	}
	if req.KeepDays != nil {
		settings.KeepDays = *req.KeepDays
	}
	if req.MinKeep != nil {
		settings.MinKeep = *req.MinKeep
	}
	if req.Encrypt != nil {
		settings.Encrypt = *req.Encrypt
	}
	if req.VerifyIntervalHours != nil {
		settings.VerifyIntervalHours = *req.VerifyIntervalHours
	}

	// The key has to exist now, not at the first backup. Recording the key ID
	// at configuration time is also what lets a restore say which key it needs
	// even after the operator has rotated to a new one.
	if settings.Encrypt {
		key, err := backup.LoadKey(s.offsite().keyOptions)
		if err != nil {
			return nil, fmt.Errorf("this job is set to encrypt its backups but no key is available: %w", err)
		}
		settings.EncryptionKeyID = key.ID()
	}

	if err := s.backupRepo.UpsertJobSettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("could not store the job settings: %w", err)
	}
	return settings, nil
}

func (s *BackupService) GetJobSettings(ctx context.Context, tenantID, jobID uuid.UUID) (*backup.JobSettings, error) {
	return s.backupRepo.GetJobSettings(ctx, tenantID, jobID)
}

// retentionClassForType maps the backup job types migration 001 allows onto
// the retention classes. It is a function rather than a map so that an
// unrecognised type gets a defensible default instead of an empty class that
// would fail a CHECK constraint at insert time.
func retentionClassForType(jobType string) string {
	switch jobType {
	case "website":
		return backup.KindWebsite
	case "database":
		return backup.KindDatabase
	case "config":
		return backup.KindConfig
	default:
		return backup.KindFiles
	}
}

// ============================================================
// Taking a backup to a destination
// ============================================================

// RunOffsiteBackup archives a job's source, encrypts it, uploads it and records
// the artifact. It returns the operation, which carries progress and can be
// cancelled while it runs.
//
// The work happens on a goroutine so an HTTP request does not have to be held
// open for the length of a backup. Everything it needs is captured first, so
// the request's context being cancelled does not stop the backup: the
// operation has its own context, and its own cancel button.
func (s *BackupService) RunOffsiteBackup(ctx context.Context, tenantID, jobID uuid.UUID) (*Operation, error) {
	backupJob, err := s.backupRepo.GetJobByID(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}
	settings, err := s.backupRepo.GetJobSettings(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}
	dest, err := s.openDestination(ctx, tenantID, settings.DestinationID)
	if err != nil {
		return nil, err
	}

	var key *backup.Key
	if settings.Encrypt {
		key, err = backup.LoadKey(s.offsite().keyOptions)
		if err != nil {
			return nil, fmt.Errorf("this job encrypts its backups and the key is not available: %w", err)
		}
		if settings.EncryptionKeyID != "" && settings.EncryptionKeyID != key.ID() {
			s.offsite().logger.Warn("the backup key has changed since this job was configured",
				zap.String("job_id", jobID.String()),
				zap.String("configured_key_id", settings.EncryptionKeyID),
				zap.String("current_key_id", key.ID()))
		}
	}

	op, tracker, opCtx := s.startOperation(tenantID, OperationBackup, &jobID, backupJob.Name)

	go func() {
		artifact, runErr := s.runBackupToDestination(opCtx, tracker, backupJob, settings, dest, key)
		s.finishOperation(op.ID, tracker, artifact, runErr)
	}()

	return op, nil
}

func (s *BackupService) runBackupToDestination(
	ctx context.Context,
	tracker *backup.Tracker,
	backupJob *models.BackupJob,
	settings *backup.JobSettings,
	dest backup.Destination,
	key *backup.Key,
) (*backup.Artifact, error) {
	source, cleanup, err := s.resolveSource(ctx, tracker, backupJob, settings)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}

	// The archive is written to a staging file rather than streamed straight
	// into the destination. It costs one archive of temporary space and buys
	// two things: the digest and size of the finished archive are known before
	// anything is uploaded, and a failure while archiving never leaves a
	// partial object at the destination for retention to count as a
	// generation.
	staging, err := os.CreateTemp(config.TmpRoot(), "vkai-archive-*")
	if err != nil {
		return nil, fmt.Errorf("could not stage the archive: %w", err)
	}
	stagingPath := staging.Name()
	defer func() {
		_ = staging.Close()
		_ = os.Remove(stagingPath)
	}()
	if err := staging.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("could not secure the staged archive: %w", err)
	}

	created, err := backup.CreateArchive(ctx, staging, backup.CreateOptions{
		Source:  source,
		Kind:    settings.RetentionClass,
		Key:     key,
		Tracker: tracker,
	})
	if err != nil {
		return nil, err
	}
	if err := staging.Sync(); err != nil {
		return nil, fmt.Errorf("could not flush the staged archive: %w", err)
	}
	if _, err := staging.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("could not rewind the staged archive: %w", err)
	}

	objectKey, err := backup.ObjectKey(
		backupJob.TenantID.String(),
		settings.RetentionClass,
		backupJob.ResourceID.String(),
		s.offsite().now().UTC(),
		archiveFileName(key != nil),
	)
	if err != nil {
		return nil, err
	}

	tracker.SetPhase(backup.PhaseUploading, "uploading to "+dest.Describe())
	tracker.SetTotals(1, created.Bytes)
	if _, err := dest.Put(ctx, objectKey, staging, created.Bytes); err != nil {
		return nil, err
	}
	tracker.Advance(1, created.Bytes)

	artifact := &backup.Artifact{
		TenantID:        backupJob.TenantID,
		JobID:           &backupJob.ID,
		DestinationID:   settings.DestinationID,
		ObjectKey:       objectKey,
		RetentionClass:  settings.RetentionClass,
		SizeBytes:       created.Bytes,
		SHA256:          created.SHA256,
		Encrypted:       created.Encrypted,
		EncryptionKeyID: created.KeyID,
		FileCount:       created.Manifest.FileCount,
		ManifestBytes:   created.Manifest.TotalSize,
		SourcePath:      defaultRestoreTarget(settings.RetentionClass, created.Manifest.Source),
	}
	if err := s.backupRepo.CreateArtifact(ctx, artifact); err != nil {
		// The object stays where it is. Deleting a good backup because the
		// panel could not write a row about it would be the wrong trade; the
		// key is named here so an operator can find it, and retention will
		// never touch an object it has no record of.
		return nil, fmt.Errorf(
			"the archive was uploaded to %s as %s but could not be recorded, so the panel cannot see it: %w",
			dest.Describe(), objectKey, err)
	}

	// Retention runs after a successful backup, never before: pruning first
	// would delete a generation to make room for one that then fails.
	if err := s.applyRetentionForJob(ctx, tracker, backupJob.TenantID, backupJob.ID, settings, dest); err != nil {
		s.offsite().logger.Warn("retention did not complete", zap.Error(err), zap.String("job_id", backupJob.ID.String()))
	}

	return artifact, nil
}

// defaultRestoreTarget is where a restore of this artifact goes when the
// operator does not name a target.
//
// For a website, a file tree or the panel configuration it is the directory the
// backup came from, which is what "restore this backup" means. For a database
// it is deliberately NOT the directory the dump was written to: that was a
// temporary directory that no longer exists, and restoring into it would be
// useless. A database restore puts the .sql file into the panel's database
// backup directory, and importing it into a live database stays a separate,
// deliberate act by an operator who has decided which database to overwrite.
func defaultRestoreTarget(retentionClass, manifestSource string) string {
	if retentionClass == backup.KindDatabase {
		return config.DatabaseBackupDir()
	}
	return manifestSource
}

// archiveFileName gives an archive an extension that says what it is. A
// ".vkab" file is a VKAI encrypted archive and gunzip will not open it; a
// ".tar.gz" is an ordinary one and will. Naming them the same would leave an
// operator with a directory of files they cannot tell apart.
func archiveFileName(encrypted bool) string {
	if encrypted {
		return "backup.tar.gz.vkab"
	}
	return "backup.tar.gz"
}

// resolveSource produces the path this job backs up.
//
// A database backup has no directory to archive, so one is made: pg_dump writes
// a plain SQL file into the panel's own temporary tree and that single file
// becomes the archive. The cleanup function removes it whatever happens, so a
// failed backup does not leave an unencrypted dump on disk.
func (s *BackupService) resolveSource(ctx context.Context, tracker *backup.Tracker, backupJob *models.BackupJob, settings *backup.JobSettings) (string, func(), error) {
	noop := func() {}

	switch settings.RetentionClass {
	case backup.KindConfig:
		return config.EtcRoot(), noop, nil

	case backup.KindDatabase:
		tracker.SetPhase(backup.PhaseScanning, "dumping the database")
		dbName := backupJob.ResourceID.String()
		if err := utils.ValidateSQLIdentifierOrUUID(dbName, "database name"); err != nil {
			return "", noop, err
		}

		dir, err := os.MkdirTemp(config.TmpRoot(), "vkai-dump-*")
		if err != nil {
			return "", noop, fmt.Errorf("could not create a directory for the dump: %w", err)
		}
		cleanup := func() { _ = os.RemoveAll(dir) }

		dumpPath := filepath.Join(dir, dbName+".sql")
		dump, err := os.OpenFile(dumpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			cleanup()
			return "", noop, fmt.Errorf("could not create the dump file: %w", err)
		}
		defer func() { _ = dump.Close() }()

		var stderr strings.Builder
		cmd := exec.CommandContext(ctx, "sudo", "-u", "postgres", "pg_dump", "--no-password", "--", dbName)
		cmd.Stdout = dump
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			cleanup()
			return "", noop, fmt.Errorf("pg_dump failed: %s: %w", strings.TrimSpace(stderr.String()), err)
		}
		return dumpPath, cleanup, nil

	default:
		// Website and file backups live one directory below the web root,
		// named by the resource id, which is how backup.go has always
		// addressed them.
		root := config.WebRoot()
		source := filepath.Join(root, backupJob.ResourceID.String())
		clean, err := utils.EnsureWithinRoot(root, source)
		if err != nil {
			return "", noop, err
		}
		if _, err := os.Stat(clean); err != nil {
			return "", noop, fmt.Errorf("nothing to back up at %s: %w", clean, err)
		}
		return clean, noop, nil
	}
}

// ============================================================
// Retention
// ============================================================

// ApplyRetention prunes one job's generations according to its policy.
func (s *BackupService) ApplyRetention(ctx context.Context, tenantID, jobID uuid.UUID) ([]backup.RetentionDecision, error) {
	settings, err := s.backupRepo.GetJobSettings(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}
	dest, err := s.openDestination(ctx, tenantID, settings.DestinationID)
	if err != nil {
		return nil, err
	}
	return s.pruneJob(ctx, nil, tenantID, jobID, settings, dest)
}

func (s *BackupService) applyRetentionForJob(ctx context.Context, tracker *backup.Tracker, tenantID, jobID uuid.UUID, settings *backup.JobSettings, dest backup.Destination) error {
	_, err := s.pruneJob(ctx, tracker, tenantID, jobID, settings, dest)
	return err
}

func (s *BackupService) pruneJob(ctx context.Context, tracker *backup.Tracker, tenantID, jobID uuid.UUID, settings *backup.JobSettings, dest backup.Destination) ([]backup.RetentionDecision, error) {
	tracker.SetPhase(backup.PhasePruning, "applying retention")

	artifacts, err := s.backupRepo.ListArtifactsByJob(ctx, tenantID, jobID)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]backup.Artifact, len(artifacts))
	generations := make([]backup.Generation, 0, len(artifacts))
	for _, artifact := range artifacts {
		// Only this job's own class is pruned here. An artifact of another
		// class under the same job would be somebody else's generation.
		if artifact.RetentionClass != settings.RetentionClass {
			continue
		}
		byID[artifact.ID.String()] = artifact
		generations = append(generations, artifact.AsGeneration())
	}

	_, expire, decisions := backup.SelectExpired(generations, settings.Policy(), s.offsite().now())

	for _, gen := range expire {
		if err := ctx.Err(); err != nil {
			return decisions, err
		}
		artifact, ok := byID[gen.ID]
		if !ok {
			continue
		}
		// The object goes first. If deleting the row succeeded and deleting
		// the object did not, the panel would have forgotten about a file it
		// is still paying to store and can no longer reach.
		if err := dest.Delete(ctx, artifact.ObjectKey); err != nil {
			return decisions, fmt.Errorf("could not delete the expired archive %s: %w", artifact.ObjectKey, err)
		}
		if err := s.backupRepo.DeleteArtifact(ctx, tenantID, artifact.ID); err != nil {
			return decisions, fmt.Errorf("the expired archive %s was deleted but its record was not: %w", artifact.ObjectKey, err)
		}
		s.offsite().logger.Info("expired a backup generation",
			zap.String("object_key", artifact.ObjectKey),
			zap.String("class", artifact.RetentionClass))
	}
	return decisions, nil
}

// ============================================================
// Restore
// ============================================================

// RestoreRequest is the API shape for a restore.
type RestoreRequest struct {
	ArtifactID uuid.UUID `json:"artifact_id" binding:"required"`
	// TargetPath is where the archive is restored to. Empty restores to the
	// path the archive was taken from.
	TargetPath string `json:"target_path" binding:"omitempty,max=512"`
	// DryRun plans the restore and changes nothing. It defaults to true in the
	// handler: the safe value is the one you get by forgetting the field.
	DryRun bool `json:"dry_run"`
	// AllowOverwrite is required for a real restore that would replace an
	// existing file whose contents differ.
	AllowOverwrite bool `json:"allow_overwrite"`
	// TargetServerID names the node this restore is for. A restore for another
	// node is refused here rather than half-performed.
	TargetServerID *uuid.UUID `json:"target_server_id"`
}

// Restore restores an artifact, or plans the restore without touching
// anything.
//
// A dry run is synchronous: it downloads one tar entry, compares the manifest
// against what is on disk and answers. A real restore is an operation, because
// it can take hours.
func (s *BackupService) Restore(ctx context.Context, tenantID uuid.UUID, req *RestoreRequest) (*backup.RestoreRecord, *Operation, error) {
	artifact, err := s.backupRepo.GetArtifact(ctx, tenantID, req.ArtifactID)
	if err != nil {
		return nil, nil, err
	}
	if req.TargetServerID != nil {
		return nil, nil, fmt.Errorf(
			"this panel restores onto the node it is running on; to restore onto another node, run the restore there - " +
				"the archive is addressed by destination and object key, not by the machine that produced it")
	}

	target, err := s.restoreTarget(artifact, req.TargetPath)
	if err != nil {
		return nil, nil, err
	}

	dest, err := s.openDestination(ctx, tenantID, artifact.DestinationID)
	if err != nil {
		return nil, nil, err
	}
	key, err := s.keyForArtifact(artifact)
	if err != nil {
		return nil, nil, err
	}

	record := &backup.RestoreRecord{
		TenantID:       tenantID,
		ArtifactID:     artifact.ID,
		TargetPath:     target,
		DryRun:         req.DryRun,
		AllowOverwrite: req.AllowOverwrite,
		Status:         backup.RestorePlanned,
	}

	if req.DryRun {
		plan, err := s.planRestore(ctx, dest, artifact, key, target)
		if err != nil {
			record.Status = backup.RestoreFailed
			record.Error = err.Error()
			finished := s.offsite().now().UTC()
			record.FinishedAt = &finished
			_ = s.backupRepo.CreateRestore(ctx, record)
			return record, nil, err
		}
		applyPlanToRecord(record, plan)
		finished := s.offsite().now().UTC()
		record.FinishedAt = &finished
		if err := s.backupRepo.CreateRestore(ctx, record); err != nil {
			return nil, nil, fmt.Errorf("could not record the restore plan: %w", err)
		}
		return record, nil, nil
	}

	record.Status = backup.RestoreRunning
	if err := s.backupRepo.CreateRestore(ctx, record); err != nil {
		return nil, nil, fmt.Errorf("could not record the restore: %w", err)
	}

	op, tracker, opCtx := s.startOperation(tenantID, OperationRestore, nil, target)

	// The goroutine works on its own copy. The record returned to the caller is
	// serialised into an HTTP response the moment this function returns, and a
	// background goroutine writing to the same struct while the encoder reads
	// it is a data race - one that would show up as a garbled response under
	// load and as nothing at all in a single-threaded test.
	tracked := *record
	go func() {
		plan, runErr := s.performRestore(opCtx, tracker, dest, artifact, key, target, req.AllowOverwrite)
		if plan != nil {
			applyPlanToRecord(&tracked, plan)
		}
		finished := s.offsite().now().UTC()
		tracked.FinishedAt = &finished
		if runErr != nil {
			tracked.Status = backup.RestoreFailed
			tracked.Error = runErr.Error()
			if errors.Is(runErr, context.Canceled) {
				tracked.Status = backup.RestoreCancelled
			}
		} else {
			tracked.Status = backup.RestoreCompleted
		}
		// The restore is over; the context it ran under may have been
		// cancelled, and the outcome still has to be written down.
		if err := s.backupRepo.UpdateRestore(context.WithoutCancel(opCtx), &tracked); err != nil {
			s.offsite().logger.Error("could not record the outcome of a restore", zap.Error(err))
		}
		s.finishOperation(op.ID, tracker, &tracked, runErr)
	}()

	return record, op, nil
}

func (s *BackupService) restoreTarget(artifact *backup.Artifact, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		if artifact.SourcePath == "" {
			return "", errors.New("this artifact does not record where it came from; supply a target path")
		}
		return artifact.SourcePath, nil
	}
	if err := utils.ValidateAbsolutePath(requested, "target path"); err != nil {
		return "", err
	}
	return filepath.Clean(requested), nil
}

// keyForArtifact returns the key an artifact needs, and refuses early and by
// name when the operator is holding a different one.
func (s *BackupService) keyForArtifact(artifact *backup.Artifact) (*backup.Key, error) {
	if !artifact.Encrypted {
		return nil, nil
	}
	key, err := backup.LoadKey(s.offsite().keyOptions)
	if err != nil {
		return nil, fmt.Errorf("this archive is encrypted with key %s and no key is configured: %w", artifact.EncryptionKeyID, err)
	}
	if artifact.EncryptionKeyID != "" && key.ID() != artifact.EncryptionKeyID {
		return nil, fmt.Errorf(
			"this archive was encrypted with key %s but the configured key is %s; "+
				"restore it with the key it was written under - there is no way to recover it without that key",
			artifact.EncryptionKeyID, key.ID())
	}
	return key, nil
}

func (s *BackupService) planRestore(ctx context.Context, dest backup.Destination, artifact *backup.Artifact, key *backup.Key, target string) (*backup.RestorePlan, error) {
	reader, err := dest.Get(ctx, artifact.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return backup.ExtractArchive(ctx, reader, backup.ExtractOptions{
		Dest:              target,
		Key:               key,
		DryRun:            true,
		SurveyDestination: true,
	})
}

func (s *BackupService) performRestore(ctx context.Context, tracker *backup.Tracker, dest backup.Destination, artifact *backup.Artifact, key *backup.Key, target string, allowOverwrite bool) (*backup.RestorePlan, error) {
	tracker.SetPhase(backup.PhaseDownload, "fetching "+artifact.ObjectKey)
	reader, err := dest.Get(ctx, artifact.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return backup.ExtractArchive(ctx, reader, backup.ExtractOptions{
		Dest:           target,
		Key:            key,
		AllowOverwrite: allowOverwrite,
		Tracker:        tracker,
	})
}

func applyPlanToRecord(record *backup.RestoreRecord, plan *backup.RestorePlan) {
	if plan == nil {
		return
	}
	record.FilesTotal = plan.FilesTotal
	record.FilesWritten = plan.FilesWritten
	record.BytesTotal = plan.BytesTotal
	record.BytesWritten = plan.BytesWritten
	record.Overwrites = len(plan.Overwrites)
	record.OverwritesChanged = len(plan.ChangedOverwrites())
	if encoded, err := json.Marshal(plan); err == nil {
		record.Plan = encoded
	}
}

func (s *BackupService) ListRestores(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.RestoreRecord, error) {
	return s.backupRepo.ListRestores(ctx, tenantID, limit)
}

func (s *BackupService) GetRestore(ctx context.Context, tenantID, id uuid.UUID) (*backup.RestoreRecord, error) {
	return s.backupRepo.GetRestore(ctx, tenantID, id)
}

// ============================================================
// Verification
// ============================================================

// VerifyArtifact restores an artifact into scratch space and checks it.
//
// It returns the stored record. A verification that FAILS is a successful call
// with a failed record: the whole point is to record the answer, and returning
// an error for a failed check would tempt a caller into discarding it.
func (s *BackupService) VerifyArtifact(ctx context.Context, tenantID, artifactID uuid.UUID) (*backup.VerificationRecord, error) {
	artifact, err := s.backupRepo.GetArtifact(ctx, tenantID, artifactID)
	if err != nil {
		return nil, err
	}
	dest, err := s.openDestination(ctx, tenantID, artifact.DestinationID)
	if err != nil {
		return nil, err
	}
	key, err := s.keyForArtifact(artifact)
	if err != nil {
		// Being unable to produce the key is a verification failure, and one
		// worth recording loudly: an archive nobody can decrypt is not a
		// backup, and the operator needs that written down.
		return s.recordVerification(ctx, tenantID, artifact, &backup.VerifyResult{
			Status:     backup.VerifyFailed,
			StartedAt:  s.offsite().now().UTC(),
			FinishedAt: s.offsite().now().UTC(),
			Failures:   []string{err.Error()},
		})
	}

	tracker, opCtx := backup.NewTracker(ctx)
	reader, err := dest.Get(opCtx, artifact.ObjectKey)
	if err != nil {
		return s.recordVerification(ctx, tenantID, artifact, &backup.VerifyResult{
			Status:     backup.VerifyFailed,
			StartedAt:  s.offsite().now().UTC(),
			FinishedAt: s.offsite().now().UTC(),
			Failures:   []string{fmt.Sprintf("the archive could not be fetched from the destination: %v", err)},
		})
	}
	defer func() { _ = reader.Close() }()

	scratch := filepath.Join(config.TmpRoot(), "verify")
	result, err := backup.Verify(opCtx, reader, backup.VerifyOptions{
		ScratchParent:  scratch,
		Key:            key,
		DatabaseImport: s.importerFor(artifact),
		Tracker:        tracker,
	})
	if err != nil {
		return nil, err
	}
	return s.recordVerification(ctx, tenantID, artifact, result)
}

// importerFor returns the database importer, but only for database backups.
// Handing one to a website backup would do nothing except make the code look
// like it might.
func (s *BackupService) importerFor(artifact *backup.Artifact) backup.DatabaseImporter {
	if artifact.RetentionClass != backup.KindDatabase {
		return nil
	}
	return s.offsite().importer
}

func (s *BackupService) recordVerification(ctx context.Context, tenantID uuid.UUID, artifact *backup.Artifact, result *backup.VerifyResult) (*backup.VerificationRecord, error) {
	finished := result.FinishedAt
	record := &backup.VerificationRecord{
		TenantID:           tenantID,
		ArtifactID:         artifact.ID,
		Status:             result.Status,
		StartedAt:          result.StartedAt,
		FinishedAt:         &finished,
		DurationMS:         result.DurationMS,
		ArchiveSHA256:      result.ArchiveSHA256,
		ArchiveBytes:       result.ArchiveBytes,
		FilesExpected:      result.FilesExpected,
		FilesRestored:      result.FilesRestored,
		BytesExpected:      result.BytesExpected,
		BytesRestored:      result.BytesRestored,
		ChecksumsChecked:   result.ChecksumsChecked,
		ChecksumMismatches: len(result.ChecksumMismatches),
		MissingFiles:       len(result.MissingFiles),
		UnexpectedFiles:    len(result.UnexpectedFiles),
		DatabaseChecked:    result.DatabaseChecked,
		DatabaseImported:   result.DatabaseImported,
		DatabaseError:      result.DatabaseError,
	}
	if encoded, err := json.Marshal(result); err == nil {
		record.Details = encoded
	}

	if err := s.backupRepo.CreateVerification(ctx, record); err != nil {
		return nil, fmt.Errorf("the verification ran but could not be recorded: %w", err)
	}
	if err := s.backupRepo.RecordArtifactVerification(ctx, tenantID, artifact.ID, finished, result.Status); err != nil {
		s.offsite().logger.Warn("could not stamp the artifact with its verification result", zap.Error(err))
	}
	if artifact.JobID != nil {
		if err := s.backupRepo.RecordJobVerification(ctx, tenantID, *artifact.JobID, finished, result.Status); err != nil {
			s.offsite().logger.Warn("could not stamp the job with its verification result", zap.Error(err))
		}
	}

	logger := s.offsite().logger
	fields := []zap.Field{
		zap.String("artifact_id", artifact.ID.String()),
		zap.String("object_key", artifact.ObjectKey),
		zap.String("status", result.Status),
		zap.String("summary", result.Summary()),
	}
	if result.Passed() {
		logger.Info("a backup was proved restorable", fields...)
	} else {
		logger.Error("a backup could NOT be restored", fields...)
	}
	return record, nil
}

// VerifyDue runs the restorability check for every job whose interval has
// elapsed, newest artifact first, and returns what it did.
//
// This is the entry point a scheduled task calls. It verifies the NEWEST
// artifact of each due job, because that is the one a restore would actually
// use; verifying an old generation proves something true about a file nobody
// would reach for.
func (s *BackupService) VerifyDue(ctx context.Context, limit int) ([]backup.VerificationRecord, error) {
	due, err := s.backupRepo.ListJobSettingsDueForVerification(ctx, s.offsite().now(), limit)
	if err != nil {
		return nil, err
	}

	results := []backup.VerificationRecord{}
	for _, settings := range due {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		artifacts, err := s.backupRepo.ListArtifactsByJob(ctx, settings.TenantID, settings.JobID)
		if err != nil {
			s.offsite().logger.Warn("could not list the artifacts of a job due for verification",
				zap.String("job_id", settings.JobID.String()), zap.Error(err))
			continue
		}
		if len(artifacts) == 0 {
			// Nothing to verify yet. Stamping the job would hide it from this
			// query for another interval, so it is deliberately left due.
			continue
		}
		record, err := s.VerifyArtifact(ctx, settings.TenantID, artifacts[0].ID)
		if err != nil {
			s.offsite().logger.Error("a scheduled verification could not run",
				zap.String("job_id", settings.JobID.String()), zap.Error(err))
			continue
		}
		results = append(results, *record)
	}
	return results, nil
}

// StartRestorabilityChecks runs the verification pass on a timer until ctx is
// cancelled.
//
// This is the difference between having a verification pass and having
// verified backups. A check that only runs when somebody presses a button gets
// pressed on the day it is written and never again; the failure it exists to
// catch - an archive that stopped being restorable months ago - is precisely
// the one nobody goes looking for.
//
// It is one line in cmd/api/main.go:
//
//	go backupService.StartRestorabilityChecks(ctx, time.Hour)
//
// The interval is how often the panel LOOKS for work, not how often each job
// is verified: each job carries its own verify_interval_hours and is only
// picked up once that has elapsed, so an hourly tick over a hundred jobs
// restores a handful of archives a day rather than a hundred an hour.
func (s *BackupService) StartRestorabilityChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	logger := s.offsite().logger
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("backup restorability checks started", zap.Duration("interval", interval))
	for {
		select {
		case <-ctx.Done():
			logger.Info("backup restorability checks stopped")
			return
		case <-ticker.C:
			// A small batch per tick: a restorability check is a full restore
			// and a full re-hash, and it must not be able to saturate the
			// machine it is protecting.
			results, err := s.VerifyDue(ctx, 5)
			if err != nil {
				logger.Error("the restorability check pass failed", zap.Error(err))
				continue
			}
			failed := 0
			for _, result := range results {
				if result.Status != backup.VerifyFailed {
					continue
				}
				failed++
			}
			if len(results) > 0 {
				logger.Info("restorability checks completed",
					zap.Int("checked", len(results)), zap.Int("failed", failed))
			}
		}
	}
}

func (s *BackupService) ListVerifications(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.VerificationRecord, error) {
	return s.backupRepo.ListVerifications(ctx, tenantID, limit)
}

func (s *BackupService) ListVerificationsForArtifact(ctx context.Context, tenantID, artifactID uuid.UUID, limit int) ([]backup.VerificationRecord, error) {
	return s.backupRepo.ListVerificationsByArtifact(ctx, tenantID, artifactID, limit)
}

func (s *BackupService) ListArtifacts(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.Artifact, error) {
	return s.backupRepo.ListArtifactsByTenant(ctx, tenantID, limit)
}

func (s *BackupService) GetArtifact(ctx context.Context, tenantID, id uuid.UUID) (*backup.Artifact, error) {
	return s.backupRepo.GetArtifact(ctx, tenantID, id)
}

// BackupHealth is the one-glance answer: how much of what is stored has been
// proved to restore.
func (s *BackupService) BackupHealth(ctx context.Context, tenantID uuid.UUID) (*repository.BackupHealth, error) {
	return s.backupRepo.GetBackupHealth(ctx, tenantID)
}

// ============================================================
// Operations
// ============================================================

func (s *BackupService) startOperation(tenantID uuid.UUID, kind string, jobID *uuid.UUID, target string) (*Operation, *backup.Tracker, context.Context) {
	// The operation is NOT parented on the request context: an HTTP client
	// hanging up must not abort a backup half way through an upload.
	tracker, opCtx := backup.NewTracker(context.Background())

	op := &runningOperation{
		meta: Operation{
			ID:       uuid.New(),
			TenantID: tenantID,
			Kind:     kind,
			JobID:    jobID,
			Target:   target,
			Progress: tracker.Snapshot(),
		},
		tracker: tracker,
	}
	s.offsite().ops.add(op)
	s.recordOperationStart(&op.meta)

	snapshot := op.meta
	snapshot.Progress = tracker.Snapshot()
	return &snapshot, tracker, opCtx
}

func (s *BackupService) finishOperation(id uuid.UUID, tracker *backup.Tracker, result any, err error) {
	switch {
	case err == nil:
		tracker.SetPhase(backup.PhaseDone, "finished")
	case errors.Is(err, context.Canceled):
		tracker.SetPhase(backup.PhaseCancelled, "cancelled")
	default:
		tracker.SetPhase(backup.PhaseFailed, err.Error())
	}

	s.offsite().ops.update(id, func(op *Operation) {
		if err != nil {
			op.Error = err.Error()
		}
		if encoded, marshalErr := json.Marshal(result); marshalErr == nil {
			op.Result = encoded
		}
	})
	s.recordOperationEnd(id, err)
	s.offsite().ops.forget(id, finishedOperationGrace)
}

// recordOperationStart writes the operation into the shared jobs table, when a
// recorder has been attached. The row id is the operation id, so
// GET /api/v1/jobs/<id> and GET /api/v1/backups/operations/<id> are the same
// operation seen from two sides.
func (s *BackupService) recordOperationStart(op *Operation) {
	recorder := s.offsite().jobs
	if recorder == nil {
		return
	}
	taskType := job.TaskTypeBackup
	if op.Kind == OperationRestore {
		taskType = job.TaskTypeRestore
	}
	payload, _ := json.Marshal(op)

	record := &job.JobRecord{
		ID:         op.ID,
		TaskType:   taskType,
		Status:     job.StatusActive,
		Queue:      "default",
		MaxRetries: 0,
		TenantID:   op.TenantID,
		Payload:    payload,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := recorder.CreateJob(ctx, record); err != nil {
		s.offsite().logger.Warn("could not record the operation in the job queue", zap.Error(err))
		return
	}
	if err := recorder.UpdateJobStarted(ctx, op.ID); err != nil {
		s.offsite().logger.Warn("could not mark the operation as started", zap.Error(err))
	}
}

func (s *BackupService) recordOperationEnd(id uuid.UUID, opErr error) {
	recorder := s.offsite().jobs
	if recorder == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch {
	case opErr == nil:
		if err := recorder.UpdateJobCompleted(ctx, id, nil); err != nil {
			s.offsite().logger.Warn("could not mark the operation as completed", zap.Error(err))
		}
	case errors.Is(opErr, context.Canceled):
		if err := recorder.UpdateJobStatus(ctx, id, job.StatusCancelled, nil, "cancelled by an operator"); err != nil {
			s.offsite().logger.Warn("could not mark the operation as cancelled", zap.Error(err))
		}
	default:
		if err := recorder.UpdateJobFailed(ctx, id, opErr.Error()); err != nil {
			s.offsite().logger.Warn("could not mark the operation as failed", zap.Error(err))
		}
	}
}

// ListOperations returns the operations this process is running for a tenant.
func (s *BackupService) ListOperations(tenantID uuid.UUID) []Operation {
	return s.offsite().ops.list(tenantID)
}

// GetOperation returns one operation's live progress.
func (s *BackupService) GetOperation(tenantID, id uuid.UUID) (*Operation, error) {
	op, ok := s.offsite().ops.get(tenantID, id)
	if !ok {
		return nil, fmt.Errorf("no such operation is running on this node")
	}
	snapshot := op.meta
	snapshot.Progress = op.tracker.Snapshot()
	return &snapshot, nil
}

// CancelOperation stops a running operation.
func (s *BackupService) CancelOperation(tenantID, id uuid.UUID) error {
	op, ok := s.offsite().ops.get(tenantID, id)
	if !ok {
		return fmt.Errorf("no such operation is running on this node")
	}
	if !op.tracker.Snapshot().Cancellable {
		return fmt.Errorf("this operation has already finished")
	}
	op.tracker.Cancel()
	return nil
}

// ============================================================
// The scratch database importer
// ============================================================

// postgresScratchImporter proves a dump imports by importing it.
//
// It creates a database whose name nothing else uses, runs the dump into it
// with ON_ERROR_STOP so the first bad statement fails the check rather than
// being skipped, and drops it again. It runs through sudo -u postgres, which
// is how every other database operation in this panel reaches PostgreSQL.
//
// If the dump does not import, this is the moment the operator finds out -
// months before they would otherwise have found out, which is during an
// incident.
func postgresScratchImporter(ctx context.Context, dumpPath string) error {
	if err := utils.ValidateAbsolutePath(dumpPath, "dump path"); err != nil {
		return err
	}
	scratchDB := "vkai_verify_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	if err := utils.ValidateSQLIdentifier(scratchDB, "scratch database name"); err != nil {
		return err
	}

	create := exec.CommandContext(ctx, "sudo", "-u", "postgres", "createdb", "--", scratchDB)
	if output, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create a scratch database to import into: %s: %w", strings.TrimSpace(string(output)), err)
	}
	defer func() {
		// The drop runs on a context of its own: a cancelled verification must
		// still take its scratch database with it.
		dropCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		drop := exec.CommandContext(dropCtx, "sudo", "-u", "postgres", "dropdb", "--if-exists", "--", scratchDB)
		_ = drop.Run()
	}()

	dump, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("could not open the restored dump: %w", err)
	}
	defer func() { _ = dump.Close() }()

	var stderr strings.Builder
	restore := exec.CommandContext(ctx, "sudo", "-u", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-q", "-d", scratchDB)
	restore.Stdin = dump
	restore.Stderr = &stderr
	if err := restore.Run(); err != nil {
		return fmt.Errorf("the dump did not import: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}
