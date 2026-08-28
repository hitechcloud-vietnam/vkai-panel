package backup

// The rows this feature stores.
//
// They live here rather than in internal/models for one reason: models.go is
// read with SELECT * by half the repository layer, and a struct added there
// has to stay in step with a table that several other people are also editing.
// These types belong to one feature, are written and read by one repository
// file, and nothing else selects them.
//
// They carry db tags because the repository scans straight into them, and json
// tags because the handler returns them. There is one field that deliberately
// has neither: the S3 secret access key. It is never on a struct that leaves
// the repository, so it cannot be leaked by a handler that returns a
// destination without thinking about it.

import (
	"time"

	"github.com/google/uuid"
)

// Destination kinds as stored.
const (
	DestinationLocal = "local"
	DestinationS3    = "s3"
)

// DestinationRecord is a configured place for archives to go.
type DestinationRecord struct {
	ID       uuid.UUID `db:"id" json:"id"`
	TenantID uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name     string    `db:"name" json:"name"`
	Kind     string    `db:"kind" json:"kind"`

	LocalRoot string `db:"local_root" json:"local_root,omitempty"`

	S3Endpoint    string `db:"s3_endpoint" json:"s3_endpoint,omitempty"`
	S3Region      string `db:"s3_region" json:"s3_region,omitempty"`
	S3Bucket      string `db:"s3_bucket" json:"s3_bucket,omitempty"`
	S3Prefix      string `db:"s3_prefix" json:"s3_prefix,omitempty"`
	S3AccessKeyID string `db:"s3_access_key_id" json:"s3_access_key_id,omitempty"`
	S3PathStyle   bool   `db:"s3_path_style" json:"s3_path_style"`

	LastProbeAt    *time.Time `db:"last_probe_at" json:"last_probe_at,omitempty"`
	LastProbeOK    *bool      `db:"last_probe_ok" json:"last_probe_ok,omitempty"`
	LastProbeError string     `db:"last_probe_error" json:"last_probe_error,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Offsite reports whether archives sent here survive the loss of this machine.
// The panel shows it next to every destination, because "we have backups" and
// "we have backups somewhere else" are different claims and the difference
// only matters once.
func (d *DestinationRecord) Offsite() bool { return d.Kind == DestinationS3 }

// JobSettings is the per-job configuration that migration 001 had nowhere to
// put.
type JobSettings struct {
	JobID         uuid.UUID `db:"job_id" json:"job_id"`
	TenantID      uuid.UUID `db:"tenant_id" json:"tenant_id"`
	DestinationID uuid.UUID `db:"destination_id" json:"destination_id"`

	RetentionClass  string `db:"retention_class" json:"retention_class"`
	KeepGenerations int    `db:"keep_generations" json:"keep_generations"`
	KeepDays        int    `db:"keep_days" json:"keep_days"`
	MinKeep         int    `db:"min_keep" json:"min_keep"`

	Encrypt         bool   `db:"encrypt" json:"encrypt"`
	EncryptionKeyID string `db:"encryption_key_id" json:"encryption_key_id,omitempty"`

	VerifyIntervalHours int        `db:"verify_interval_hours" json:"verify_interval_hours"`
	LastVerifiedAt      *time.Time `db:"last_verified_at" json:"last_verified_at,omitempty"`
	LastVerifyStatus    string     `db:"last_verify_status" json:"last_verify_status,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Policy is the retention policy these settings describe.
func (s *JobSettings) Policy() RetentionPolicy {
	return RetentionPolicy{
		KeepGenerations: s.KeepGenerations,
		KeepDays:        s.KeepDays,
		MinKeep:         s.MinKeep,
	}
}

// Artifact is one archive that reached a destination.
type Artifact struct {
	ID            uuid.UUID  `db:"id" json:"id"`
	TenantID      uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	JobID         *uuid.UUID `db:"job_id" json:"job_id,omitempty"`
	RecordID      *uuid.UUID `db:"record_id" json:"record_id,omitempty"`
	DestinationID uuid.UUID  `db:"destination_id" json:"destination_id"`

	ObjectKey      string `db:"object_key" json:"object_key"`
	RetentionClass string `db:"retention_class" json:"retention_class"`
	SizeBytes      int64  `db:"size_bytes" json:"size_bytes"`
	SHA256         string `db:"sha256" json:"sha256"`

	Encrypted       bool   `db:"encrypted" json:"encrypted"`
	EncryptionKeyID string `db:"encryption_key_id" json:"encryption_key_id,omitempty"`

	FileCount     int    `db:"file_count" json:"file_count"`
	ManifestBytes int64  `db:"manifest_bytes" json:"manifest_bytes"`
	SourcePath    string `db:"source_path" json:"source_path,omitempty"`

	LastVerifiedAt   *time.Time `db:"last_verified_at" json:"last_verified_at,omitempty"`
	LastVerifyStatus string     `db:"last_verify_status" json:"last_verify_status,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// AsGeneration converts an artifact into what retention reasons about.
func (a *Artifact) AsGeneration() Generation {
	return Generation{
		ID:        a.ID.String(),
		Key:       a.ObjectKey,
		Class:     a.RetentionClass,
		CreatedAt: a.CreatedAt,
		Size:      a.SizeBytes,
		Verified:  a.LastVerifyStatus == VerifyPassed,
	}
}

// VerificationRecord is one restorability check as stored.
type VerificationRecord struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	TenantID   uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	ArtifactID uuid.UUID  `db:"artifact_id" json:"artifact_id"`
	Status     string     `db:"status" json:"status"`
	StartedAt  time.Time  `db:"started_at" json:"started_at"`
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	DurationMS int64      `db:"duration_ms" json:"duration_ms"`

	ArchiveSHA256 string `db:"archive_sha256" json:"archive_sha256,omitempty"`
	ArchiveBytes  int64  `db:"archive_bytes" json:"archive_bytes"`

	FilesExpected      int   `db:"files_expected" json:"files_expected"`
	FilesRestored      int   `db:"files_restored" json:"files_restored"`
	BytesExpected      int64 `db:"bytes_expected" json:"bytes_expected"`
	BytesRestored      int64 `db:"bytes_restored" json:"bytes_restored"`
	ChecksumsChecked   int   `db:"checksums_checked" json:"checksums_checked"`
	ChecksumMismatches int   `db:"checksum_mismatches" json:"checksum_mismatches"`
	MissingFiles       int   `db:"missing_files" json:"missing_files"`
	UnexpectedFiles    int   `db:"unexpected_files" json:"unexpected_files"`

	DatabaseChecked  bool   `db:"database_checked" json:"database_checked"`
	DatabaseImported bool   `db:"database_imported" json:"database_imported"`
	DatabaseError    string `db:"database_error" json:"database_error,omitempty"`

	Details   []byte    `db:"details" json:"-"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// RestoreRecord is one restore, dry run or real.
type RestoreRecord struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	TenantID   uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	ArtifactID uuid.UUID  `db:"artifact_id" json:"artifact_id"`
	JobRowID   *uuid.UUID `db:"job_row_id" json:"job_row_id,omitempty"`

	TargetPath     string     `db:"target_path" json:"target_path"`
	TargetServerID *uuid.UUID `db:"target_server_id" json:"target_server_id,omitempty"`

	DryRun         bool   `db:"dry_run" json:"dry_run"`
	AllowOverwrite bool   `db:"allow_overwrite" json:"allow_overwrite"`
	Status         string `db:"status" json:"status"`

	FilesTotal        int   `db:"files_total" json:"files_total"`
	FilesWritten      int   `db:"files_written" json:"files_written"`
	BytesTotal        int64 `db:"bytes_total" json:"bytes_total"`
	BytesWritten      int64 `db:"bytes_written" json:"bytes_written"`
	Overwrites        int   `db:"overwrites" json:"overwrites"`
	OverwritesChanged int   `db:"overwrites_changed" json:"overwrites_changed"`

	Plan  []byte `db:"plan" json:"-"`
	Error string `db:"error" json:"error,omitempty"`

	StartedAt  time.Time  `db:"started_at" json:"started_at"`
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
}

// Restore statuses as stored.
const (
	RestorePlanned   = "planned"
	RestoreRunning   = "running"
	RestoreCompleted = "completed"
	RestoreFailed    = "failed"
	RestoreCancelled = "cancelled"
)
