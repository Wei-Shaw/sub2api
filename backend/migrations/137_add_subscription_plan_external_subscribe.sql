ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS external_subscribe_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS external_subscribe_url VARCHAR(500) NOT NULL DEFAULT '';
