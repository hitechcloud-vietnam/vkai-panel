-- ============================================================
-- PENDING MIGRATION: notification delivery (outbox + alert dedup state)
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_notify.sql when it is folded in; nothing here depends on the number.
--
-- NOTE FOR WHOEVER FOLDS THIS IN: deploy/install.sh runs
--     find "${CORE_DIR}/migrations" -maxdepth 1 -name '*.sql'
-- so migrations/pending/ is NOT applied on a customer install. Until this file
-- is renumbered into migrations/, the tables below do not exist on an installed
-- panel and the delivery worker will refuse to start (it probes for them and
-- logs an error rather than failing every alert silently).
--
-- WHY NEW TABLES rather than reusing what 011_notifications.sql already has:
--
--   notifications           - an in-panel inbox row. It is the *record* that
--                             something happened, read by a human in the UI.
--                             It has no concept of an outbound attempt, a
--                             retry, or a channel, so it cannot carry delivery.
--   notification_channels   - where to send. Reused as-is; this migration adds
--                             no column to it. The senders read its `config`
--                             JSONB (smtp_password, bot_token, ...), which is
--                             why the API layer redacts that column on read.
--   notification_templates  - what to say. Reused as-is.
--   notification_preferences- who wants what. Reused as-is.
--
-- The two tables below are the parts that were missing: a durable outbox so an
-- alert survives a panel restart, and the per-alert state that makes
-- deduplication and the single "resolved" message possible.
-- ============================================================

-- ------------------------------------------------------------
-- notification_deliveries: the outbox. One row per (alert event, channel).
--
-- The row is written on the request path and nothing is sent there. A
-- background dispatcher claims due rows with FOR UPDATE SKIP LOCKED, sends,
-- and moves the row to 'sent' or back to 'pending' with a later
-- next_attempt_at. After max_attempts it becomes 'dead_letter' - never
-- deleted, never silently dropped, and visible at
-- GET /api/v1/notifications/deliveries?status=dead_letter.
--
-- Why Postgres and not the Redis job queue: the asynq queue in internal/job
-- keeps a task only in Redis, and its notification handler is a stub that
-- sleeps and returns nil. An alert enqueued there is lost on a Redis restart
-- with no trace anywhere. An alert that is trusted and dropped is worse than
-- no alerting, so the queue of record is the database.
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_deliveries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel_id        UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,

    -- The caller's grouping key, carried through so a dead letter can be tied
    -- back to the alert it belonged to.
    dedup_key         TEXT NOT NULL DEFAULT '',

    -- 'firing' | 'resolved' | 'test'
    event_kind        VARCHAR(16) NOT NULL,

    -- Rendered at enqueue time, not at send time: what the operator is told
    -- must not change because a template was edited between the two.
    subject           TEXT NOT NULL DEFAULT '',
    body              TEXT NOT NULL DEFAULT '',

    -- The structured alert, for senders that post JSON rather than text.
    payload           JSONB NOT NULL DEFAULT '{}',

    -- 'pending' | 'sending' | 'sent' | 'dead_letter'
    status            VARCHAR(16) NOT NULL DEFAULT 'pending',

    attempts          INTEGER NOT NULL DEFAULT 0,
    max_attempts      INTEGER NOT NULL DEFAULT 5,
    next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Plain string in Go, so never NULL. Secrets are scrubbed before anything
    -- is written here: the Telegram bot token lives in the request URL and
    -- would otherwise land in this column via a transport error.
    last_error        TEXT NOT NULL DEFAULT '',

    sent_at           TIMESTAMPTZ,
    dead_lettered_at  TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT notification_deliveries_status_check
        CHECK (status IN ('pending', 'sending', 'sent', 'dead_letter')),
    CONSTRAINT notification_deliveries_event_kind_check
        CHECK (event_kind IN ('firing', 'resolved', 'test'))
);

-- The dispatcher's hot path: due pending rows, oldest first. Partial, because
-- 'sent' rows accumulate and must not widen the index the worker scans.
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_due
    ON notification_deliveries (next_attempt_at)
    WHERE status = 'pending';

-- The stale-lease reaper: rows left 'sending' by a worker that died.
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_sending
    ON notification_deliveries (updated_at)
    WHERE status = 'sending';

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_tenant
    ON notification_deliveries (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_channel
    ON notification_deliveries (channel_id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_dedup
    ON notification_deliveries (tenant_id, dedup_key);

-- ------------------------------------------------------------
-- notification_alert_state: one row per live alert, keyed by the grouping key
-- the caller supplies.
--
-- This is what stops a disk at 92% sending one message per check for six
-- hours. The first observation notifies; observations inside the quiet period
-- only bump last_seen_at and occurrences; the first observation after the
-- quiet period notifies again (a reminder, because an unresolved outage should
-- not go quiet forever). When the caller reports the condition clear, the row
-- flips to 'resolved' and exactly one resolution message is sent - and only if
-- the alert had actually fired, so a resolve for something that never fired
-- sends nothing.
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_alert_state (
    tenant_id            UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    dedup_key            TEXT NOT NULL,

    -- 'firing' | 'resolved'
    state                VARCHAR(16) NOT NULL DEFAULT 'firing',

    first_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- NULL until something is actually enqueued for this key. The quiet period
    -- is measured from here, not from last_seen_at, so a suppressed
    -- observation does not push the next reminder further away.
    last_notified_at     TIMESTAMPTZ,

    -- How many observations have been folded into this incident, including the
    -- suppressed ones. Rendered into the reminder so an operator can see the
    -- alert has been repeating.
    occurrences          INTEGER NOT NULL DEFAULT 0,

    -- Seconds. 0 disables suppression entirely (every observation notifies).
    quiet_period_seconds INTEGER NOT NULL DEFAULT 3600,

    last_value           DOUBLE PRECISION,
    threshold            DOUBLE PRECISION,

    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (tenant_id, dedup_key),
    CONSTRAINT notification_alert_state_state_check
        CHECK (state IN ('firing', 'resolved'))
);

CREATE INDEX IF NOT EXISTS idx_notification_alert_state_firing
    ON notification_alert_state (tenant_id, last_seen_at)
    WHERE state = 'firing';

-- ------------------------------------------------------------
-- Permissions. 011_notifications.sql already seeds notifications:read and
-- notifications:write, and middleware.RequirePermission maps the HTTP method
-- to the action, so the delivery endpoints under /notifications are covered by
-- the existing pair. Nothing new is seeded here on purpose: an extra resource
-- would leave every existing role unable to see its own dead letters.
-- ------------------------------------------------------------
