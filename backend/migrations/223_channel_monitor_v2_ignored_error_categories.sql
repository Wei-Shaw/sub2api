-- Categories excluded from error_rate / health scoring (still shown in error breakdown).
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN IF NOT EXISTS ignored_error_categories TEXT[];

-- The singleton config row already exists. Keeping the schema expansion
-- nullable avoids blocking old images during blue-green deployment.
-- sub2api-managed-update: reviewed-compatible
UPDATE channel_monitor_v2_config
SET ignored_error_categories = '{}'
WHERE ignored_error_categories IS NULL;
