-- Persist per-subscription expiry reminder state so each reminder bucket is sent once per expiry cycle.

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS expiry_reminder_key VARCHAR(32),
    ADD COLUMN IF NOT EXISTS expiry_reminder_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS expiry_reminder_sent_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_expiry_reminder_state
    ON user_subscriptions (expiry_reminder_key, expiry_reminder_expires_at)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN user_subscriptions.expiry_reminder_key IS 'Last subscription expiry reminder bucket sent, e.g. 7d, 3d, 1d.';
COMMENT ON COLUMN user_subscriptions.expiry_reminder_expires_at IS 'Subscription expiry timestamp associated with the last expiry reminder.';
COMMENT ON COLUMN user_subscriptions.expiry_reminder_sent_at IS 'Timestamp when the last subscription expiry reminder was sent.';
