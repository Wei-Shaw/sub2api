SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS subscription_reset_audits (
    id                          BIGSERIAL PRIMARY KEY,
    subscription_id             BIGINT NOT NULL,
    user_id                     BIGINT NOT NULL,
    group_id                    BIGINT NOT NULL,
    operator_id                 BIGINT NOT NULL,
    operator_type               VARCHAR(20) NOT NULL DEFAULT 'user',
    deducted_seconds            INTEGER NOT NULL DEFAULT 86400,
    before_expires_at           TIMESTAMPTZ NOT NULL,
    after_expires_at            TIMESTAMPTZ NOT NULL,
    before_daily_usage_usd      DECIMAL(20,10) NOT NULL DEFAULT 0,
    after_daily_usage_usd       DECIMAL(20,10) NOT NULL DEFAULT 0,
    before_daily_window_start   TIMESTAMPTZ,
    after_daily_window_start    TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS subscriptionresetaudit_subscription_id ON subscription_reset_audits (subscription_id);
CREATE INDEX IF NOT EXISTS subscriptionresetaudit_user_id ON subscription_reset_audits (user_id);
CREATE INDEX IF NOT EXISTS subscriptionresetaudit_group_id ON subscription_reset_audits (group_id);
CREATE INDEX IF NOT EXISTS subscriptionresetaudit_operator_id ON subscription_reset_audits (operator_id);
CREATE INDEX IF NOT EXISTS subscriptionresetaudit_created_at ON subscription_reset_audits (created_at DESC);
