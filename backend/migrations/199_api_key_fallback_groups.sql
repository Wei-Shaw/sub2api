ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS fallback_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN api_keys.fallback_group_ids IS
    'Ordered fallback group IDs for personal API keys';
