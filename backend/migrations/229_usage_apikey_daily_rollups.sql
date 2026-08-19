-- 按 API Key 的用量日汇总，支撑 /keys 页面的 api-keys-usage 接口。
--
-- 背景：
-- GetBatchAPIKeyUsageStats（usage_log_repo_stats.go）在 usage_logs 上按 api_key_id 聚合最近 30 天。
-- usage_logs 在高写入部署下可达千万行级别，而热点 key 往往集中了绝大部分流量，
-- 单次请求要聚合的行数与保留窗口成正比，实测需要数十秒。
--
-- 该表与 usage_group_daily_rollups 同构，并复用后者的水位（usage_group_rollup_state.closed_before）
-- 与归档屏障：两者都是"按天结算 usage_logs"，在同一个分块事务里用 GROUPING SETS 一次扫描产出，
-- 因此不需要独立的水位表或调度。

CREATE TABLE IF NOT EXISTS usage_apikey_daily_rollups (
    bucket_date DATE NOT NULL,
    api_key_id BIGINT NOT NULL,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    request_count BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, api_key_id)
);

COMMENT ON TABLE usage_apikey_daily_rollups IS '按 API Key 聚合的用量日桶，口径与 usage_group_daily_rollups 一致（服务端配置时区的自然日）。';
COMMENT ON COLUMN usage_apikey_daily_rollups.bucket_date IS 'usage_group_rollup_state.timezone_name 对应时区的自然日。';

-- 查询侧按 api_key_id 取最近 N 天，故以 api_key_id 前导。
CREATE INDEX IF NOT EXISTS idx_usage_apikey_daily_rollups_key_date
    ON usage_apikey_daily_rollups (api_key_id, bucket_date DESC);
