-- Factory presets for Channel Monitor V2 config:
-- 1) ignored_error_categories: non-ops client/policy failures that should not
--    dominate health error_rate by default (still shown greyed in breakdown).
-- 2) health_thresholds cache floors: 85% watch / 60% critical (higher is better).
--
-- Only apply when the row still looks like factory empty ignore list and/or
-- zero cache thresholds (operator customizations are left alone).

UPDATE channel_monitor_v2_config
SET ignored_error_categories = ARRAY[
    'authentication',
    'client_cancelled',
    'content_policy',
    'context_limit',
    'group_access',
    'model_unsupported',
    'not_found',
    'quota_or_balance'
]::text[]
WHERE id = 1
  AND COALESCE(cardinality(ignored_error_categories), 0) = 0;

UPDATE channel_monitor_v2_config
SET health_thresholds = health_thresholds
    || jsonb_build_object(
        'warning_cache_rate', 0.85,
        'critical_cache_rate', 0.60
    )
WHERE id = 1
  AND COALESCE((health_thresholds->>'warning_cache_rate')::float8, 0) = 0
  AND COALESCE((health_thresholds->>'critical_cache_rate')::float8, 0) = 0;
