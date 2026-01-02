-- Seed additional ops alert rules (idempotent)

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'Global error rate > 1%',
    'Trigger when the 1-minute error rate exceeds 1% for 2 consecutive minutes.',
    TRUE,
    'error_rate',
    '>',
    1,
    1,
    2,
    'P1',
    TRUE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    10
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'Global error rate > 1%');

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'P99 latency > 2000ms',
    'Trigger when the 5-minute P99 latency exceeds 2000ms for 2 consecutive samples.',
    TRUE,
    'p99_latency_ms',
    '>',
    2000,
    5,
    2,
    'P1',
    TRUE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    15
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'P99 latency > 2000ms');

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'HTTP/2 errors > 20',
    'Trigger when HTTP/2 errors exceed 20 in the last minute for 2 consecutive minutes.',
    TRUE,
    'http2_errors',
    '>',
    20,
    1,
    2,
    'P2',
    FALSE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    10
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'HTTP/2 errors > 20');

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'CPU usage > 85%',
    'Trigger when CPU usage exceeds 85% for 5 consecutive minutes.',
    TRUE,
    'cpu_usage_percent',
    '>',
    85,
    1,
    5,
    'P2',
    FALSE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    15
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'CPU usage > 85%');

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'Memory usage > 90%',
    'Trigger when memory usage exceeds 90% for 5 consecutive minutes.',
    TRUE,
    'memory_usage_percent',
    '>',
    90,
    1,
    5,
    'P2',
    FALSE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    15
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'Memory usage > 90%');

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    severity,
    notify_email,
    notify_webhook,
    webhook_url,
    cooldown_minutes
)
SELECT
    'Queue depth > 50',
    'Trigger when concurrency queue depth exceeds 50 for 2 consecutive minutes.',
    TRUE,
    'concurrency_queue_depth',
    '>',
    50,
    1,
    2,
    'P2',
    FALSE,
    CASE
        WHEN (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1) IS NULL THEN FALSE
        ELSE TRUE
    END,
    (SELECT webhook_url FROM ops_alert_rules WHERE webhook_url IS NOT NULL AND webhook_url <> '' LIMIT 1),
    10
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules WHERE name = 'Queue depth > 50');
