CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS dedupe_hash TEXT;

UPDATE chat_messages
SET dedupe_hash = encode(
    digest(
        length(trim(role))::text || ':' || trim(role) || '|' ||
        length(trim(direction))::text || ':' || trim(direction) || '|' ||
        length(trim(content_text))::text || ':' || trim(content_text) || '|' ||
        length(COALESCE(content_json::text, ''))::text || ':' || COALESCE(content_json::text, ''),
        'sha256'
    ),
    'hex'
)
WHERE dedupe_hash IS NULL;

DELETE FROM chat_messages cm
USING chat_messages dup
WHERE cm.session_id = dup.session_id
  AND cm.dedupe_hash = dup.dedupe_hash
  AND cm.dedupe_hash IS NOT NULL
  AND cm.id > dup.id;
