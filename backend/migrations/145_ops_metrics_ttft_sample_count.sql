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

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS ttft_sample_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE ops_metrics_daily
    ADD COLUMN IF NOT EXISTS ttft_sample_count BIGINT NOT NULL DEFAULT 0;
