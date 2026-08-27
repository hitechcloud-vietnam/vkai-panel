-- vKAI Panel Database Schema - Git Deployments
-- PostgreSQL

-- ============================================================
-- GIT DEPLOYMENTS
-- ============================================================
CREATE TABLE git_deployments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    repository_url VARCHAR(500) NOT NULL,
    branch VARCHAR(255) NOT NULL DEFAULT 'main',
    deploy_path VARCHAR(500) NOT NULL,
    deploy_key TEXT,
    webhook_secret VARCHAR(255),
    webhook_url VARCHAR(500),
    auto_deploy BOOLEAN DEFAULT FALSE,
    deploy_script TEXT,
    pre_deploy_hook TEXT,
    post_deploy_hook TEXT,
    environment JSONB DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_active BOOLEAN DEFAULT TRUE,
    last_deploy_at TIMESTAMP WITH TIME ZONE,
    last_commit_hash VARCHAR(40),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, name)
);

CREATE INDEX idx_git_deployments_tenant ON git_deployments(tenant_id);
CREATE INDEX idx_git_deployments_server ON git_deployments(server_id);
CREATE INDEX idx_git_deployments_website ON git_deployments(website_id);
CREATE INDEX idx_git_deployments_status ON git_deployments(status);

-- ============================================================
-- GIT DEPLOYMENT LOGS
-- ============================================================
CREATE TABLE git_deployment_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    deployment_id UUID NOT NULL REFERENCES git_deployments(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    commit_hash VARCHAR(40),
    commit_msg TEXT,
    author VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    output TEXT,
    error TEXT,
    duration INT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_git_deployment_logs_deployment ON git_deployment_logs(deployment_id);
CREATE INDEX idx_git_deployment_logs_tenant ON git_deployment_logs(tenant_id);
CREATE INDEX idx_git_deployment_logs_created ON git_deployment_logs(created_at);

-- ============================================================
-- SEED DATA: Default git deployment permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('gitdeployment', 'read'),
    ('gitdeployment', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign git deployment permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'gitdeployment'
ON CONFLICT DO NOTHING;
