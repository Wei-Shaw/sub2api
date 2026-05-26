-- Add resolution tracking to ops_error_logs, persist retry results, and standardize error classification enums.
--
-- This migration is intentionally idempotent.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- ============================================
-- 1) ops_error_logs: resolution fields
-- ============================================

ALTER TABLE ops_error_logs
  ADD COLUMN IF NOT EXISTS resolved BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE ops_error_logs
  ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

ALTER TABLE ops_error_logs
  ADD COLUMN IF NOT EXISTS resolved_by_user_id BIGINT;

ALTER TABLE ops_error_logs
  ADD COLUMN IF NOT EXISTS resolved_retry_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_resolved_time
  ON ops_error_logs (resolved, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_unresolved_time
  ON ops_error_logs (created_at DESC)
  WHERE resolved = false;

-- ============================================
-- 2) ops_retry_attempts: persist execution results
-- ============================================

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS success BOOLEAN;

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS http_status_code INT;

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS used_account_id BIGINT;

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS response_preview TEXT;

ALTER TABLE ops_retry_attempts
  ADD COLUMN IF NOT EXISTS response_truncated BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_ops_retry_attempts_success_time
  ON ops_retry_attempts (success, created_at DESC);

-- Backfill best-effort fields for existing rows.
UPDATE ops_retry_attempts
SET success = (LOWER(COALESCE(status, '')) = 'succeeded')
WHERE success IS NULL;

UPDATE ops_retry_attempts
SET upstream_request_id = result_request_id
WHERE upstream_request_id IS NULL AND result_request_id IS NOT NULL;

-- ============================================
-- 3) Standardize classification enums in ops_error_logs
--
-- New enums:
--   error_phase:  request|auth|routing|upstream|network|internal
--   error_owner:  client|provider|platform
--   error_source: client_request|upstream_http|gateway
-- ============================================

-- Owner: legacy sub2api => platform.
UPDATE ops_error_logs
SET error_owner = 'platform'
WHERE LOWER(COALESCE(error_owner, '')) = 'sub2api';

-- Owner: normalize empty/null to platform (best-effort).
UPDATE ops_error_logs
SET error_owner = 'platform'
WHERE COALESCE(TRIM(error_owner), '') = '';

-- Phase: map legacy phases.
UPDATE ops_error_logs
SET error_phase = CASE
  WHEN COALESCE(TRIM(error_phase), '') = '' THEN 'internal'
  WHEN LOWER(error_phase) IN ('billing', 'concurrency', 'response') THEN 'request'
  WHEN LOWER(error_phase) IN ('scheduling') THEN 'routing'
  WHEN LOWER(error_phase) IN ('request', 'auth', 'routing', 'upstream', 'network', 'internal') THEN LOWER(error_phase)
  ELSE 'internal'
END;

-- Source: map legacy sources.
UPDATE ops_error_logs
SET error_source = CASE
  WHEN COALESCE(TRIM(error_source), '') = '' THEN 'gateway'
  WHEN LOWER(error_source) IN ('billing', 'concurrency') THEN 'client_request'
  WHEN LOWER(error_source) IN ('upstream_http') THEN 'upstream_http'
  WHEN LOWER(error_source) IN ('upstream_network') THEN 'gateway'
  WHEN LOWER(error_source) IN ('internal') THEN 'gateway'
  WHEN LOWER(error_source) IN ('client_request', 'upstream_http', 'gateway') THEN LOWER(error_source)
  ELSE 'gateway'
END;

-- Auto-resolve recovered upstream errors (client status < 400).
UPDATE ops_error_logs
SET
  resolved = true,
  resolved_at = COALESCE(resolved_at, created_at)
WHERE resolved = false AND COALESCE(status_code, 0) > 0 AND COALESCE(status_code, 0) < 400;
