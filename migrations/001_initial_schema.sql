-- Initial schema for TierGuard
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

-- Seed initial tiers
INSERT INTO tiers (id, name, hierarchy_level, max_monthly_requests) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Free', 1, 1000),
    ('22222222-2222-2222-2222-222222222222', 'Pro', 2, 50000),
    ('33333333-3333-3333-3333-333333333333', 'Enterprise', 3, 1000000)
ON CONFLICT (name) DO NOTHING;

-- Seed initial features
INSERT INTO features (id, name, required_tier_level) VALUES
    ('44444444-4444-4444-4444-444444444444', 'basic_analytics', 1),
    ('55555555-5555-5555-5555-555555555555', 'export_reports', 2),
    ('66666666-6666-6666-6666-666666666666', 'custom_webhooks', 3)
ON CONFLICT (name) DO NOTHING;

-- Seed sample tenants
INSERT INTO tenants (id, company_name, tier_id) VALUES
    ('77777777-7777-7777-7777-777777777777', 'Acme Corp', '22222222-2222-2222-2222-222222222222'),
    ('88888888-8888-8888-8888-888888888888', 'Startup Inc', '11111111-1111-1111-1111-111111111111'),
    ('99999999-9999-9999-9999-999999999999', 'Mega Enterprises', '33333333-3333-3333-3333-333333333333')
ON CONFLICT DO NOTHING;
