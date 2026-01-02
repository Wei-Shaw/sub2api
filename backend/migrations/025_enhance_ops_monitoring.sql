-- 运维监控中心 2.0 - 数据库 Schema 增强
-- 创建时间: 2026-01-02
-- 说明: 扩展监控指标,支持多维度分析和告警管理

-- ============================================
-- 1. 扩展 ops_system_metrics 表
-- ============================================

-- 添加 RED 指标列
ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS qps DECIMAL(10,2) DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tps DECIMAL(10,2) DEFAULT 0,

    -- 错误分类
    ADD COLUMN IF NOT EXISTS error_4xx_count BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_5xx_count BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_timeout_count BIGINT DEFAULT 0,

    -- 延迟指标扩展
    ADD COLUMN IF NOT EXISTS latency_p50 DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS latency_p999 DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS latency_avg DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS latency_max DECIMAL(10,2),

    -- 上游延迟
    ADD COLUMN IF NOT EXISTS upstream_latency_avg DECIMAL(10,2),

    -- 资源指标
    ADD COLUMN IF NOT EXISTS disk_used BIGINT,
    ADD COLUMN IF NOT EXISTS disk_total BIGINT,
    ADD COLUMN IF NOT EXISTS disk_iops BIGINT,
    ADD COLUMN IF NOT EXISTS network_in_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS network_out_bytes BIGINT,

    -- 饱和度指标
    ADD COLUMN IF NOT EXISTS goroutine_count INT,
    ADD COLUMN IF NOT EXISTS db_conn_active INT,
    ADD COLUMN IF NOT EXISTS db_conn_idle INT,
    ADD COLUMN IF NOT EXISTS db_conn_waiting INT,

    -- 业务指标
    ADD COLUMN IF NOT EXISTS token_consumed BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS token_rate DECIMAL(10,2) DEFAULT 0,
    ADD COLUMN IF NOT EXISTS active_subscriptions INT DEFAULT 0,

    -- 维度标签 (支持多维度分析)
    ADD COLUMN IF NOT EXISTS tags JSONB;

-- 添加 JSONB 索引以加速标签查询
CREATE INDEX IF NOT EXISTS idx_ops_metrics_tags ON ops_system_metrics USING GIN(tags);

-- 添加注释
COMMENT ON COLUMN ops_system_metrics.qps IS '每秒查询数 (Queries Per Second)';
COMMENT ON COLUMN ops_system_metrics.tps IS '每秒事务数 (Transactions Per Second)';
COMMENT ON COLUMN ops_system_metrics.error_4xx_count IS '客户端错误数量 (4xx)';
COMMENT ON COLUMN ops_system_metrics.error_5xx_count IS '服务端错误数量 (5xx)';
COMMENT ON COLUMN ops_system_metrics.error_timeout_count IS '超时错误数量';
COMMENT ON COLUMN ops_system_metrics.upstream_latency_avg IS '上游 API 平均延迟 (ms)';
COMMENT ON COLUMN ops_system_metrics.goroutine_count IS 'Goroutine 数量 (检测泄露)';
COMMENT ON COLUMN ops_system_metrics.tags IS '维度标签 (JSON), 如: {"account_id": "123", "api_path": "/v1/chat"REDACTED';

-- ============================================
-- 2. 创建维度统计表
-- ============================================

CREATE TABLE IF NOT EXISTS ops_dimension_stats (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,

    -- 维度类型: account, api_path, provider, region
    dimension_type VARCHAR(50) NOT NULL,
    dimension_value VARCHAR(255) NOT NULL,

    -- 统计指标
    request_count BIGINT DEFAULT 0,
    success_count BIGINT DEFAULT 0,
    error_count BIGINT DEFAULT 0,
    success_rate DECIMAL(5,2),
    error_rate DECIMAL(5,2),

    -- 性能指标
    latency_p50 DECIMAL(10,2),
    latency_p95 DECIMAL(10,2),
    latency_p99 DECIMAL(10,2),

    -- 业务指标
    token_consumed BIGINT DEFAULT 0,
    cost_usd DECIMAL(10,4) DEFAULT 0,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 创建复合索引以加速维度查询
CREATE INDEX IF NOT EXISTS idx_ops_dim_type_value_time
    ON ops_dimension_stats(dimension_type, dimension_value, timestamp DESC);

-- 创建单独的时间索引用于范围查询
CREATE INDEX IF NOT EXISTS idx_ops_dim_timestamp
    ON ops_dimension_stats(timestamp DESC);

-- 添加注释
COMMENT ON TABLE ops_dimension_stats IS '多维度统计表,支持按账户/API/Provider等维度下钻分析';
COMMENT ON COLUMN ops_dimension_stats.dimension_type IS '维度类型: account(账户), api_path(接口), provider(上游), region(地域)';
COMMENT ON COLUMN ops_dimension_stats.dimension_value IS '维度值,如: 账户ID, /v1/chat, openai, us-east-1';

-- ============================================
-- 3. 创建告警规则表
-- ============================================

ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS dimension_filters JSONB,
    ADD COLUMN IF NOT EXISTS notify_channels JSONB,
    ADD COLUMN IF NOT EXISTS notify_config JSONB,
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(100),
    ADD COLUMN IF NOT EXISTS last_triggered_at TIMESTAMPTZ;

-- ============================================
-- 4. 告警历史表 (使用现有的 ops_alert_events)
-- ============================================
-- 注意: 后端代码使用 ops_alert_events 表,不创建新表

-- ============================================
-- 5. 创建数据清理配置表
-- ============================================

CREATE TABLE IF NOT EXISTS ops_data_retention_config (
    id SERIAL PRIMARY KEY,
    table_name VARCHAR(100) NOT NULL UNIQUE,
    retention_days INT NOT NULL, -- 保留天数
    enabled BOOLEAN DEFAULT true,
    last_cleanup_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 插入默认配置
INSERT INTO ops_data_retention_config (table_name, retention_days) VALUES
    ('ops_system_metrics', 30),      -- 系统指标保留 30 天
    ('ops_dimension_stats', 30),     -- 维度统计保留 30 天
    ('ops_error_logs', 30),          -- 错误日志保留 30 天
    ('ops_alert_events', 90),        -- 告警事件保留 90 天
    ('usage_logs', 90)               -- 使用日志保留 90 天
ON CONFLICT (table_name) DO NOTHING;

COMMENT ON TABLE ops_data_retention_config IS '数据保留策略配置表';
COMMENT ON COLUMN ops_data_retention_config.retention_days IS '数据保留天数,超过此天数的数据将被自动清理';

-- ============================================
-- 6. 创建辅助函数
-- ============================================

-- 函数: 计算健康度评分
-- 权重: SLA(40%) + 错误率(30%) + 延迟(20%) + 资源(10%)
CREATE OR REPLACE FUNCTION calculate_health_score(
    p_success_rate DECIMAL,
    p_error_rate DECIMAL,
    p_latency_p99 DECIMAL,
    p_cpu_usage DECIMAL
) RETURNS INT AS $$
DECLARE
    sla_score INT;
    error_score INT;
    latency_score INT;
    resource_score INT;
BEGIN
    -- SLA 评分 (40分)
    sla_score := CASE
        WHEN p_success_rate >= 99.9 THEN 40
        WHEN p_success_rate >= 99.5 THEN 35
        WHEN p_success_rate >= 99.0 THEN 30
        WHEN p_success_rate >= 95.0 THEN 20
        ELSE 10
    END;

    -- 错误率评分 (30分)
    error_score := CASE
        WHEN p_error_rate <= 0.1 THEN 30
        WHEN p_error_rate <= 0.5 THEN 25
        WHEN p_error_rate <= 1.0 THEN 20
        WHEN p_error_rate <= 5.0 THEN 10
        ELSE 5
    END;

    -- 延迟评分 (20分)
    latency_score := CASE
        WHEN p_latency_p99 <= 500 THEN 20
        WHEN p_latency_p99 <= 1000 THEN 15
        WHEN p_latency_p99 <= 3000 THEN 10
        WHEN p_latency_p99 <= 5000 THEN 5
        ELSE 0
    END;

    -- 资源评分 (10分)
    resource_score := CASE
        WHEN p_cpu_usage <= 50 THEN 10
        WHEN p_cpu_usage <= 70 THEN 7
        WHEN p_cpu_usage <= 85 THEN 5
        ELSE 2
    END;

    RETURN sla_score + error_score + latency_score + resource_score;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION calculate_health_score IS '计算系统健康度评分 (0-100),权重: SLA 40% + 错误率 30% + 延迟 20% + 资源 10%';

-- ============================================
-- 7. 创建视图: 最新指标快照
-- ============================================

CREATE OR REPLACE VIEW ops_latest_metrics AS
SELECT
    m.*,
    calculate_health_score(
        m.success_rate::DECIMAL,
        m.error_rate::DECIMAL,
        m.p99_latency_ms::DECIMAL,
        m.cpu_usage_percent::DECIMAL
    ) AS health_score
FROM ops_system_metrics m
WHERE m.window_minutes = 1
  AND m.created_at = (SELECT MAX(created_at) FROM ops_system_metrics WHERE window_minutes = 1)
LIMIT 1;

COMMENT ON VIEW ops_latest_metrics IS '最新的系统指标快照,包含健康度评分';

-- ============================================
-- 8. 创建视图: 活跃告警列表
-- ============================================

CREATE OR REPLACE VIEW ops_active_alerts AS
SELECT
    e.id,
    e.rule_id,
    r.name AS rule_name,
    r.metric_type,
    e.fired_at,
    e.metric_value,
    e.threshold_value,
    r.severity,
    EXTRACT(EPOCH FROM (NOW() - e.fired_at))::INT AS duration_seconds
FROM ops_alert_events e
JOIN ops_alert_rules r ON e.rule_id = r.id
WHERE e.status = 'firing'
ORDER BY e.fired_at DESC;

COMMENT ON VIEW ops_active_alerts IS '当前活跃的告警列表';

-- ============================================
-- 9. 权限设置 (可选)
-- ============================================

-- 如果有专门的 ops 用户,可以授权
-- GRANT SELECT, INSERT, UPDATE ON ops_system_metrics TO ops_user;
-- GRANT SELECT, INSERT ON ops_dimension_stats TO ops_user;
-- GRANT ALL ON ops_alert_rules TO ops_user;
-- GRANT ALL ON ops_alert_events TO ops_user;

-- ============================================
-- 10. 数据完整性检查
-- ============================================

-- 确保现有数据的兼容性
UPDATE ops_system_metrics
SET
    qps = COALESCE(request_count / (window_minutes * 60.0), 0),
    error_rate = COALESCE((error_count::DECIMAL / NULLIF(request_count, 0)) * 100, 0)
WHERE qps = 0 AND request_count > 0;

-- ============================================
-- 完成
-- ============================================
