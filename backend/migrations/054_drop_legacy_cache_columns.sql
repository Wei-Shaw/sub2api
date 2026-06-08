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

ALTER TABLE usage_logs DROP COLUMN IF EXISTS cache_creation5m_tokens;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS cache_creation1h_tokens;
