-- MySQL 方言版本（翻译自 ../205_channel_monitor_v2_reset_factory_cache_thresholds.sql）
-- Keep factory cache scoring tolerant. Migration 203 briefly wrote 0.85/0.60;
-- reset only that exact factory pair, preserving operator-customized values.
--
-- 说明：health_thresholds || jsonb_build_object(...)（右侧键优先）→
--       JSON_MERGE_PATCH(health_thresholds, JSON_OBJECT(...))；
--       (x->>'k')::float8 → CAST(JSON_UNQUOTE(JSON_EXTRACT(x, '$.k')) AS DECIMAL(20,10))。
UPDATE channel_monitor_v2_config
SET health_thresholds = JSON_MERGE_PATCH(health_thresholds, JSON_OBJECT(
        'warning_cache_rate', 0,
        'critical_cache_rate', 0
    ))
WHERE id = 1
  AND COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(health_thresholds, '$.warning_cache_rate')) AS DECIMAL(20,10)), 0) = 0.85
  AND COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(health_thresholds, '$.critical_cache_rate')) AS DECIMAL(20,10)), 0) = 0.60
  AND version = 1
  AND updated_by IS NULL;
