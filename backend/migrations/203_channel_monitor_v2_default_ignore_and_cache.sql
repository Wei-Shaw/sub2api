-- Factory presets for Channel Monitor V2 config:
-- 1) ignored_error_categories: non-ops client/policy failures that should not
--    dominate health error_rate by default (still shown greyed in breakdown).
-- 2) health_thresholds cache floors are intentionally left at the tolerant
--    zero/zero defaults from migration 198. Platforms without prompt caching
--    must not be marked unhealthy by factory configuration.
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
