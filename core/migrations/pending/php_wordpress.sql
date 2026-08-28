-- ============================================================
-- PENDING MIGRATION: multi-version PHP pools and the WordPress toolkit
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_php_wordpress_runtime.sql when it is folded in; nothing here depends on
-- the number.
--
-- Why new tables instead of columns on php_pools and wordpress_sites:
--   deploy/install.sh applies this directory in "tolerant" mode and documents
--   the invariant that every file here CREATEs tables and adds nothing to an
--   existing one, so that applying it twice is a no-op and a syntax error costs
--   one feature rather than the whole install. An ALTER TABLE on php_pools
--   would break that invariant. It would also break every existing query:
--   repository/php.go selects php_pools with an explicit column list, and
--   repository/wordpress.go uses SELECT *, so a new column on wordpress_sites
--   changes what scans into models.WordPressSite.
--
-- What each table is for:
--   php_pool_settings      the per-site pool settings that reach the pool file
--                          (memory limit, execution time, upload size,
--                          extensions), plus the record of what was last
--                          written and where.
--   wordpress_site_runtime the system identity a site's WP-CLI and FPM pool run
--                          as. This is the answer to "which user did that run
--                          as", and it has to be storable, not inferred.
--   wordpress_staging      one staging environment per production site, and the
--                          record of the last push including the database
--                          decision that was made and where production was
--                          backed up first.
-- ============================================================

-- ============================================================
-- PER-SITE PHP-FPM POOL SETTINGS
--
-- One row per pool. The columns are the settings that are rendered into the
-- pool file as php_admin_value, so a site cannot raise them again with
-- ini_set() from its own PHP code.
--
-- extensions is TEXT[] rather than JSONB because it is a set of names that is
-- only ever read whole and compared, and because the existing php_versions
-- .extensions is TEXT[] - one representation for one concept.
-- ============================================================
CREATE TABLE IF NOT EXISTS php_pool_settings (
    pool_id             UUID PRIMARY KEY REFERENCES php_pools(id) ON DELETE CASCADE,
    tenant_id           UUID NOT NULL REFERENCES tenants(id),

    -- The four the panel promises reach the pool file.
    memory_limit        VARCHAR(20)  NOT NULL DEFAULT '256M',
    max_execution_time  INT          NOT NULL DEFAULT 30,
    upload_max_filesize VARCHAR(20)  NOT NULL DEFAULT '64M',
    extensions          TEXT[]       NOT NULL DEFAULT '{}',

    -- The rest of the per-pool ini surface.
    post_max_size       VARCHAR(20)  NOT NULL DEFAULT '64M',
    max_input_time      INT          NOT NULL DEFAULT 60,
    max_file_uploads    INT          NOT NULL DEFAULT 20,
    timezone            VARCHAR(64)  NOT NULL DEFAULT 'UTC',
    display_errors      BOOLEAN      NOT NULL DEFAULT FALSE,
    disabled_functions  TEXT[]       NOT NULL DEFAULT '{}',
    open_basedir        TEXT[]       NOT NULL DEFAULT '{}',

    -- What was last actually written to disk, as opposed to what was asked
    -- for. These two differing is the signal that a write or a reload failed
    -- and was rolled back: the panel's intent is above, the host's reality is
    -- here, and an operator can see the gap instead of guessing.
    applied_php_version VARCHAR(20),
    pool_file           VARCHAR(500),
    socket_path         VARCHAR(500),
    last_applied_at     TIMESTAMP WITH TIME ZONE,
    last_error          TEXT,

    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_php_pool_settings_tenant ON php_pool_settings(tenant_id);

-- ============================================================
-- WORDPRESS SITE RUNTIME IDENTITY
--
-- run_as_user is the single most important column in this file. It is the unix
-- user that both the site's PHP-FPM pool and every WP-CLI command for the site
-- run as. WP-CLI is never run as root; this is the row that says who it IS run
-- as, and internal/wpcli refuses to execute when it resolves to uid 0.
--
-- It is NOT called system_user. PostgreSQL 16 made SYSTEM_USER a reserved
-- keyword (the SQL:2023 function that reports the authenticated identity), so
-- a column of that name is a syntax error that no amount of reading the file
-- reveals - it only appears when the migration is run against a real
-- PostgreSQL 16, which is how this one was found.
-- ============================================================
CREATE TABLE IF NOT EXISTS wordpress_site_runtime (
    site_id          UUID PRIMARY KEY REFERENCES wordpress_sites(id) ON DELETE CASCADE,
    tenant_id        UUID NOT NULL REFERENCES tenants(id),

    run_as_user      VARCHAR(32) NOT NULL,
    run_as_group     VARCHAR(32) NOT NULL,
    php_version      VARCHAR(20),

    -- The version WP-CLI last reported from the installation itself, as
    -- opposed to wordpress_sites.version, which is what the panel believes.
    installed_version VARCHAR(20),
    -- The identity string of the last WP-CLI run, e.g. "site-a:site-a (uid
    -- 1201, gid 1201)". Kept so the answer to "what did that run as" survives
    -- a log rotation.
    last_ran_as      VARCHAR(128),
    last_command     VARCHAR(255),
    last_ran_at      TIMESTAMP WITH TIME ZONE,

    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT wordpress_site_runtime_not_root CHECK (run_as_user <> 'root')
);

CREATE INDEX IF NOT EXISTS idx_wordpress_site_runtime_tenant ON wordpress_site_runtime(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wordpress_site_runtime_user  ON wordpress_site_runtime(run_as_user);

-- ============================================================
-- WORDPRESS STAGING
--
-- last_push_database is the column this table exists for. Pushing a staging
-- database over production is how a customer loses a week of orders, so the
-- decision is never defaulted and never inferred: the API requires it, and the
-- value that was chosen is recorded here next to the path production was
-- backed up to before the push ran.
-- ============================================================
CREATE TABLE IF NOT EXISTS wordpress_staging (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id),
    production_site_id  UUID NOT NULL REFERENCES wordpress_sites(id) ON DELETE CASCADE,

    staging_domain      VARCHAR(255) NOT NULL,
    staging_path        VARCHAR(500) NOT NULL,
    staging_url         VARCHAR(500) NOT NULL,
    staging_db_name     VARCHAR(255) NOT NULL,
    staging_db_user     VARCHAR(255) NOT NULL,
    staging_db_password VARCHAR(255) NOT NULL,
    staging_db_host     VARCHAR(255) NOT NULL DEFAULT 'localhost',

    status              VARCHAR(20) NOT NULL DEFAULT 'ready',
    block_indexing      BOOLEAN NOT NULL DEFAULT TRUE,

    last_clone_at       TIMESTAMP WITH TIME ZONE,
    last_push_at        TIMESTAMP WITH TIME ZONE,
    -- 'keep_production', 'overwrite_production' or 'database_only'. There is
    -- deliberately no default: a NULL here means no push has ever run, and a
    -- non-NULL is a decision somebody made on the record.
    last_push_database  VARCHAR(32),
    last_push_backup    VARCHAR(500),
    last_push_db_backup VARCHAR(500),
    last_error          TEXT,

    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- One staging environment per production site. A second one would share
    -- the staging database name and quietly overwrite the first.
    CONSTRAINT wordpress_staging_one_per_site UNIQUE (production_site_id),
    CONSTRAINT wordpress_staging_push_choice CHECK (
        last_push_database IS NULL
        OR last_push_database IN ('keep_production', 'overwrite_production', 'database_only')
    )
);

CREATE INDEX IF NOT EXISTS idx_wordpress_staging_tenant     ON wordpress_staging(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wordpress_staging_production ON wordpress_staging(production_site_id);
