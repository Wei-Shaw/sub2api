-- Enterprise API keys.
--
-- An API key may optionally bind to a company (organization) subscription.
-- When organization_subscription_id is set, requests made with this key consume
-- the referenced organization_subscriptions row's usage counters instead of the
-- owner's personal user subscription. NULL means a regular personal key.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS organization_subscription_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_api_keys_org_subscription_id
    ON api_keys(organization_subscription_id)
    WHERE organization_subscription_id IS NOT NULL;
