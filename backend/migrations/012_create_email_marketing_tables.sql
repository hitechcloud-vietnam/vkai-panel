-- Email Marketing Module Tables
-- Migration 012

CREATE TABLE IF NOT EXISTS email_campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    html_content TEXT NOT NULL,
    plain_text TEXT DEFAULT '',
    status VARCHAR(50) DEFAULT 'draft',
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    total_recipients INTEGER DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    open_count INTEGER DEFAULT 0,
    click_count INTEGER DEFAULT 0,
    bounce_count INTEGER DEFAULT 0,
    unsubscribe_count INTEGER DEFAULT 0,
    from_name VARCHAR(255) NOT NULL,
    from_email VARCHAR(255) NOT NULL,
    reply_to VARCHAR(255) DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_campaigns_tenant ON email_campaigns(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_email_campaigns_status ON email_campaigns(status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS email_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    email VARCHAR(255) NOT NULL,
    first_name VARCHAR(255) DEFAULT '',
    last_name VARCHAR(255) DEFAULT '',
    status VARCHAR(50) DEFAULT 'active',
    source VARCHAR(50) DEFAULT 'manual',
    tags TEXT[] DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_contacts_tenant ON email_contacts(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_email_contacts_email ON email_contacts(email) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS email_lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    contact_count INTEGER DEFAULT 0,
    double_opt_in BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_lists_tenant ON email_lists(tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS email_list_contacts (
    list_id UUID NOT NULL REFERENCES email_lists(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES email_contacts(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (list_id, contact_id)
);

CREATE TABLE IF NOT EXISTS email_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    html_content TEXT NOT NULL,
    category VARCHAR(100) DEFAULT '',
    thumbnail VARCHAR(500) DEFAULT '',
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_templates_tenant ON email_templates(tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS email_send_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES email_campaigns(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES email_contacts(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'queued',
    sent_at TIMESTAMPTZ,
    opened_at TIMESTAMPTZ,
    clicked_at TIMESTAMPTZ,
    bounced_at TIMESTAMPTZ,
    error_msg TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_send_logs_campaign ON email_send_logs(campaign_id);
CREATE INDEX IF NOT EXISTS idx_email_send_logs_contact ON email_send_logs(contact_id);
CREATE INDEX IF NOT EXISTS idx_email_send_logs_status ON email_send_logs(status);

CREATE TABLE IF NOT EXISTS email_automations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    trigger_type VARCHAR(50) NOT NULL,
    trigger_config TEXT DEFAULT '',
    status VARCHAR(50) DEFAULT 'draft',
    emails_sent INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_email_automations_tenant ON email_automations(tenant_id) WHERE deleted_at IS NULL;
