-- Traffic sufficient statistics for the ops overview's hourly preaggregation.
-- NULL marks old/not-recomputed buckets, not observed zero traffic. Auto mode
-- falls back to raw logs for such windows; forced preagg reports not-ready.
-- The existing hourly aggregation job populates this column without a new timer.
-- Historical buckets outside its overlap need an explicit hourly recomputation
-- to use forced preagg; this migration deliberately does not scan usage_logs or
-- rewrite historical rows with fabricated zero-byte observations.
-- Source: ops_traffic_events (migration 248), one finalized HTTP request per row.
-- Aggregate byte sums are NULL when unknown; paired sums preserve ratio coverage.
-- Daily buckets are not used by the overview and do not store traffic.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS traffic JSONB;

COMMENT ON COLUMN ops_metrics_hourly.traffic IS
    'Additive HTTP body byte sums and observation counts from the HTTP request ledger; NULL means traffic preaggregation unavailable';
