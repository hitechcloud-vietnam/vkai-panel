-- Migration: 013_job_queue.sql
-- Description: Job queue system tables

-- Jobs table for tracking async jobs
CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id VARCHAR(255) UNIQUE,
    task_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    queue VARCHAR(50) NOT NULL DEFAULT 'default',
    payload JSONB,
    result JSONB,
    error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    scheduled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes for jobs
CREATE INDEX IF NOT EXISTS idx_jobs_task_id ON jobs(task_id);
CREATE INDEX IF NOT EXISTS idx_jobs_task_type ON jobs(task_type);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_queue ON jobs(queue);
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_id ON jobs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_jobs_server_id ON jobs(server_id);
CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_at ON jobs(scheduled_at);

-- Job schedules for recurring jobs
CREATE TABLE IF NOT EXISTS job_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    task_type VARCHAR(100) NOT NULL,
    queue VARCHAR(50) NOT NULL DEFAULT 'default',
    payload JSONB,
    cron_expression VARCHAR(100),
    interval_seconds INTEGER,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL
);

-- Indexes for job schedules
CREATE INDEX IF NOT EXISTS idx_job_schedules_tenant_id ON job_schedules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_schedules_enabled ON job_schedules(enabled);
CREATE INDEX IF NOT EXISTS idx_job_schedules_next_run_at ON job_schedules(next_run_at);

-- Seed permissions for job management
INSERT INTO permissions (name, description, resource, action) VALUES
    ('jobs.create', 'Create jobs', 'jobs', 'create'),
    ('jobs.read', 'View jobs', 'jobs', 'read'),
    ('jobs.update', 'Update jobs', 'jobs', 'update'),
    ('jobs.delete', 'Delete jobs', 'jobs', 'delete'),
    ('jobs.cancel', 'Cancel jobs', 'jobs', 'cancel'),
    ('jobs.retry', 'Retry failed jobs', 'jobs', 'retry'),
    ('jobs.stats', 'View job statistics', 'jobs', 'stats'),
    ('job_schedules.create', 'Create job schedules', 'job_schedules', 'create'),
    ('job_schedules.read', 'View job schedules', 'job_schedules', 'read'),
    ('job_schedules.update', 'Update job schedules', 'job_schedules', 'update'),
    ('job_schedules.delete', 'Delete job schedules', 'job_schedules', 'delete')
ON CONFLICT (name) DO NOTHING;

-- Assign job permissions to super_admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'super_admin' AND p.name LIKE 'jobs.%' OR p.name LIKE 'job_schedules.%'
ON CONFLICT DO NOTHING;

-- Assign job permissions to admin role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name LIKE 'jobs.%' OR p.name LIKE 'job_schedules.%'
ON CONFLICT DO NOTHING;
