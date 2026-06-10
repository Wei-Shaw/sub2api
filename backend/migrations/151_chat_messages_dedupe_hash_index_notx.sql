CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_session_dedupe_hash
    ON chat_messages (session_id, dedupe_hash)
    WHERE dedupe_hash IS NOT NULL;
