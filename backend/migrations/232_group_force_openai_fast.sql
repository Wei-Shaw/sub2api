ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS force_openai_fast BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.force_openai_fast IS
    'Force OpenAI gateway requests in this group to use service_tier=priority before global Fast/Flex policy evaluation';
