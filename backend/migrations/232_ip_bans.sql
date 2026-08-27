CREATE TABLE IF NOT EXISTS ip_bans (
    id BIGSERIAL PRIMARY KEY,
    ip_address VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ip_bans_created_at ON ip_bans (created_at DESC, id DESC);
