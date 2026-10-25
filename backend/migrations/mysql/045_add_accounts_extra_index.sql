-- MySQL 方言版本（翻译自 ../045_add_accounts_extra_index.sql）
-- Migration: 045_add_accounts_extra_index
-- 为 accounts.extra 字段添加索引，优化 FindByExtraField 查询性能
-- 用于支持通过 extra 字段中的 linked_openai_account_id 快速查找关联的 Sora 账号
--
-- 说明：MySQL 无法直接对 JSON 列建索引（PG 的 GIN(jsonb)），改为针对该查询
-- 实际使用的 JSON 键（$.linked_openai_account_id）建立函数索引。

CREATE INDEX idx_accounts_extra_gin
ON accounts ((CAST(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.linked_openai_account_id')) AS CHAR(64))));

-- 查询示例（使用 JSON_CONTAINS 等价改写 @> 操作符）
-- EXPLAIN ANALYZE
-- SELECT * FROM accounts
-- WHERE platform = 'sora'
--   AND JSON_CONTAINS(extra, CAST('{"linked_openai_account_id": 123}' AS JSON))
--   AND deleted_at IS NULL;
