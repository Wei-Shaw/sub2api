CREATE TABLE IF NOT EXISTS subscription_plans (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price DECIMAL(20,2) NOT NULL,
    original_price DECIMAL(20,2),
    validity_days INT NOT NULL DEFAULT 30,
    validity_unit VARCHAR(10) NOT NULL DEFAULT 'day',
    features TEXT NOT NULL DEFAULT '',
    product_name VARCHAR(100) NOT NULL DEFAULT '',
    for_sale BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_group_id ON subscription_plans(group_id);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_for_sale ON subscription_plans(for_sale);
