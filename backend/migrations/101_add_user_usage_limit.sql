BEGIN;

ALTER TABLE users
    ADD COLUMN usage_limit_enabled BOOLEAN,
    ADD COLUMN daily_usage_limit_usd NUMERIC(20, 8);

CREATE TABLE user_usage_limit_rules (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_ids       JSONB NOT NULL DEFAULT '[]'::jsonb,
    daily_limit_usd NUMERIC(20, 8) NOT NULL,
    period          VARCHAR(16) NOT NULL DEFAULT 'daily',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_usage_limit_rules_user ON user_usage_limit_rules(user_id);

COMMIT;
