-- MySQL 方言版本（翻译自 ../034_ops_upstream_error_events.sql）

-- Add upstream error events list (JSON) to ops_error_logs for per-request correlation.
--
-- This is intentionally idempotent.

ALTER TABLE ops_error_logs
    ADD COLUMN upstream_errors JSON NULL;

-- (comment skipped)
