-- Account statistics pricing: allow channels to configure custom pricing for account cost tracking.

-- 1. Channel-level toggle
ALTER TABLE channels ADD COLUMN IF NOT EXISTS apply_pricing_to_account_stats BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Account stats pricing rules (ordered list per channel)
CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_rules (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL DEFAULT '',
    group_ids BIGINT[] NOT NULL DEFAULT '{}',
    account_ids BIGINT[] NOT NULL DEFAULT '{}',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cas_pricing_rules_channel_id ON channel_account_stats_pricing_rules(channel_id);

-- 3. Model pricing for each rule (same structure as channel_model_pricing)
CREATE TABLE IF NOT EXISTS channel_account_stats_model_pricing (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES channel_account_stats_pricing_rules(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL DEFAULT '',
    models JSONB NOT NULL DEFAULT '[]',
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'token',
    input_price NUMERIC(20,10),
    output_price NUMERIC(20,10),
    cache_write_price NUMERIC(20,10),
    cache_read_price NUMERIC(20,10),
    image_output_price NUMERIC(20,10),
    per_request_price NUMERIC(20,10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cas_model_pricing_rule_id ON channel_account_stats_model_pricing(rule_id);

-- 4. Usage logs: pre-computed account stats cost (NULL = use default formula)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS account_stats_cost NUMERIC(20,10);
