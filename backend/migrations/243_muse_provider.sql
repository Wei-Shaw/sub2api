-- Native provider registration. Session accounts still require verification.
ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK(platform IN ('anthropic','openai','gemini','antigravity','grok','kimi','zhipu','deepseek','minimax','opencode_go','typesafe','muse'));
ALTER TABLE composite_model_routes DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
ALTER TABLE composite_model_routes ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK(target_platform IN ('anthropic','openai','gemini','antigravity','grok','kimi','zhipu','deepseek','minimax','opencode_go','typesafe','muse'));

CREATE TABLE IF NOT EXISTS muse_account_profiles (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE RESTRICT,
    verified_account_updated_at TIMESTAMPTZ NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '{}',
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS requested_model VARCHAR(100);
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS observed_model VARCHAR(200);
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS subscription_id BIGINT REFERENCES user_subscriptions(id) ON DELETE RESTRICT;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS billing_command JSONB;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS usage_log_id BIGINT REFERENCES usage_logs(id) ON DELETE RESTRICT;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS settled_at TIMESTAMPTZ;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS reported_usage JSONB;
ALTER TABLE muse_account_profiles ADD COLUMN IF NOT EXISTS renewal_not_before TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_muse_turns_pending_settlement ON muse_turns(updated_at) WHERE billing_command IS NOT NULL AND settled_at IS NULL;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS settlement_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS settlement_not_before TIMESTAMPTZ;
