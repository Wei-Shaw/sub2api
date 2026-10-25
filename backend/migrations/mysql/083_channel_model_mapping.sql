-- MySQL 方言版本（翻译自 ../083_channel_model_mapping.sql）

-- PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

ALTER TABLE channels ADD COLUMN model_mapping JSON DEFAULT ('{}');
-- (comment skipped) COMMENT ON COLUMN channels.model_mapping
