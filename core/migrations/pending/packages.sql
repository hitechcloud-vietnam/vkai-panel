-- ============================================================
-- PENDING MIGRATION: hosting packages and quota (roadmap P0-2)
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_packages.sql when it is folded in; nothing here depends on the number.
--
-- WHAT IT IS FOR
--
-- Until now the only limits in the schema were tenants.max_servers and
-- tenants.max_websites, and no code read either of them. A reseller who sells
-- "10GB disk, 5 websites, 20GB bandwidth" had nothing behind the promise. These
-- tables are what makes the promise mean something.
--
-- THE SHAPE
--
--   hosting_packages          what is sold: the limits and the policy
--   tenant_packages           which account bought which package, and whether
--                             the account is suspended
--   tenant_quota_overrides    the numeric exception for one account
--   tenant_feature_overrides  the feature exception for one account
--   tenant_quota_usage        the measured resources: disk and bandwidth
--   tenant_quota_events       what the enforcer did and why
--
-- An "account" is a tenant. The panel has no other notion of a customer, and
-- inventing one here would leave two answers to "who owns this website".
--
-- UNITS
--
-- Every size is MEBIBYTES: 1 MB = 1048576 bytes, 1 GB = 1024 MB. A 10GB package
-- is disk_mb = 10240. One unit, everywhere, so the number a customer is quoted
-- and the number the enforcer compares are the same number.
--
-- NULL VERSUS ZERO
--
-- A NULL limit means UNLIMITED. Zero means zero: a package that includes no
-- mailboxes sets max_mailboxes = 0, and that has to refuse the first mailbox.
-- Making 0 mean "unlimited" - the usual shortcut - turns every unconfigured
-- package into a free-for-all, which is the failure this table exists to stop.
--
-- COUNTS ARE NOT STORED
--
-- There is deliberately no websites_used / databases_used column. Counted
-- resources are counted from the tables that own them, at the moment of the
-- check, by one query in internal/quota. A cached count is how a limit drifts
-- from the truth and stays generous forever; a COUNT(*) on an indexed
-- tenant_id is cheaper than the bug.
--
-- Only disk and bandwidth are stored, because they cannot be counted - they
-- have to be measured, and measuring them is expensive enough that it happens
-- on a schedule rather than on the request path. See internal/quota/measure.go
-- for the cost and the budget that bounds it.
-- ============================================================

-- ------------------------------------------------------------
-- hosting_packages: what is sold
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS hosting_packages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- NULL means the package belongs to the panel operator and every account
    -- may be put on it. A non-NULL owner is a reseller's own package; the
    -- reseller hierarchy itself is P1-1 and is not built here, but the column
    -- means that work does not have to rewrite this table.
    owner_tenant_id   UUID REFERENCES tenants(id) ON DELETE CASCADE,

    name              VARCHAR(120) NOT NULL,
    slug              VARCHAR(80)  NOT NULL,
    description       TEXT         NOT NULL DEFAULT '',

    -- NULL = unlimited for every limit below. See "NULL VERSUS ZERO" above.
    disk_mb           BIGINT  CHECK (disk_mb           IS NULL OR disk_mb           >= 0),
    bandwidth_mb      BIGINT  CHECK (bandwidth_mb      IS NULL OR bandwidth_mb      >= 0),
    max_websites      INTEGER CHECK (max_websites      IS NULL OR max_websites      >= 0),
    max_databases     INTEGER CHECK (max_databases     IS NULL OR max_databases     >= 0),
    max_mailboxes     INTEGER CHECK (max_mailboxes     IS NULL OR max_mailboxes     >= 0),
    max_subdomains    INTEGER CHECK (max_subdomains    IS NULL OR max_subdomains    >= 0),
    max_cron_jobs     INTEGER CHECK (max_cron_jobs     IS NULL OR max_cron_jobs     >= 0),

    -- Which optional features this package includes, as {"feature": true}.
    -- A feature absent from the object is not allowed; the panel's feature
    -- names live in internal/quota/feature.go so the two cannot drift.
    features          JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- What happens when a MEASURED resource goes past its limit. Counted
    -- resources are always refused at the limit - "5 websites" cannot mean 6 -
    -- so this policy governs disk and bandwidth only.
    --   warn    : record it, refuse nothing, suspend nothing
    --   refuse  : refuse new resources of every kind; serve what exists
    --   suspend : refuse, and take the account's sites offline
    over_quota_action VARCHAR(16) NOT NULL DEFAULT 'refuse'
        CHECK (over_quota_action IN ('warn', 'refuse', 'suspend')),

    -- The customer hears about it before anything is refused.
    warn_percent      SMALLINT NOT NULL DEFAULT 90
        CHECK (warn_percent BETWEEN 1 AND 100),

    -- The grace band on a measured resource, as a percentage of the limit with
    -- an absolute floor. A customer at 10.001GB of a 10GB quota is inside the
    -- error bar of the measurement itself - the disk sample is up to one
    -- sampling interval old, and block counts differ from apparent size on any
    -- filesystem with compression or tail packing. Refusing there is a false
    -- accusation. 2% of 10GB is 204MB; the floor keeps the band meaningful on a
    -- 512MB package. Counted resources get no grace: they are exact integers.
    grace_percent     NUMERIC(5,2) NOT NULL DEFAULT 2.00
        CHECK (grace_percent >= 0 AND grace_percent <= 100),
    grace_floor_mb    BIGINT NOT NULL DEFAULT 16
        CHECK (grace_floor_mb >= 0),

    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_hosting_packages_slug ON hosting_packages(slug);
CREATE INDEX IF NOT EXISTS idx_hosting_packages_owner ON hosting_packages(owner_tenant_id);

-- ------------------------------------------------------------
-- tenant_packages: which account is on which package
--
-- One row per account, so an account cannot be on two packages and have two
-- answers to the same limit. Suspension lives here because it is a property of
-- the account, and because putting it here makes it a single boolean to clear:
-- suspension must be reversible, and it must never be expressed by deleting
-- anything.
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_packages (
    tenant_id               UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,

    -- RESTRICT, not CASCADE: deleting a package that customers are on must
    -- fail loudly rather than silently un-limit them.
    package_id              UUID NOT NULL REFERENCES hosting_packages(id) ON DELETE RESTRICT,

    assigned_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by             UUID REFERENCES users(id) ON DELETE SET NULL,

    suspended               BOOLEAN NOT NULL DEFAULT FALSE,
    suspended_at            TIMESTAMP WITH TIME ZONE,
    suspended_reason        TEXT NOT NULL DEFAULT '',
    -- TRUE when the quota sampler suspended it, FALSE when an operator did.
    -- An operator's suspension must not be lifted by usage dropping.
    suspended_automatically BOOLEAN NOT NULL DEFAULT FALSE,

    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT tenant_packages_suspension_dated
        CHECK (suspended = FALSE OR suspended_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_tenant_packages_package ON tenant_packages(package_id);
CREATE INDEX IF NOT EXISTS idx_tenant_packages_suspended ON tenant_packages(suspended) WHERE suspended;

-- ------------------------------------------------------------
-- tenant_quota_overrides: the exception always arrives
--
-- One row raises (or lowers) one limit for one account without minting a new
-- package for every negotiated deal. limit_value NULL means "unlimited for this
-- account"; the absence of a row means "no exception, use the package".
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_quota_overrides (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    resource    VARCHAR(32) NOT NULL
        CHECK (resource IN ('disk', 'bandwidth', 'websites', 'databases',
                            'mailboxes', 'subdomains', 'cron_jobs')),
    limit_value BIGINT CHECK (limit_value IS NULL OR limit_value >= 0),
    reason      TEXT NOT NULL DEFAULT '',
    -- "2GB extra until the end of the month" is the common case. An expired
    -- override is ignored by the enforcer rather than deleted, so the history
    -- of what was granted survives.
    expires_at  TIMESTAMP WITH TIME ZONE,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (tenant_id, resource)
);

-- ------------------------------------------------------------
-- tenant_feature_overrides: the same exception, for a boolean
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_feature_overrides (
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    feature    VARCHAR(64) NOT NULL,
    allowed    BOOLEAN NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, feature)
);

-- ------------------------------------------------------------
-- tenant_quota_usage: the measured resources
--
-- Written by the sampler in internal/quota, never on the request path. The
-- cost columns are not decoration: a disk walk that starts taking 40 seconds
-- over 3 million inodes is an outage forming, and this is where an operator
-- sees it before it arrives.
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_quota_usage (
    tenant_id              UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,

    disk_used_mb           BIGINT NOT NULL DEFAULT 0 CHECK (disk_used_mb >= 0),
    disk_file_count        BIGINT NOT NULL DEFAULT 0 CHECK (disk_file_count >= 0),
    disk_measured_at       TIMESTAMP WITH TIME ZONE,
    disk_measure_ms        INTEGER NOT NULL DEFAULT 0 CHECK (disk_measure_ms >= 0),
    -- TRUE when the walk hit its inode or time budget and stopped early. The
    -- number is then a LOWER BOUND, and the enforcer must not refuse on it:
    -- accusing a customer of being over quota on an incomplete measurement is
    -- worse than being late to notice.
    disk_partial           BOOLEAN NOT NULL DEFAULT FALSE,

    bandwidth_used_mb      BIGINT NOT NULL DEFAULT 0 CHECK (bandwidth_used_mb >= 0),
    -- First day of the calendar month the counter covers, in UTC. When the
    -- month rolls over the sampler writes a new period and the counter starts
    -- again; the package limit is per month, so the period has to be explicit
    -- rather than implied by "whenever this row was last touched".
    bandwidth_period_start DATE NOT NULL DEFAULT (date_trunc('month', NOW() AT TIME ZONE 'UTC'))::date,
    bandwidth_measured_at  TIMESTAMP WITH TIME ZONE,

    updated_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ------------------------------------------------------------
-- tenant_quota_events: what the enforcer did, and why
--
-- Without this, "warn, then refuse, then suspend" is a policy nobody can
-- observe. A support ticket that says "I could not create a database" is
-- answered from this table.
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_quota_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    resource    VARCHAR(32) NOT NULL,
    event       VARCHAR(16) NOT NULL
        CHECK (event IN ('warn', 'refuse', 'suspend', 'resume')),
    limit_value BIGINT,
    usage_value BIGINT NOT NULL DEFAULT 0,
    message     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_quota_events_tenant
    ON tenant_quota_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tenant_quota_events_recent
    ON tenant_quota_events(tenant_id, resource, event, created_at DESC);

-- ------------------------------------------------------------
-- The legacy package, and the accounts that already exist
--
-- Every account that exists when this migration runs is put on a package with
-- no limits at all. That is the only safe default: inventing a 10GB cap here
-- would take an installed panel's customers over quota the moment the migration
-- ran, and the first thing they would notice is a refusal.
--
-- It also means "this account has no package row" stops being the normal case
-- and becomes an anomaly the panel can report. internal/quota treats an account
-- with no package as unmanaged and imposes nothing - see the note in
-- internal/quota/doc.go about why that, and not a refusal, is correct.
-- ------------------------------------------------------------
INSERT INTO hosting_packages (name, slug, description, over_quota_action, is_active)
VALUES (
    'Unmetered (legacy)',
    'unmetered',
    'No limits. Assigned to every account that existed before hosting packages '
        || 'were introduced, so that adding quota enforcement changed nothing for them. '
        || 'Move accounts off this package deliberately, one at a time.',
    'warn',
    TRUE
)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO tenant_packages (tenant_id, package_id)
SELECT t.id, p.id
FROM tenants t
CROSS JOIN hosting_packages p
WHERE p.slug = 'unmetered'
  AND t.deleted_at IS NULL
ON CONFLICT (tenant_id) DO NOTHING;
