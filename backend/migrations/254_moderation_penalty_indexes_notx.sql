-- Build on the live log outside the transaction that adds user and key columns.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_moderation_event_id ON content_moderation_logs(event_id) WHERE event_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_moderation_key_hits ON content_moderation_logs(user_id, api_key_id, occurred_at)
    WHERE flagged AND penalty_scope = 'api_key';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_moderation_user_hits ON content_moderation_logs(user_id, COALESCE(occurred_at, created_at))
    WHERE flagged AND (penalty_scope = 'user' OR event_id IS NULL);
