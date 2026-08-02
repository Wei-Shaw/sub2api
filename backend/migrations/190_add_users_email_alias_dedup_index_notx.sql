-- Registration alias dedup (repository.existsByEmailAliasWithClient) probes users
-- by the dot-stripped email form, on the public register / send-verify-code paths.
-- Index that exact expression so the probes stay index lookups instead of a
-- sequential scan. text_pattern_ops serves both the equality probe and the
-- "local+%@domain" prefix probe regardless of database collation.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email_dot_stripped
    ON users ((REPLACE(LOWER(TRIM(email)), '.', '')) text_pattern_ops)
    WHERE deleted_at IS NULL;
