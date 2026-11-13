-- =============================================================================
-- 251: Upstream attempt trail + account status event history
-- =============================================================================
-- 目的：让「账号追溯」有三条可 join 的证据链。
--
-- 1) upstream_attempts —— 每次上游尝试一行（含失败后的 failover 每一跳）。
--    现有实现把这条链塞在 ops_error_logs.upstream_errors 的 JSONB 里
--    （migrations/034_ops_upstream_error_events.sql, service/ops_upstream_context.go
--    的 OpsUpstreamErrorEvent）。JSONB 能看单条请求，但无法按账号聚合、
--    无法建索引、无法 join 账号状态，也无法独立设置保留期，所以在写入侧
--    仍然保留 JSONB（兼容既有排障页面）的同时，额外落一张归一化表。
--
-- 2) account_status_events —— 账号状态变更事件流。现有 account 表只有
--    RateLimitedAt / RateLimitResetAt / TempUnschedulableUntil 这几个「当前值」，
--    被下一次状态变更覆盖，因此「这个号昨天为什么被摘、摘了几次、每次多久」
--    无法回答。
--
-- 3) request_id 是两张表与 usage_logs / ops_error_logs 的唯一关联键。
--
-- 保留策略与 usage_logs 一致，由 usage_cleanup 任务按 created_at 滚动删除，
-- 因此建 (created_at) 索引供批量删除使用。
-- =============================================================================

CREATE TABLE IF NOT EXISTS upstream_attempts (
    id                  BIGSERIAL PRIMARY KEY,

    -- 关联键：与 usage_logs.request_id / ops_error_logs.request_id 同源
    request_id          VARCHAR(64) NOT NULL,
    -- 同一请求内的第几次尝试，1 起
    attempt_no          INT NOT NULL DEFAULT 1,
    -- 本次尝试命中的账号；配置被删除后允许为 NULL
    account_id          BIGINT,
    -- 上一跳账号（failover 来源）；首次尝试为 NULL
    retry_from          BIGINT,

    platform            VARCHAR(32) NOT NULL,
    model_req           VARCHAR(200) NOT NULL DEFAULT '',
    model_upstream      VARCHAR(200),
    upstream_url        VARCHAR(256),

    -- 上游真实 HTTP 状态码；连接层错误（无响应）为 NULL
    upstream_status     INT,
    -- 归一化失败类型，取值与 internal/upstream.FailureKind 对齐
    failure_kind        VARCHAR(16) NOT NULL DEFAULT 'none'
        CHECK (failure_kind IN ('none','auth','rate_limit','overload','transient','client')),
    -- 上游错误码，如 insufficient_quota / MODEL_CAPACITY_EXHAUSTED
    error_code          VARCHAR(64),
    -- 已脱敏并截断的错误信息，禁止存整段响应体
    error_msg           VARCHAR(512),

    latency_ms          INT,
    -- 记录尝试发生时该账号的状态，便于区分「本来就冷却」与「这次才变冷却」
    account_state       VARCHAR(16),
    cooling_until       TIMESTAMPTZ,
    -- 该次尝试所用凭据的指纹（hash 前 8 字节，不可逆）。换过 token 的账号
    -- 行为会变，只记 account_id 无法区分「账号废了」和「这版凭据有问题」。
    credential_fp       VARCHAR(16),
    -- 出口代理；代理变更会直接影响上游风控判定
    proxy_id            BIGINT,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upstream_attempts_request_id
    ON upstream_attempts (request_id);
CREATE INDEX IF NOT EXISTS idx_upstream_attempts_account_time
    ON upstream_attempts (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_upstream_attempts_created_at
    ON upstream_attempts (created_at);

COMMENT ON TABLE upstream_attempts IS
    'Per-attempt upstream trail (one row per failover hop) for account traceability.';

CREATE TABLE IF NOT EXISTS account_status_events (
    id              BIGSERIAL PRIMARY KEY,
    account_id      BIGINT NOT NULL,

    -- 变更前后状态：active / cooling / degraded / invalid / disabled
    from_state      VARCHAR(16),
    to_state        VARCHAR(16) NOT NULL,
    -- 触发原因（自由文本短标签，如 rate_limit / auth / overload / manual / refresh / expiry）
    reason          VARCHAR(64),
    error_code      VARCHAR(64),
    cooling_until   TIMESTAMPTZ,
    -- 变更来源，决定「谁该负责恢复」
    source          VARCHAR(16) NOT NULL DEFAULT 'classify'
        CHECK (source IN ('classify','refresh','manual','expiry','scheduled_test')),
    -- 触发本次变更的那次请求
    request_id      VARCHAR(64),
    -- source=manual 时的操作人
    operator_id     BIGINT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_status_events_account_time
    ON account_status_events (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_status_events_created_at
    ON account_status_events (created_at);
CREATE INDEX IF NOT EXISTS idx_account_status_events_request_id
    ON account_status_events (request_id);

COMMENT ON TABLE account_status_events IS
    'Append-only account state transition history: why an account left service and for how long.';

-- 账号维度 1 小时聚合视图：成功/失败/限流计数与失败率。
-- 失败率突增告警可直接读这个视图，无需扫描原始日志表。
CREATE OR REPLACE VIEW account_attempt_stats_1h AS
SELECT
    account_id,
    count(*)                                                        AS attempts,
    count(*) FILTER (WHERE failure_kind = 'none')                    AS ok,
    count(*) FILTER (WHERE failure_kind = 'rate_limit')              AS rate_limited,
    count(*) FILTER (WHERE failure_kind = 'auth')                    AS auth_failed,
    count(*) FILTER (WHERE failure_kind IN ('overload','transient')) AS upstream_errors,
    round(avg(latency_ms)::numeric, 1)                               AS avg_latency_ms,
    round(
        100.0 * count(*) FILTER (WHERE failure_kind <> 'none') / NULLIF(count(*), 0),
        2
    )                                                                AS failure_pct
FROM upstream_attempts
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY account_id;

COMMENT ON VIEW account_attempt_stats_1h IS
    'Rolling 1h per-account success/failure breakdown for dashboards and error-rate alerting.';
