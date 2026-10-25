-- MySQL 方言版本（翻译自 ../232_group_force_openai_fast.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字需反引号；ADD COLUMN IF NOT EXISTS → ADD COLUMN；
--           COMMENT ON COLUMN 整句跳过。

ALTER TABLE `groups`
    ADD COLUMN force_openai_fast BOOLEAN NOT NULL DEFAULT FALSE;

-- (comment skipped)
