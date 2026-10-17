-- Migration: 000001_init_schema.up.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS tiers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    hierarchy_level INT NOT NULL,
    max_monthly_requests INT NOT NULL
);

CREATE TABLE IF NOT EXISTS features (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    required_tier_level INT NOT NULL
);

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_name VARCHAR(255) NOT NULL,
    tier_id UUID NOT NULL REFERENCES tiers(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS usage_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    request_count INT NOT NULL DEFAULT 0,
    month VARCHAR(7) NOT NULL, -- Format: YYYY-MM
    CONSTRAINT unique_tenant_month UNIQUE (tenant_id, month)
);

CREATE INDEX IF NOT EXISTS idx_tenants_tier_id ON tenants(tier_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_tenant_month ON usage_logs(tenant_id, month);
