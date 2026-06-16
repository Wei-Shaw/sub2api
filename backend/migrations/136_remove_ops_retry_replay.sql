-- Remove unused Ops retry/replay storage.
-- The retry endpoints are no longer exposed, so keeping request bodies and
-- retry audit rows only increases write width, memory retention, and DB size.

DROP TABLE IF EXISTS ops_retry_attempts CASCADE;

ALTER TABLE ops_error_logs
  DROP COLUMN IF EXISTS request_body,
  DROP COLUMN IF EXISTS request_headers,
  DROP COLUMN IF EXISTS request_body_truncated,
  DROP COLUMN IF EXISTS request_body_bytes,
  DROP COLUMN IF EXISTS is_retryable,
  DROP COLUMN IF EXISTS retry_count,
  DROP COLUMN IF EXISTS resolved_retry_id;

COMMENT ON TABLE ops_error_logs IS 'Ops error logs (vNext). Stores sanitized error details; request replay storage removed.';
