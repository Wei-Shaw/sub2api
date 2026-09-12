-- ops 面板 trend 查询的分钟级预聚合。
--
-- 背景：
-- ops_metrics_hourly/daily 只服务 overview，GetThroughputTrend / GetErrorTrend 是 raw-only
-- （见 ops_repo_trends.go），每次请求直扫 usage_logs + ops_error_logs 两张明细表，
-- 其中 switch_count 还要 CROSS JOIN LATERAL 展开 upstream_errors JSONB。
-- 在千万行级别的部署下，throughput-trend 与 snapshot-v2 均需数十秒。
--
-- trend 使用 60/300/3600 秒桶，小时表撑不起分钟粒度，因此补一张分钟表。
-- config 的 ops.cleanup.minute_metrics_retention_days（默认 30）早已预留，此前无表可清。
--
-- 口径说明（重要）：
-- 本表刻意对齐 trend 的 raw 口径（buildUsageWhere / buildErrorWhere），而不是照抄
-- UpsertHourlyMetrics：后者 usage 侧用 INNER JOIN groups，会丢掉 group_id IS NULL 的行。
-- trend 的 preagg 段与 head/tail raw 段必须同口径，否则拼接处数字会跳。
-- 因此 trend 的三种桶粒度统一由本表聚合得出，不混用 ops_metrics_hourly。

CREATE TABLE IF NOT EXISTS ops_metrics_minute (
    id BIGSERIAL PRIMARY KEY,

    bucket_start TIMESTAMPTZ NOT NULL,
    platform VARCHAR(32),
    group_id BIGINT,

    success_count BIGINT NOT NULL DEFAULT 0,
    error_count_total BIGINT NOT NULL DEFAULT 0,
    business_limited_count BIGINT NOT NULL DEFAULT 0,
    error_count_sla BIGINT NOT NULL DEFAULT 0,

    upstream_error_count_excl_429_529 BIGINT NOT NULL DEFAULT 0,
    upstream_429_count BIGINT NOT NULL DEFAULT 0,
    upstream_529_count BIGINT NOT NULL DEFAULT 0,

    token_consumed BIGINT NOT NULL DEFAULT 0,

    -- 账号切换次数：由写入侧展开 ops_error_logs.upstream_errors 得到，
    -- 使查询侧不再需要 CROSS JOIN LATERAL jsonb_array_elements。
    switch_count BIGINT NOT NULL DEFAULT 0,

    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 与 ops_metrics_hourly 一致：三种维度模式（overall / platform / group）靠 COALESCE 表达式唯一，
-- 因为 Postgres 的 UNIQUE 把 NULL 视为互不相同。
CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_metrics_minute_unique_dim
    ON ops_metrics_minute (
        bucket_start,
        COALESCE(platform, ''),
        COALESCE(group_id, 0)
    );

CREATE INDEX IF NOT EXISTS idx_ops_metrics_minute_bucket
    ON ops_metrics_minute (bucket_start DESC);

CREATE INDEX IF NOT EXISTS idx_ops_metrics_minute_platform_bucket
    ON ops_metrics_minute (platform, bucket_start DESC)
    WHERE platform IS NOT NULL AND platform <> '' AND group_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_ops_metrics_minute_group_bucket
    ON ops_metrics_minute (group_id, bucket_start DESC)
    WHERE group_id IS NOT NULL AND group_id <> 0;

COMMENT ON TABLE ops_metrics_minute IS 'ops 面板 trend 的分钟级预聚合，口径对齐 buildUsageWhere/buildErrorWhere 的 raw 路径。';
COMMENT ON COLUMN ops_metrics_minute.switch_count IS '账号切换次数，写入侧展开 upstream_errors 得出，避免查询期解析 JSONB。';
