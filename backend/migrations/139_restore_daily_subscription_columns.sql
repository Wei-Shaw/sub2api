-- Restore daily subscription columns still required by the current scheduler and subscription code.
ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20, 8) DEFAULT NULL;

ALTER TABLE user_subscriptions
  ADD COLUMN IF NOT EXISTS daily_window_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS daily_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0;
