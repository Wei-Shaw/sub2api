-- MySQL 方言版本（翻译自 ../101_add_channel_features_config.sql）
-- 翻译说明：JSONB NOT NULL DEFAULT '{}' → JSON NOT NULL DEFAULT ('{}')（表达式默认值）；
--           COMMENT ON COLUMN 整句跳过。

ALTER TABLE channels ADD COLUMN features_config JSON NOT NULL DEFAULT ('{}');
-- (comment skipped)
