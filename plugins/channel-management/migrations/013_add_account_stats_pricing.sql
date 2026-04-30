-- Account statistics pricing rules: allow channels to configure independent
-- model pricing for the account-side statistics cost calculation, decoupled
-- from user billing.
--
-- Notes vs the host counterpart (release/custom-0.1.120 backend/migrations/101):
--   * usage_logs.account_stats_cost is NOT added here — usage_logs is a
--     host-owned table that the plugin SQL gate refuses to ALTER. The host
--     ships its own follow-up migration (106_add_usage_logs_account_stats_cost)
--     to add that column under host ownership.
--   * NUMERIC precision matches the rest of the plugin pricing tables
--     (channel_model_pricing → 20,12) instead of the host's 20,10. Values
--     pass through Go float64, so the extra two digits are headroom only.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 1. Channel-level toggle
ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS apply_pricing_to_account_stats BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN channels.apply_pricing_to_account_stats IS
    '账号统计费用是否使用渠道模型定价 (true) 还是默认 total_cost*account_rate_multiplier (false)';

-- 2. Account stats pricing rules (ordered list per channel)
CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_rules (
    id          BIGSERIAL    PRIMARY KEY,
    channel_id  BIGINT       NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL DEFAULT '',
    group_ids   BIGINT[]     NOT NULL DEFAULT '{}',
    account_ids BIGINT[]     NOT NULL DEFAULT '{}',
    sort_order  INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cas_pricing_rules_channel_id
    ON channel_account_stats_pricing_rules (channel_id);

COMMENT ON TABLE channel_account_stats_pricing_rules IS
    '账号统计自定义定价规则：按 group/account 命中后用于覆写 account_stats_cost';
COMMENT ON COLUMN channel_account_stats_pricing_rules.group_ids IS
    '匹配的分组 ID 列表（任意命中即视为规则匹配）';
COMMENT ON COLUMN channel_account_stats_pricing_rules.account_ids IS
    '匹配的账号 ID 列表（任意命中即视为规则匹配）';
COMMENT ON COLUMN channel_account_stats_pricing_rules.sort_order IS
    '规则排序，按数值升序遍历，先命中即返回';

-- 3. Per-rule model pricing (same shape as channel_model_pricing flat fields)
CREATE TABLE IF NOT EXISTS channel_account_stats_model_pricing (
    id                 BIGSERIAL      PRIMARY KEY,
    rule_id            BIGINT         NOT NULL REFERENCES channel_account_stats_pricing_rules(id) ON DELETE CASCADE,
    platform           VARCHAR(50)    NOT NULL DEFAULT '',
    models             JSONB          NOT NULL DEFAULT '[]',
    billing_mode       VARCHAR(20)    NOT NULL DEFAULT 'token',
    input_price        NUMERIC(20,12),
    output_price       NUMERIC(20,12),
    cache_write_price  NUMERIC(20,12),
    cache_read_price   NUMERIC(20,12),
    image_output_price NUMERIC(20,12),
    per_request_price  NUMERIC(20,12),
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cas_model_pricing_rule_id
    ON channel_account_stats_model_pricing (rule_id);

COMMENT ON TABLE channel_account_stats_model_pricing IS
    '账号统计定价规则下的模型定价行（结构与 channel_model_pricing flat fields 对齐）';
