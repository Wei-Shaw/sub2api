ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS fast_multiplier NUMERIC(12,6);

ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS flex_multiplier NUMERIC(12,6);

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS input_multiplier NUMERIC(12,6);

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS output_multiplier NUMERIC(12,6);

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS cache_write_multiplier NUMERIC(12,6);

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS cache_read_multiplier NUMERIC(12,6);

-- Keep the checks NOT VALID during rollout: old rows may remain NULL, while
-- every new or changed value is constrained immediately without a table scan.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_model_pricing
    DROP CONSTRAINT IF EXISTS channel_model_pricing_fast_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_model_pricing
    ADD CONSTRAINT channel_model_pricing_fast_multiplier_positive
    CHECK (fast_multiplier IS NULL OR fast_multiplier > 0) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_model_pricing
    DROP CONSTRAINT IF EXISTS channel_model_pricing_flex_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_model_pricing
    ADD CONSTRAINT channel_model_pricing_flex_multiplier_positive
    CHECK (flex_multiplier IS NULL OR flex_multiplier > 0) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    DROP CONSTRAINT IF EXISTS channel_pricing_intervals_input_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_input_multiplier_positive
    CHECK (input_multiplier IS NULL OR input_multiplier > 0) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    DROP CONSTRAINT IF EXISTS channel_pricing_intervals_output_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_output_multiplier_positive
    CHECK (output_multiplier IS NULL OR output_multiplier > 0) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    DROP CONSTRAINT IF EXISTS channel_pricing_intervals_cache_write_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_cache_write_multiplier_positive
    CHECK (cache_write_multiplier IS NULL OR cache_write_multiplier > 0) NOT VALID;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    DROP CONSTRAINT IF EXISTS channel_pricing_intervals_cache_read_multiplier_positive;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_cache_read_multiplier_positive
    CHECK (cache_read_multiplier IS NULL OR cache_read_multiplier > 0) NOT VALID;
