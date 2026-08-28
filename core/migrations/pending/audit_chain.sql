-- ============================================================
-- PENDING MIGRATION: tamper-evident audit log (hash chain, append-only)
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_audit_chain.sql when it is folded in; nothing here depends on the number.
--
-- WHY NOTHING IS ADDED TO audit_logs
--
--   internal/repository/audit.go reads audit_logs with `SELECT *` and, in
--   Search() and GetStats(), scans the result POSITIONALLY into eleven
--   destinations. Appending a column with ALTER TABLE puts it at the end of the
--   physical column order and breaks every one of those reads. Measured against
--   PostgreSQL 16.15 with one extra column added to audit_logs:
--
--       Search()  -> sql: expected 12 destination arguments in Scan, not 11
--       GetByID() -> missing destination name probe_extra in *models.AuditLog
--
--   So the shape of audit_logs is left exactly as migration 012 created it, and
--   the chain lives in side tables keyed by audit_logs.id. Repository code was
--   also converted to explicit column lists in the same change, so a future
--   column is survivable; this migration does not rely on that.
--
-- THE FORMAT (this is the part that must never change silently)
--
--   F(s)  = 8-byte big-endian unsigned length of the UTF-8 encoding of s,
--           followed by those UTF-8 bytes. Every field is framed this way, so
--           no value can be confused with a concatenation of other values.
--
--   content_hash = lowercase hex of SHA-256 over, in this exact order:
--       F("vkai-audit-content-v1")
--       F(audit_logs.id           as lowercase hyphenated UUID)
--       F(audit_logs.tenant_id    as lowercase hyphenated UUID)
--       F(audit_logs.user_id      as UUID, or "" when NULL)
--       F(audit_logs.action)
--       F(audit_logs.resource)
--       F(audit_logs.resource_id  as UUID, or "" when NULL)
--       F(audit_logs.details      as jsonb text, "{}" when NULL)
--       F(audit_logs.ip_address   or "" when NULL)
--       F(audit_logs.user_agent   or "" when NULL)
--       F(audit_logs.status)
--       F(audit_logs.created_at   as YYYY-MM-DDTHH:MM:SS.ffffffZ in UTC)
--
--   entry_hash = lowercase hex of SHA-256 over, in this exact order:
--       F("vkai-audit-chain-v1")
--       F(prev_hash)   -- entry_hash of seq-1, or 64 '0' characters at seq 1
--       F(tenant_id)
--       F(seq          as decimal ASCII, no padding, first entry is 1)
--       F(content_hash)
--
--   The two levels are deliberate. entry_hash depends only on the previous
--   hash, the tenant, the sequence number and content_hash, so the whole chain
--   can be walked without reading audit_logs at all - that is what makes
--   verification affordable on a large table. content_hash binds the row's
--   contents, and can be re-checked for any subrange independently because it
--   carries no ordering dependency.
--
--   details is hashed as PostgreSQL's own jsonb text rendering (keys sorted,
--   ", " and ": " separators) and the writer obtains that exact string with
--   INSERT ... RETURNING details::text. Nothing re-serialises JSON on either
--   side, so no canonicalisation disagreement is possible.
--
-- WHAT IS ENFORCED WHERE
--
--   1. REVOKE UPDATE, DELETE, TRUNCATE from PUBLIC and from the role that runs
--      this migration (the panel's own role in a default install). Measured on
--      PostgreSQL 16.15: revoking from the table owner does block the owner.
--      The owner can GRANT the privilege back to itself - that is one explicit
--      statement the panel never issues, and it is the reason for layers 2/3.
--   2. ENABLE ALWAYS triggers that refuse UPDATE, DELETE and TRUNCATE. UPDATE
--      and TRUNCATE have no exemption at all. DELETE has exactly one: the
--      transaction must carry a seal id in vkai.audit_prune AND a matching
--      audit_chain_seal row must already exist covering that entry. You cannot
--      remove entries without leaving an indelible record that you did.
--      (session_replication_role, the usual trigger bypass, is superuser-only;
--      verified on this cluster.)
--   3. The hash chain itself, which is detective rather than preventive and is
--      the only layer that still holds when the database superuser is the
--      attacker.
--   4. Checkpoint seals in audit_chain_seal, written by a clean full
--      verification pass. audit_chain_head is mutable - it advances on every
--      entry - so an attacker who removes the newest entries can move the head
--      to match and leave a chain that verifies. A seal cannot be moved. It is
--      the only witness against a competent truncation, and it is why the head
--      is not treated as evidence anywhere in this file.
--
-- WHAT THIS DOES NOT DO
--
--   An entry that was never written cannot be detected by any of this: a chain
--   proves what is in it, never what is missing from the front of it. The
--   defences against that are elsewhere - the append trigger fires on every
--   INSERT into audit_logs whoever wrote it, and coverage of security-relevant
--   actions is a matter of call sites, not of schema.
--
-- RETENTION
--
--   See audit_chain_prune() at the bottom of this file.
-- ============================================================

-- ------------------------------------------------------------
-- Canonical encoding and the two hashes, in SQL.
--
-- These are the authority for the format. internal/audit holds a Go
-- implementation and internal/audit/verify_audit_export.py a Python one; the
-- test suite asserts all three agree, which is what makes the published format
-- a specification rather than a description of one program.
-- ------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_chain_field(p_text text)
RETURNS bytea
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT int8send(octet_length(convert_to(COALESCE(p_text, ''), 'UTF8'))::bigint)
        || convert_to(COALESCE(p_text, ''), 'UTF8')
$$;

COMMENT ON FUNCTION audit_chain_field(text) IS
    'Length-prefixed framing of one hashed field: 8-byte big-endian length, then UTF-8 bytes.';

CREATE OR REPLACE FUNCTION audit_chain_timestamp(p_at timestamptz)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT to_char(p_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
$$;

COMMENT ON FUNCTION audit_chain_timestamp(timestamptz) IS
    'The hashed rendering of a timestamp: UTC, always six fractional digits, always a trailing Z.';

CREATE OR REPLACE FUNCTION audit_content_hash(
    p_id          uuid,
    p_tenant_id   uuid,
    p_user_id     uuid,
    p_action      text,
    p_resource    text,
    p_resource_id uuid,
    p_details     jsonb,
    p_ip_address  text,
    p_user_agent  text,
    p_status      text,
    p_created_at  timestamptz
) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT encode(sha256(
           audit_chain_field('vkai-audit-content-v1')
        || audit_chain_field(p_id::text)
        || audit_chain_field(p_tenant_id::text)
        || audit_chain_field(COALESCE(p_user_id::text, ''))
        || audit_chain_field(COALESCE(p_action, ''))
        || audit_chain_field(COALESCE(p_resource, ''))
        || audit_chain_field(COALESCE(p_resource_id::text, ''))
        || audit_chain_field(COALESCE(p_details, '{}'::jsonb)::text)
        || audit_chain_field(COALESCE(p_ip_address, ''))
        || audit_chain_field(COALESCE(p_user_agent, ''))
        || audit_chain_field(COALESCE(p_status, ''))
        || audit_chain_field(audit_chain_timestamp(p_created_at))
    ), 'hex')
$$;

CREATE OR REPLACE FUNCTION audit_entry_hash(
    p_prev_hash    text,
    p_tenant_id    uuid,
    p_seq          bigint,
    p_content_hash text
) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT encode(sha256(
           audit_chain_field('vkai-audit-chain-v1')
        || audit_chain_field(p_prev_hash)
        || audit_chain_field(p_tenant_id::text)
        || audit_chain_field(p_seq::text)
        || audit_chain_field(p_content_hash)
    ), 'hex')
$$;

CREATE OR REPLACE FUNCTION audit_chain_genesis()
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT repeat('0', 64) $$;

-- ------------------------------------------------------------
-- Tables
-- ------------------------------------------------------------

-- One row per audit_logs row. Append-only.
--
-- The hash columns are TEXT with a CHECK rather than CHAR(64): char(n) is
-- blank-padded and compares with trailing blanks ignored, which is a subtlety
-- nobody should have to hold in their head when reading a comparison of two
-- hashes. The CHECK also means a malformed hash cannot be stored at all. Deliberately carries a copy of
-- created_at so a verification pass can name the timestamp of a break even
-- when the audit_logs row it points at has been removed.
CREATE TABLE IF NOT EXISTS audit_log_chain (
    tenant_id    UUID        NOT NULL REFERENCES tenants(id),
    seq          BIGINT      NOT NULL CHECK (seq > 0),
    audit_log_id UUID        NOT NULL,
    prev_hash    TEXT        NOT NULL CHECK (prev_hash    ~ '^[0-9a-f]{64}$'),
    content_hash TEXT        NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    entry_hash   TEXT        NOT NULL CHECK (entry_hash   ~ '^[0-9a-f]{64}$'),
    created_at   TIMESTAMPTZ NOT NULL,
    linked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, seq)
);

CREATE UNIQUE INDEX IF NOT EXISTS audit_log_chain_log_id_key
    ON audit_log_chain (audit_log_id);
CREATE INDEX IF NOT EXISTS audit_log_chain_created_idx
    ON audit_log_chain (tenant_id, created_at);

-- The moving tip of each tenant's chain. This one IS mutable - it has to be, it
-- advances on every entry - so it is a pointer and not evidence, and it is the
-- one part of this design an attacker holding the panel's database role can
-- rewrite. A verification pass recomputes everything from audit_log_chain and
-- consults the head only as the CHEAP way to notice a truncated tail; the
-- undeniable way is a checkpoint seal, which lives in the append-only table
-- below. See audit_chain_seal.
CREATE TABLE IF NOT EXISTS audit_chain_head (
    tenant_id  UUID PRIMARY KEY REFERENCES tenants(id),
    seq        BIGINT      NOT NULL,
    prev_hash  TEXT        NOT NULL CHECK (prev_hash ~ '^[0-9a-f]{64}$'),
    head_hash  TEXT        NOT NULL CHECK (head_hash ~ '^[0-9a-f]{64}$'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A seal is a permanent statement that the chain had a particular hash at a
-- particular sequence number, in an append-only table. Three things create
-- seals:
--
--   prune       needed, or the surviving chain has nothing to anchor on.
--   export      so an exported bundle can be tied back to the live table later.
--   checkpoint  written by a clean full verification pass. This is what makes
--               a truncated tail undeniable: audit_chain_head is mutable by
--               design - it has to be, it moves on every entry - so an attacker
--               who lops off the newest entries can move the head to match and
--               leave a chain that verifies. A checkpoint says "this chain
--               reached sequence N with hash H at time T" in a table that
--               cannot be rewritten, so any later state with fewer than N
--               entries is a provable deletion no matter what the head says.
CREATE TABLE IF NOT EXISTS audit_chain_seal (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id),
    kind        VARCHAR(16) NOT NULL CHECK (kind IN ('prune', 'export', 'checkpoint', 'manual')),
    seq         BIGINT      NOT NULL,
    entry_hash  TEXT        NOT NULL CHECK (entry_hash ~ '^[0-9a-f]{64}$'),
    first_seq   BIGINT,
    last_seq    BIGINT,
    entry_count BIGINT      NOT NULL DEFAULT 0,
    cutoff      TIMESTAMPTZ,
    note        TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_chain_seal_seq_idx
    ON audit_chain_seal (tenant_id, seq);

-- The record of verification passes. Insert-only in practice, but not guarded:
-- it is a cache of work already done, not evidence. An incremental pass starts
-- from the newest successful row here; a full pass ignores it.
CREATE TABLE IF NOT EXISTS audit_chain_verification (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id),
    mode          VARCHAR(16) NOT NULL,
    from_seq      BIGINT      NOT NULL,
    to_seq        BIGINT      NOT NULL,
    checked       BIGINT      NOT NULL,
    ok            BOOLEAN     NOT NULL,
    break_seq     BIGINT,
    break_at      TIMESTAMPTZ,
    break_reason  TEXT,
    break_log_id  UUID,
    duration_ms   BIGINT      NOT NULL DEFAULT 0,
    ran_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_chain_verification_ok_idx
    ON audit_chain_verification (tenant_id, ok, to_seq DESC);

-- ------------------------------------------------------------
-- Append: a trigger, not repository code.
--
-- Every INSERT into audit_logs is chained, whatever wrote it - the repository,
-- a future service that forgets to go through it, a migration, or an operator
-- at a psql prompt. A chain that only covers the rows one Go function wrote is
-- not a chain.
--
-- Concurrency: the ON CONFLICT DO UPDATE on audit_chain_head takes a row lock
-- held to commit, so concurrent inserts for one tenant are serialised and no
-- two entries can claim the same seq or the same prev_hash. Different tenants
-- do not contend. Audit writes are low-volume by nature; this is the right
-- trade for a chain that cannot fork.
--
-- Cost, measured on PostgreSQL 16.15 with 2000 entries each in its own
-- transaction, which is the panel's write path: 0.61 ms per entry with this
-- trigger off, 2.14 ms with it on. The chain costs about 1.5 ms per audit
-- write. For a log that records operator actions rather than traffic, that is
-- the right side of the trade.
--
-- Writing many entries inside ONE transaction degrades superlinearly: every
-- entry leaves another version of the same head row and each one has to walk
-- the chain of them. A bulk operation that would write thousands of entries
-- should commit in batches rather than in one transaction. Generating a large
-- test fixture disables this trigger entirely and builds the chain set-based
-- instead; see generateSyntheticChain in
-- internal/repository/audit_chain_pg_test.go.
-- ------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_logs_chain_append()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    v_seq     bigint;
    v_prev    text;
    v_content text;
    v_entry   text;
    v_genesis text;
BEGIN
    v_content := audit_content_hash(NEW.id, NEW.tenant_id, NEW.user_id, NEW.action,
                                    NEW.resource, NEW.resource_id, NEW.details,
                                    NEW.ip_address, NEW.user_agent, NEW.status,
                                    NEW.created_at);
    v_genesis := audit_chain_genesis();

    -- One statement claims the sequence number, reads the hash it chains back
    -- to and writes the new tip. Doing it as an upsert plus a separate UPDATE
    -- would leave two row versions of the head per entry instead of one, and
    -- the head is the single hottest row in this design.
    INSERT INTO audit_chain_head AS h (tenant_id, seq, prev_hash, head_hash)
         VALUES (NEW.tenant_id, 1, v_genesis,
                 audit_entry_hash(v_genesis, NEW.tenant_id, 1, v_content))
    ON CONFLICT (tenant_id) DO UPDATE
            SET seq        = h.seq + 1,
                prev_hash  = h.head_hash,
                head_hash  = audit_entry_hash(h.head_hash, NEW.tenant_id, h.seq + 1, v_content),
                updated_at = NOW()
      RETURNING h.seq, h.prev_hash, h.head_hash
           INTO v_seq, v_prev, v_entry;

    INSERT INTO audit_log_chain (tenant_id, seq, audit_log_id, prev_hash,
                                 content_hash, entry_hash, created_at)
         VALUES (NEW.tenant_id, v_seq, NEW.id, v_prev, v_content, v_entry,
                 NEW.created_at);

    RETURN NULL;
END
$$;

DROP TRIGGER IF EXISTS audit_logs_chain_append ON audit_logs;
CREATE TRIGGER audit_logs_chain_append
    AFTER INSERT ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_chain_append();
ALTER TABLE audit_logs ENABLE ALWAYS TRIGGER audit_logs_chain_append;

-- ------------------------------------------------------------
-- Backfill: rows written before this migration.
--
-- A fresh install has none and this is a no-op. An upgrade must not leave an
-- unchained prefix, or the first verification pass reports the whole history as
-- missing. Ordered by (created_at, id) per tenant, which is the only total
-- order the old table can offer.
-- ------------------------------------------------------------

WITH RECURSIVE ordered AS (
    SELECT l.id, l.tenant_id, l.created_at,
           row_number() OVER (PARTITION BY l.tenant_id ORDER BY l.created_at, l.id) AS rn,
           audit_content_hash(l.id, l.tenant_id, l.user_id, l.action, l.resource,
                              l.resource_id, l.details, l.ip_address, l.user_agent,
                              l.status, l.created_at) AS content_hash
      FROM audit_logs l
     WHERE NOT EXISTS (SELECT 1 FROM audit_log_chain c WHERE c.audit_log_id = l.id)
), chained AS (
    SELECT o.tenant_id, o.rn, o.id, o.created_at, o.content_hash,
           audit_chain_genesis() AS prev_hash,
           audit_entry_hash(audit_chain_genesis(), o.tenant_id, o.rn, o.content_hash) AS entry_hash
      FROM ordered o
     WHERE o.rn = 1
    UNION ALL
    SELECT o.tenant_id, o.rn, o.id, o.created_at, o.content_hash,
           c.entry_hash,
           audit_entry_hash(c.entry_hash, o.tenant_id, o.rn, o.content_hash)
      FROM chained c
      JOIN ordered o ON o.tenant_id = c.tenant_id AND o.rn = c.rn + 1
)
INSERT INTO audit_log_chain (tenant_id, seq, audit_log_id, prev_hash, content_hash, entry_hash, created_at)
SELECT tenant_id, rn, id, prev_hash, content_hash, entry_hash, created_at FROM chained;

INSERT INTO audit_chain_head (tenant_id, seq, prev_hash, head_hash)
SELECT DISTINCT ON (c.tenant_id) c.tenant_id, c.seq, c.prev_hash, c.entry_hash
  FROM audit_log_chain c
 ORDER BY c.tenant_id, c.seq DESC
ON CONFLICT (tenant_id) DO NOTHING;

-- ------------------------------------------------------------
-- Append-only enforcement
-- ------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_append_only_row()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    v_token text;
    v_seq   bigint;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_token := NULLIF(current_setting('vkai.audit_prune', true), '');
        IF v_token IS NOT NULL THEN
            IF TG_TABLE_NAME = 'audit_log_chain' THEN
                v_seq := OLD.seq;
            ELSE
                SELECT c.seq INTO v_seq FROM audit_log_chain c WHERE c.audit_log_id = OLD.id;
            END IF;

            IF EXISTS (
                SELECT 1 FROM audit_chain_seal s
                 WHERE s.id::text  = v_token
                   AND s.kind      = 'prune'
                   AND s.tenant_id = OLD.tenant_id
                   AND (v_seq IS NULL OR s.last_seq >= v_seq)
            ) THEN
                RETURN OLD;
            END IF;
        END IF;
    END IF;

    RAISE EXCEPTION
        'audit log is append-only: % on % was refused', TG_OP, TG_TABLE_NAME
        USING ERRCODE = '42501',
              HINT = 'Removing entries is only possible through audit_chain_prune(), which records a seal first.';
END
$$;

CREATE OR REPLACE FUNCTION audit_append_only_truncate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'audit log is append-only: TRUNCATE on % was refused', TG_TABLE_NAME
        USING ERRCODE = '42501';
END
$$;

DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['audit_logs', 'audit_log_chain', 'audit_chain_seal'] LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_append_only', t);
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION audit_append_only_row()',
            t || '_append_only', t);
        EXECUTE format('ALTER TABLE %I ENABLE ALWAYS TRIGGER %I', t, t || '_append_only');

        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_no_truncate', t);
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION audit_append_only_truncate()',
            t || '_no_truncate', t);
        EXECUTE format('ALTER TABLE %I ENABLE ALWAYS TRIGGER %I', t, t || '_no_truncate');
    END LOOP;
END
$$;

-- Layer 1: privileges. PUBLIC first, then the role this migration runs as,
-- which in a default install is the same role the panel connects with.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs, audit_log_chain, audit_chain_seal FROM PUBLIC;

DO $$
BEGIN
    EXECUTE format(
        'REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs, audit_log_chain, audit_chain_seal FROM %I',
        current_user);
END
$$;

-- ------------------------------------------------------------
-- Verification
--
-- audit_chain_verify(tenant, from_seq, to_seq, deep) walks a range and returns
-- the FIRST break with its sequence number and timestamp, or ok = true.
--
--   deep = false  reads audit_log_chain only: an ordered index scan of one
--                 narrow table, one SHA-256 over ~200 bytes per row, no join.
--                 Catches a removed entry (sequence gap), a reordered one
--                 (entry_hash binds seq), a relinked one, and a rewritten
--                 chain row.
--   deep = true   additionally re-reads audit_logs and recomputes content_hash.
--                 This is what catches an edited entry. It costs a join, so it
--                 is the expensive half - and because content_hash carries no
--                 ordering dependency, any subrange of it can be checked on its
--                 own, in parallel, or as a sample. See the note on
--                 AS MATERIALIZED below for why the join is shaped as it is.
--
-- Truncation: everything above can be perfect on a chain whose newest entries
-- were thrown away, so two independent witnesses are checked. audit_chain_head
-- is the cheap one and can be rewritten by an attacker holding the database.
-- A checkpoint or export seal is the one that cannot: it is in an append-only
-- table and it says the chain once reached a sequence number. Either witness
-- naming a higher sequence than survives is reported as truncated_tail.
--
-- Anchoring: the entry at from_seq must chain back to something. In order of
-- preference that is the chain row at from_seq - 1, then a seal recorded at
-- from_seq - 1 (which is what a prune leaves behind), then the genesis hash if
-- from_seq is 1. If none of those exist the range is unanchored and that is
-- itself reported as the break.
-- ------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_chain_verify(
    p_tenant uuid,
    p_from   bigint DEFAULT 1,
    p_to     bigint DEFAULT 9223372036854775807,
    p_deep   boolean DEFAULT true
) RETURNS TABLE (
    ok           boolean,
    checked      bigint,
    first_seq    bigint,
    last_seq     bigint,
    break_seq    bigint,
    break_at     timestamptz,
    break_reason text,
    break_log_id uuid,
    head_seq     bigint,
    head_ok      boolean
)
LANGUAGE plpgsql STABLE
AS $$
DECLARE
    v_first     bigint;
    v_last      bigint;
    v_count     bigint;
    v_anchor    text;
    v_head_seq  bigint;
    v_head_hash text;
    v_last_hash text;
    v_bseq      bigint;
    v_bat       timestamptz;
    v_breason   text;
    v_blog      uuid;
    v_dseq      bigint;
    v_dat       timestamptz;
    v_dreason   text;
    v_dlog      uuid;
    v_last_at   timestamptz;
    v_head_ok   boolean;
    v_seal_seq  bigint;
BEGIN
    SELECT min(c.seq), max(c.seq), count(*) INTO v_first, v_last, v_count
      FROM audit_log_chain c
     WHERE c.tenant_id = p_tenant AND c.seq BETWEEN p_from AND p_to;

    SELECT h.seq, h.head_hash INTO v_head_seq, v_head_hash
      FROM audit_chain_head h WHERE h.tenant_id = p_tenant;

    IF v_count = 0 THEN
        -- No entries in the range. For a tenant that has never written one,
        -- that is the truth and nothing more. But a chain that HAD a head, or
        -- that something once sealed, and now has nothing in it, has not gone
        -- quiet - it has been emptied, and reporting that as "intact, nothing
        -- to check" would be the single most useful lie this function could
        -- tell. The p_to guard keeps a request for an interior range that
        -- happens to be empty from reading as a deletion.
        SELECT max(s.seq) INTO v_seal_seq
          FROM audit_chain_seal s
         WHERE s.tenant_id = p_tenant AND s.kind IN ('checkpoint', 'export');

        IF (v_head_seq IS NOT NULL AND p_to >= v_head_seq)
           OR (v_seal_seq IS NOT NULL AND p_to >= v_seal_seq) THEN
            RETURN QUERY SELECT false, 0::bigint, NULL::bigint, NULL::bigint,
                                1::bigint, NULL::timestamptz, 'truncated_tail'::text, NULL::uuid,
                                v_head_seq, false;
            RETURN;
        END IF;

        RETURN QUERY SELECT true, 0::bigint, NULL::bigint, NULL::bigint,
                            NULL::bigint, NULL::timestamptz, NULL::text, NULL::uuid,
                            v_head_seq, (v_head_seq IS NULL);
        RETURN;
    END IF;

    -- Resolve the anchor for v_first.
    SELECT c.entry_hash INTO v_anchor
      FROM audit_log_chain c WHERE c.tenant_id = p_tenant AND c.seq = v_first - 1;
    IF v_anchor IS NULL THEN
        SELECT s.entry_hash INTO v_anchor
          FROM audit_chain_seal s
         WHERE s.tenant_id = p_tenant AND s.seq = v_first - 1
         ORDER BY s.created_at DESC LIMIT 1;
    END IF;
    IF v_anchor IS NULL AND v_first = 1 THEN
        v_anchor := audit_chain_genesis();
    END IF;

    IF v_anchor IS NULL THEN
        SELECT c.created_at, c.audit_log_id INTO v_bat, v_blog
          FROM audit_log_chain c WHERE c.tenant_id = p_tenant AND c.seq = v_first;
        RETURN QUERY SELECT false, 0::bigint, v_first, v_last,
                            v_first, v_bat, 'missing_anchor'::text, v_blog,
                            v_head_seq, false;
        RETURN;
    END IF;

    -- Structural pass. Cheap: audit_log_chain only.
    SELECT w.seq, w.created_at, w.audit_log_id, w.reason
      INTO v_bseq, v_bat, v_blog, v_breason
      FROM (
        SELECT b.seq, b.created_at, b.audit_log_id,
               CASE
                 WHEN b.prev_seq IS NULL AND b.prev_hash <> v_anchor
                      THEN 'anchor_mismatch'
                 WHEN b.prev_seq IS NOT NULL AND b.prev_seq <> b.seq - 1
                      THEN 'sequence_gap'
                 WHEN b.prev_seq IS NOT NULL AND b.prev_entry <> b.prev_hash
                      THEN 'chain_broken'
                 WHEN audit_entry_hash(b.prev_hash, p_tenant, b.seq, b.content_hash) <> b.entry_hash
                      THEN 'entry_hash_mismatch'
               END AS reason
          FROM (
            SELECT c.seq, c.created_at, c.audit_log_id, c.prev_hash,
                   c.content_hash, c.entry_hash,
                   lag(c.seq)        OVER (ORDER BY c.seq) AS prev_seq,
                   lag(c.entry_hash) OVER (ORDER BY c.seq) AS prev_entry
              FROM audit_log_chain c
             WHERE c.tenant_id = p_tenant AND c.seq BETWEEN p_from AND p_to
          ) b
      ) w
     WHERE w.reason IS NOT NULL
     ORDER BY w.seq
     LIMIT 1;

    -- Content pass, only if asked for and only up to any structural break: a
    -- break already tells the operator where to look.
    IF p_deep THEN
        -- AS MATERIALIZED is load-bearing, not decoration.
        --
        -- Without it the planner sees ORDER BY seq LIMIT 1, hopes to stop at
        -- the first row, and picks a nested loop: one index lookup into
        -- audit_logs per chain row. On an intact log there is no first row, so
        -- it pays that ten million times, in audit_log_id order - which is
        -- random, because the ids are random UUIDs. Measured on ten million
        -- entries: a nested loop over 2.5 GB of audit_logs with 128 MB of
        -- shared_buffers is bound by random I/O.
        --
        -- MATERIALIZED makes it plan for every row instead. It then hash joins
        -- two sequential scans, and the estimated cost drops from 49.6M to
        -- 1.7M. The trade is that a break near the start no longer stops the
        -- scan early - but the structural pass above has usually already found
        -- one and bounded this range through COALESCE(v_bseq, p_to), and the
        -- case worth optimising is the one that runs on a schedule against a
        -- log that is fine.
        WITH bad AS MATERIALIZED (
            SELECT c.seq, c.created_at, c.audit_log_id,
                   CASE WHEN l.id IS NULL THEN 'entry_missing'
                        ELSE 'content_altered' END AS reason
              FROM audit_log_chain c
              LEFT JOIN audit_logs l ON l.id = c.audit_log_id
             WHERE c.tenant_id = p_tenant
               AND c.seq BETWEEN p_from AND COALESCE(v_bseq, p_to)
               AND (l.id IS NULL
                    OR audit_content_hash(l.id, l.tenant_id, l.user_id, l.action,
                                          l.resource, l.resource_id, l.details,
                                          l.ip_address, l.user_agent, l.status,
                                          l.created_at) <> c.content_hash)
        )
        SELECT b.seq, b.created_at, b.audit_log_id, b.reason
          INTO v_dseq, v_dat, v_dlog, v_dreason
          FROM bad b
         ORDER BY b.seq
         LIMIT 1;

        IF v_dseq IS NOT NULL AND (v_bseq IS NULL OR v_dseq < v_bseq) THEN
            v_bseq    := v_dseq;
            v_bat     := v_dat;
            v_blog    := v_dlog;
            v_breason := v_dreason;
        END IF;
    END IF;

    -- The head is the only witness that the NEWEST entries are still there:
    -- lopping off the tail leaves a chain that is internally perfect. The check
    -- only applies when the requested range reaches the head; verifying an
    -- interior slice says nothing about the tip.
    SELECT c.entry_hash, c.created_at INTO v_last_hash, v_last_at
      FROM audit_log_chain c WHERE c.tenant_id = p_tenant AND c.seq = v_last;

    IF v_head_seq IS NULL OR p_to >= v_head_seq THEN
        v_head_ok := (v_head_seq IS NOT NULL
                      AND v_head_seq = v_last
                      AND v_head_hash = v_last_hash);
    ELSE
        v_head_ok := true;
    END IF;

    -- The head can be rewritten by whoever can write to this database, so it
    -- is not the last word on a truncated tail. A checkpoint or export seal is:
    -- it says, in a table that refuses UPDATE and DELETE, that the chain once
    -- reached a sequence number. Fewer entries than that now is a deletion, and
    -- no amount of tidying the head hides it.
    SELECT max(s.seq) INTO v_seal_seq
      FROM audit_chain_seal s
     WHERE s.tenant_id = p_tenant AND s.kind IN ('checkpoint', 'export');

    IF v_seal_seq IS NOT NULL AND v_seal_seq > v_last AND p_to >= v_seal_seq THEN
        v_head_ok := false;
    END IF;

    IF NOT v_head_ok AND v_bseq IS NULL THEN
        -- break_seq is the first sequence number that should be there and is
        -- not; break_at is the timestamp of the last entry that survived, which
        -- is the earliest moment the truncation can have happened.
        v_bseq    := v_last + 1;
        v_bat     := v_last_at;
        v_breason := 'truncated_tail';
    END IF;

    RETURN QUERY SELECT
        (v_bseq IS NULL),
        v_count,
        v_first,
        v_last,
        v_bseq,
        v_bat,
        v_breason,
        v_blog,
        v_head_seq,
        v_head_ok;
END
$$;

-- ------------------------------------------------------------
-- Retention
--
-- Immutability and retention pull against each other and there is no way to
-- have both. What this design refuses to do is pretend: entries can be removed,
-- but only from the OLD end, only in one documented way, and never without
-- leaving a seal that says how many went and what the chain hash was at the cut.
--
-- What an operator must do (the panel cannot do this for them - the panel's own
-- role has had DELETE revoked, deliberately):
--
--   BEGIN;
--     GRANT DELETE ON audit_logs, audit_log_chain TO vkai;
--     SELECT * FROM audit_chain_prune(
--         '<tenant-uuid>'::uuid,
--         NOW() - INTERVAL '365 days',
--         'annual retention policy');
--     REVOKE DELETE ON audit_logs, audit_log_chain FROM vkai;
--   COMMIT;
--
-- Do the export first if the entries need to outlive the table. The preview
-- names the sequence number the cut will land on:
--
--   GET /api/v1/audit/chain/retention?days=365   -> seal_seq
--   GET /api/v1/audit/chain/export?to_seq=<seal_seq>
--
-- A bundle is capped at 50 000 entries, so a long history comes out as a series:
-- take the next with from_seq set to one past the last entry of the previous
-- bundle. They chain: the anchor of each names the last entry hash of the one
-- before it.
--
-- The exported bundle stays independently verifiable forever, because every
-- entry carries its own prev_hash and entry_hash. Pruning does not invalidate
-- it; it just means the live table no longer holds those rows.
--
-- How the surviving chain stays verifiable: the seal records the entry_hash at
-- the cut point, and audit_chain_verify() accepts a seal as the anchor for the
-- oldest surviving entry. Sequence numbers are never reused - the head only
-- moves forward - so a pruned chain reads as "verified from seq N, anchored on
-- the seal recorded at N-1 on <date>", not as a gap.
--
-- What is genuinely lost: the contents of the pruned entries, and the ability
-- to prove the chain from genesis without the exported bundle. That is the
-- price of retention and it is stated here rather than hidden.
--
-- The prune itself is written into the audit log, after the deletion, so it is
-- the first thing in the surviving chain that mentions it.
-- ------------------------------------------------------------

CREATE OR REPLACE FUNCTION audit_chain_prune(
    p_tenant uuid,
    p_before timestamptz,
    p_note   text DEFAULT ''
) RETURNS TABLE (
    pruned    bigint,
    first_seq bigint,
    seal_seq  bigint,
    seal_hash text,
    seal_id   uuid
)
LANGUAGE plpgsql
AS $$
DECLARE
    v_seq   bigint;
    v_hash  text;
    v_first bigint;
    v_count bigint;
    v_seal  uuid;
BEGIN
    SELECT c.seq, c.entry_hash INTO v_seq, v_hash
      FROM audit_log_chain c
     WHERE c.tenant_id = p_tenant AND c.created_at < p_before
     ORDER BY c.seq DESC
     LIMIT 1;

    IF v_seq IS NULL THEN
        RETURN QUERY SELECT 0::bigint, NULL::bigint, NULL::bigint, NULL::text, NULL::uuid;
        RETURN;
    END IF;

    SELECT min(c.seq), count(*) INTO v_first, v_count
      FROM audit_log_chain c
     WHERE c.tenant_id = p_tenant AND c.seq <= v_seq;

    INSERT INTO audit_chain_seal (tenant_id, kind, seq, entry_hash, first_seq,
                                  last_seq, entry_count, cutoff, note)
         VALUES (p_tenant, 'prune', v_seq, v_hash, v_first, v_seq, v_count,
                 p_before, COALESCE(p_note, ''))
      RETURNING id INTO v_seal;

    -- Narrow, transaction-local exemption. The append-only trigger checks that
    -- this names a real seal covering the row being removed, so a deletion
    -- without a seal is impossible even for a caller holding DELETE.
    PERFORM set_config('vkai.audit_prune', v_seal::text, true);

    DELETE FROM audit_logs
     WHERE id IN (SELECT c.audit_log_id FROM audit_log_chain c
                   WHERE c.tenant_id = p_tenant AND c.seq <= v_seq);
    DELETE FROM audit_log_chain
     WHERE tenant_id = p_tenant AND seq <= v_seq;

    PERFORM set_config('vkai.audit_prune', '', true);

    INSERT INTO audit_logs (tenant_id, action, resource, details, status)
         VALUES (p_tenant, 'audit.prune', 'audit',
                 jsonb_build_object(
                     'seal_id',     v_seal,
                     'first_seq',   v_first,
                     'seal_seq',    v_seq,
                     'seal_hash',   v_hash,
                     'entry_count', v_count,
                     'cutoff',      audit_chain_timestamp(p_before),
                     'note',        COALESCE(p_note, '')),
                 'success');

    RETURN QUERY SELECT v_count, v_first, v_seq, v_hash, v_seal;
END
$$;

-- A read-only answer to "what would a prune remove?", which the panel CAN run
-- because it needs no privilege the panel has been denied.
CREATE OR REPLACE FUNCTION audit_chain_prune_preview(
    p_tenant uuid,
    p_before timestamptz
) RETURNS TABLE (
    prunable  bigint,
    first_seq bigint,
    seal_seq  bigint,
    seal_hash text,
    oldest_at timestamptz,
    newest_at timestamptz
)
LANGUAGE sql STABLE
AS $$
    WITH cut AS (
        SELECT c.seq, c.entry_hash
          FROM audit_log_chain c
         WHERE c.tenant_id = p_tenant AND c.created_at < p_before
         ORDER BY c.seq DESC
         LIMIT 1
    )
    SELECT COALESCE(count(c.seq), 0)::bigint,
           min(c.seq), (SELECT seq FROM cut), (SELECT entry_hash FROM cut),
           min(c.created_at), max(c.created_at)
      FROM audit_log_chain c
     WHERE c.tenant_id = p_tenant
       AND c.seq <= COALESCE((SELECT seq FROM cut), -1);
$$;
