-- Admin-managed absolute display overrides for per-user usage summaries.
-- NULL override values mean the calculated value from usage_logs should be used.

CREATE TABLE IF NOT EXISTS user_usage_overrides (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    today_requests      BIGINT CHECK (today_requests IS NULL OR today_requests >= 0),
    today_tokens        BIGINT CHECK (today_tokens IS NULL OR today_tokens >= 0),
    today_actual_cost   DECIMAL(20,10) CHECK (today_actual_cost IS NULL OR today_actual_cost >= 0),
    total_tokens        BIGINT CHECK (total_tokens IS NULL OR total_tokens >= 0),
    total_actual_cost   DECIMAL(20,10) CHECK (total_actual_cost IS NULL OR total_actual_cost >= 0),
    notes               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_usage_overrides_updated_at_idx
    ON user_usage_overrides (updated_at DESC);
