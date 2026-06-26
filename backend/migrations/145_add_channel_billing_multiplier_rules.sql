-- Channel billing multiplier rules: allow channel pricing to apply an
-- additional multiplier to user-facing actual cost by group/account scope.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS channel_billing_multiplier_rules (
    id              BIGSERIAL      PRIMARY KEY,
    channel_id      BIGINT         NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name            VARCHAR(100)   NOT NULL DEFAULT '',
    group_ids       BIGINT[]       NOT NULL DEFAULT '{}',
    account_ids     BIGINT[]       NOT NULL DEFAULT '{}',
    rate_multiplier NUMERIC(10,4)  NOT NULL DEFAULT 1.0,
    sort_order      INT            NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_billing_multiplier_rules_channel_id
    ON channel_billing_multiplier_rules(channel_id);

COMMENT ON TABLE channel_billing_multiplier_rules IS '渠道实际计费倍率规则：按分组/账号命中后影响用户余额、订阅和 API Key quota';
COMMENT ON COLUMN channel_billing_multiplier_rules.group_ids IS '匹配的分组 ID；规则至少需要一个分组';
COMMENT ON COLUMN channel_billing_multiplier_rules.account_ids IS '可选的账号 ID 限定；配置后需要同时匹配 group_ids 与 account_ids';
COMMENT ON COLUMN channel_billing_multiplier_rules.rate_multiplier IS '额外计费倍率，>=0；最终 ActualCost = 基础费用 × 分组/用户倍率 × 此倍率';
