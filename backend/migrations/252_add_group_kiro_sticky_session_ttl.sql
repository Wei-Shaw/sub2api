-- Add a per-group Kiro sticky session binding TTL in seconds.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_sticky_session_ttl_seconds INT NOT NULL DEFAULT 3600;
