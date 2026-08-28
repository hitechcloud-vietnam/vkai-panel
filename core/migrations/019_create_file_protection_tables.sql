-- Migration 019: Create File Protection tables
-- File integrity monitoring, change events, quarantine

CREATE TABLE file_protection_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    path VARCHAR(1000) NOT NULL,
    recursive BOOLEAN DEFAULT true,
    file_pattern VARCHAR(255) DEFAULT '*',
    watch_create BOOLEAN DEFAULT false,
    watch_modify BOOLEAN DEFAULT true,
    watch_delete BOOLEAN DEFAULT true,
    watch_permissions BOOLEAN DEFAULT true,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_protection_rules_tenant ON file_protection_rules(tenant_id);

CREATE TABLE file_integrity_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES file_protection_rules(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_path VARCHAR(1000) NOT NULL,
    sha256_hash VARCHAR(64) NOT NULL,
    file_size BIGINT DEFAULT 0,
    file_mode VARCHAR(20),
    owner VARCHAR(100),
    scanned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_integrity_rule ON file_integrity_records(rule_id);
CREATE INDEX idx_file_integrity_tenant ON file_integrity_records(tenant_id);
CREATE UNIQUE INDEX idx_file_integrity_path ON file_integrity_records(rule_id, file_path);

CREATE TABLE file_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES file_protection_rules(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_path VARCHAR(1000) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    old_hash VARCHAR(64),
    new_hash VARCHAR(64),
    old_mode VARCHAR(20),
    new_mode VARCHAR(20),
    details TEXT,
    severity VARCHAR(20) DEFAULT 'medium',
    is_read BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_change_events_tenant ON file_change_events(tenant_id);
CREATE INDEX idx_file_change_events_rule ON file_change_events(rule_id);
CREATE INDEX idx_file_change_events_created ON file_change_events(created_at);

CREATE TABLE file_quarantine (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    original_path VARCHAR(1000) NOT NULL,
    quarantine_path VARCHAR(1000) NOT NULL,
    sha256_hash VARCHAR(64),
    file_size BIGINT DEFAULT 0,
    reason TEXT,
    restored_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_quarantine_tenant ON file_quarantine(tenant_id);
