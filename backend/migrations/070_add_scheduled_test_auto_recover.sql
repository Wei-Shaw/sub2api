-- 070: Add auto_recover column to scheduled_test_plans
-- When enabled, automatically recovers account from error/rate-limited state on successful test

ALTER TABLE scheduled_test_plans ADD COLUMN IF NOT EXISTS auto_recover BOOLEAN NOT NULL DEFAULT false;
