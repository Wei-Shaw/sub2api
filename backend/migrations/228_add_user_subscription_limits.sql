ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20,10);

COMMENT ON COLUMN user_subscriptions.daily_limit_usd IS
    'Optional per-user subscription daily USD limit; NULL falls back to the group limit';
COMMENT ON COLUMN user_subscriptions.weekly_limit_usd IS
    'Optional per-user subscription weekly USD limit; NULL falls back to the group limit';
COMMENT ON COLUMN user_subscriptions.monthly_limit_usd IS
    'Optional per-user subscription monthly USD limit; NULL falls back to the group limit';
