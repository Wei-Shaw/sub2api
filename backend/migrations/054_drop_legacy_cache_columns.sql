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
-- But upgraded instances may still have post-009 writes in legacy columns.
-- Backfill once more before dropping to prevent data loss.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation5m_tokens'
    ) THEN
        UPDATE usage_logs
        SET cache_creation_5m_tokens = cache_creation5m_tokens
        WHERE cache_creation_5m_tokens = 0
          AND cache_creation5m_tokens <> 0;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation1h_tokens'
    ) THEN
        UPDATE usage_logs
        SET cache_creation_1h_tokens = cache_creation1h_tokens
        WHERE cache_creation_1h_tokens = 0
          AND cache_creation1h_tokens <> 0;
    END IF;
END $$;

ALTER TABLE usage_logs DROP COLUMN IF EXISTS cache_creation5m_tokens;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS cache_creation1h_tokens;
