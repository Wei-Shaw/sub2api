-- MySQL 方言版本（翻译自 ../231_add_usage_log_native_compaction_v2.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN；COMMENT ON COLUMN 整句跳过。

ALTER TABLE usage_logs
    ADD COLUMN native_compaction_v2 BOOLEAN NOT NULL DEFAULT FALSE;

-- (comment skipped)
