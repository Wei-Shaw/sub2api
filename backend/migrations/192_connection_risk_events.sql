-- Abnormal user connection risk events (方案 B).
-- Append-heavy operational table; soft lifecycle via status, not hard delete required.
-- Single-row DELETE is allowed for admin ops (Phase A); bulk retention via cleanup job later.

CREATE TABLE IF NOT EXISTS connection_risk_events (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    subject_type    VARCHAR(16)  NOT NULL,          -- api_key | user
    user_id         BIGINT,
    api_key_id      BIGINT,
    api_key_prefix  VARCHAR(32)  NOT NULL DEFAULT '',

    rules_fired     JSONB        NOT NULL DEFAULT '[]'::jsonb,
    severity        VARCHAR(16)  NOT NULL DEFAULT '',
    score           DOUBLE PRECISION NOT NULL DEFAULT 0,
    status          VARCHAR(16)  NOT NULL DEFAULT 'open',  -- open|acknowledged|resolved|suppressed

    title           VARCHAR(256) NOT NULL DEFAULT '',
    summary         TEXT         NOT NULL DEFAULT '',
    evidence        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    metrics         JSONB        NOT NULL DEFAULT '{}'::jsonb,

    dedupe_key      VARCHAR(191) NOT NULL DEFAULT '',
    action_taken    VARCHAR(64)  NOT NULL DEFAULT 'none',
    resolver_id     BIGINT,
    resolved_at     TIMESTAMPTZ,

    first_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    window_start    TIMESTAMPTZ,
    window_end      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_connection_risk_events_status_last_seen
    ON connection_risk_events (status, last_seen_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_connection_risk_events_user_last_seen
    ON connection_risk_events (user_id, last_seen_at DESC)
    WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_connection_risk_events_key_last_seen
    ON connection_risk_events (api_key_id, last_seen_at DESC)
    WHERE api_key_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_connection_risk_events_severity_last_seen
    ON connection_risk_events (severity, last_seen_at DESC);

-- At most one open event per dedupe_key (empty key excluded).
CREATE UNIQUE INDEX IF NOT EXISTS idx_connection_risk_events_open_dedupe
    ON connection_risk_events (dedupe_key)
    WHERE status = 'open' AND dedupe_key <> '';
