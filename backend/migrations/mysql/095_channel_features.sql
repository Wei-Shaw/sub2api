-- MySQL 方言版本（翻译自 ../095_channel_features.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（执行器保证单次执行）；
--           TEXT NOT NULL DEFAULT '' → TEXT NOT NULL DEFAULT ('')（表达式默认值）；
--           COMMENT ON COLUMN 整句跳过。

ALTER TABLE channels ADD COLUMN features TEXT NOT NULL DEFAULT ('');
-- (comment skipped)
