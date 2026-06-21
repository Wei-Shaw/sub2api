ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS content_storage TEXT,
    ADD COLUMN IF NOT EXISTS content_path TEXT,
    ADD COLUMN IF NOT EXISTS content_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS content_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS content_stored_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS content_compression TEXT,
    ADD COLUMN IF NOT EXISTS processed_status TEXT,
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS processed_error TEXT;

UPDATE chat_messages
SET content_storage = content_json ->> 'storage',
    content_path = content_json ->> 'path',
    content_sha256 = content_json ->> 'sha256',
    content_bytes = CASE
        WHEN content_json ->> 'bytes' ~ '^[0-9]+$' THEN (content_json ->> 'bytes')::BIGINT
        ELSE content_bytes
    END,
    content_stored_bytes = CASE
        WHEN content_json ->> 'stored_bytes' ~ '^[0-9]+$' THEN (content_json ->> 'stored_bytes')::BIGINT
        ELSE content_stored_bytes
    END,
    content_compression = content_json ->> 'compression',
    processed_status = COALESCE(processed_status, 'pending')
WHERE content_json ->> 'storage' = 'file'
  AND content_path IS NULL;
