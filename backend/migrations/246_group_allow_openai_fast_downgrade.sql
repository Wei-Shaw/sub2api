ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_openai_fast_downgrade BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.allow_openai_fast_downgrade IS
    'Allow composite OpenAI requests to remove service_tier when the selected account does not support Fast; Force Fast remains strict';
