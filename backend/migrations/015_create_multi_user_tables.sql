-- Migration 015: Multi-user management tables
-- User sessions for tracking active logins
CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    token_hash VARCHAR(512) NOT NULL,
    ip_address VARCHAR(45),
    user_agent TEXT,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_active_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_tenant ON user_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);

-- User activity log
CREATE TABLE IF NOT EXISTS user_activities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(200),
    details TEXT,
    ip_address VARCHAR(45),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_activities_user ON user_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_activities_tenant ON user_activities(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_activities_created ON user_activities(created_at DESC);

-- API keys for programmatic access
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(200) NOT NULL,
    key_hash VARCHAR(512) NOT NULL,
    key_prefix VARCHAR(20) NOT NULL,
    scopes TEXT[],
    expires_at TIMESTAMP WITH TIME ZONE,
    last_used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

-- Seed default permissions if not exist
INSERT INTO permissions (id, resource, action) VALUES
    (uuid_generate_v4(), 'users', 'read'),
    (uuid_generate_v4(), 'users', 'write'),
    (uuid_generate_v4(), 'users', 'delete'),
    (uuid_generate_v4(), 'roles', 'read'),
    (uuid_generate_v4(), 'roles', 'write'),
    (uuid_generate_v4(), 'roles', 'delete'),
    (uuid_generate_v4(), 'websites', 'read'),
    (uuid_generate_v4(), 'websites', 'write'),
    (uuid_generate_v4(), 'websites', 'delete'),
    (uuid_generate_v4(), 'databases', 'read'),
    (uuid_generate_v4(), 'databases', 'write'),
    (uuid_generate_v4(), 'databases', 'delete'),
    (uuid_generate_v4(), 'files', 'read'),
    (uuid_generate_v4(), 'files', 'write'),
    (uuid_generate_v4(), 'files', 'delete'),
    (uuid_generate_v4(), 'dns', 'read'),
    (uuid_generate_v4(), 'dns', 'write'),
    (uuid_generate_v4(), 'dns', 'delete'),
    (uuid_generate_v4(), 'ssl', 'read'),
    (uuid_generate_v4(), 'ssl', 'write'),
    (uuid_generate_v4(), 'ssl', 'delete'),
    (uuid_generate_v4(), 'firewall', 'read'),
    (uuid_generate_v4(), 'firewall', 'write'),
    (uuid_generate_v4(), 'firewall', 'delete'),
    (uuid_generate_v4(), 'cron', 'read'),
    (uuid_generate_v4(), 'cron', 'write'),
    (uuid_generate_v4(), 'cron', 'delete'),
    (uuid_generate_v4(), 'backups', 'read'),
    (uuid_generate_v4(), 'backups', 'write'),
    (uuid_generate_v4(), 'backups', 'delete'),
    (uuid_generate_v4(), 'monitoring', 'read'),
    (uuid_generate_v4(), 'mail', 'read'),
    (uuid_generate_v4(), 'mail', 'write'),
    (uuid_generate_v4(), 'mail', 'delete'),
    (uuid_generate_v4(), 'settings', 'read'),
    (uuid_generate_v4(), 'settings', 'write')
ON CONFLICT DO NOTHING;

-- Seed default admin role if not exists
DO $$
DECLARE
    admin_role_id UUID;
    admin_user_id UUID := 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11';
    admin_tenant_id UUID := 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11';
BEGIN
    -- Create Super Admin role if not exists
    SELECT id INTO admin_role_id FROM roles WHERE name = 'Super Admin' AND tenant_id = admin_tenant_id;
    IF admin_role_id IS NULL THEN
        INSERT INTO roles (id, tenant_id, name, description, is_system)
        VALUES (uuid_generate_v4(), admin_tenant_id, 'Super Admin', 'Full system access', true)
        RETURNING id INTO admin_role_id;
    END IF;

    -- Assign all permissions to Super Admin
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT admin_role_id, id FROM permissions
    ON CONFLICT DO NOTHING;

    -- Assign admin user to Super Admin role
    INSERT INTO user_roles (user_id, role_id)
    VALUES (admin_user_id, admin_role_id)
    ON CONFLICT DO NOTHING;
END $$;
