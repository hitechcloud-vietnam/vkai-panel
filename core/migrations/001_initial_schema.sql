-- vKAI Panel Database Schema
-- PostgreSQL

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- TENANTS
-- ============================================================
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    domain VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    plan VARCHAR(50) DEFAULT 'standard',
    max_servers INT DEFAULT 10,
    max_websites INT DEFAULT 50,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_tenants_slug ON tenants(slug);
CREATE INDEX idx_tenants_status ON tenants(status);

-- ============================================================
-- USERS
-- ============================================================
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip VARCHAR(45),
    mfa_enabled BOOLEAN DEFAULT FALSE,
    mfa_secret VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);

-- ============================================================
-- ROLES & PERMISSIONS
-- ============================================================
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resource VARCHAR(100) NOT NULL,
    action VARCHAR(50) NOT NULL,
    UNIQUE(resource, action)
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- ============================================================
-- SERVERS
-- ============================================================
CREATE TABLE servers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    ipv6_address VARCHAR(45),
    ssh_port INT DEFAULT 22,
    agent_status VARCHAR(50) DEFAULT 'offline',
    agent_token VARCHAR(255) UNIQUE NOT NULL,
    os VARCHAR(100),
    kernel VARCHAR(100),
    cpu_cores INT,
    ram_total BIGINT,
    disk_total BIGINT,
    location VARCHAR(100),
    tags TEXT[],
    role VARCHAR(50),
    status VARCHAR(50) DEFAULT 'active',
    last_seen_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_servers_tenant ON servers(tenant_id);
CREATE INDEX idx_servers_status ON servers(status);
CREATE INDEX idx_servers_agent_token ON servers(agent_token);

CREATE TABLE server_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_percent DOUBLE PRECISION,
    ram_used BIGINT,
    ram_total BIGINT,
    disk_used BIGINT,
    disk_total BIGINT,
    net_in BIGINT,
    net_out BIGINT,
    load1 DOUBLE PRECISION,
    load5 DOUBLE PRECISION,
    load15 DOUBLE PRECISION,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_server_metrics_server ON server_metrics(server_id);
CREATE INDEX idx_server_metrics_timestamp ON server_metrics(timestamp);

-- ============================================================
-- WEBSITES
-- ============================================================
CREATE TABLE websites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    domain VARCHAR(255) NOT NULL,
    root_dir VARCHAR(500),
    web_server_type VARCHAR(50) NOT NULL,
    php_version VARCHAR(20),
    site_type VARCHAR(50) DEFAULT 'php',
    status VARCHAR(50) DEFAULT 'active',
    ssl_enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_websites_tenant ON websites(tenant_id);
CREATE INDEX idx_websites_server ON websites(server_id);
CREATE INDEX idx_websites_domain ON websites(domain);

CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID REFERENCES websites(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) DEFAULT 'primary',
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_domains_tenant ON domains(tenant_id);
CREATE INDEX idx_domains_website ON domains(website_id);
CREATE INDEX idx_domains_name ON domains(name);

-- ============================================================
-- SSL CERTIFICATES
-- ============================================================
CREATE TABLE ssl_certificates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID REFERENCES websites(id),
    domain VARCHAR(255) NOT NULL,
    issuer VARCHAR(255),
    certificate TEXT,
    private_key TEXT,
    chain_cert TEXT,
    not_before TIMESTAMP WITH TIME ZONE,
    not_after TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) DEFAULT 'pending',
    auto_renew BOOLEAN DEFAULT TRUE,
    source VARCHAR(50) DEFAULT 'letsencrypt',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ssl_tenant ON ssl_certificates(tenant_id);
CREATE INDEX idx_ssl_domain ON ssl_certificates(domain);
CREATE INDEX idx_ssl_status ON ssl_certificates(status);

-- ============================================================
-- DATABASES
-- ============================================================
CREATE TABLE database_servers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    type VARCHAR(50) NOT NULL,
    version VARCHAR(20),
    port INT,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE database_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    database_server_id UUID NOT NULL REFERENCES database_servers(id),
    name VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL,
    password VARCHAR(255),
    charset VARCHAR(50) DEFAULT 'utf8mb4',
    -- "collation" is a reserved word in PostgreSQL and cannot be an unquoted
    -- column name. The value itself is a MySQL collation: this row describes a
    -- database the panel manages on the customer's MySQL server, not this one.
    collation_name VARCHAR(100) DEFAULT 'utf8mb4_unicode_ci',
    size BIGINT DEFAULT 0,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_db_entries_tenant ON database_entries(tenant_id);
CREATE INDEX idx_db_entries_server ON database_entries(database_server_id);

-- ============================================================
-- DNS
-- ============================================================
CREATE TABLE dns_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(100),
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE dns_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    zone_id UUID NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    type VARCHAR(10) NOT NULL,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    ttl INT DEFAULT 3600,
    priority INT,
    status VARCHAR(50) DEFAULT 'active'
);

CREATE INDEX idx_dns_records_zone ON dns_records(zone_id);
CREATE INDEX idx_dns_records_type ON dns_records(type);

-- ============================================================
-- DOCKER
-- ============================================================
CREATE TABLE docker_hosts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    version VARCHAR(20),
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE docker_containers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id UUID NOT NULL REFERENCES docker_hosts(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    image VARCHAR(255) NOT NULL,
    status VARCHAR(50),
    ports TEXT,
    state VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_docker_containers_host ON docker_containers(host_id);

-- ============================================================
-- CRON JOBS
-- ============================================================
CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    name VARCHAR(255) NOT NULL,
    command TEXT NOT NULL,
    schedule VARCHAR(100) NOT NULL,
    type VARCHAR(50) DEFAULT 'shell',
    status VARCHAR(50) DEFAULT 'active',
    last_run_at TIMESTAMP WITH TIME ZONE,
    next_run_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_cron_jobs_tenant ON cron_jobs(tenant_id);
CREATE INDEX idx_cron_jobs_server ON cron_jobs(server_id);

-- ============================================================
-- FIREWALL RULES
-- ============================================================
CREATE TABLE firewall_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID NOT NULL REFERENCES servers(id),
    protocol VARCHAR(10) NOT NULL,
    port VARCHAR(20) NOT NULL,
    source VARCHAR(100),
    action VARCHAR(10) NOT NULL,
    direction VARCHAR(10) DEFAULT 'in',
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_firewall_tenant ON firewall_rules(tenant_id);
CREATE INDEX idx_firewall_server ON firewall_rules(server_id);

-- ============================================================
-- BACKUPS
-- ============================================================
CREATE TABLE backup_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    resource_id UUID NOT NULL,
    destination VARCHAR(50) DEFAULT 'local',
    schedule VARCHAR(100),
    retention INT DEFAULT 7,
    encrypted BOOLEAN DEFAULT FALSE,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE backup_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES backup_jobs(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    size BIGINT DEFAULT 0,
    path VARCHAR(500),
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    error_msg TEXT
);

CREATE INDEX idx_backup_records_job ON backup_records(job_id);
CREATE INDEX idx_backup_records_tenant ON backup_records(tenant_id);

-- ============================================================
-- JOBS
-- ============================================================
-- The jobs table is defined in 017_job_queue.sql together with
-- job_schedules. That migration owns the job queue schema and its shape is
-- the one internal/repository/job.go queries; do not redefine jobs here.

-- ============================================================
-- API KEYS
-- ============================================================
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(20) NOT NULL,
    scopes TEXT[],
    last_used TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

-- ============================================================
-- AUDIT LOGS
-- ============================================================
-- The audit_logs table is defined in 012_audit_logging.sql. That migration
-- owns the audit schema and its shape is the one internal/models/audit.go and
-- internal/repository/audit.go expect; do not redefine audit_logs here.

-- ============================================================
-- NOTIFICATIONS
-- ============================================================
-- The notifications table is defined in 011_notifications.sql alongside
-- notification_templates, notification_channels and notification_preferences.
-- That migration owns the notification schema; do not redefine notifications
-- here.

-- ============================================================
-- DEPLOYMENTS
-- ============================================================
CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL REFERENCES websites(id),
    source VARCHAR(50) DEFAULT 'git',
    branch VARCHAR(255),
    commit_hash VARCHAR(40),
    status VARCHAR(50) DEFAULT 'pending',
    logs TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_deployments_tenant ON deployments(tenant_id);
CREATE INDEX idx_deployments_website ON deployments(website_id);

-- ============================================================
-- SEED DATA: Default permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('server', 'read'),
    ('server', 'write'),
    ('website', 'read'),
    ('website', 'write'),
    ('database', 'read'),
    ('database', 'write'),
    ('dns', 'read'),
    ('dns', 'write'),
    ('ssl', 'read'),
    ('ssl', 'write'),
    ('docker', 'read'),
    ('docker', 'write'),
    ('terminal', 'execute'),
    ('backup', 'read'),
    ('backup', 'write'),
    ('settings', 'write'),
    ('user', 'read'),
    ('user', 'write'),
    ('audit', 'read');

-- ============================================================
-- SEED DATA: Default tenant and admin user
-- ============================================================
INSERT INTO tenants (id, name, slug, status, plan, max_servers, max_websites)
VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Default', 'default', 'active', 'enterprise', 100, 1000);

-- Default admin user (password: admin123 - change in production!)
INSERT INTO users (id, tenant_id, username, email, password_hash, first_name, last_name, status)
VALUES (
    'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    'admin',
    'admin@vkai.local',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    'System',
    'Admin',
    'active'
);

-- Default super_admin role
INSERT INTO roles (id, tenant_id, name, description, is_system)
VALUES ('c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'super_admin', 'Super Administrator', TRUE);

-- Assign all permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions;

-- Assign super_admin to admin user
INSERT INTO user_roles (user_id, role_id)
VALUES ('b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11');
