-- vKAI Panel Database Schema - Node.js Applications
-- PostgreSQL

-- ============================================================
-- NODE.JS APPLICATIONS
-- ============================================================
CREATE TABLE node_apps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    path VARCHAR(500) NOT NULL,
    port INT NOT NULL,
    node_version VARCHAR(20) NOT NULL DEFAULT '18',
    npm_version VARCHAR(20),
    start_script VARCHAR(500) NOT NULL DEFAULT 'npm start',
    stop_script VARCHAR(500) DEFAULT 'kill $PID',
    restart_script VARCHAR(500),
    env_file VARCHAR(500),
    log_file VARCHAR(500),
    pid_file VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'stopped',
    is_active BOOLEAN DEFAULT TRUE,
    auto_restart BOOLEAN DEFAULT TRUE,
    max_restarts INT DEFAULT 5,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, name)
);

CREATE INDEX idx_node_apps_tenant ON node_apps(tenant_id);
CREATE INDEX idx_node_apps_server ON node_apps(server_id);
CREATE INDEX idx_node_apps_website ON node_apps(website_id);
CREATE INDEX idx_node_apps_status ON node_apps(status);

-- ============================================================
-- NODE.JS APP DEPENDENCIES
-- ============================================================
CREATE TABLE node_app_dependencies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES node_apps(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    is_dev BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(app_id, name)
);

CREATE INDEX idx_node_app_dependencies_app ON node_app_dependencies(app_id);

-- ============================================================
-- NODE.JS APP ENVIRONMENT VARIABLES
-- ============================================================
CREATE TABLE node_app_environments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES node_apps(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    is_secret BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(app_id, key)
);

CREATE INDEX idx_node_app_environments_app ON node_app_environments(app_id);

-- ============================================================
-- SEED DATA: Default Node.js permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('nodeapp', 'read'),
    ('nodeapp', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign Node.js permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'nodeapp'
ON CONFLICT DO NOTHING;
