-- Web3 Deposit: seed default Ops alert rules.
--
-- These rules reuse the existing Ops alert evaluator, events, email
-- notification, silencing, and cooldown behavior. Thresholds are conservative
-- defaults and should be tuned per deployment.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'Web3 RPC无健康端点',
    '当 Web3 充值 RPC endpoint 池无健康端点且持续 2 分钟时触发告警',
    true, 'web3_rpc_unhealthy', '>=', 1.0, 1, 2, 'P0', true, 10, NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'Web3 Scanner区块延迟过高',
    '当 Web3 充值 scanner 延迟超过 120 个区块且持续 5 分钟时触发告警',
    true, 'web3_scanner_lag_blocks', '>', 120.0, 1, 5, 'P1', true, 20, NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'Web3 Finalizer区块延迟过高',
    '当 Web3 充值 finalizer 延迟超过 60 个 finalized 区块且持续 5 分钟时触发告警',
    true, 'web3_finalizer_lag_blocks', '>', 60.0, 1, 5, 'P1', true, 20, NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'Web3人工审核积压',
    '当 Web3 充值 manual_review 记录超过 10 条且持续 10 分钟时触发告警',
    true, 'web3_manual_review_count', '>', 10.0, 1, 10, 'P2', true, 30, NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'Web3入账失败',
    '当 Web3 充值 credit failed 计数大于 0 且持续 1 分钟时触发告警',
    true, 'web3_credit_failures_total', '>', 0.0, 1, 1, 'P1', true, 20, NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;
