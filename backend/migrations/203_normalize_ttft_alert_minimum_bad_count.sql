-- TTFT percentiles already encode tail latency and do not have a meaningful
-- per-request bad count. Setting this field to zero is compatible with both
-- old and new application images during blue-green rollout and rollback.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_alert_rules
SET minimum_bad_count = 0,
    updated_at = NOW()
WHERE metric_type IN (
    'ttft_p95_seconds',
    'ttft_p99_seconds',
    'ttft_max_seconds'
)
AND minimum_bad_count <> 0;
