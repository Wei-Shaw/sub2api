ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS cache_write_1h_price NUMERIC(20,12);

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS cache_write_1h_price NUMERIC(20,12);

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN IF NOT EXISTS cache_write_1h_price NUMERIC(20,12);

ALTER TABLE channel_account_stats_pricing_intervals
    ADD COLUMN IF NOT EXISTS cache_write_1h_price NUMERIC(20,12);

COMMENT ON COLUMN channel_model_pricing.cache_write_1h_price IS
    '1h cache write price per token; NULL preserves legacy cache_write_price behavior';
COMMENT ON COLUMN channel_pricing_intervals.cache_write_1h_price IS
    'Interval-specific 1h cache write price per token';
COMMENT ON COLUMN channel_account_stats_model_pricing.cache_write_1h_price IS
    '1h cache write price per token for account stats pricing';
COMMENT ON COLUMN channel_account_stats_pricing_intervals.cache_write_1h_price IS
    'Interval-specific 1h cache write price per token for account stats pricing';
