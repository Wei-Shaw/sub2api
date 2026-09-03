ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS disable_openai_fast BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.disable_openai_fast IS
    'Strip service_tier from OpenAI gateway requests in this group so Fast/Flex is never used; takes precedence over force_openai_fast';
