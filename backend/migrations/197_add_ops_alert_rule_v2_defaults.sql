-- Add the v2 availability rules without rewriting rows that an older image
-- may still be evaluating during a blue-green deployment. Untouched legacy
-- defaults are interpreted by the repository compatibility layer; operator-
-- modified rules are never replaced or overwritten.

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '基础设施可用性缓慢下降',
    '30 分钟 SLA 合格请求失败率达到 5%，失败至少 10 次且样本至少 100；持续 10 分钟后触发',
    TRUE, 'P1', 'availability_failure_rate', '>=', 5,
    30, 10, 20, 'availability', 100, 10, '<', 2.5, 10, FALSE, TRUE, NULL
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters
) VALUES (
    '基础设施可用性快速下降',
    '5 分钟 SLA 合格请求失败率达到 20%，失败至少 10 次且样本至少 30；持续 3 分钟后触发',
    TRUE, 'P0', 'availability_failure_rate', '>=', 20,
    5, 3, 15, 'availability', 30, 10, '<', 10, 5, FALSE, TRUE, NULL
) ON CONFLICT (name) DO NOTHING;
