CREATE TABLE IF NOT EXISTS traffic_packs (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price DECIMAL(20, 2) NOT NULL,
    credit_usd DECIMAL(20, 10) NOT NULL,
    validity_days INT NOT NULL,
    platform VARCHAR(30) NOT NULL DEFAULT 'openai',
    for_sale BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_traffic_packs_for_sale_sort ON traffic_packs (for_sale, sort_order, id);

CREATE TABLE IF NOT EXISTS user_traffic_credits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id BIGINT NOT NULL UNIQUE REFERENCES payment_orders(id) ON DELETE RESTRICT,
    pack_id BIGINT REFERENCES traffic_packs(id) ON DELETE SET NULL,
    platform VARCHAR(30) NOT NULL DEFAULT 'openai',
    initial_usd DECIMAL(20, 10) NOT NULL,
    remaining_usd DECIMAL(20, 10) NOT NULL,
    credited_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_traffic_credits_user_available
    ON user_traffic_credits (user_id, platform, expires_at, credited_at, id)
    WHERE remaining_usd > 0;

CREATE TABLE IF NOT EXISTS traffic_credit_ledger (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credit_id BIGINT REFERENCES user_traffic_credits(id) ON DELETE SET NULL,
    order_id BIGINT REFERENCES payment_orders(id) ON DELETE SET NULL,
    request_id VARCHAR(128) NOT NULL DEFAULT '',
    entry_type VARCHAR(30) NOT NULL,
    amount_usd DECIMAL(20, 10) NOT NULL,
    balance_after_usd DECIMAL(20, 10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_traffic_credit_ledger_user_time ON traffic_credit_ledger (user_id, created_at DESC);

INSERT INTO traffic_packs (code, name, description, price, credit_usd, validity_days, platform, for_sale, sort_order)
VALUES
    ('gpt_traffic_5usd_2cny', 'GPT 流量包 5 刀', '2 元购买 5 USD GPT 额度，有效期 365 天，可用于写代码和生图。', 2.00, 5.0000000000, 365, 'openai', TRUE, 10),
    ('gpt_traffic_10usd_3cny', 'GPT 流量包 10 刀', '3 元购买 10 USD GPT 额度，有效期 365 天，可用于写代码和生图。', 3.00, 10.0000000000, 365, 'openai', TRUE, 20),
    ('gpt_traffic_20usd_5cny', 'GPT 流量包 20 刀', '5 元购买 20 USD GPT 额度，有效期 365 天，可用于写代码和生图。', 5.00, 20.0000000000, 365, 'openai', TRUE, 30)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    price = EXCLUDED.price,
    credit_usd = EXCLUDED.credit_usd,
    validity_days = EXCLUDED.validity_days,
    platform = EXCLUDED.platform,
    for_sale = EXCLUDED.for_sale,
    sort_order = EXCLUDED.sort_order,
    updated_at = NOW();
