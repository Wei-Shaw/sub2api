-- MySQL 方言版本（翻译自 ../147_ops_error_log_api_key_prefix.sql）
-- 翻译说明：
--   - PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。

-- 有效(未删除)key 报错时,在 ops 落库层快照该 key 的脱敏前缀(前 8 位),
-- 便于在 /admin/ops 错误详情识别是用户的哪一个 key 出的错。
-- 与 attempted_key_prefix 互补且互斥:
--   api_key_id 非空(有效 key 报错)        => api_key_prefix
--   api_key_id 为空(INVALID_API_KEY 无效) => attempted_key_prefix
-- 落库快照(而非读时 JOIN api_keys):key 之后被删时 api_keys.key 会被 tombstone
-- 覆盖,快照可保留报错当时的真实前缀。

ALTER TABLE ops_error_logs
    ADD COLUMN api_key_prefix VARCHAR(32) NULL;
