-- MySQL 方言版本（翻译自 ../198_channel_monitor_v2_health_thresholds.sql）
-- Configurable Channel Monitor V2 health scoring thresholds.
-- Defaults are intentionally tolerant: small amounts of real traffic errors or
-- zero cache rate should not immediately paint the user page as "abnormal".
--
-- 说明：JSONB NOT NULL DEFAULT '<json>'::jsonb → JSON NOT NULL DEFAULT ('<json>')（8.0.13+ 表达式默认值）；
--       UPDATE 中 defaults::jsonb || health_thresholds（右侧键优先）→
--       JSON_MERGE_PATCH(defaults, health_thresholds)（两参数顶层值均为标量，浅合并语义等价）。
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN health_thresholds JSON NOT NULL DEFAULT ('{
      "minimum_sample": 50,
      "warning_error_rate": 0.05,
      "critical_error_rate": 0.20,
      "target_ttft_ms": 3000,
      "warning_ttft_ms": 8000,
      "critical_ttft_ms": 20000,
      "warning_cache_rate": 0,
      "critical_cache_rate": 0,
      "error_weight": 0.60,
      "ttft_weight": 0.20,
      "cache_weight": 0.20
    }');

UPDATE channel_monitor_v2_config
SET health_thresholds = JSON_MERGE_PATCH(CAST('{
      "minimum_sample": 50,
      "warning_error_rate": 0.05,
      "critical_error_rate": 0.20,
      "target_ttft_ms": 3000,
      "warning_ttft_ms": 8000,
      "critical_ttft_ms": 20000,
      "warning_cache_rate": 0,
      "critical_cache_rate": 0,
      "error_weight": 0.60,
      "ttft_weight": 0.20,
      "cache_weight": 0.20
    }' AS JSON), health_thresholds)
WHERE id = 1;
