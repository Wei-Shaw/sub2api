-- Cache one bounded custom icon per normalized upstream deployment root.
-- cache_key is SHA-256(root URL); the root URL itself is never persisted.
CREATE TABLE IF NOT EXISTS upstream_site_logos (
    cache_key CHAR(64) PRIMARY KEY,
    content_type VARCHAR(32),
    content BYTEA,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cache_key ~ '^[0-9a-f]{64}$'),
    CHECK (
        (content_type IS NULL AND content IS NULL)
        OR
        (
            content_type IN ('image/png', 'image/x-icon', 'image/webp')
            AND OCTET_LENGTH(content) BETWEEN 1 AND 65536
        )
    )
);
