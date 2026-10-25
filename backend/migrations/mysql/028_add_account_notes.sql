-- MySQL 方言版本（翻译自 ../028_add_account_notes.sql）
-- 028_add_account_notes.sql
-- Add optional admin notes for accounts.
--
-- 翻译说明：ADD COLUMN IF NOT EXISTS → 去掉修饰词；COMMENT ON COLUMN → 整句跳过。

ALTER TABLE accounts
ADD COLUMN notes TEXT;

-- (comment skipped) COMMENT ON COLUMN accounts.notes IS 'Admin-only notes for account'
