-- Error classification v2 starts with newly written rows. Existing rows stay
-- nullable and are interpreted conservatively by repository queries, avoiding
-- a long-running historical rewrite during application startup.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS final_outcome VARCHAR(32);
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS responsibility VARCHAR(32);
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS error_category VARCHAR(64);
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS counts_toward_sla BOOLEAN;
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS alert_family VARCHAR(64);
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS classification_reason VARCHAR(128);
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS classification_version INTEGER;

ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS metric_definition_version INTEGER;

ALTER TABLE ops_metrics_daily
    ADD COLUMN IF NOT EXISTS metric_definition_version INTEGER;

-- New classified alert rules are additive. The existing unique name index
-- makes this safe on databases where an administrator already created a rule
-- with the same name; operator-owned settings are never overwritten.
INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '未知责任失败率', '未知责任最终失败达到 1%，至少 5 次且样本至少 50；影子运行用于发现分类缺口',
    TRUE, 'P1', 'unknown_failure_rate', '>=', 1,
    15, 5, 30, 'unknown_failure', 50, 5, '<', 0.2, 10, TRUE, TRUE, NULL
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '产品兼容性错误突发', '同一窗口的协议映射或错误状态改写达到 20 次；不进入基础设施 SLA',
    TRUE, 'P2', 'compatibility_error_count', '>=', 20,
    15, 5, 60, 'compatibility', 20, 20, '<', 5, 10, TRUE, FALSE, NULL
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '已恢复上游异常增加', '请求最终成功但经历上游限流、5xx 或切换达到 20 次；用于发现隐性供应商退化',
    TRUE, 'P2', 'recovered_provider_error_count', '>=', 20,
    15, 5, 60, 'provider_health', 20, 20, '<', 5, 10, TRUE, FALSE, NULL
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '安全策略拒绝突发', '安全或风控策略拒绝达到 100 次；进入安全摘要，不触发基础设施 paging',
    TRUE, 'P3', 'security_blocked_count', '>=', 100,
    15, 5, 60, 'security', 100, 100, '<', 20, 10, TRUE, FALSE, NULL
) ON CONFLICT (name) DO NOTHING;
