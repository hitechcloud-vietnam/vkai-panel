-- Migration: WAF event context and WAF soft deletes
--
-- Two defects, both of the same shape: internal/repository/waf.go names columns
-- that migration 010 never created, so every WAF endpoint in the panel fails at
-- runtime on a real install. Verified against PostgreSQL 16 by applying
-- migrations 001-023 and preparing the repository's statements.
--
-- 1. waf_events is missing website_id, severity, action and details.
--    models.WAFEvent declares all four, ListEvents and GetStats select all
--    four, and CreateEvent inserts all four. Nothing in the schema comes close
--    to those names, so this is not a typo in the query: the columns were
--    simply left out of 010. A WAF event that cannot say which site was hit,
--    how bad it was, what the WAF did about it, or what matched is not a WAF
--    event, so the schema is what is wrong here and the columns are added.
--
--    website_id is nullable because a request can be blocked before it is
--    resolved to a site (unknown Host header, direct IP access), which is
--    exactly the traffic a WAF sees most of. It matches models.WAFEvent's
--    *uuid.UUID.
--
--    severity, action and details are NOT NULL DEFAULT '' because the Go
--    fields are plain strings: a NULL would fail to scan and take the events
--    list down. The defaults mirror waf_rules, whose severity and action carry
--    the same vocabulary.
--
-- 2. waf_rules and waf_policies are missing deleted_at. The repository is
--    built on soft deletes end to end: DeleteRule and DeletePolicy set
--    deleted_at, and every read filters "deleted_at IS NULL". That is a
--    deliberate design the whole file agrees on, and models.WAFRule and
--    models.WAFPolicy both declare DeletedAt, so the column is added rather
--    than the soft delete being torn out.

-- 1. WAF event context.
ALTER TABLE waf_events
    ADD COLUMN IF NOT EXISTS website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS severity VARCHAR(20) NOT NULL DEFAULT 'medium',
    ADD COLUMN IF NOT EXISTS action VARCHAR(20) NOT NULL DEFAULT 'block',
    ADD COLUMN IF NOT EXISTS details TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_waf_events_website_id ON waf_events(website_id);
CREATE INDEX IF NOT EXISTS idx_waf_events_severity ON waf_events(severity);

-- user_agent and attack_type are read into plain Go strings by the same
-- queries, so a NULL in either is the same crash. CreateEvent has always
-- written a string, so tightening them changes no behaviour the panel has.
UPDATE waf_events SET user_agent = '' WHERE user_agent IS NULL;
UPDATE waf_events SET attack_type = '' WHERE attack_type IS NULL;
ALTER TABLE waf_events
    ALTER COLUMN user_agent SET DEFAULT '',
    ALTER COLUMN user_agent SET NOT NULL,
    ALTER COLUMN attack_type SET DEFAULT '',
    ALTER COLUMN attack_type SET NOT NULL;

-- 2. Soft deletes for WAF rules and policies.
ALTER TABLE waf_rules
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE waf_policies
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

-- The reads all filter on deleted_at IS NULL, so the indexes that serve them
-- only need the live rows.
CREATE INDEX IF NOT EXISTS idx_waf_rules_tenant_live
    ON waf_rules(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_waf_policies_tenant_live
    ON waf_policies(tenant_id) WHERE deleted_at IS NULL;

-- description is read into a plain Go string in both tables, same as above.
UPDATE waf_rules SET description = '' WHERE description IS NULL;
UPDATE waf_policies SET description = '' WHERE description IS NULL;
ALTER TABLE waf_rules
    ALTER COLUMN description SET DEFAULT '',
    ALTER COLUMN description SET NOT NULL;
ALTER TABLE waf_policies
    ALTER COLUMN description SET DEFAULT '',
    ALTER COLUMN description SET NOT NULL;
