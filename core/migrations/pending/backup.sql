-- ============================================================
-- PENDING MIGRATION: offsite backup, one-action restore, and proof of
-- restorability
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_backup_offsite.sql when it is folded in; nothing here depends on the
-- number. Until then it is applied by hand, because deploy/install.sh walks
-- migrations/ with -maxdepth 1 and does not descend into pending/.
--
-- WHY EVERY COLUMN HERE IS IN A NEW TABLE
--
-- migration 001 created backup_jobs and backup_records, and
-- internal/repository/backup.go reads both with SELECT * into
-- models.BackupJob and models.BackupRecord. A column added to either of those
-- tables breaks every backup query the moment this migration is applied and
-- until those structs change in the same commit - which is exactly the kind of
-- split-across-two-places change that leaves an installed panel broken while
-- CI stays green. So nothing here alters an existing table. An installation
-- that has not applied this migration keeps taking backups the old way; one
-- that has gains destinations, artifacts, verification and restores.
--
-- WHAT THE TABLES ARE FOR
--
--   backup_destinations   where archives go: a local directory or an
--                         S3-compatible bucket. One row per configured
--                         target, per tenant.
--   backup_job_settings   the per-job settings that did not exist in 001:
--                         which destination, which retention class, how many
--                         generations, whether to encrypt, how often to prove
--                         the backup restores.
--   backup_artifacts      one row per archive that actually reached a
--                         destination: its key, its size, its digest, the id
--                         of the key it was encrypted under, and the file
--                         count and total size from its manifest.
--   backup_verifications  one row per restorability check: what was restored
--                         into scratch space, how many checksums were
--                         recomputed, how many matched, whether a database
--                         dump imported. This is the table that turns "we
--                         have backups" into a date and a number.
--   backup_restores       one row per restore, including dry runs. A dry run
--                         is recorded because "what would this have
--                         overwritten" is a question asked after the fact at
--                         least as often as before.
--
-- WHAT IS DELIBERATELY NOT HERE
--
-- No encryption key, and nothing from which one could be derived. The panel
-- stores only the key ID - an HMAC of a fixed label under the key, which
-- identifies a key without revealing it - so that a restore can say which key
-- it needs. An operator who loses their key has lost the archives, and this
-- schema is built so that no amount of database access changes that.
-- ============================================================

-- ------------------------------------------------------------
-- Destinations
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS backup_destinations (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                  VARCHAR(128) NOT NULL,

    -- 'local' or 's3'. Constrained rather than free text: the service switches
    -- on it to build a destination, and an unknown value there is a backup
    -- that silently goes nowhere.
    kind                  VARCHAR(16) NOT NULL,

    -- Local destinations: an absolute directory, validated against the backup
    -- root before it is ever used.
    local_root            TEXT,

    -- S3-compatible destinations. Nothing secret is in this group.
    s3_endpoint           TEXT,
    s3_region             VARCHAR(64),
    s3_bucket             VARCHAR(255),
    s3_prefix             VARCHAR(512) NOT NULL DEFAULT '',
    s3_access_key_id      VARCHAR(255),
    s3_path_style         BOOLEAN NOT NULL DEFAULT FALSE,

    -- The secret access key, AES-256-GCM encrypted under VKAI_SECRET_KEY, the
    -- same key that protects managed database passwords. It is a separate
    -- column from the rest of the S3 settings so that a query which only needs
    -- to display a destination never selects it.
    s3_secret_key_enc     TEXT,

    -- The result of the last write-read-delete probe against this
    -- destination. Listing a bucket succeeds without write permission, so the
    -- probe writes; this records what it found.
    last_probe_at         TIMESTAMPTZ,
    last_probe_ok         BOOLEAN,
    last_probe_error      TEXT NOT NULL DEFAULT '',

    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT backup_destinations_kind_known
        CHECK (kind IN ('local', 's3')),

    -- A destination that is missing the fields its kind needs is a backup that
    -- fails at 03:00 rather than at the moment it was configured.
    CONSTRAINT backup_destinations_local_complete
        CHECK (kind <> 'local' OR (local_root IS NOT NULL AND local_root <> '')),
    CONSTRAINT backup_destinations_s3_complete
        CHECK (kind <> 's3' OR (
            s3_endpoint IS NOT NULL AND s3_endpoint <> '' AND
            s3_region IS NOT NULL AND s3_region <> '' AND
            s3_bucket IS NOT NULL AND s3_bucket <> '' AND
            s3_access_key_id IS NOT NULL AND s3_access_key_id <> '' AND
            s3_secret_key_enc IS NOT NULL AND s3_secret_key_enc <> ''
        ))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_backup_destinations_tenant_name
    ON backup_destinations(tenant_id, lower(name));
CREATE INDEX IF NOT EXISTS idx_backup_destinations_tenant
    ON backup_destinations(tenant_id);

-- ------------------------------------------------------------
-- Per-job settings
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS backup_job_settings (
    job_id                UUID PRIMARY KEY REFERENCES backup_jobs(id) ON DELETE CASCADE,
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- The destination archives go to. ON DELETE RESTRICT, not CASCADE: losing
    -- a destination row must not silently take the settings of every job that
    -- used it, leaving jobs that appear configured and are not.
    destination_id        UUID NOT NULL REFERENCES backup_destinations(id) ON DELETE RESTRICT,

    -- The retention class. It is separate from backup_jobs.type because the
    -- panel's own configuration backup has no equivalent there, and because
    -- retention is the only thing this value drives.
    retention_class       VARCHAR(16) NOT NULL,

    -- Retention, per class. keep_generations 0 means the count rule is off;
    -- keep_days 0 means the age rule is off; min_keep is the floor and is
    -- never allowed below 1 - a policy that can empty a destination is not a
    -- policy.
    keep_generations      INTEGER NOT NULL DEFAULT 7,
    keep_days             INTEGER NOT NULL DEFAULT 30,
    min_keep              INTEGER NOT NULL DEFAULT 2,

    -- Whether archives from this job are encrypted before they leave the
    -- machine, and the id of the key the operator held when it was configured.
    -- The id is stored so a restore can say WHICH key it needs; it is not the
    -- key and cannot be turned into one.
    encrypt               BOOLEAN NOT NULL DEFAULT TRUE,
    encryption_key_id     VARCHAR(64) NOT NULL DEFAULT '',

    -- How often this job's newest archive is restored into scratch space and
    -- checked. 0 disables it, which is allowed and reported: an operator may
    -- have their own verification, but they must have chosen to turn ours off.
    verify_interval_hours INTEGER NOT NULL DEFAULT 168,
    last_verified_at      TIMESTAMPTZ,
    last_verify_status    VARCHAR(16) NOT NULL DEFAULT '',

    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT backup_job_settings_class_known
        CHECK (retention_class IN ('website', 'database', 'files', 'config')),
    CONSTRAINT backup_job_settings_counts_sane
        CHECK (keep_generations >= 0 AND keep_days >= 0 AND min_keep >= 1),
    CONSTRAINT backup_job_settings_verify_interval_sane
        CHECK (verify_interval_hours >= 0),
    CONSTRAINT backup_job_settings_verify_status_known
        CHECK (last_verify_status IN ('', 'passed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_backup_job_settings_tenant
    ON backup_job_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_backup_job_settings_destination
    ON backup_job_settings(destination_id);
-- Finding the jobs that are due a restorability check is one indexed scan.
CREATE INDEX IF NOT EXISTS idx_backup_job_settings_verify_due
    ON backup_job_settings(last_verified_at)
    WHERE verify_interval_hours > 0;

-- ------------------------------------------------------------
-- Artifacts: archives that actually reached a destination
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS backup_artifacts (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    job_id                UUID REFERENCES backup_jobs(id) ON DELETE SET NULL,

    -- The backup_records row this archive came from. SET NULL rather than
    -- CASCADE: an artifact that is still sitting in a bucket must not vanish
    -- from the panel's view because a record row was tidied away, or the
    -- panel would stop being able to restore or delete it.
    record_id             UUID REFERENCES backup_records(id) ON DELETE SET NULL,

    destination_id        UUID NOT NULL REFERENCES backup_destinations(id) ON DELETE RESTRICT,

    -- Where it is and what it is.
    object_key            TEXT NOT NULL,
    retention_class       VARCHAR(16) NOT NULL,
    size_bytes            BIGINT NOT NULL DEFAULT 0,

    -- SHA-256 of the archive as it was written: after compression and after
    -- encryption. It is the digest a download is checked against, and the one
    -- the verification pass records so a verdict is tied to specific bytes.
    sha256                CHAR(64) NOT NULL,

    encrypted             BOOLEAN NOT NULL DEFAULT FALSE,
    encryption_key_id     VARCHAR(64) NOT NULL DEFAULT '',

    -- Counts from the manifest inside the archive, so that "how many files was
    -- this" can be answered without downloading it.
    file_count            INTEGER NOT NULL DEFAULT 0,
    manifest_bytes        BIGINT NOT NULL DEFAULT 0,
    source_path           TEXT NOT NULL DEFAULT '',

    -- The verification state, denormalised onto the artifact because retention
    -- consults it on every pass: the newest generation that has passed a
    -- restore test is never deleted, and that rule must not need a join per
    -- candidate.
    last_verified_at      TIMESTAMPTZ,
    last_verify_status    VARCHAR(16) NOT NULL DEFAULT '',

    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT backup_artifacts_class_known
        CHECK (retention_class IN ('website', 'database', 'files', 'config')),
    CONSTRAINT backup_artifacts_verify_status_known
        CHECK (last_verify_status IN ('', 'passed', 'failed')),
    CONSTRAINT backup_artifacts_encrypted_has_key_id
        CHECK (NOT encrypted OR encryption_key_id <> '')
);

-- One archive per key per destination: re-recording the same object would
-- give retention two generations where there is one file.
CREATE UNIQUE INDEX IF NOT EXISTS idx_backup_artifacts_destination_key
    ON backup_artifacts(destination_id, object_key);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_tenant
    ON backup_artifacts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_job
    ON backup_artifacts(job_id);
-- The retention scan: everything for one job, one class, newest first.
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_retention
    ON backup_artifacts(tenant_id, job_id, retention_class, created_at DESC);

-- ------------------------------------------------------------
-- Verifications: the proof that an archive restores
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS backup_verifications (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    artifact_id           UUID NOT NULL REFERENCES backup_artifacts(id) ON DELETE CASCADE,

    status                VARCHAR(16) NOT NULL,
    started_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at           TIMESTAMPTZ,
    duration_ms           BIGINT NOT NULL DEFAULT 0,

    -- Which bytes were checked. Recording the digest is what stops a verdict
    -- from being inherited by a later archive stored under the same key.
    archive_sha256        CHAR(64) NOT NULL DEFAULT '',
    archive_bytes         BIGINT NOT NULL DEFAULT 0,

    files_expected        INTEGER NOT NULL DEFAULT 0,
    files_restored        INTEGER NOT NULL DEFAULT 0,
    bytes_expected        BIGINT NOT NULL DEFAULT 0,
    bytes_restored        BIGINT NOT NULL DEFAULT 0,
    checksums_checked     INTEGER NOT NULL DEFAULT 0,
    checksum_mismatches   INTEGER NOT NULL DEFAULT 0,
    missing_files         INTEGER NOT NULL DEFAULT 0,
    unexpected_files      INTEGER NOT NULL DEFAULT 0,

    database_checked      BOOLEAN NOT NULL DEFAULT FALSE,
    database_imported     BOOLEAN NOT NULL DEFAULT FALSE,
    database_error        TEXT NOT NULL DEFAULT '',

    -- The full result document, including the names of the files that did not
    -- match. The counted columns above exist so the common queries do not have
    -- to open this.
    details               JSONB,

    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT backup_verifications_status_known
        CHECK (status IN ('running', 'passed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_backup_verifications_artifact
    ON backup_verifications(artifact_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_tenant
    ON backup_verifications(tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_failures
    ON backup_verifications(tenant_id, started_at DESC)
    WHERE status = 'failed';

-- ------------------------------------------------------------
-- Restores, including dry runs
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS backup_restores (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    artifact_id           UUID NOT NULL REFERENCES backup_artifacts(id) ON DELETE CASCADE,

    -- The row in jobs(id) that carries progress and cancellation for this
    -- restore, when it was run as a background operation.
    job_row_id            UUID REFERENCES jobs(id) ON DELETE SET NULL,

    -- Where it went. target_server_id is the node the restore was for; a
    -- restore onto a node other than this one is recorded here and executed by
    -- that node, because the archive is addressed by destination and key
    -- rather than by which machine produced it.
    target_path           TEXT NOT NULL,
    target_server_id      UUID REFERENCES servers(id) ON DELETE SET NULL,

    dry_run               BOOLEAN NOT NULL DEFAULT TRUE,
    allow_overwrite       BOOLEAN NOT NULL DEFAULT FALSE,
    status                VARCHAR(16) NOT NULL DEFAULT 'planned',

    files_total           INTEGER NOT NULL DEFAULT 0,
    files_written         INTEGER NOT NULL DEFAULT 0,
    bytes_total           BIGINT NOT NULL DEFAULT 0,
    bytes_written         BIGINT NOT NULL DEFAULT 0,
    overwrites            INTEGER NOT NULL DEFAULT 0,
    overwrites_changed    INTEGER NOT NULL DEFAULT 0,

    -- The plan: every file that would be or was overwritten, with the digest
    -- of both versions. It is kept after the restore so that "what did this
    -- replace" is answerable afterwards.
    plan                  JSONB,
    error                 TEXT NOT NULL DEFAULT '',

    started_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at           TIMESTAMPTZ,

    CONSTRAINT backup_restores_status_known
        CHECK (status IN ('planned', 'running', 'completed', 'failed', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_backup_restores_tenant
    ON backup_restores(tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_restores_artifact
    ON backup_restores(artifact_id, started_at DESC);

-- ------------------------------------------------------------
-- Permissions
-- ------------------------------------------------------------
-- 001 seeded ('backup','read') and ('backup','write'), and the router gates
-- /backups on the 'backup' resource. The two rows below add the actions this
-- feature introduces, using the same resource so no route changes gate.
INSERT INTO permissions (resource, action) VALUES
    ('backup', 'restore'),
    ('backup', 'verify')
ON CONFLICT (resource, action) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name IN ('super_admin', 'admin')
  AND p.resource = 'backup'
  AND p.action IN ('restore', 'verify')
ON CONFLICT DO NOTHING;
