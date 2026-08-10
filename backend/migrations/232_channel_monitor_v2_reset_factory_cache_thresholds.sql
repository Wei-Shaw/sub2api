-- Keep factory cache scoring tolerant. Migration 203 briefly wrote 0.85/0.60;
-- reset only that exact factory pair, preserving operator-customized values.
UPDATE channel_monitor_v2_config
SET health_thresholds = health_thresholds
    || jsonb_build_object(
        'warning_cache_rate', 0,
        'critical_cache_rate', 0
    )
WHERE id = 1
  AND COALESCE((health_thresholds->>'warning_cache_rate')::float8, 0) = 0.85
  AND COALESCE((health_thresholds->>'critical_cache_rate')::float8, 0) = 0.60
  AND version = 1
  AND updated_by IS NULL;
