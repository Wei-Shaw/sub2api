-- Add upstream error events list (JSONB) to ops_error_logs for per-request correlation.
--
-- This is intentionally idempotent.

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS upstream_errors JSONB;

COMMENT ON COLUMN ops_error_logs.upstream_errors IS
    'Sanitized upstream error events list (JSON array), correlated per gateway request (request_id/client_request_id); used for per-request upstream debugging.';
