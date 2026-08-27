-- Website Statistics Pro Migration
-- Creates tables for website analytics and visitor tracking

-- Daily aggregated statistics per website
CREATE TABLE IF NOT EXISTS website_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL,
    date DATE NOT NULL,
    page_views BIGINT DEFAULT 0,
    unique_visitors BIGINT DEFAULT 0,
    total_bandwidth BIGINT DEFAULT 0,
    avg_response_time DOUBLE PRECISION DEFAULT 0,
    bounce_rate DOUBLE PRECISION DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, website_id, date)
);

-- Per-page statistics
CREATE TABLE IF NOT EXISTS website_page_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL,
    path TEXT NOT NULL,
    page_views BIGINT DEFAULT 0,
    unique_views BIGINT DEFAULT 0,
    avg_time_on_page DOUBLE PRECISION DEFAULT 0,
    date DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, website_id, path, date)
);

-- Individual visitor logs
CREATE TABLE IF NOT EXISTS website_visitor_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL,
    visitor_ip VARCHAR(45),
    user_agent TEXT,
    path TEXT NOT NULL,
    method VARCHAR(10) DEFAULT 'GET',
    status_code INTEGER DEFAULT 200,
    response_time DOUBLE PRECISION DEFAULT 0,
    referer TEXT,
    country VARCHAR(100),
    bandwidth BIGINT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Referrer statistics
CREATE TABLE IF NOT EXISTS website_referrer_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL,
    referer TEXT NOT NULL,
    visits BIGINT DEFAULT 0,
    date DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, website_id, referer, date)
);

-- Country statistics
CREATE TABLE IF NOT EXISTS website_country_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    website_id UUID NOT NULL,
    country VARCHAR(100) NOT NULL,
    visitors BIGINT DEFAULT 0,
    date DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, website_id, country, date)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_website_stats_tenant_website_date ON website_stats(tenant_id, website_id, date);
CREATE INDEX IF NOT EXISTS idx_website_page_stats_tenant_website_date ON website_page_stats(tenant_id, website_id, date);
CREATE INDEX IF NOT EXISTS idx_website_visitor_logs_tenant_website ON website_visitor_logs(tenant_id, website_id);
CREATE INDEX IF NOT EXISTS idx_website_visitor_logs_created_at ON website_visitor_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_website_referrer_stats_tenant_website_date ON website_referrer_stats(tenant_id, website_id, date);
CREATE INDEX IF NOT EXISTS idx_website_country_stats_tenant_website_date ON website_country_stats(tenant_id, website_id, date);
