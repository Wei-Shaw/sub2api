-- Add a per-group Kiro sticky session binding TTL in seconds.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_sticky_session_ttl_seconds INT;

-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET kiro_sticky_session_ttl_seconds = 3600
WHERE kiro_sticky_session_ttl_seconds IS NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_sticky_session_ttl_seconds SET DEFAULT 3600;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_sticky_session_ttl_seconds SET NOT NULL;
