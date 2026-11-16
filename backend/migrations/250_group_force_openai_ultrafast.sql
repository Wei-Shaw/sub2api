ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS force_openai_ultrafast BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.force_openai_ultrafast IS
    'Force OpenAI gateway requests in this group to use service_tier=ultrafast before global Fast/Flex policy evaluation';
