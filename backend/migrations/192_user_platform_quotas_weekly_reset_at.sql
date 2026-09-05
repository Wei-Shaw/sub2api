ALTER TABLE user_platform_quotas
    ADD COLUMN IF NOT EXISTS weekly_window_reset_at TIMESTAMPTZ;
