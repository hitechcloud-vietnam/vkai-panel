-- vKAI Panel Database Schema - Log Management
-- PostgreSQL

-- ============================================================
-- LOG ENTRIES
-- ============================================================
CREATE TABLE log_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    source VARCHAR(255) NOT NULL,
    level VARCHAR(20) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    details JSONB DEFAULT '{}',
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_log_entries_server ON log_entries(server_id);
CREATE INDEX idx_log_entries_tenant ON log_entries(tenant_id);
CREATE INDEX idx_log_entries_source ON log_entries(source);
CREATE INDEX idx_log_entries_level ON log_entries(level);
CREATE INDEX idx_log_entries_timestamp ON log_entries(timestamp);
CREATE INDEX idx_log_entries_message ON log_entries USING gin(to_tsvector('english', message));

-- ============================================================
-- LOG SOURCES
-- ============================================================
CREATE TABLE log_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    path TEXT NOT NULL,
    format VARCHAR(50) DEFAULT 'plain',
    is_active BOOLEAN DEFAULT TRUE,
    last_read_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_log_sources_tenant ON log_sources(tenant_id);
CREATE INDEX idx_log_sources_server ON log_sources(server_id);

-- ============================================================
-- LOG ROTATIONS
-- ============================================================
CREATE TABLE log_rotations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    source VARCHAR(255) NOT NULL,
    max_size_mb INT NOT NULL DEFAULT 100,
    max_age_days INT NOT NULL DEFAULT 30,
    max_files INT NOT NULL DEFAULT 10,
    compress_old BOOLEAN DEFAULT TRUE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_log_rotations_tenant ON log_rotations(tenant_id);
CREATE INDEX idx_log_rotations_server ON log_rotations(server_id);

-- ============================================================
-- SEED DATA: Default log permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('logs', 'read'),
    ('logs', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign log permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'logs'
ON CONFLICT DO NOTHING;
