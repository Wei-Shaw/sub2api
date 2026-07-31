-- Keep the expansion nullable and without a database default so older binaries
-- remain compatible during managed blue-green deployment and image rollback.
-- The new application maps NULL to its versioned default policy and writes the
-- complete object whenever a channel is created or updated.
ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS service_tier_config JSONB;

-- NOT VALID avoids an installation-time table scan. NULL preserves legacy rows;
-- any non-NULL value written by the new application must be a JSON object.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE channels
    ADD CONSTRAINT channels_service_tier_config_object
    CHECK (service_tier_config IS NULL OR jsonb_typeof(service_tier_config) = 'object') NOT VALID;
