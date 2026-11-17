ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS max_reasoning_effort_multiplier NUMERIC(10,4);

COMMENT ON COLUMN channel_model_pricing.max_reasoning_effort_multiplier IS
    'Billing/quota multiplier applied when the forwarded reasoning effort is max';

ALTER TABLE channel_model_pricing
    DROP CONSTRAINT IF EXISTS chk_channel_model_pricing_max_reasoning_effort_multiplier_positive;
ALTER TABLE channel_model_pricing
    ADD CONSTRAINT chk_channel_model_pricing_max_reasoning_effort_multiplier_positive
    CHECK (max_reasoning_effort_multiplier IS NULL OR max_reasoning_effort_multiplier > 0);
