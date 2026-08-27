-- Migration: Cluster & HA Management
-- Phase 7: Cluster, High Availability, Load Balancing

-- Clusters
CREATE TABLE IF NOT EXISTS clusters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL DEFAULT 'active-passive', -- active-active, active-passive, load-balanced
    status VARCHAR(50) NOT NULL DEFAULT 'creating', -- creating, active, degraded, failed, maintenance
    config JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Cluster Nodes
CREATE TABLE IF NOT EXISTS cluster_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'worker', -- master, worker, standby
    status VARCHAR(50) NOT NULL DEFAULT 'joining', -- joining, active, inactive, failed, maintenance
    ip_address VARCHAR(45),
    port INTEGER DEFAULT 8080,
    weight INTEGER DEFAULT 100,
    metadata JSONB DEFAULT '{}',
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(cluster_id, server_id)
);

-- Load Balancers
CREATE TABLE IF NOT EXISTS load_balancers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cluster_id UUID REFERENCES clusters(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'nginx', -- nginx, haproxy, traefik, caddy
    algorithm VARCHAR(50) NOT NULL DEFAULT 'round-robin', -- round-robin, least-connections, ip-hash, weighted
    status VARCHAR(50) NOT NULL DEFAULT 'creating', -- creating, active, inactive, error
    listen_port INTEGER NOT NULL DEFAULT 80,
    ssl_port INTEGER DEFAULT 443,
    ssl_enabled BOOLEAN DEFAULT FALSE,
    config JSONB DEFAULT '{}',
    health_check JSONB DEFAULT '{"enabled": true, "interval": 30, "timeout": 5, "path": "/health"}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Load Balancer Backends
CREATE TABLE IF NOT EXISTS load_balancer_backends (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    load_balancer_id UUID NOT NULL REFERENCES load_balancers(id) ON DELETE CASCADE,
    node_id UUID REFERENCES cluster_nodes(id) ON DELETE SET NULL,
    address VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 80,
    weight INTEGER DEFAULT 100,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, inactive, draining, failed
    health_status VARCHAR(50) DEFAULT 'unknown', -- healthy, unhealthy, unknown
    last_check TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- HA Pairs
CREATE TABLE IF NOT EXISTS ha_pairs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    primary_server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    secondary_server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    virtual_ip VARCHAR(45),
    status VARCHAR(50) NOT NULL DEFAULT 'configuring', -- configuring, active, failed-over, error
    failover_mode VARCHAR(50) NOT NULL DEFAULT 'automatic', -- automatic, manual
    last_sync TIMESTAMP WITH TIME ZONE,
    last_failover TIMESTAMP WITH TIME ZONE,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Indexes
CREATE INDEX idx_clusters_tenant_id ON clusters(tenant_id);
CREATE INDEX idx_clusters_status ON clusters(status);
CREATE INDEX idx_cluster_nodes_cluster_id ON cluster_nodes(cluster_id);
CREATE INDEX idx_cluster_nodes_server_id ON cluster_nodes(server_id);
CREATE INDEX idx_cluster_nodes_status ON cluster_nodes(status);
CREATE INDEX idx_cluster_nodes_heartbeat ON cluster_nodes(last_heartbeat);
CREATE INDEX idx_load_balancers_tenant_id ON load_balancers(tenant_id);
CREATE INDEX idx_load_balancers_cluster_id ON load_balancers(cluster_id);
CREATE INDEX idx_load_balancer_backends_lb_id ON load_balancer_backends(load_balancer_id);
CREATE INDEX idx_ha_pairs_tenant_id ON ha_pairs(tenant_id);
CREATE INDEX idx_ha_pairs_primary_server ON ha_pairs(primary_server_id);
CREATE INDEX idx_ha_pairs_secondary_server ON ha_pairs(secondary_server_id);

-- Permissions
INSERT INTO permissions (name, description, resource, action) VALUES
    ('clusters.create', 'Create clusters', 'clusters', 'create'),
    ('clusters.read', 'View clusters', 'clusters', 'read'),
    ('clusters.update', 'Update clusters', 'clusters', 'update'),
    ('clusters.delete', 'Delete clusters', 'clusters', 'delete'),
    ('clusters.manage_nodes', 'Manage cluster nodes', 'clusters', 'manage_nodes'),
    ('load_balancers.create', 'Create load balancers', 'load_balancers', 'create'),
    ('load_balancers.read', 'View load balancers', 'load_balancers', 'read'),
    ('load_balancers.update', 'Update load balancers', 'load_balancers', 'update'),
    ('load_balancers.delete', 'Delete load balancers', 'load_balancers', 'delete'),
    ('ha_pairs.create', 'Create HA pairs', 'ha_pairs', 'create'),
    ('ha_pairs.read', 'View HA pairs', 'ha_pairs', 'read'),
    ('ha_pairs.update', 'Update HA pairs', 'ha_pairs', 'update'),
    ('ha_pairs.delete', 'Delete HA pairs', 'ha_pairs', 'delete'),
    ('ha_pairs.failover', 'Trigger failover', 'ha_pairs', 'failover')
ON CONFLICT (name) DO NOTHING;

-- Assign permissions to super_admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'super_admin' AND p.name IN (
    'clusters.create', 'clusters.read', 'clusters.update', 'clusters.delete', 'clusters.manage_nodes',
    'load_balancers.create', 'load_balancers.read', 'load_balancers.update', 'load_balancers.delete',
    'ha_pairs.create', 'ha_pairs.read', 'ha_pairs.update', 'ha_pairs.delete', 'ha_pairs.failover'
)
ON CONFLICT DO NOTHING;

-- Assign read permissions to admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name IN (
    'clusters.read', 'load_balancers.read', 'ha_pairs.read'
)
ON CONFLICT DO NOTHING;
