ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS five_hour_limit_usd DECIMAL(20,8);

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS five_hour_window_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS five_hour_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0;
