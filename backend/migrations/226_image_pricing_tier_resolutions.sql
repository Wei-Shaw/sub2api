ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_resolution_1k VARCHAR(32) NOT NULL DEFAULT '1024x1024',
    ADD COLUMN IF NOT EXISTS image_resolution_2k VARCHAR(32) NOT NULL DEFAULT '2048x2048',
    ADD COLUMN IF NOT EXISTS image_resolution_4k VARCHAR(32) NOT NULL DEFAULT '4096x4096';

ALTER TABLE channel_pricing_intervals
    ADD COLUMN IF NOT EXISTS resolution VARCHAR(32);

ALTER TABLE channel_account_stats_pricing_intervals
    ADD COLUMN IF NOT EXISTS resolution VARCHAR(32),
    ADD COLUMN IF NOT EXISTS quality VARCHAR(16);

UPDATE channel_pricing_intervals
SET resolution = CASE UPPER(tier_label)
    WHEN '1K' THEN '1024x1024'
    WHEN '2K' THEN '2048x2048'
    WHEN '4K' THEN '4096x4096'
    ELSE resolution
END
WHERE COALESCE(resolution, '') = '';

UPDATE channel_account_stats_pricing_intervals
SET resolution = CASE UPPER(tier_label)
    WHEN '1K' THEN '1024x1024'
    WHEN '2K' THEN '2048x2048'
    WHEN '4K' THEN '4096x4096'
    ELSE resolution
END
WHERE COALESCE(resolution, '') = '';

-- Collapse the previous six resolution rows to the new fixed 1K/2K/4K keys.
-- Prefer already-migrated keys so the migration remains idempotent.
UPDATE groups
SET image_pricing_matrix = jsonb_strip_nulls(jsonb_build_object(
    '1K', COALESCE(image_pricing_matrix -> '1K', image_pricing_matrix -> '1024x1024', image_pricing_matrix -> '1024x768', image_pricing_matrix -> '1024x1536'),
    '2K', COALESCE(image_pricing_matrix -> '2K', image_pricing_matrix -> '2560x1440', image_pricing_matrix -> '1920x1080'),
    '4K', COALESCE(image_pricing_matrix -> '4K', image_pricing_matrix -> '3840x2160')
))
WHERE image_pricing_matrix IS NOT NULL
  AND image_pricing_matrix <> '{}'::jsonb;
