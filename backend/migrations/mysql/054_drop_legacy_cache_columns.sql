-- MySQL 方言版本（翻译自 ../054_drop_legacy_cache_columns.sql）
-- Drop legacy cache token columns that lack the underscore separator.
-- These were created by GORM's automatic snake_case conversion:
--   CacheCreation5mTokens → cache_creation5m_tokens  (incorrect)
--   CacheCreation1hTokens → cache_creation1h_tokens  (incorrect)
--
-- The canonical columns are:
--   cache_creation_5m_tokens  (defined in 001_init.sql)
--   cache_creation_1h_tokens  (defined in 001_init.sql)
--
-- Migration 009 already copied data from legacy → canonical columns.
-- This migration drops the legacy columns to avoid confusion.
--
-- 注意：MySQL 不支持 DROP COLUMN IF EXISTS。若 MySQL 部署从未产生过这两个
-- GORM 自动转换列（001_init.sql 建表即使用正确列名），本迁移会报错，
-- 需在部署前确认列是否存在（见迁移报告疑点）。

DROP PROCEDURE IF EXISTS __migration_054_tmp;
CREATE PROCEDURE __migration_054_tmp()
BEGIN
    -- 仅当 GORM 自动转换的遗留列存在时才删除（MySQL 无 DROP COLUMN IF EXISTS）。
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
                 AND column_name = 'cache_creation5m_tokens') THEN
        ALTER TABLE usage_logs DROP COLUMN cache_creation5m_tokens;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
                 AND column_name = 'cache_creation1h_tokens') THEN
        ALTER TABLE usage_logs DROP COLUMN cache_creation1h_tokens;
    END IF;
END;
CALL __migration_054_tmp();
DROP PROCEDURE __migration_054_tmp;
