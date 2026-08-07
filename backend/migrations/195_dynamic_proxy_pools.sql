-- Add dynamic_proxy_pools table for dynamic IP extraction pools
CREATE TABLE IF NOT EXISTS dynamic_proxy_pools (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(100) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source_type VARCHAR(20) NOT NULL DEFAULT 'extract_api',
    subscription_id BIGINT REFERENCES proxy_subscriptions(id) ON DELETE SET NULL,
    extract_url TEXT NOT NULL DEFAULT '',
    protocol VARCHAR(20) NOT NULL DEFAULT 'http',
    auth_mode VARCHAR(20) NOT NULL DEFAULT 'none',
    username VARCHAR(200) NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    response_format VARCHAR(20) NOT NULL DEFAULT 'txt',
    line_separator VARCHAR(20) NOT NULL DEFAULT '\r\n',
    ip_field_path VARCHAR(200) NOT NULL DEFAULT '',
    port_field_path VARCHAR(200) NOT NULL DEFAULT '',
    refresh_interval_sec INT NOT NULL DEFAULT 300,
    ip_duration_sec INT NOT NULL DEFAULT 300,
    extract_count INT NOT NULL DEFAULT 1,
    min_alive INT NOT NULL DEFAULT 1,
    name_prefix VARCHAR(40) NOT NULL DEFAULT 'dpool-',
    last_extract_at TIMESTAMPTZ,
    last_extract_status VARCHAR(40) NOT NULL DEFAULT '',
    last_extract_error TEXT NOT NULL DEFAULT '',
    alive_count INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_dynamic_proxy_pools_enabled ON dynamic_proxy_pools(enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dynamic_proxy_pools_name_prefix ON dynamic_proxy_pools(name_prefix);