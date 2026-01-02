-- Seed default ops alert rules (idempotent)

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
    'Global success rate < 99%',
    'Trigger when the 1-minute success rate drops below 99% for 2 consecutive minutes.',
    TRUE,
    'success_rate',
    '<',
    99,
    1,
    2,
    'P1',
    TRUE,
    FALSE,
    NULL,
    10
WHERE NOT EXISTS (SELECT 1 FROM ops_alert_rules);
