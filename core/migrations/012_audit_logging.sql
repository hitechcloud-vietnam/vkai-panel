-- vKAI Panel Database Schema - Audit Logging
-- PostgreSQL

-- ============================================================
-- AUDIT LOGS
-- ============================================================
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(100) NOT NULL,
    resource_id UUID,
    details JSONB DEFAULT '{}',
    ip_address VARCHAR(45),
    user_agent TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'success',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id);
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource);
CREATE INDEX idx_audit_logs_resource_id ON audit_logs(resource_id);
CREATE INDEX idx_audit_logs_status ON audit_logs(status);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at);

-- ============================================================
-- SEED DATA: Default audit permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('audit', 'read')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign audit permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'audit'
ON CONFLICT DO NOTHING;
