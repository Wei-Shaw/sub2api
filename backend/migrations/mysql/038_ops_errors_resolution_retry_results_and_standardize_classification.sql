-- MySQL 方言版本（翻译自 ../038_ops_errors_resolution_retry_results_and_standardize_classification.sql）

-- Add resolution tracking to ops_error_logs, persist retry results, and standardize error classification enums.
--
-- This migration is intentionally idempotent.

-- PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

-- ============================================
-- 1) ops_error_logs: resolution fields
-- ============================================

ALTER TABLE ops_error_logs
  ADD COLUMN resolved BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE ops_error_logs
  ADD COLUMN resolved_at DATETIME(3) NULL;

ALTER TABLE ops_error_logs
  ADD COLUMN resolved_by_user_id BIGINT NULL;

ALTER TABLE ops_error_logs
  ADD COLUMN resolved_retry_id BIGINT NULL;

CREATE INDEX idx_ops_error_logs_resolved_time
  ON ops_error_logs (resolved, created_at DESC);

-- PG 部分索引（WHERE resolved = false）→ 非唯一部分索引 → 去掉 WHERE（普通索引，仅性能差异）
CREATE INDEX idx_ops_error_logs_unresolved_time
  ON ops_error_logs (created_at DESC);

-- ============================================
-- 2) ops_retry_attempts: persist execution results
-- ============================================

ALTER TABLE ops_retry_attempts
  ADD COLUMN success BOOLEAN NULL;

ALTER TABLE ops_retry_attempts
  ADD COLUMN http_status_code INT NULL;

ALTER TABLE ops_retry_attempts
  ADD COLUMN upstream_request_id VARCHAR(128) NULL;

ALTER TABLE ops_retry_attempts
  ADD COLUMN used_account_id BIGINT NULL;

ALTER TABLE ops_retry_attempts
  ADD COLUMN response_preview TEXT NULL;

ALTER TABLE ops_retry_attempts
  ADD COLUMN response_truncated BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_ops_retry_attempts_success_time
  ON ops_retry_attempts (success, created_at DESC);

-- Backfill best-effort fields for existing rows.
-- （BOOLEAN 比较结果 1/0，写入 TINYINT(1) 列语义一致）
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
  resolved = TRUE,
  resolved_at = COALESCE(resolved_at, created_at)
WHERE resolved = FALSE AND COALESCE(status_code, 0) > 0 AND COALESCE(status_code, 0) < 400;
