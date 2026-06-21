CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_content_path
    ON chat_messages (content_path)
    WHERE content_path IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_content_sha256
    ON chat_messages (content_sha256)
    WHERE content_sha256 IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_processed_status
    ON chat_messages (processed_status, created_at)
    WHERE processed_status IS NOT NULL;
