-- Migration 013: Create Mail Server tables
-- Mail domains, accounts, aliases, queue, spam filters, server config

CREATE TABLE IF NOT EXISTS mail_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    is_verified BOOLEAN DEFAULT false,
    mx_record VARCHAR(500),
    spf_record VARCHAR(500),
    dkim_enabled BOOLEAN DEFAULT false,
    dmarc_record VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_mail_domains_tenant ON mail_domains(tenant_id);
CREATE UNIQUE INDEX idx_mail_domains_domain ON mail_domains(domain);

CREATE TABLE IF NOT EXISTS mail_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES mail_domains(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    password VARCHAR(500) NOT NULL,
    quota_mb INTEGER DEFAULT 1024,
    used_mb INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    forward_to VARCHAR(500),
    auto_reply BOOLEAN DEFAULT false,
    auto_reply_msg TEXT,
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_mail_accounts_tenant ON mail_accounts(tenant_id);
CREATE INDEX idx_mail_accounts_domain ON mail_accounts(domain_id);
CREATE UNIQUE INDEX idx_mail_accounts_email ON mail_accounts(email);

CREATE TABLE IF NOT EXISTS mail_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES mail_domains(id) ON DELETE CASCADE,
    source VARCHAR(255) NOT NULL,
    destination VARCHAR(500) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_mail_aliases_tenant ON mail_aliases(tenant_id);
CREATE INDEX idx_mail_aliases_domain ON mail_aliases(domain_id);

CREATE TABLE IF NOT EXISTS mail_dkim_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES mail_domains(id) ON DELETE CASCADE,
    selector VARCHAR(100) NOT NULL DEFAULT 'default',
    public_key TEXT NOT NULL,
    private_key TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_mail_dkim_domain ON mail_dkim_keys(domain_id);

CREATE TABLE IF NOT EXISTS mail_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    from_address VARCHAR(255) NOT NULL,
    to_address VARCHAR(255) NOT NULL,
    subject VARCHAR(500),
    status VARCHAR(50) DEFAULT 'queued',
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    scheduled_at TIMESTAMP WITH TIME ZONE,
    sent_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_mail_queue_tenant ON mail_queue(tenant_id);
CREATE INDEX idx_mail_queue_status ON mail_queue(status);

CREATE TABLE IF NOT EXISTS mail_spam_filters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT true,
    spam_threshold DOUBLE PRECISION DEFAULT 5.0,
    reject_score DOUBLE PRECISION DEFAULT 15.0,
    greylisting BOOLEAN DEFAULT false,
    blacklist TEXT[],
    whitelist TEXT[],
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_mail_spam_filters_tenant ON mail_spam_filters(tenant_id);

CREATE TABLE IF NOT EXISTS mail_server_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    hostname VARCHAR(255) DEFAULT 'mail.example.com',
    smtp_port INTEGER DEFAULT 25,
    smtps_port INTEGER DEFAULT 587,
    imap_port INTEGER DEFAULT 143,
    imaps_port INTEGER DEFAULT 993,
    max_message_size INTEGER DEFAULT 25,
    max_mailboxes INTEGER DEFAULT 0,
    tls_enabled BOOLEAN DEFAULT true,
    cert_path VARCHAR(500),
    key_path VARCHAR(500),
    is_running BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_mail_server_configs_tenant ON mail_server_configs(tenant_id);
