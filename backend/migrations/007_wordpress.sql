-- vKAI Panel Database Schema - WordPress
-- PostgreSQL

-- ============================================================
-- WORDPRESS SITES
-- ============================================================
CREATE TABLE wordpress_sites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    path VARCHAR(500) NOT NULL,
    db_name VARCHAR(255) NOT NULL,
    db_user VARCHAR(255) NOT NULL,
    db_password VARCHAR(255) NOT NULL,
    db_host VARCHAR(255) NOT NULL DEFAULT 'localhost',
    db_prefix VARCHAR(50) NOT NULL DEFAULT 'wp_',
    admin_user VARCHAR(255) NOT NULL,
    admin_password VARCHAR(255) NOT NULL,
    admin_email VARCHAR(255) NOT NULL,
    version VARCHAR(20) NOT NULL DEFAULT 'latest',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_active BOOLEAN DEFAULT TRUE,
    auto_update BOOLEAN DEFAULT FALSE,
    last_update_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, name)
);

CREATE INDEX idx_wordpress_sites_tenant ON wordpress_sites(tenant_id);
CREATE INDEX idx_wordpress_sites_server ON wordpress_sites(server_id);
CREATE INDEX idx_wordpress_sites_website ON wordpress_sites(website_id);
CREATE INDEX idx_wordpress_sites_domain ON wordpress_sites(domain);
CREATE INDEX idx_wordpress_sites_status ON wordpress_sites(status);

-- ============================================================
-- WORDPRESS PLUGINS
-- ============================================================
CREATE TABLE wordpress_plugins (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id UUID NOT NULL REFERENCES wordpress_sites(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    version VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_active BOOLEAN DEFAULT TRUE,
    auto_update BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(site_id, slug)
);

CREATE INDEX idx_wordpress_plugins_site ON wordpress_plugins(site_id);
CREATE INDEX idx_wordpress_plugins_tenant ON wordpress_plugins(tenant_id);

-- ============================================================
-- WORDPRESS THEMES
-- ============================================================
CREATE TABLE wordpress_themes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id UUID NOT NULL REFERENCES wordpress_sites(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    version VARCHAR(20) NOT NULL,
    is_active BOOLEAN DEFAULT FALSE,
    auto_update BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(site_id, slug)
);

CREATE INDEX idx_wordpress_themes_site ON wordpress_themes(site_id);
CREATE INDEX idx_wordpress_themes_tenant ON wordpress_themes(tenant_id);

-- ============================================================
-- SEED DATA: Default WordPress permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('wordpress', 'read'),
    ('wordpress', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign WordPress permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'wordpress'
ON CONFLICT DO NOTHING;
