-- Optional time-of-day pricing overrides for channel and account-stat pricing.
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS time_pricing JSONB;

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN IF NOT EXISTS time_pricing JSONB;

COMMENT ON COLUMN channel_model_pricing.time_pricing IS
    'Optional time pricing config. Matching period prices override the base or interval price.';

COMMENT ON COLUMN channel_account_stats_model_pricing.time_pricing IS
    'Optional time pricing config for account statistics pricing rules.';
