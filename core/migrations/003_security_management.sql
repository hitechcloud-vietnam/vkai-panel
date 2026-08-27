-- vKAI Panel Database Schema - Security Management
-- PostgreSQL

-- ============================================================
-- SECURITY SCANS
-- ============================================================
CREATE TABLE security_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    scan_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    score INT DEFAULT 0,
    total_checks INT DEFAULT 0,
    passed_checks INT DEFAULT 0,
    failed_checks INT DEFAULT 0,
    warnings INT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_security_scans_tenant ON security_scans(tenant_id);
CREATE INDEX idx_security_scans_server ON security_scans(server_id);
CREATE INDEX idx_security_scans_status ON security_scans(status);
CREATE INDEX idx_security_scans_created ON security_scans(created_at DESC);

-- ============================================================
-- SECURITY VULNERABILITIES
-- ============================================================
CREATE TABLE security_vulnerabilities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id UUID NOT NULL REFERENCES security_scans(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    severity VARCHAR(20) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    affected VARCHAR(500),
    solution TEXT,
    cve VARCHAR(50),
    cvss DECIMAL(3,1),
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_security_vulnerabilities_scan ON security_vulnerabilities(scan_id);
CREATE INDEX idx_security_vulnerabilities_tenant ON security_vulnerabilities(tenant_id);
CREATE INDEX idx_security_vulnerabilities_severity ON security_vulnerabilities(severity);
CREATE INDEX idx_security_vulnerabilities_status ON security_vulnerabilities(status);

-- ============================================================
-- SECURITY CHECKS
-- ============================================================
CREATE TABLE security_checks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id UUID NOT NULL REFERENCES security_scans(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    category VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    details TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_security_checks_scan ON security_checks(scan_id);
CREATE INDEX idx_security_checks_tenant ON security_checks(tenant_id);
CREATE INDEX idx_security_checks_category ON security_checks(category);

-- ============================================================
-- SECURITY POLICIES
-- ============================================================
CREATE TABLE security_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100) NOT NULL,
    rules JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_security_policies_tenant ON security_policies(tenant_id);
CREATE INDEX idx_security_policies_category ON security_policies(category);

-- ============================================================
-- SEED DATA: Default security permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('security', 'read'),
    ('security', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign security permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'security'
ON CONFLICT DO NOTHING;
