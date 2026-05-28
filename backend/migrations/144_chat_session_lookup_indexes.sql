-- Improve API-key scoped chat session browsing.

CREATE INDEX IF NOT EXISTS idx_chat_sessions_user_api_key_time
    ON chat_sessions (user_id, api_key_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session_created
    ON chat_messages (session_id, created_at DESC, id DESC);
