ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS quota_daily_reset_mode varchar(10),
    ADD COLUMN IF NOT EXISTS quota_daily_reset_hour integer,
    ADD COLUMN IF NOT EXISTS quota_weekly_reset_mode varchar(10),
    ADD COLUMN IF NOT EXISTS quota_weekly_reset_day integer,
    ADD COLUMN IF NOT EXISTS quota_weekly_reset_hour integer,
    ADD COLUMN IF NOT EXISTS quota_monthly_reset_mode varchar(10),
    ADD COLUMN IF NOT EXISTS quota_monthly_reset_day integer,
    ADD COLUMN IF NOT EXISTS quota_monthly_reset_hour integer,
    ADD COLUMN IF NOT EXISTS quota_reset_timezone varchar(64);
