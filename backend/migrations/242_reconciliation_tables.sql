-- 经营对账：原 sub2api-companion 旁路服务的对账能力收编进主程序。
-- 全部为新建表，不修改任何现有表；历史数据不迁移，从切换时刻开始记账。
--
-- 依赖说明：
--   reconciliation_upstream_bills.matched_usage_log_id -> usage_logs.id
--   reconciliation_usage_extras.usage_log_id           -> usage_logs.id
-- 刻意不加外键约束，避免影响 usage_logs 的高频写入路径。

-- 调用侧扩展快照。只存主库没有、且必须冻结的两样东西：
--   1) 调用发生时该账号归属哪个上游令牌（规则以后会改，历史不能重算）
--   2) 采集时的美元->人民币换算数字（汇率以后会改，历史收入不能重算）
-- 下游收入原值仍在 usage_logs.actual_cost，本表不复制它。
CREATE TABLE IF NOT EXISTS reconciliation_usage_extras (
    id                BIGSERIAL PRIMARY KEY,
    usage_log_id      BIGINT NOT NULL,
    account_id        BIGINT NOT NULL,
    rule_provider     VARCHAR(16) NOT NULL DEFAULT '',
    rule_external_key VARCHAR(128) NOT NULL DEFAULT '',
    rule_version      BIGINT NOT NULL DEFAULT 0,
    revenue_original  NUMERIC(20, 10) NOT NULL DEFAULT 0,
    fx_rate_to_cny    NUMERIC(20, 10) NOT NULL DEFAULT 1,
    revenue_cny       NUMERIC(20, 10) NOT NULL DEFAULT 0,
    collected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_usage_extras_usage_key
    ON reconciliation_usage_extras (usage_log_id);

CREATE INDEX IF NOT EXISTS idx_reconciliation_usage_extras_collected
    ON reconciliation_usage_extras (collected_at);

CREATE INDEX IF NOT EXISTS idx_reconciliation_usage_extras_account
    ON reconciliation_usage_extras (account_id, collected_at);

-- 令牌改名后按历史快照回查未对账记录，避免旧账单变成孤儿。
CREATE INDEX IF NOT EXISTS idx_reconciliation_usage_extras_rule_key
    ON reconciliation_usage_extras (rule_external_key);

-- 上游逐笔账单（A6）。成本与汇率在导入时冻结：ON CONFLICT 不更新 cost/fx 列。
CREATE TABLE IF NOT EXISTS reconciliation_upstream_bills (
    id                    BIGSERIAL PRIMARY KEY,
    provider              VARCHAR(16) NOT NULL DEFAULT 'a6',
    upstream_request_id   VARCHAR(128) NOT NULL,
    occurred_at           TIMESTAMPTZ NOT NULL,
    billing_date          DATE,
    model                 VARCHAR(128) NOT NULL DEFAULT '',
    token_name            VARCHAR(128) NOT NULL DEFAULT '',
    input_tokens          INT NOT NULL DEFAULT 0,
    output_tokens         INT NOT NULL DEFAULT 0,
    cache_read_tokens     INT NOT NULL DEFAULT 0,
    cache_creation_tokens INT NOT NULL DEFAULT 0,
    -- 上游口径的缓存合计。上游可能只回一个合并值而不分读写，此时读写两列为 0、
    -- 本列保留合并值；组合匹配一律以本列为准，避免拿 0 去比对而全部匹配失败。
    cache_tokens_total    INT NOT NULL DEFAULT 0,
    cost_original         NUMERIC(20, 10) NOT NULL DEFAULT 0,
    currency              VARCHAR(8) NOT NULL DEFAULT 'USD',
    fx_rate_to_cny        NUMERIC(20, 10) NOT NULL DEFAULT 1,
    cost_cny              NUMERIC(20, 10) NOT NULL DEFAULT 0,
    source                VARCHAR(32) NOT NULL DEFAULT 'a6',
    match_state           VARCHAR(16) NOT NULL DEFAULT 'staging',
    match_method          VARCHAR(48) NOT NULL DEFAULT '',
    matched_usage_log_id  BIGINT,
    matched_account_id    BIGINT,
    raw                   JSONB,
    imported_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 幂等导入：同一上游请求 ID 只落一条。
CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_upstream_bills_request_key
    ON reconciliation_upstream_bills (provider, upstream_request_id);

-- 「一笔下游调用最多挂一笔上游账单」的硬保证，不允许删除。
CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_upstream_bills_usage_key
    ON reconciliation_upstream_bills (matched_usage_log_id)
    WHERE matched_usage_log_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_reconciliation_upstream_bills_occurred
    ON reconciliation_upstream_bills (occurred_at);

CREATE INDEX IF NOT EXISTS idx_reconciliation_upstream_bills_token_time
    ON reconciliation_upstream_bills (token_name, occurred_at);

CREATE INDEX IF NOT EXISTS idx_reconciliation_upstream_bills_match_state
    ON reconciliation_upstream_bills (match_state);

-- 账号规则：一个账号一条。
-- 刻意不对 (provider, external_key) 建唯一约束：同一个 A6 令牌允许多个账号共用。
CREATE TABLE IF NOT EXISTS reconciliation_account_rules (
    id           BIGSERIAL PRIMARY KEY,
    account_id   BIGINT NOT NULL,
    provider     VARCHAR(16) NOT NULL DEFAULT 'a6',
    external_key VARCHAR(128) NOT NULL DEFAULT '',
    multiplier   NUMERIC(20, 10),
    version      BIGINT NOT NULL DEFAULT 1,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_account_rules_account_key
    ON reconciliation_account_rules (account_id);

-- 同步状态 KV，沿用 settings 表的形态。
-- 键约定：usage_last_collected_at、a6_last_sync_unix、a6_last_sync_error、
--        a6_last_sync_error_at、a6_bootstrap_done:<token>、
--        a6_backfill_status|_from|_to|_cursor|_processed|_error
CREATE TABLE IF NOT EXISTS reconciliation_sync_state (
    id         BIGSERIAL PRIMARY KEY,
    key        VARCHAR(100) NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_sync_state_key
    ON reconciliation_sync_state (key);
