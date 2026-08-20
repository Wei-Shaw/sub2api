-- Migration: 229_account_window_usage_history
-- 账号滚动窗口用量历史（纯被动统计）：
--   1. 新表 account_window_usage_histories：每账号每滚动窗口类型（5h/7d/7d-sonnet/
--      7d-fable/weekly）一行开放记录，窗口关闭（finalized_at 非空）后由 usage_logs
--      聚合回填 token 明细；局部唯一索引保证「每账号每窗口类型至多一行开放记录」
--      （采集器原子 upsert 的冲突目标）
--   2. 观测来源全部为既有数据流，不产生任何新的上游调用：
--      - OpenAI/Codex：网关已把真实流量响应头（x-codex-primary-*）归一化落库到
--        accounts.extra（codex_5h/7d_used_percent + reset_at），本功能仅读取
--      - Anthropic / 国产 coding plan：渠道监控明细历史（channel_monitor_histories）
--        已持久化按账号抓取的配额快照 JSON，本功能仅按水位回放
--   3. last_sample_at 记录行内最新采样的观测时刻：使同一观测被重复回放
--      （多副本 / 重启回填）时 sample_count 恰好计数一次
--   4. 保留期 90 天，由每日维护任务物理删除（日志类表，不用软删除）

CREATE TABLE IF NOT EXISTS account_window_usage_histories (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    window_type VARCHAR(32) NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    peak_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    sample_count INT NOT NULL DEFAULT 0,
    last_sample_at TIMESTAMPTZ,
    requests BIGINT,
    tokens_total BIGINT,
    tokens_input BIGINT,
    tokens_output BIGINT,
    tokens_cache_creation BIGINT,
    tokens_cache_read BIGINT,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_account_window_usage_range CHECK (window_end > window_start)
);

COMMENT ON TABLE account_window_usage_histories IS
    '账号滚动窗口用量历史（纯被动统计：观测来自流量头快照与渠道监控历史，无主动探测）';

-- 每账号每窗口类型至多一行开放记录：采集器 upsert 的冲突目标
CREATE UNIQUE INDEX IF NOT EXISTS uq_account_window_usage_open
    ON account_window_usage_histories(account_id, window_type)
    WHERE finalized_at IS NULL;

-- 管理端统计弹窗的历史查询
CREATE INDEX IF NOT EXISTS idx_account_window_usage_history
    ON account_window_usage_histories(account_id, window_type, window_end DESC);

-- finalize 扫描
CREATE INDEX IF NOT EXISTS idx_account_window_usage_open_scan
    ON account_window_usage_histories(window_end) WHERE finalized_at IS NULL;
