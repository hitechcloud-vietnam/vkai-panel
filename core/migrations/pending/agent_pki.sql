-- ============================================================
-- PENDING MIGRATION: retire the static agent token
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_agent_pki.sql when it is folded in; nothing here depends on the number.
--
-- WHAT IT IS FOR
--
-- servers.agent_token is the channel that protects every managed server today:
-- one uuid.New().String() per server, stored in the clear, sent on every agent
-- request, compared for equality, never expiring. The replacement is the
-- panel's internal CA (core/internal/agentpki): an operator mints a single-use,
-- time-limited enrolment token, the agent exchanges it once for a certificate,
-- and from then on the certificate is the identity.
--
-- The two channels have to coexist for exactly one upgrade, because an existing
-- installation has agents in the field that hold static tokens and cannot be
-- told anything until they check in. This migration is the bookkeeping that
-- makes the crossing visible and finishable.
--
-- WHY A TABLE AND NOT COLUMNS ON `servers`
--
-- repository/server.go reads servers with `SELECT *` into models.Server, so any
-- column added here breaks every server query until that struct is changed in
-- the same commit. A separate table is additive: an installation that has not
-- yet run this migration keeps working, and the code degrades to "no
-- bookkeeping" rather than to "no servers".
--
-- WHAT IS DELIBERATELY NOT HERE
--
-- No table for enrolment tokens, issued certificates or the deny list. That
-- state belongs to the CA, lives beside the CA key in
-- <SSLRoot>/agent-pki/state.json, and is written 0600 by a process that already
-- holds the key. Copying it into the database would put certificate state
-- somewhere the CA key is not, and give a database dump a second thing worth
-- stealing. agentpki.Store is the seam if a multi-process panel ever needs it.
-- ============================================================

-- One row per server that the panel has anything to say about, channel-wise.
-- Absence of a row means "never enrolled, token never used since this migration
-- ran", which is the state every existing server starts in.
CREATE TABLE IF NOT EXISTS server_agent_channel (
    server_id            UUID PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,

    -- 'static-token' or 'mutual-tls'. It records the last channel the panel
    -- actually accepted for this server, not an intention.
    channel              VARCHAR(32) NOT NULL DEFAULT 'static-token',

    -- The PKI identity, once the server has enrolled. It is the CommonName in
    -- the agent's certificate: agentpki.AgentRecord.AgentID.
    agent_id             VARCHAR(128),
    enrolled_at          TIMESTAMP WITH TIME ZONE,

    -- The last time the DEPRECATED channel was used. This is what tells an
    -- operator whether a server that has not enrolled is still alive and
    -- reporting, or simply gone.
    token_last_used_at   TIMESTAMP WITH TIME ZONE,

    -- Set when the static token is replaced by a value carrying the
    -- 'retired-' prefix. From that moment the old value authenticates nothing,
    -- and neither does the new one: agentpki.IsRetiredToken refuses it before
    -- any lookup happens.
    token_retired_at     TIMESTAMP WITH TIME ZONE,
    token_retired_by     VARCHAR(128),

    created_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT server_agent_channel_channel_check
        CHECK (channel IN ('static-token', 'mutual-tls'))
);

-- The census query at startup asks "which servers are still on the old
-- channel", which is a scan of servers left-joined onto this table; the index
-- is for the report that lists them by state.
CREATE INDEX IF NOT EXISTS idx_server_agent_channel_channel
    ON server_agent_channel(channel);
CREATE INDEX IF NOT EXISTS idx_server_agent_channel_retired
    ON server_agent_channel(token_retired_at);

COMMENT ON TABLE server_agent_channel IS
    'Which channel each managed server authenticates over while the deprecated static agent token is being retired.';
COMMENT ON COLUMN server_agent_channel.token_last_used_at IS
    'Last acceptance of the deprecated servers.agent_token. A server with a recent value here has not been enrolled yet.';

-- The old column is documented as deprecated rather than dropped. An operator
-- reading the schema should not have to read the Go to find out that this is
-- not a credential any more.
COMMENT ON COLUMN servers.agent_token IS
    'DEPRECATED. The pre-PKI static agent token: never expiring, stored in the clear, equality-compared. It is accepted only for a server that has not enrolled, and refused outright once the value carries the ''retired-'' prefix. Superseded by the certificate channel in core/internal/agentpki.';

-- ============================================================
-- WHEN THE MIGRATION IS FINISHED
--
-- The column goes when the startup census has reported static_token_only=0 on
-- every installation that will ever upgrade - not before, because dropping it
-- is what strands an agent that has not enrolled. The two statements below are
-- the whole of that removal and are deliberately left commented out:
--
--   ALTER TABLE servers DROP CONSTRAINT IF EXISTS servers_agent_token_key;
--   DROP INDEX IF EXISTS idx_servers_agent_token;
--   ALTER TABLE servers DROP COLUMN agent_token;
--
-- Dropping the column also means deleting Server.AgentToken from
-- internal/models/models.go, GetByAgentToken from repository/server.go, and
-- the static-token half of agentpki.Gateway.Authenticate, in one commit.
-- ============================================================
