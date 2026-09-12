ALTER TABLE subscription_reset_policies DROP CONSTRAINT IF EXISTS subscription_reset_policies_mode_check;
ALTER TABLE subscription_reset_policies ADD CONSTRAINT subscription_reset_policies_mode_check CHECK (mode IN ('off','observe','auto'));
DROP INDEX IF EXISTS subscription_reset_policies_enabled_idx;
CREATE INDEX subscription_reset_policies_enabled_idx ON subscription_reset_policies(group_id) WHERE mode IN ('observe','auto');

-- The event identifier is the durable idempotency key for a group grant.
CREATE TABLE IF NOT EXISTS subscription_reset_executions (
    event_id TEXT PRIMARY KEY REFERENCES subscription_reset_events(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    group_revision BIGINT NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    affected_subscriptions BIGINT NOT NULL,
    dimensions JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS subscription_reset_outbox (
    event_id TEXT PRIMARY KEY REFERENCES subscription_reset_executions(event_id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL,
    group_revision BIGINT NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
