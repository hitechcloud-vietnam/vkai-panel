-- vKAI Panel Database Schema - PHP Management
-- PostgreSQL

-- ============================================================
-- PHP VERSIONS
-- ============================================================
CREATE TABLE php_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    version VARCHAR(20) NOT NULL,
    path VARCHAR(500) NOT NULL,
    fpm_path VARCHAR(500),
    fpm_config VARCHAR(500),
    ini_path VARCHAR(500),
    extensions TEXT[] DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, version)
);

CREATE INDEX idx_php_versions_tenant ON php_versions(tenant_id);
CREATE INDEX idx_php_versions_server ON php_versions(server_id);
CREATE INDEX idx_php_versions_version ON php_versions(version);

-- ============================================================
-- PHP-FPM POOLS
-- ============================================================
CREATE TABLE php_pools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    php_version_id UUID NOT NULL REFERENCES php_versions(id) ON DELETE CASCADE,
    website_id UUID REFERENCES websites(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    "user" VARCHAR(100) NOT NULL DEFAULT 'www-data',
    "group" VARCHAR(100) NOT NULL DEFAULT 'www-data',
    listen VARCHAR(500) NOT NULL,
    listen_owner VARCHAR(100) DEFAULT 'www-data',
    listen_group VARCHAR(100) DEFAULT 'www-data',
    listen_mode VARCHAR(10) DEFAULT '0660',
    pm VARCHAR(20) NOT NULL DEFAULT 'dynamic',
    pm_max_children INT NOT NULL DEFAULT 50,
    pm_start_servers INT NOT NULL DEFAULT 5,
    pm_min_spare_servers INT NOT NULL DEFAULT 5,
    pm_max_spare_servers INT NOT NULL DEFAULT 35,
    pm_max_requests INT NOT NULL DEFAULT 500,
    pm_process_idle_timeout VARCHAR(20) DEFAULT '10s',
    status_path VARCHAR(500) DEFAULT '/status',
    access_log VARCHAR(500),
    error_log VARCHAR(500),
    php_admin_flag JSONB DEFAULT '{}',
    php_value JSONB DEFAULT '{}',
    php_admin_value JSONB DEFAULT '{}',
    env JSONB DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, server_id, name)
);

CREATE INDEX idx_php_pools_tenant ON php_pools(tenant_id);
CREATE INDEX idx_php_pools_server ON php_pools(server_id);
CREATE INDEX idx_php_pools_php_version ON php_pools(php_version_id);
CREATE INDEX idx_php_pools_website ON php_pools(website_id);

-- ============================================================
-- PHP EXTENSIONS
-- ============================================================
CREATE TABLE php_extensions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    php_version_id UUID NOT NULL REFERENCES php_versions(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50),
    description TEXT,
    is_installed BOOLEAN DEFAULT FALSE,
    is_enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, php_version_id, name)
);

CREATE INDEX idx_php_extensions_tenant ON php_extensions(tenant_id);
CREATE INDEX idx_php_extensions_php_version ON php_extensions(php_version_id);
CREATE INDEX idx_php_extensions_name ON php_extensions(name);

-- ============================================================
-- PHP CONFIGURATION
-- ============================================================
CREATE TABLE php_configs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    php_version_id UUID NOT NULL REFERENCES php_versions(id) ON DELETE CASCADE,
    memory_limit VARCHAR(20) DEFAULT '128M',
    max_execution_time INT DEFAULT 30,
    max_input_time INT DEFAULT 60,
    post_max_size VARCHAR(20) DEFAULT '8M',
    upload_max_filesize VARCHAR(20) DEFAULT '2M',
    max_file_uploads INT DEFAULT 20,
    error_reporting VARCHAR(50) DEFAULT 'E_ALL & ~E_DEPRECATED & ~E_STRICT',
    display_errors BOOLEAN DEFAULT FALSE,
    log_errors BOOLEAN DEFAULT TRUE,
    error_log VARCHAR(500),
    date_format VARCHAR(50) DEFAULT 'Y-m-d H:i:s',
    timezone VARCHAR(50) DEFAULT 'UTC',
    opcache_enabled BOOLEAN DEFAULT TRUE,
    opcache_memory INT DEFAULT 128,
    opcache_max_files INT DEFAULT 10000,
    opcache_revalidate_freq INT DEFAULT 2,
    custom_settings JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, php_version_id)
);

CREATE INDEX idx_php_configs_tenant ON php_configs(tenant_id);
CREATE INDEX idx_php_configs_php_version ON php_configs(php_version_id);

-- ============================================================
-- SEED DATA: Default PHP permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('php', 'read'),
    ('php', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign PHP permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'php'
ON CONFLICT DO NOTHING;
