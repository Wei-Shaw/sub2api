-- Add a rolling five-hour quota window for user x platform limits.
-- Existing rows remain unlimited for this window because the limit is NULL.
-- reset_generation prevents stale Redis snapshots from overwriting an admin reset.

ALTER TABLE user_platform_quotas
    ADD COLUMN IF NOT EXISTS five_hour_limit_usd DECIMAL(20,10),
    ADD COLUMN IF NOT EXISTS five_hour_usage_usd DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS five_hour_window_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reset_generation BIGINT NOT NULL DEFAULT 0;
