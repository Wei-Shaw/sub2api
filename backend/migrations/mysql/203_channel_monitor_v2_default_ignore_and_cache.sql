-- MySQL 方言版本（翻译自 ../203_channel_monitor_v2_default_ignore_and_cache.sql）
-- Factory presets for Channel Monitor V2 config:
-- 1) ignored_error_categories: non-ops client/policy failures that should not
--    dominate health error_rate by default (still shown greyed in breakdown).
-- 2) health_thresholds cache floors: 85% watch / 60% critical (higher is better).
--
-- Only apply when the row still looks like factory empty ignore list and/or
-- zero cache thresholds (operator customizations are left alone).
--
-- 说明：
--   - TEXT[]（ARRAY[...]::text[]）→ JSON 数组字面量（196 迁移已将该列收敛为 JSON）。
--   - cardinality(x) → JSON_LENGTH(x)，NULL → COALESCE(..., 0)。
--   - health_thresholds || jsonb_build_object(...)（右侧键优先）→
--     JSON_MERGE_PATCH(health_thresholds, JSON_OBJECT(...))。
--   - (x->>'k')::float8 → CAST(JSON_UNQUOTE(JSON_EXTRACT(x, '$.k')) AS DECIMAL(20,10))
--     （规则：::numeric → DECIMAL(20,10) 精度）。

UPDATE channel_monitor_v2_config
SET ignored_error_categories = CAST('["authentication","client_cancelled","content_policy","context_limit","group_access","model_unsupported","not_found","quota_or_balance"]' AS JSON)
WHERE id = 1
  AND COALESCE(JSON_LENGTH(ignored_error_categories), 0) = 0;

UPDATE channel_monitor_v2_config
SET health_thresholds = JSON_MERGE_PATCH(health_thresholds, JSON_OBJECT(
        'warning_cache_rate', 0.85,
        'critical_cache_rate', 0.60
    ))
WHERE id = 1
  AND COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(health_thresholds, '$.warning_cache_rate')) AS DECIMAL(20,10)), 0) = 0
  AND COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(health_thresholds, '$.critical_cache_rate')) AS DECIMAL(20,10)), 0) = 0;
