ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS quota_sticky_default_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS quota_sticky_user_override_allowed boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS session_model_stability_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS unified_retry_budget_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS quota_sticky_mode varchar(16) NOT NULL DEFAULT 'inherit';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_quota_sticky_mode_check'
    ) THEN
        ALTER TABLE api_keys ADD CONSTRAINT api_keys_quota_sticky_mode_check
            CHECK (quota_sticky_mode IN ('inherit', 'enabled', 'disabled'));
    END IF;
END $$;
