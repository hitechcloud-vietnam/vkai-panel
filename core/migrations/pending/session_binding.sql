-- ============================================================
-- PENDING MIGRATION: panel sessions bound to their origin
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_session_binding.sql when it is folded in; nothing here depends on the
-- number. Every statement is guarded, so applying it twice is a no-op.
--
-- Why a new table rather than columns on user_sessions:
--
--   internal/repository/multi_user.go reads user_sessions with `SELECT *` into
--   models.UserSession. Any column added to that table breaks every one of
--   those reads until the struct is changed in the same commit, and that table
--   belongs to the multi-user module, not to authentication. Two features
--   writing different meanings into one row is how a schema stops being
--   understandable.
--
--   The two are also about different things. user_sessions is a record of
--   logins for the operator's activity screen; it is written once and read for
--   display. panel_sessions is consulted on EVERY authenticated request and
--   decides whether the request happens, so it is keyed by the access token's
--   jti and carries the state that decision needs.
--
-- What this table buys, and it is the thing a stateless JWT cannot do on its
-- own: ending a session takes effect on the next request instead of whenever
-- the token would have expired.
-- ============================================================

CREATE TABLE IF NOT EXISTS panel_sessions (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- The jti of the access token this session belongs to. Unique: one token,
    -- one session row, and the uniqueness is what makes establishment safe
    -- when two requests carrying a brand new token arrive at once.
    token_id           VARCHAR(64) NOT NULL UNIQUE,

    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id          UUID NOT NULL REFERENCES tenants(id),

    -- Where the session was established from, and the network that address
    -- sits in (a /24 for IPv4, a /48 for IPv6). The network is stored rather
    -- than recomputed so that changing the mask width later does not silently
    -- re-scope every live session.
    origin_ip          VARCHAR(45) NOT NULL,
    origin_network     VARCHAR(64) NOT NULL DEFAULT '',

    -- SHA-256 of the normalised User-Agent. The raw header is kept beside it
    -- for the operator's "which devices are signed in" screen; the comparison
    -- is over the hash of the normalised form, so a browser updating itself
    -- does not read as a different device.
    device_fingerprint VARCHAR(64) NOT NULL DEFAULT '',
    user_agent         TEXT NOT NULL DEFAULT '',

    last_seen_ip       VARCHAR(45),
    last_seen_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- How many times this session has been seen from outside its bound
    -- network. It is shown to the operator: a session that has moved eleven
    -- times today is worth a second look even when every move was legitimate.
    origin_changes     INTEGER NOT NULL DEFAULT 0,

    -- Set when the session moved far enough that the next state-changing
    -- request must wait for the password. Cleared when the operator proves it.
    reauth_required    BOOLEAN NOT NULL DEFAULT FALSE,

    -- Revocation. The row is kept until the token would have expired anyway:
    -- deleting it would let the next request carrying the same token establish
    -- a brand new session and undo the revocation.
    revoked_at         TIMESTAMP WITH TIME ZONE,
    revoked_reason     VARCHAR(64),

    created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at         TIMESTAMP WITH TIME ZONE NOT NULL
);

-- The operator's own "active sessions" list.
CREATE INDEX IF NOT EXISTS idx_panel_sessions_user ON panel_sessions(user_id, created_at DESC);

-- An administrator ending every session of an account, tenant scoped.
CREATE INDEX IF NOT EXISTS idx_panel_sessions_tenant ON panel_sessions(tenant_id);

-- The janitor that removes rows whose token has expired.
CREATE INDEX IF NOT EXISTS idx_panel_sessions_expires ON panel_sessions(expires_at);
