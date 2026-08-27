-- Migration: 014_config_rollback.sql
-- Description: Configuration rollback system tables

-- Config snapshots table
CREATE TABLE IF NOT EXISTS config_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    path VARCHAR(500),
    content TEXT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT false,
    is_automatic BOOLEAN NOT NULL DEFAULT false,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes for config snapshots
CREATE INDEX IF NOT EXISTS idx_config_snapshots_config_type ON config_snapshots(config_type);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_name ON config_snapshots(name);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_server_id ON config_snapshots(server_id);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_tenant_id ON config_snapshots(tenant_id);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_is_active ON config_snapshots(is_active);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_version ON config_snapshots(version);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_created_at ON config_snapshots(created_at);

-- Unique constraint for active snapshots
CREATE UNIQUE INDEX IF NOT EXISTS idx_config_snapshots_active 
ON config_snapshots(config_type, name, server_id) 
WHERE is_active = true;

-- Config templates table
CREATE TABLE IF NOT EXISTS config_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    config_type VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    description TEXT,
    variables JSONB,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
);

-- Indexes for config templates
CREATE INDEX IF NOT EXISTS idx_config_templates_config_type ON config_templates(config_type);
CREATE INDEX IF NOT EXISTS idx_config_templates_tenant_id ON config_templates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_config_templates_is_default ON config_templates(is_default);

-- Seed permissions for config management
INSERT INTO permissions (name, description, resource, action) VALUES
    ('config.create', 'Create config snapshots', 'config', 'create'),
    ('config.read', 'View config snapshots', 'config', 'read'),
    ('config.update', 'Update config snapshots', 'config', 'update'),
    ('config.delete', 'Delete config snapshots', 'config', 'delete'),
    ('config.rollback', 'Rollback config changes', 'config', 'rollback'),
    ('config.diff', 'View config differences', 'config', 'diff'),
    ('config.validate', 'Validate config', 'config', 'validate'),
    ('config_templates.create', 'Create config templates', 'config_templates', 'create'),
    ('config_templates.read', 'View config templates', 'config_templates', 'read'),
    ('config_templates.update', 'Update config templates', 'config_templates', 'update'),
    ('config_templates.delete', 'Delete config templates', 'config_templates', 'delete')
ON CONFLICT (name) DO NOTHING;

-- Assign config permissions to super_admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'super_admin' AND p.name LIKE 'config.%' OR p.name LIKE 'config_templates.%'
ON CONFLICT DO NOTHING;

-- Assign config permissions to admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name LIKE 'config.%' OR p.name LIKE 'config_templates.%'
ON CONFLICT DO NOTHING;
