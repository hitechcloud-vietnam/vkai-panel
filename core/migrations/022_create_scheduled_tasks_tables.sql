-- Migration 022: Scheduled Tasks Pro tables
CREATE TABLE scheduled_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(200) NOT NULL,
    description TEXT DEFAULT '',
    task_type VARCHAR(50) NOT NULL DEFAULT 'command',
    command TEXT DEFAULT '',
    script_content TEXT DEFAULT '',
    http_endpoint TEXT DEFAULT '',
    http_method VARCHAR(10) DEFAULT 'GET',
    schedule VARCHAR(100) NOT NULL,
    schedule_desc VARCHAR(200) DEFAULT '',
    timezone VARCHAR(50) DEFAULT 'UTC',
    is_enabled BOOLEAN DEFAULT true,
    priority INT DEFAULT 2,
    timeout INT DEFAULT 0,
    max_retries INT DEFAULT 0,
    retry_delay INT DEFAULT 60,
    tags TEXT[] DEFAULT '{}',
    environment TEXT[] DEFAULT '{}',
    notify_on_success BOOLEAN DEFAULT false,
    notify_on_failure BOOLEAN DEFAULT true,
    notify_emails TEXT[] DEFAULT '{}',
    last_run_at TIMESTAMP WITH TIME ZONE,
    last_status VARCHAR(20) DEFAULT '',
    next_run_at TIMESTAMP WITH TIME ZONE,
    run_count INT DEFAULT 0,
    fail_count INT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_scheduled_tasks_tenant ON scheduled_tasks(tenant_id);
CREATE INDEX idx_scheduled_tasks_enabled ON scheduled_tasks(is_enabled);
CREATE INDEX idx_scheduled_tasks_type ON scheduled_tasks(task_type);

CREATE TABLE task_executions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP WITH TIME ZONE,
    finished_at TIMESTAMP WITH TIME ZONE,
    duration INT DEFAULT 0,
    exit_code INT,
    output TEXT DEFAULT '',
    error_output TEXT DEFAULT '',
    retry_count INT DEFAULT 0,
    triggered_by VARCHAR(20) DEFAULT 'schedule',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_task_executions_task ON task_executions(task_id);
CREATE INDEX idx_task_executions_tenant ON task_executions(tenant_id);
CREATE INDEX idx_task_executions_status ON task_executions(status);
CREATE INDEX idx_task_executions_created ON task_executions(created_at DESC);

CREATE TABLE task_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(200) NOT NULL,
    description TEXT DEFAULT '',
    task_type VARCHAR(50) NOT NULL DEFAULT 'command',
    command TEXT DEFAULT '',
    script_content TEXT DEFAULT '',
    schedule VARCHAR(100) DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    is_public BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_task_templates_tenant ON task_templates(tenant_id);

CREATE TABLE task_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(200) NOT NULL,
    description TEXT DEFAULT '',
    color VARCHAR(20) DEFAULT '#3B82F6',
    task_count INT DEFAULT 0,
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_task_groups_tenant ON task_groups(tenant_id);
