SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS service_tier_config JSONB NOT NULL DEFAULT
    '{"standard":{"enabled":true,"multiplier":1.0},"priority":{"enabled":true,"multiplier":2.0},"flex":{"enabled":true,"multiplier":0.5}}'::jsonb;

ALTER TABLE channels
    DROP CONSTRAINT IF EXISTS channels_service_tier_config_object;

ALTER TABLE channels
    ADD CONSTRAINT channels_service_tier_config_object
    CHECK (jsonb_typeof(service_tier_config) = 'object');

COMMENT ON COLUMN channels.service_tier_config IS
    'OpenAI commercial service tier availability and billing multipliers';
