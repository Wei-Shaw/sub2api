-- MySQL 方言版本（翻译自 ../242_group_i18n.sql）
-- 翻译说明：
--   - `groups` 是 MySQL 8 保留字（窗口函数 GROUPS），加反引号；
--   - JSONB NOT NULL DEFAULT '{}'::jsonb → JSON NOT NULL DEFAULT ('{}')
--     （8.0.13+ 表达式默认值）；
--   - ADD COLUMN IF NOT EXISTS 按迁移执行器单次执行语义去掉修饰词；
--   - COMMENT ON COLUMN 整句跳过。

ALTER TABLE `groups`
    ADD COLUMN i18n JSON NOT NULL DEFAULT ('{}');

-- (comment skipped) COMMENT ON COLUMN groups.i18n IS
--     'Per-locale display text for the group: {"<locale>":{"name":string,"description":string}}; locales or fields that are absent fall back to groups.name / groups.description'
