-- Persist only detected changes in an OpenAI API-key account's upstream
-- billing-rate declaration. Credentials, Base URLs and raw responses are never
-- copied into this table.
CREATE TABLE IF NOT EXISTS account_upstream_billing_rate_events (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    detected_at TIMESTAMPTZ NOT NULL,
    group_rate_multiplier DOUBLE PRECISION NOT NULL,
    user_rate_multiplier DOUBLE PRECISION,
    peak_rate_enabled BOOLEAN NOT NULL,
    peak_start VARCHAR(5),
    peak_end VARCHAR(5),
    peak_timezone VARCHAR(100),
    peak_rate_multiplier DOUBLE PRECISION,
    resolved_rate_multiplier DOUBLE PRECISION NOT NULL,
    effective_rate_multiplier DOUBLE PRECISION NOT NULL,
    UNIQUE (account_id, detected_at),
    CHECK (group_rate_multiplier >= 0 AND group_rate_multiplier < 'Infinity'::DOUBLE PRECISION),
    CHECK (user_rate_multiplier IS NULL OR
        (user_rate_multiplier >= 0 AND user_rate_multiplier < 'Infinity'::DOUBLE PRECISION)),
    CHECK (resolved_rate_multiplier >= 0 AND resolved_rate_multiplier < 'Infinity'::DOUBLE PRECISION),
    CHECK (effective_rate_multiplier >= 0 AND effective_rate_multiplier < 'Infinity'::DOUBLE PRECISION),
    CHECK (peak_rate_multiplier IS NULL OR
        (peak_rate_multiplier >= 0 AND peak_rate_multiplier < 'Infinity'::DOUBLE PRECISION)),
    CHECK (
        (peak_rate_enabled
            AND peak_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            AND peak_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            AND peak_start < peak_end
            AND peak_timezone IS NOT NULL
            AND peak_timezone <> ''
            AND peak_rate_multiplier IS NOT NULL)
        OR
        (NOT peak_rate_enabled
            AND peak_start IS NULL
            AND peak_end IS NULL
            AND peak_timezone IS NULL
            AND peak_rate_multiplier IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_account_upstream_rate_events_account_time
    ON account_upstream_billing_rate_events (account_id, detected_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_account_upstream_rate_events_retention
    ON account_upstream_billing_rate_events (detected_at, id);
