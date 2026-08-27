-- WAF (Web Application Firewall) tables

-- WAF Rules
CREATE TABLE IF NOT EXISTS waf_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    rule_type VARCHAR(50) NOT NULL, -- sql_injection, xss, path_traversal, etc.
    severity VARCHAR(20) NOT NULL DEFAULT 'medium', -- low, medium, high, critical
    action VARCHAR(20) NOT NULL DEFAULT 'block', -- block, log, allow
    pattern TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- WAF Policies
CREATE TABLE IF NOT EXISTS waf_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    mode VARCHAR(20) NOT NULL DEFAULT 'detection', -- detection, prevention
    paranoia_level INTEGER NOT NULL DEFAULT 1, -- 1-4
    anomaly_threshold INTEGER NOT NULL DEFAULT 5,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- WAF Events
CREATE TABLE IF NOT EXISTS waf_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id UUID REFERENCES waf_rules(id) ON DELETE SET NULL,
    source_ip INET NOT NULL,
    method VARCHAR(10) NOT NULL,
    path TEXT NOT NULL,
    user_agent TEXT,
    attack_type VARCHAR(50),
    blocked BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_waf_rules_tenant_id ON waf_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_waf_rules_enabled ON waf_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_waf_policies_tenant_id ON waf_policies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_waf_events_tenant_id ON waf_events(tenant_id);
CREATE INDEX IF NOT EXISTS idx_waf_events_created_at ON waf_events(created_at);
CREATE INDEX IF NOT EXISTS idx_waf_events_source_ip ON waf_events(source_ip);
CREATE INDEX IF NOT EXISTS idx_waf_events_attack_type ON waf_events(attack_type);
