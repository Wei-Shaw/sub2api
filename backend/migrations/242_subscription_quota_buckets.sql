-- Presence of a group state opts it into permanent bucket accounting.
-- Creating these tables alone does not enable accounting or reset any quota.
CREATE SEQUENCE IF NOT EXISTS subscription_quota_state_version;

CREATE TABLE IF NOT EXISTS subscription_group_quota_state (
 group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
 enabled_at TIMESTAMPTZ NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE IF NOT EXISTS subscription_usage_buckets (
 id BIGSERIAL PRIMARY KEY,
 subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
 dimension TEXT NOT NULL CHECK (dimension IN ('daily','weekly','monthly')),
 term_epoch BIGINT NOT NULL CHECK (term_epoch > 0),
 group_revision BIGINT NOT NULL CHECK (group_revision > 0),
 reason TEXT NOT NULL,
 window_start TIMESTAMPTZ,
 term_starts_at TIMESTAMPTZ NOT NULL,
 legacy_identity TEXT,
 used_usd NUMERIC(20,10) NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE (subscription_id,dimension,legacy_identity)
);
CREATE INDEX IF NOT EXISTS idx_subscription_usage_buckets_subscription
 ON subscription_usage_buckets(subscription_id,id);

CREATE TABLE IF NOT EXISTS subscription_quota_state (
 subscription_id BIGINT PRIMARY KEY REFERENCES user_subscriptions(id) ON DELETE CASCADE,
 term_epoch BIGINT NOT NULL DEFAULT 1 CHECK (term_epoch > 0),
 state_version BIGINT NOT NULL DEFAULT nextval('subscription_quota_state_version'),
 daily_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id),
 weekly_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id),
 monthly_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id)
);

CREATE TABLE IF NOT EXISTS subscription_quota_charges (
 request_id TEXT NOT NULL,
 api_key_id BIGINT NOT NULL,
 subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
 daily_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id),
 weekly_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id),
 monthly_bucket_id BIGINT NOT NULL REFERENCES subscription_usage_buckets(id),
 admitted_at TIMESTAMPTZ NOT NULL,
 cost_usd NUMERIC(20,8) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(request_id,api_key_id)
);
CREATE INDEX IF NOT EXISTS idx_subscription_quota_charges_subscription
 ON subscription_quota_charges(subscription_id,created_at);
