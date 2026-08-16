-- Concurrent index for 238's nullable matched_keyword column.
-- Existing tables cannot take a blocking CREATE INDEX during managed updates.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prompt_audit_events_matched_keyword
    ON prompt_audit_events(matched_keyword);
