-- ============================================================
-- PENDING MIGRATION: scoped API keys, rotation and revocation
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_api_key_scopes.sql when it is folded in; nothing here depends on the
-- number. Every statement is guarded, so applying it twice is a no-op.
--
-- The api_keys table itself is 001_initial_schema.sql. It already has the two
-- columns this feature needs most - key_hash and the TEXT[] scopes column -
-- and what was missing was everything that makes a key manageable over its
-- life: a way to retire one without a flag day, a revocation that takes effect
-- on the next request, and a record of where the key is actually being used
-- from.
--
-- IMPORTANT for anyone adding a column here: internal/repository/multi_user.go
-- reads this table with `SELECT *` into models.APIKey. Every column added below
-- is also a field on that struct, in the same commit; without that, sqlx fails
-- every read of api_keys with "missing destination name", which is exactly how
-- the jobs table came to be broken on every install.
-- ============================================================

ALTER TABLE api_keys
    -- Revocation. Deleting the row would work too, but it destroys the audit
    -- trail's reference and makes "this key was revoked at 14:02 by whom"
    -- unanswerable, so a revoked key stays and is refused.
    ADD COLUMN IF NOT EXISTS revoked_at        TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS revoked_reason    VARCHAR(120),

    -- Rotation. The replacement key carries rotated_from pointing at the key
    -- it replaces; the key being replaced carries rotation_deadline, the
    -- instant it stops being accepted. Both keys authenticate until then,
    -- which is the whole point: an operator can deploy the new key to twelve
    -- machines over an afternoon instead of arranging for all twelve to change
    -- in the same second. A rotation with no overlap does not get done, and a
    -- key that never gets rotated is a key that outlives the person who made
    -- it.
    ADD COLUMN IF NOT EXISTS rotated_from      UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS rotation_deadline TIMESTAMP WITH TIME ZONE,

    -- Where the key was last used from. last_used already records when. An
    -- operator deciding whether a key is still needed needs both, and an
    -- operator investigating a leak needs the address more than the time.
    ADD COLUMN IF NOT EXISTS last_used_ip      VARCHAR(45),

    -- Optional source restriction for the key: a list of CIDR blocks or bare
    -- addresses the key may be presented from. NULL or empty means no
    -- restriction. A key pinned to the address of the machine that holds it is
    -- worth much less to anyone who copies it out of that machine.
    ADD COLUMN IF NOT EXISTS allowed_cidrs     TEXT[];

-- status vocabulary, all lower case:
--   active      the key authenticates
--   superseded  a replacement has been minted; the key authenticates until
--               rotation_deadline and is refused after it
--   revoked     refused from the next request onwards, revoked_at is set
--
-- No CHECK constraint is added: this column already exists on installed
-- databases with a default of 'active' and no constraint, and a constraint
-- added here would fail the migration on any row an older build wrote with a
-- value not in the list. The vocabulary is enforced in
-- internal/service/apikey.go, which is the only writer.

-- Finding the successor of a key, and reporting a rotation chain.
CREATE INDEX IF NOT EXISTS idx_api_keys_rotated_from ON api_keys(rotated_from);

-- The authentication lookup is by key_prefix (idx_api_keys_prefix, already in
-- 001). This partial index serves the "which keys are live right now" listing
-- that the rotation and expiry reporting walk, without carrying the revoked
-- rows that accumulate over the life of an installation.
CREATE INDEX IF NOT EXISTS idx_api_keys_live
    ON api_keys(tenant_id, expires_at)
    WHERE revoked_at IS NULL;
