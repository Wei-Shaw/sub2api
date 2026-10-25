-- MySQL 方言版本（翻译自 ../070_add_scheduled_test_auto_recover.sql）
-- 070: Add auto_recover column to scheduled_test_plans
-- When enabled, automatically recovers account from error/rate-limited state on successful test

ALTER TABLE scheduled_test_plans ADD COLUMN auto_recover BOOLEAN NOT NULL DEFAULT FALSE;
