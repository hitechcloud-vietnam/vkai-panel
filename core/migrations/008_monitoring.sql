-- vKAI Panel Database Schema - Monitoring
-- PostgreSQL

-- ============================================================
-- MONITORING METRICS
-- ============================================================
CREATE TABLE monitoring_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    metric VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit VARCHAR(50),
    tags JSONB DEFAULT '{}',
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_monitoring_metrics_server ON monitoring_metrics(server_id);
CREATE INDEX idx_monitoring_metrics_tenant ON monitoring_metrics(tenant_id);
CREATE INDEX idx_monitoring_metrics_metric ON monitoring_metrics(metric);
CREATE INDEX idx_monitoring_metrics_timestamp ON monitoring_metrics(timestamp);

-- ============================================================
-- MONITORING ALERTS
-- ============================================================
CREATE TABLE monitoring_alerts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    metric VARCHAR(255) NOT NULL,
    condition VARCHAR(20) NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    duration INT DEFAULT 300,
    severity VARCHAR(20) NOT NULL DEFAULT 'warning',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_active BOOLEAN DEFAULT TRUE,
    last_triggered_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_monitoring_alerts_tenant ON monitoring_alerts(tenant_id);
CREATE INDEX idx_monitoring_alerts_server ON monitoring_alerts(server_id);
CREATE INDEX idx_monitoring_alerts_status ON monitoring_alerts(status);

-- ============================================================
-- MONITORING ALERT LOGS
-- ============================================================
CREATE TABLE monitoring_alert_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    alert_id UUID NOT NULL REFERENCES monitoring_alerts(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    server_id UUID REFERENCES servers(id),
    value DOUBLE PRECISION NOT NULL,
    message TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'triggered',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_monitoring_alert_logs_alert ON monitoring_alert_logs(alert_id);
CREATE INDEX idx_monitoring_alert_logs_tenant ON monitoring_alert_logs(tenant_id);
CREATE INDEX idx_monitoring_alert_logs_created ON monitoring_alert_logs(created_at);

-- ============================================================
-- MONITORING DASHBOARDS
-- ============================================================
CREATE TABLE monitoring_dashboards (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    layout JSONB DEFAULT '{}',
    widgets JSONB DEFAULT '[]',
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_monitoring_dashboards_tenant ON monitoring_dashboards(tenant_id);

-- ============================================================
-- SEED DATA: Default monitoring permissions
-- ============================================================
INSERT INTO permissions (resource, action) VALUES
    ('monitoring', 'read'),
    ('monitoring', 'write')
ON CONFLICT (resource, action) DO NOTHING;

-- Assign monitoring permissions to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', id FROM permissions WHERE resource = 'monitoring'
ON CONFLICT DO NOTHING;
