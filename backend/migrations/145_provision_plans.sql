-- 管理员 API Key 自动开通：套餐模板与订单幂等记录。

CREATE TABLE IF NOT EXISTS provision_plans (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    balance DECIMAL(20,8) NOT NULL DEFAULT 0,
    quota DECIMAL(20,8) NOT NULL DEFAULT 0,
    expires_in_days INTEGER,
    rate_limit_5h DECIMAL(20,8) NOT NULL DEFAULT 0,
    rate_limit_1d DECIMAL(20,8) NOT NULL DEFAULT 0,
    rate_limit_7d DECIMAL(20,8) NOT NULL DEFAULT 0,
    concurrency INTEGER NOT NULL DEFAULT 5,
    rpm_limit INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_provision_plans_enabled ON provision_plans(enabled);
CREATE INDEX IF NOT EXISTS idx_provision_plans_group_id ON provision_plans(group_id);

CREATE TABLE IF NOT EXISTS provision_orders (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(128) NOT NULL UNIQUE,
    plan_id BIGINT REFERENCES provision_plans(id) ON DELETE SET NULL,
    plan_code VARCHAR(64) NOT NULL,
    plan_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    customer_label TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_provision_orders_plan_code ON provision_orders(plan_code);
CREATE INDEX IF NOT EXISTS idx_provision_orders_status ON provision_orders(status);

-- API Key 开通模式下，不开放普通用户注册，也不允许第三方登录绕过注册开关。
INSERT INTO settings (key, value)
VALUES
    ('registration_enabled', 'false'),
    ('dingtalk_connect_bypass_registration', 'false')
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = NOW();
