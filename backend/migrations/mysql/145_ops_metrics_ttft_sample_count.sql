-- MySQL 方言版本（翻译自 ../145_ops_metrics_ttft_sample_count.sql）
-- 翻译说明：
--   - PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。

-- Add ttft_sample_count to ops_metrics_hourly / ops_metrics_daily.
--
-- first_token_ms (TTFT) is only recorded for streaming requests, but the
-- dashboard previously weighted merged TTFT percentiles by success_count
-- (all successful requests, streaming + non-streaming). That diluted/skewed
-- the merged TTFT figures whenever non-streaming traffic was present.
--
-- Store the real streaming sample count per bucket so TTFT percentiles can be
-- weighted by the number of rows that actually contributed a first_token_ms.
--
-- Existing rows default to 0; they will be repopulated on the next hourly
-- re-aggregation pass. Migration is idempotent and safe to re-run.

ALTER TABLE ops_metrics_hourly
    ADD COLUMN ttft_sample_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE ops_metrics_daily
    ADD COLUMN ttft_sample_count BIGINT NOT NULL DEFAULT 0;
