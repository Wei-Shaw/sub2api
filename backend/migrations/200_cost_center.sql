-- Cost center stores only events created after this migration is enabled.
CREATE TABLE IF NOT EXISTS cost_center_events (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'settled',
    source_type VARCHAR(32) NOT NULL DEFAULT 'manual',
    source_id VARCHAR(128),
    idempotency_key VARCHAR(255),
    account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    plan_id BIGINT,
    platform VARCHAR(32) NOT NULL DEFAULT '',
    group_id BIGINT,
    model VARCHAR(128) NOT NULL DEFAULT '',
    category VARCHAR(64) NOT NULL DEFAULT '',
    amount_usd NUMERIC(20,10) NOT NULL CHECK (amount_usd >= 0),
    original_amount NUMERIC(20,10),
    original_currency VARCHAR(12),
    fx_rate NUMERIC(20,10),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reversal_of BIGINT REFERENCES cost_center_events(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_center_event_type_check CHECK (event_type IN ('income','expense','consumption','promotional_consumption','subscription_recognition','reversal')),
    CONSTRAINT cost_center_event_status_check CHECK (status IN ('pending','settled','cancelled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS cost_center_events_idempotency_key
    ON cost_center_events(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS cost_center_events_occurred_at ON cost_center_events(occurred_at);
CREATE INDEX IF NOT EXISTS cost_center_events_account_occurred ON cost_center_events(account_id, occurred_at);
CREATE INDEX IF NOT EXISTS cost_center_events_source ON cost_center_events(source_type, source_id);
CREATE INDEX IF NOT EXISTS cost_center_events_category ON cost_center_events(category);
CREATE INDEX IF NOT EXISTS cost_center_events_dimensions ON cost_center_events(platform, group_id, model, plan_id);

CREATE TABLE IF NOT EXISTS cost_center_expense_plans (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    category VARCHAR(64) NOT NULL,
    amount_usd NUMERIC(20,10) NOT NULL CHECK (amount_usd > 0),
    interval_unit VARCHAR(16) NOT NULL,
    interval_value INT NOT NULL DEFAULT 1 CHECK (interval_value > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ,
    next_due_at TIMESTAMPTZ NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    note TEXT NOT NULL DEFAULT '',
    operator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_center_plan_interval_check CHECK (interval_unit IN ('day','week','month','quarter','year','custom'))
);
CREATE INDEX IF NOT EXISTS cost_center_expense_plans_due ON cost_center_expense_plans(active, next_due_at);
CREATE INDEX IF NOT EXISTS cost_center_expense_plans_account ON cost_center_expense_plans(account_id);

CREATE TABLE IF NOT EXISTS cost_center_subscription_entitlements (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL UNIQUE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    plan_id BIGINT,
    group_id BIGINT,
    price_usd NUMERIC(20,10) NOT NULL,
    standard_quota_tokens BIGINT NOT NULL DEFAULT 0,
    realization_factor NUMERIC(30,15) NOT NULL DEFAULT 0,
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    recognized_usd NUMERIC(20,10) NOT NULL DEFAULT 0,
    consumed_tokens BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE cost_center_subscription_entitlements ADD COLUMN IF NOT EXISTS group_id BIGINT;
CREATE INDEX IF NOT EXISTS cost_center_subscription_entitlements_expiry ON cost_center_subscription_entitlements(expires_at);
CREATE INDEX IF NOT EXISTS cost_center_subscription_entitlements_lookup ON cost_center_subscription_entitlements(user_id, group_id, starts_at, expires_at);
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS standard_quota_tokens BIGINT NOT NULL DEFAULT 0;
