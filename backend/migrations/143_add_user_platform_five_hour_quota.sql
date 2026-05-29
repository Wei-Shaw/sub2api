-- Add aligned 5-hour quota fields to user_platform_quotas.
ALTER TABLE user_platform_quotas ADD COLUMN IF NOT EXISTS five_hour_limit_usd decimal(20,10);
ALTER TABLE user_platform_quotas ADD COLUMN IF NOT EXISTS five_hour_usage_usd decimal(20,10) NOT NULL DEFAULT 0;
ALTER TABLE user_platform_quotas ADD COLUMN IF NOT EXISTS five_hour_window_start timestamptz;
ALTER TABLE user_platform_quotas ADD COLUMN IF NOT EXISTS five_hour_align_minutes integer NOT NULL DEFAULT 0;

ALTER TABLE user_platform_quotas
	ADD CONSTRAINT user_platform_quotas_five_hour_align_minutes_check
	CHECK (five_hour_align_minutes >= 0 AND five_hour_align_minutes < 1440);
