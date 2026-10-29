-- Persist the explicit client-provided request session/conversation identifier
-- (e.g. the session_id / X-Session-Id / X-Conversation-ID request headers) so
-- usage rows can be correlated by session for querying.
--
-- Nullable with no default: on PostgreSQL 11+ this is a metadata-only change, so
-- it does NOT rewrite the (potentially large) usage_logs table. Absent/invalid
-- session identifiers stay NULL. Only explicit client correlation identity is
-- stored here; prompt_cache_key and content-derived sticky hashes are never
-- persisted as session_id.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS session_id VARCHAR(255);

-- Batch image usage is recorded asynchronously after the originating HTTP
-- request has finished, so retain the same sanitized identifier on the job.
ALTER TABLE batch_image_jobs ADD COLUMN IF NOT EXISTS session_id VARCHAR(255);
