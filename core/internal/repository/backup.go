package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/backup"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type BackupRepository struct {
	db *sqlx.DB
}

func NewBackupRepository(db *sqlx.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

// Backup Job operations
func (r *BackupRepository) CreateJob(ctx context.Context, job *models.BackupJob) error {
	query := `
		INSERT INTO backup_jobs (id, tenant_id, name, type, resource_id, destination, schedule, retention, encrypted, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING created_at, updated_at`

	job.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		job.ID, job.TenantID, job.Name, job.Type, job.ResourceID,
		job.Destination, job.Schedule, job.Retention, job.Encrypted, job.Status,
	).Scan(&job.CreatedAt, &job.UpdatedAt)
}

func (r *BackupRepository) GetJobByID(ctx context.Context, tenantID, id uuid.UUID) (*models.BackupJob, error) {
	var job models.BackupJob
	query := `SELECT * FROM backup_jobs WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &job, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("backup job not found: %w", err)
	}
	return &job, nil
}

func (r *BackupRepository) ListJobsByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.BackupJob, error) {
	var jobs []models.BackupJob
	query := `SELECT * FROM backup_jobs WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &jobs, query, tenantID); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *BackupRepository) UpdateJob(ctx context.Context, job *models.BackupJob) error {
	query := `UPDATE backup_jobs SET name = $2, destination = $3, schedule = $4, retention = $5, status = $6, updated_at = NOW() WHERE id = $1 AND tenant_id = $7`
	_, err := r.db.ExecContext(ctx, query,
		job.ID, job.Name, job.Destination, job.Schedule, job.Retention, job.Status, job.TenantID,
	)
	return err
}

func (r *BackupRepository) DeleteJob(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM backup_jobs WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// Backup Record operations
func (r *BackupRepository) CreateRecord(ctx context.Context, rec *models.BackupRecord) error {
	query := `
		INSERT INTO backup_records (id, job_id, tenant_id, size, path, status, started_at, error_msg)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
		RETURNING started_at`

	rec.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		rec.ID, rec.JobID, rec.TenantID, rec.Size, rec.Path, rec.Status, rec.ErrorMsg,
	).Scan(&rec.StartedAt)
}

func (r *BackupRepository) UpdateRecord(ctx context.Context, rec *models.BackupRecord) error {
	query := `UPDATE backup_records SET size = $2, status = $3, completed_at = $4, error_msg = $5 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, rec.ID, rec.Size, rec.Status, rec.CompletedAt, rec.ErrorMsg)
	return err
}

func (r *BackupRepository) ListRecordsByJob(ctx context.Context, jobID uuid.UUID) ([]models.BackupRecord, error) {
	var records []models.BackupRecord
	query := `SELECT * FROM backup_records WHERE job_id = $1 ORDER BY started_at DESC`
	if err := r.db.SelectContext(ctx, &records, query, jobID); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *BackupRepository) ListRecordsByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]models.BackupRecord, error) {
	var records []models.BackupRecord
	query := `SELECT * FROM backup_records WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &records, query, tenantID, limit); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *BackupRepository) DeleteRecord(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM backup_records WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// ============================================================
// Offsite backup: destinations, artifacts, verification, restores
// ============================================================
//
// Everything below reads and writes the tables added by
// migrations/pending/backup.sql. None of it touches backup_jobs or
// backup_records, which are read with SELECT * into models structs above and
// whose shape must not change.
//
// Two rules hold throughout:
//
//   - Every query is scoped by tenant_id, in the WHERE clause, not by the
//     caller having looked it up first. A backup archive is the most valuable
//     object this panel stores and a missing tenant predicate is a cross
//     tenant read of it.
//   - Columns are listed explicitly, never SELECT *. That is what lets this
//     migration add a column later without breaking every query here, which
//     is the failure the numbered migrations already produced once.

// destinationColumns is the projection every destination read uses. The
// encrypted secret access key is deliberately absent: it is fetched by
// DestinationSecret alone, so a handler cannot return it by accident.
const destinationColumns = `
	id, tenant_id, name, kind,
	COALESCE(local_root, '')       AS local_root,
	COALESCE(s3_endpoint, '')      AS s3_endpoint,
	COALESCE(s3_region, '')        AS s3_region,
	COALESCE(s3_bucket, '')        AS s3_bucket,
	COALESCE(s3_prefix, '')        AS s3_prefix,
	COALESCE(s3_access_key_id, '') AS s3_access_key_id,
	s3_path_style,
	last_probe_at, last_probe_ok, last_probe_error,
	created_at, updated_at`

// CreateDestination stores a destination. secretEnc is the already-encrypted
// S3 secret access key; this layer never sees the plaintext.
func (r *BackupRepository) CreateDestination(ctx context.Context, dest *backup.DestinationRecord, secretEnc string) error {
	if dest.ID == uuid.Nil {
		dest.ID = uuid.New()
	}
	query := `
		INSERT INTO backup_destinations (
			id, tenant_id, name, kind,
			local_root,
			s3_endpoint, s3_region, s3_bucket, s3_prefix, s3_access_key_id, s3_path_style, s3_secret_key_enc,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, NULLIF($10, ''), $11, NULLIF($12, ''), NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		dest.ID, dest.TenantID, dest.Name, dest.Kind,
		dest.LocalRoot,
		dest.S3Endpoint, dest.S3Region, dest.S3Bucket, dest.S3Prefix, dest.S3AccessKeyID, dest.S3PathStyle, secretEnc,
	).Scan(&dest.CreatedAt, &dest.UpdatedAt)
}

func (r *BackupRepository) GetDestination(ctx context.Context, tenantID, id uuid.UUID) (*backup.DestinationRecord, error) {
	var dest backup.DestinationRecord
	query := `SELECT ` + destinationColumns + ` FROM backup_destinations WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &dest, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("backup destination not found: %w", err)
	}
	return &dest, nil
}

func (r *BackupRepository) ListDestinations(ctx context.Context, tenantID uuid.UUID) ([]backup.DestinationRecord, error) {
	destinations := []backup.DestinationRecord{}
	query := `SELECT ` + destinationColumns + ` FROM backup_destinations WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &destinations, query, tenantID); err != nil {
		return nil, err
	}
	return destinations, nil
}

// DestinationSecret returns the encrypted S3 secret access key. It is the only
// query that selects that column.
func (r *BackupRepository) DestinationSecret(ctx context.Context, tenantID, id uuid.UUID) (string, error) {
	var secret sql.NullString
	query := `SELECT s3_secret_key_enc FROM backup_destinations WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &secret, query, id, tenantID); err != nil {
		return "", fmt.Errorf("backup destination not found: %w", err)
	}
	return secret.String, nil
}

// DeleteDestination removes a destination. The foreign keys from
// backup_job_settings and backup_artifacts are ON DELETE RESTRICT, so a
// destination still in use refuses to go rather than orphaning the jobs that
// point at it.
func (r *BackupRepository) DeleteDestination(ctx context.Context, tenantID, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM backup_destinations WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("backup destination not found")
	}
	return nil
}

// RecordProbe stores the outcome of a write-read-delete probe.
func (r *BackupRepository) RecordProbe(ctx context.Context, tenantID, id uuid.UUID, ok bool, probeErr string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE backup_destinations
		   SET last_probe_at = NOW(), last_probe_ok = $3, last_probe_error = $4, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2`, id, tenantID, ok, probeErr)
	return err
}

// ------------------------------------------------------------
// Job settings
// ------------------------------------------------------------

const jobSettingsColumns = `
	job_id, tenant_id, destination_id, retention_class,
	keep_generations, keep_days, min_keep,
	encrypt, encryption_key_id,
	verify_interval_hours, last_verified_at, last_verify_status,
	created_at, updated_at`

// UpsertJobSettings writes the settings for a job, replacing any that exist.
func (r *BackupRepository) UpsertJobSettings(ctx context.Context, settings *backup.JobSettings) error {
	query := `
		INSERT INTO backup_job_settings (
			job_id, tenant_id, destination_id, retention_class,
			keep_generations, keep_days, min_keep,
			encrypt, encryption_key_id,
			verify_interval_hours, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (job_id) DO UPDATE SET
			destination_id        = EXCLUDED.destination_id,
			retention_class       = EXCLUDED.retention_class,
			keep_generations      = EXCLUDED.keep_generations,
			keep_days             = EXCLUDED.keep_days,
			min_keep              = EXCLUDED.min_keep,
			encrypt               = EXCLUDED.encrypt,
			encryption_key_id     = EXCLUDED.encryption_key_id,
			verify_interval_hours = EXCLUDED.verify_interval_hours,
			updated_at            = NOW()
		RETURNING created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		settings.JobID, settings.TenantID, settings.DestinationID, settings.RetentionClass,
		settings.KeepGenerations, settings.KeepDays, settings.MinKeep,
		settings.Encrypt, settings.EncryptionKeyID,
		settings.VerifyIntervalHours,
	).Scan(&settings.CreatedAt, &settings.UpdatedAt)
}

func (r *BackupRepository) GetJobSettings(ctx context.Context, tenantID, jobID uuid.UUID) (*backup.JobSettings, error) {
	var settings backup.JobSettings
	query := `SELECT ` + jobSettingsColumns + ` FROM backup_job_settings WHERE job_id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &settings, query, jobID, tenantID); err != nil {
		return nil, fmt.Errorf("this backup job has no offsite settings: %w", err)
	}
	return &settings, nil
}

// ListJobSettingsDueForVerification returns the jobs whose newest archive has
// not been proved restorable inside their own interval.
//
// A job that has never been verified is due immediately: "we have never tried"
// and "we tried and it worked a long time ago" are both overdue, and the first
// is the more urgent of the two.
func (r *BackupRepository) ListJobSettingsDueForVerification(ctx context.Context, now time.Time, limit int) ([]backup.JobSettings, error) {
	if limit <= 0 {
		limit = 20
	}
	settings := []backup.JobSettings{}
	query := `
		SELECT ` + jobSettingsColumns + `
		  FROM backup_job_settings
		 WHERE verify_interval_hours > 0
		   -- $1 is cast explicitly: without it PostgreSQL cannot resolve the
		   -- '-' operator against an unknown parameter and picks
		   -- interval - interval, which fails at execution time rather than at
		   -- prepare time. Found by running this query, not by reading it.
		   AND (last_verified_at IS NULL
		        OR last_verified_at < $1::timestamptz - make_interval(hours => verify_interval_hours))
		 ORDER BY last_verified_at ASC NULLS FIRST
		 LIMIT $2`
	if err := r.db.SelectContext(ctx, &settings, query, now, limit); err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *BackupRepository) RecordJobVerification(ctx context.Context, tenantID, jobID uuid.UUID, at time.Time, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE backup_job_settings
		   SET last_verified_at = $3, last_verify_status = $4, updated_at = NOW()
		 WHERE job_id = $1 AND tenant_id = $2`, jobID, tenantID, at, status)
	return err
}

// ------------------------------------------------------------
// Artifacts
// ------------------------------------------------------------

const artifactColumns = `
	id, tenant_id, job_id, record_id, destination_id,
	object_key, retention_class, size_bytes, sha256,
	encrypted, encryption_key_id,
	file_count, manifest_bytes, source_path,
	last_verified_at, last_verify_status, created_at`

func (r *BackupRepository) CreateArtifact(ctx context.Context, artifact *backup.Artifact) error {
	if artifact.ID == uuid.Nil {
		artifact.ID = uuid.New()
	}
	query := `
		INSERT INTO backup_artifacts (
			id, tenant_id, job_id, record_id, destination_id,
			object_key, retention_class, size_bytes, sha256,
			encrypted, encryption_key_id,
			file_count, manifest_bytes, source_path, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
		RETURNING created_at`

	return r.db.QueryRowContext(ctx, query,
		artifact.ID, artifact.TenantID, artifact.JobID, artifact.RecordID, artifact.DestinationID,
		artifact.ObjectKey, artifact.RetentionClass, artifact.SizeBytes, artifact.SHA256,
		artifact.Encrypted, artifact.EncryptionKeyID,
		artifact.FileCount, artifact.ManifestBytes, artifact.SourcePath,
	).Scan(&artifact.CreatedAt)
}

func (r *BackupRepository) GetArtifact(ctx context.Context, tenantID, id uuid.UUID) (*backup.Artifact, error) {
	var artifact backup.Artifact
	query := `SELECT ` + artifactColumns + ` FROM backup_artifacts WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &artifact, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("backup artifact not found: %w", err)
	}
	return &artifact, nil
}

func (r *BackupRepository) ListArtifactsByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.Artifact, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	artifacts := []backup.Artifact{}
	query := `SELECT ` + artifactColumns + `
		  FROM backup_artifacts WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &artifacts, query, tenantID, limit); err != nil {
		return nil, err
	}
	return artifacts, nil
}

// ListArtifactsByJob returns every generation of one job, newest first. It is
// what retention and verification both read.
func (r *BackupRepository) ListArtifactsByJob(ctx context.Context, tenantID, jobID uuid.UUID) ([]backup.Artifact, error) {
	artifacts := []backup.Artifact{}
	query := `SELECT ` + artifactColumns + `
		  FROM backup_artifacts
		 WHERE tenant_id = $1 AND job_id = $2
		 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &artifacts, query, tenantID, jobID); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (r *BackupRepository) DeleteArtifact(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM backup_artifacts WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

func (r *BackupRepository) RecordArtifactVerification(ctx context.Context, tenantID, id uuid.UUID, at time.Time, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE backup_artifacts
		   SET last_verified_at = $3, last_verify_status = $4
		 WHERE id = $1 AND tenant_id = $2`, id, tenantID, at, status)
	return err
}

// jsonbOrNULL is what a nil JSON document has to become before it reaches a
// jsonb column. lib/pq sends a nil []byte as an empty string, and an empty
// string is not valid JSON, so the insert fails with "invalid input syntax for
// type json" - at run time, on the first restore with no plan to record.
func jsonbOrNULL(document []byte) any {
	if len(document) == 0 {
		return nil
	}
	return document
}

// ------------------------------------------------------------
// Verifications
// ------------------------------------------------------------

const verificationColumns = `
	id, tenant_id, artifact_id, status, started_at, finished_at, duration_ms,
	archive_sha256, archive_bytes,
	files_expected, files_restored, bytes_expected, bytes_restored,
	checksums_checked, checksum_mismatches, missing_files, unexpected_files,
	database_checked, database_imported, database_error,
	details, created_at`

func (r *BackupRepository) CreateVerification(ctx context.Context, verification *backup.VerificationRecord) error {
	if verification.ID == uuid.Nil {
		verification.ID = uuid.New()
	}
	query := `
		INSERT INTO backup_verifications (
			id, tenant_id, artifact_id, status, started_at, finished_at, duration_ms,
			archive_sha256, archive_bytes,
			files_expected, files_restored, bytes_expected, bytes_restored,
			checksums_checked, checksum_mismatches, missing_files, unexpected_files,
			database_checked, database_imported, database_error, details, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, NOW())
		RETURNING created_at`

	return r.db.QueryRowContext(ctx, query,
		verification.ID, verification.TenantID, verification.ArtifactID, verification.Status,
		verification.StartedAt, verification.FinishedAt, verification.DurationMS,
		verification.ArchiveSHA256, verification.ArchiveBytes,
		verification.FilesExpected, verification.FilesRestored, verification.BytesExpected, verification.BytesRestored,
		verification.ChecksumsChecked, verification.ChecksumMismatches, verification.MissingFiles, verification.UnexpectedFiles,
		verification.DatabaseChecked, verification.DatabaseImported, verification.DatabaseError,
		jsonbOrNULL(verification.Details),
	).Scan(&verification.CreatedAt)
}

func (r *BackupRepository) ListVerifications(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.VerificationRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	verifications := []backup.VerificationRecord{}
	query := `SELECT ` + verificationColumns + `
		  FROM backup_verifications WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &verifications, query, tenantID, limit); err != nil {
		return nil, err
	}
	return verifications, nil
}

func (r *BackupRepository) ListVerificationsByArtifact(ctx context.Context, tenantID, artifactID uuid.UUID, limit int) ([]backup.VerificationRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	verifications := []backup.VerificationRecord{}
	query := `SELECT ` + verificationColumns + `
		  FROM backup_verifications
		 WHERE tenant_id = $1 AND artifact_id = $2
		 ORDER BY started_at DESC LIMIT $3`
	if err := r.db.SelectContext(ctx, &verifications, query, tenantID, artifactID, limit); err != nil {
		return nil, err
	}
	return verifications, nil
}

// BackupHealth is the answer to the only question that matters at a glance:
// how much of what we are storing has actually been proved to restore.
type BackupHealth struct {
	Artifacts        int        `db:"artifacts" json:"artifacts"`
	Verified         int        `db:"verified" json:"verified"`
	FailedLastCheck  int        `db:"failed_last_check" json:"failed_last_check"`
	NeverVerified    int        `db:"never_verified" json:"never_verified"`
	OffsiteArtifacts int        `db:"offsite_artifacts" json:"offsite_artifacts"`
	OldestUnverified *time.Time `db:"oldest_unverified" json:"oldest_unverified,omitempty"`
	LastVerifiedAt   *time.Time `db:"last_verified_at" json:"last_verified_at,omitempty"`
}

// GetBackupHealth summarises the tenant's artifacts.
func (r *BackupRepository) GetBackupHealth(ctx context.Context, tenantID uuid.UUID) (*BackupHealth, error) {
	var health BackupHealth
	query := `
		SELECT
			COUNT(*)                                                              AS artifacts,
			COUNT(*) FILTER (WHERE a.last_verify_status = 'passed')               AS verified,
			COUNT(*) FILTER (WHERE a.last_verify_status = 'failed')               AS failed_last_check,
			COUNT(*) FILTER (WHERE a.last_verify_status = '')                     AS never_verified,
			COUNT(*) FILTER (WHERE d.kind <> 'local')                             AS offsite_artifacts,
			MIN(a.created_at) FILTER (WHERE a.last_verify_status <> 'passed')      AS oldest_unverified,
			MAX(a.last_verified_at)                                                AS last_verified_at
		  FROM backup_artifacts a
		  JOIN backup_destinations d ON d.id = a.destination_id
		 WHERE a.tenant_id = $1`
	if err := r.db.GetContext(ctx, &health, query, tenantID); err != nil {
		return nil, err
	}
	return &health, nil
}

// ------------------------------------------------------------
// Restores
// ------------------------------------------------------------

const restoreColumns = `
	id, tenant_id, artifact_id, job_row_id,
	target_path, target_server_id,
	dry_run, allow_overwrite, status,
	files_total, files_written, bytes_total, bytes_written,
	overwrites, overwrites_changed,
	plan, error, started_at, finished_at`

func (r *BackupRepository) CreateRestore(ctx context.Context, restore *backup.RestoreRecord) error {
	if restore.ID == uuid.Nil {
		restore.ID = uuid.New()
	}
	query := `
		INSERT INTO backup_restores (
			id, tenant_id, artifact_id, job_row_id,
			target_path, target_server_id,
			dry_run, allow_overwrite, status,
			files_total, files_written, bytes_total, bytes_written,
			overwrites, overwrites_changed,
			plan, error, started_at, finished_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), $18)
		RETURNING started_at`

	return r.db.QueryRowContext(ctx, query,
		restore.ID, restore.TenantID, restore.ArtifactID, restore.JobRowID,
		restore.TargetPath, restore.TargetServerID,
		restore.DryRun, restore.AllowOverwrite, restore.Status,
		restore.FilesTotal, restore.FilesWritten, restore.BytesTotal, restore.BytesWritten,
		restore.Overwrites, restore.OverwritesChanged,
		jsonbOrNULL(restore.Plan), restore.Error, restore.FinishedAt,
	).Scan(&restore.StartedAt)
}

func (r *BackupRepository) UpdateRestore(ctx context.Context, restore *backup.RestoreRecord) error {
	query := `
		UPDATE backup_restores
		   SET status = $3, files_total = $4, files_written = $5,
		       bytes_total = $6, bytes_written = $7,
		       overwrites = $8, overwrites_changed = $9,
		       plan = $10, error = $11, finished_at = $12
		 WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query,
		restore.ID, restore.TenantID, restore.Status,
		restore.FilesTotal, restore.FilesWritten, restore.BytesTotal, restore.BytesWritten,
		restore.Overwrites, restore.OverwritesChanged,
		jsonbOrNULL(restore.Plan), restore.Error, restore.FinishedAt,
	)
	return err
}

func (r *BackupRepository) GetRestore(ctx context.Context, tenantID, id uuid.UUID) (*backup.RestoreRecord, error) {
	var restore backup.RestoreRecord
	query := `SELECT ` + restoreColumns + ` FROM backup_restores WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &restore, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("restore not found: %w", err)
	}
	return &restore, nil
}

func (r *BackupRepository) ListRestores(ctx context.Context, tenantID uuid.UUID, limit int) ([]backup.RestoreRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	restores := []backup.RestoreRecord{}
	query := `SELECT ` + restoreColumns + `
		  FROM backup_restores WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT $2`
	if err := r.db.SelectContext(ctx, &restores, query, tenantID, limit); err != nil {
		return nil, err
	}
	return restores, nil
}
