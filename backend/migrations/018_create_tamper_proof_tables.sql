-- Tamper Proof for Enterprise Pro - Database Migration
-- File integrity monitoring, intrusion detection, audit trail

-- Protected paths (files/directories under monitoring)
CREATE TABLE IF NOT EXISTS tamper_protected_paths (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    path TEXT NOT NULL,
    path_type VARCHAR(20) NOT NULL DEFAULT 'file', -- file, directory
    recursive BOOLEAN NOT NULL DEFAULT false,
    algorithm VARCHAR(20) NOT NULL DEFAULT 'sha256', -- sha256, sha512, md5
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    alert_on_change BOOLEAN NOT NULL DEFAULT true,
    alert_on_delete BOOLEAN NOT NULL DEFAULT true,
    alert_on_create BOOLEAN NOT NULL DEFAULT false,
    ignore_patterns TEXT[] DEFAULT '{}',
    description TEXT DEFAULT '',
    file_count INTEGER DEFAULT 0,
    last_scan_at TIMESTAMPTZ,
    last_alert_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tamper_paths_tenant ON tamper_protected_paths(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tamper_paths_enabled ON tamper_protected_paths(tenant_id, is_enabled);

-- File baselines (checksums of monitored files)
CREATE TABLE IF NOT EXISTS tamper_baselines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    protected_id UUID NOT NULL REFERENCES tamper_protected_paths(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    checksum VARCHAR(128) NOT NULL,
    file_size BIGINT DEFAULT 0,
    file_mode VARCHAR(20) DEFAULT '',
    owner_user VARCHAR(100) DEFAULT '',
    owner_group VARCHAR(100) DEFAULT '',
    mod_time TIMESTAMPTZ,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, protected_id, file_path)
);
CREATE INDEX IF NOT EXISTS idx_tamper_baselines_tenant ON tamper_baselines(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tamper_baselines_protected ON tamper_baselines(tenant_id, protected_id);

-- Tamper alerts (integrity violations)
CREATE TABLE IF NOT EXISTS tamper_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    protected_id UUID NOT NULL REFERENCES tamper_protected_paths(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    alert_type VARCHAR(30) NOT NULL, -- modified, deleted, created, permission_changed
    severity VARCHAR(20) NOT NULL DEFAULT 'medium', -- low, medium, high, critical
    old_checksum VARCHAR(128) DEFAULT '',
    new_checksum VARCHAR(128) DEFAULT '',
    old_size BIGINT DEFAULT 0,
    new_size BIGINT DEFAULT 0,
    old_mode VARCHAR(20) DEFAULT '',
    new_mode VARCHAR(20) DEFAULT '',
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    resolved_by VARCHAR(100) DEFAULT '',
    resolved_at TIMESTAMPTZ,
    notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tamper_alerts_tenant ON tamper_alerts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tamper_alerts_unresolved ON tamper_alerts(tenant_id, is_resolved) WHERE is_resolved = false;
CREATE INDEX IF NOT EXISTS idx_tamper_alerts_created ON tamper_alerts(tenant_id, created_at DESC);

-- Scan results
CREATE TABLE IF NOT EXISTS tamper_scan_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    protected_id UUID NOT NULL REFERENCES tamper_protected_paths(id) ON DELETE CASCADE,
    status VARCHAR(30) NOT NULL DEFAULT 'clean', -- clean, violations_found, error
    total_files INTEGER DEFAULT 0,
    scanned_files INTEGER DEFAULT 0,
    violations INTEGER DEFAULT 0,
    new_files INTEGER DEFAULT 0,
    deleted_files INTEGER DEFAULT 0,
    modified_files INTEGER DEFAULT 0,
    duration INTEGER DEFAULT 0, -- milliseconds
    scan_log TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tamper_scans_tenant ON tamper_scan_results(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tamper_scans_protected ON tamper_scan_results(tenant_id, protected_id);
CREATE INDEX IF NOT EXISTS idx_tamper_scans_created ON tamper_scan_results(tenant_id, created_at DESC);

-- Audit logs
CREATE TABLE IF NOT EXISTS tamper_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL, -- scan, baseline_update, alert_resolved, path_added, path_removed, path_updated
    target TEXT NOT NULL,
    details TEXT DEFAULT '',
    ip_address VARCHAR(45) DEFAULT '',
    user_id UUID,
    username VARCHAR(100) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tamper_audit_tenant ON tamper_audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tamper_audit_created ON tamper_audit_logs(tenant_id, created_at DESC);
