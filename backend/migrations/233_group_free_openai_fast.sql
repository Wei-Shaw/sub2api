ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS free_openai_fast BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.free_openai_fast IS
    'Whether Fast/priority requests in this OpenAI/Composite group are billed to users at Standard price';
