-- MySQL 方言版本（翻译自 ../136_remove_ops_retry_replay.sql）
-- 翻译说明：
--   - DROP TABLE ... CASCADE → DROP TABLE（MySQL 无该 CASCADE 语义；ops_* 表本就无外键依赖）。
--   - DROP COLUMN IF EXISTS → DROP COLUMN（IF EXISTS 按迁移执行器单次执行语义去掉）。
--   - PG 的 COMMENT ON TABLE 整句跳过。

-- Remove unused Ops retry/replay storage.
-- The retry endpoints are no longer exposed, so keeping request bodies and
-- retry audit rows only increases write width, memory retention, and DB size.

DROP TABLE IF EXISTS ops_retry_attempts;

ALTER TABLE ops_error_logs
  DROP COLUMN request_body,
  DROP COLUMN request_headers,
  DROP COLUMN request_body_truncated,
  DROP COLUMN request_body_bytes,
  DROP COLUMN is_retryable,
  DROP COLUMN retry_count,
  DROP COLUMN resolved_retry_id;

-- (comment skipped)
