-- Observation only: these tables never update user_subscriptions or billing.
CREATE TABLE IF NOT EXISTS subscription_reset_policies (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    version BIGINT NOT NULL CHECK (version > 0),
    mode TEXT NOT NULL CHECK (mode IN ('off', 'observe')),
    policy JSONB NOT NULL CHECK (jsonb_typeof(policy) = 'object'),
    state JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(state) = 'object'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS subscription_reset_policies_enabled_idx
    ON subscription_reset_policies(group_id) WHERE mode = 'observe';

CREATE TABLE IF NOT EXISTS subscription_reset_events (
    id TEXT PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    policy_version BIGINT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object')
);
CREATE INDEX IF NOT EXISTS subscription_reset_events_group_recent_idx
    ON subscription_reset_events(group_id, opened_at DESC, id DESC);
