-- Migration 021: Daily Report Pro tables
CREATE TABLE daily_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    report_date VARCHAR(10) NOT NULL,
    report_type VARCHAR(20) NOT NULL DEFAULT 'daily',
    title VARCHAR(500) NOT NULL,
    summary TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    sent_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_daily_reports_tenant ON daily_reports(tenant_id);
CREATE INDEX idx_daily_reports_date ON daily_reports(report_date DESC);
CREATE INDEX idx_daily_reports_type ON daily_reports(report_type);

CREATE TABLE report_sections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id UUID NOT NULL REFERENCES daily_reports(id) ON DELETE CASCADE,
    section_key VARCHAR(100) NOT NULL,
    title VARCHAR(200) NOT NULL,
    content TEXT,
    data_json TEXT,
    sort_order INT DEFAULT 0
);

CREATE INDEX idx_report_sections_report ON report_sections(report_id);

CREATE TABLE report_schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(200) NOT NULL,
    report_type VARCHAR(20) NOT NULL DEFAULT 'daily',
    frequency VARCHAR(100) NOT NULL,
    recipients TEXT[] NOT NULL DEFAULT '{}',
    sections TEXT[] NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT true,
    last_sent_at TIMESTAMP WITH TIME ZONE,
    next_send_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_report_schedules_tenant ON report_schedules(tenant_id);
CREATE INDEX idx_report_schedules_active ON report_schedules(is_active);

CREATE TABLE report_deliveries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id UUID NOT NULL REFERENCES daily_reports(id) ON DELETE CASCADE,
    schedule_id UUID REFERENCES report_schedules(id) ON DELETE SET NULL,
    recipient VARCHAR(500) NOT NULL,
    channel VARCHAR(50) NOT NULL DEFAULT 'email',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error TEXT,
    sent_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_report_deliveries_report ON report_deliveries(report_id);
CREATE INDEX idx_report_deliveries_status ON report_deliveries(status);
