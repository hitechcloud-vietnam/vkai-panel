-- vKAI Panel Database Schema - Reverse Proxy
-- PostgreSQL

-- ============================================================
-- REVERSE PROXIES
-- ============================================================
CREATE TABLE reverse_proxies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    listen_port INT NOT NULL DEFAULT 80,
    target_url VARCHAR(500),
    target_host VARCHAR(255) NOT NULL,
    target_port INT NOT NULL,
    protocol VARCHAR(20) NOT NULL DEFAULT 'http',
    ssl_enabled BOOLEAN DEFAULT FALSE,
    ssl_redirect BOOLEAN DEFAULT FALSE,
    ssl_cert_path VARCHAR(500),
    ssl_key_path VARCHAR(500),
    headers JSONB DEFAULT '{}',
    websocket BOOLEAN DEFAULT FALSE,
    load_balancer BOOLEAN DEFAULT FALSE,
    backend_servers JSONB DEFAULT '[]',
    health_check VARCHAR(500),
    health_interval INT DEFAULT 30,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, name)
);

CREATE INDEX idx_reverse_proxies_tenant ON reverse_proxies(tenant_id);
CREATE INDEX idx_reverse_proxies_server ON reverse_proxies(server_id);
CREATE INDEX idx_reverse_proxies_website ON reverse_proxies(website_id);
CREATE INDEX idx_reverse_proxies_domain ON reverse_proxies(domain);
CREATE INDEX idx_reverse_proxies_status ON reverse_proxies(status);

-- ============================================================
-- REVERSE PROXY ACCESS LOGS
-- ============================================================
CREATE TABLE reverse_proxy_access_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    proxy_id UUID NOT NULL REFERENCES reverse_proxies(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    remote_addr VARCHAR(45) NOT NULL,
    method VARCHAR(10) NOT NULL,
    request_uri VARCHAR(500) NOT NULL,
    status INT NOT NULL,
    body_bytes BIGINT DEFAULT 0,
    referer VARCHAR(500),
    user_agent VARCHAR(500),
    response_time FLOAT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_reverse_proxy_access_logs_proxy ON reverse_proxy_access_logs(proxy_id);
CREATE INDEX idx_reverse_proxy_access_logs_tenant ON reverse_proxy_access_logs(tenant_id);
CREATE INDEX idx_reverse_proxy_access_logs_created ON reverse_proxy_access_logs(created_at);

-- ============================================================
-- SEED DATA: Default reverse proxy permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('reverseproxy', 'read'),
    ('reverseproxy', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign reverse proxy permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'reverseproxy'
ON CONFLICT DO NOTHING;
