DROP INDEX IF EXISTS uq_prompt_records_dedupe;

ALTER TABLE prompt_records
    ADD COLUMN IF NOT EXISTS session_id VARCHAR(128) NOT NULL DEFAULT '';

UPDATE prompt_records
SET session_id = LEFT(COALESCE(
    NULLIF(request_headers, '')::jsonb #>> '{Session-Id,0}',
    NULLIF(request_headers, '')::jsonb #>> '{X-Claude-Code-Session-Id,0}',
    ''
), 128)
WHERE session_id = '';

ALTER TABLE prompt_records
    DROP COLUMN IF EXISTS request_id;

CREATE INDEX IF NOT EXISTS idx_prompt_records_session_created
    ON prompt_records (session_id, created_at DESC, id DESC);
