-- ============================================================
-- PENDING MIGRATION: two-factor authentication (TOTP + recovery codes)
--
-- This file is not numbered yet. Rename it into the sequence as
-- 0NN_two_factor.sql when it is folded in; nothing here depends on the number.
--
-- Why new tables instead of columns on `users`:
--   repository/user.go reads users with `SELECT *` into models.User, so any
--   column added to `users` breaks every user query until that struct is
--   updated in the same commit. The existing users.mfa_enabled flag is kept in
--   step by the service and stays the panel-wide "is this account protected"
--   answer; users.mfa_secret is deliberately left NULL forever - the secret
--   belongs in the encrypted column below, never in a plaintext one.
-- ============================================================

CREATE TABLE IF NOT EXISTS user_two_factor (
    user_id           UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tenant_id         UUID NOT NULL REFERENCES tenants(id),

    -- AES-256-GCM ciphertext of the 160-bit TOTP secret, stored as
    -- "<key_version>:<base64(nonce||ciphertext)>". The key is derived from the
    -- panel master key (VKAI_SECRET_KEY) with a two-factor specific label, so a
    -- database dump on its own yields no usable second factor.
    secret_ciphertext TEXT NOT NULL,
    key_version       INT NOT NULL DEFAULT 1,

    algorithm         VARCHAR(16) NOT NULL DEFAULT 'SHA1',
    digits            SMALLINT NOT NULL DEFAULT 6,
    period_seconds    SMALLINT NOT NULL DEFAULT 30,

    -- FALSE until the user has proved one code from this secret. An account is
    -- never treated as protected on the strength of a generated secret alone.
    enabled           BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at      TIMESTAMP WITH TIME ZONE,

    -- Highest RFC 6238 time step already spent. Verification only accepts a
    -- strictly greater step, which is what stops a code being replayed inside
    -- its own ninety second validity window.
    last_step         BIGINT NOT NULL DEFAULT 0,
    last_used_at      TIMESTAMP WITH TIME ZONE,

    -- Per-account lockout. The rate limiter is keyed by source address; this
    -- follows the account, so spreading an attack over many addresses does not
    -- evade it.
    failed_attempts   INT NOT NULL DEFAULT 0,
    locked_until      TIMESTAMP WITH TIME ZONE,

    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT user_two_factor_digits_ck CHECK (digits BETWEEN 6 AND 10),
    CONSTRAINT user_two_factor_period_ck CHECK (period_seconds BETWEEN 15 AND 120),
    CONSTRAINT user_two_factor_confirmed_ck CHECK (
        (enabled = FALSE) OR (confirmed_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_user_two_factor_tenant ON user_two_factor(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_two_factor_enabled ON user_two_factor(enabled) WHERE enabled = TRUE;

-- ============================================================
-- Recovery codes: ten single-use bypasses, shown once at enrolment.
-- Only bcrypt hashes are stored, with the same hashing used for passwords,
-- because a recovery code is a password that a human types.
-- ============================================================

CREATE TABLE IF NOT EXISTS user_two_factor_recovery_codes (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,

    -- NULL means unused. A code is spent by a conditional UPDATE on this
    -- column, so two concurrent requests carrying the same code cannot both
    -- succeed.
    used_at    TIMESTAMP WITH TIME ZONE,
    used_ip    VARCHAR(45),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_two_factor_recovery_user
    ON user_two_factor_recovery_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_two_factor_recovery_unused
    ON user_two_factor_recovery_codes(user_id) WHERE used_at IS NULL;
