-- Durable notification email delivery queue. This is intentionally additive so
-- older application images can continue serving during a blue-green rollout.

CREATE TABLE notification_email_deliveries (
    id                  BIGSERIAL PRIMARY KEY,
    dedup_key           CHAR(64) NOT NULL UNIQUE CHECK (dedup_key ~ '^[0-9a-f]{64}$'),
    event               VARCHAR(100) NOT NULL,
    channel             VARCHAR(64) NOT NULL,
    recipient_email     TEXT NOT NULL,
    recipient_hash      CHAR(64) NOT NULL CHECK (recipient_hash ~ '^[0-9a-f]{64}$'),
    recipient_name      TEXT NOT NULL DEFAULT '',
    user_id             BIGINT NOT NULL DEFAULT 0,
    source_type         VARCHAR(100) NOT NULL,
    source_id           VARCHAR(200) NOT NULL,
    reminder_key        VARCHAR(200) NOT NULL DEFAULT '',
    locale              VARCHAR(16) NOT NULL DEFAULT '',
    variables           JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_html_variables  JSONB NOT NULL DEFAULT '{}'::jsonb,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'processing', 'retry_wait', 'sent', 'failed', 'suppressed')),
    attempt_count       INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts        INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner         TEXT,
    lease_expires_at    TIMESTAMPTZ,
    last_error_category VARCHAR(32),
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at             TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_email_deliveries_claim
    ON notification_email_deliveries (next_attempt_at, id)
    WHERE status IN ('pending', 'retry_wait', 'processing');

CREATE INDEX IF NOT EXISTS idx_notification_email_deliveries_recent
    ON notification_email_deliveries (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_notification_email_deliveries_source
    ON notification_email_deliveries (source_type, source_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_email_deliveries_event_status
    ON notification_email_deliveries (event, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_email_deliveries_terminal_cleanup
    ON notification_email_deliveries (updated_at, id)
    WHERE status IN ('sent', 'suppressed', 'failed');
