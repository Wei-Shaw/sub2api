-- Proxy pools provide persistent account-to-pool binding while keeping the
-- existing accounts.proxy_id hot path unchanged.

CREATE TABLE IF NOT EXISTS proxy_pools (
    id                      BIGSERIAL PRIMARY KEY,
    name                    VARCHAR(100) NOT NULL,
    description             TEXT,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active',
    health_interval_seconds INT NOT NULL DEFAULT 300,
    failure_threshold       INT NOT NULL DEFAULT 2,
    auto_rebind             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_pools_active_name
    ON proxy_pools (LOWER(name)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_proxy_pools_status ON proxy_pools(status);

ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_id BIGINT REFERENCES proxy_pools(id) ON DELETE SET NULL;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_health VARCHAR(20) NOT NULL DEFAULT 'unknown';
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_checked_at TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_failures INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_proxies_pool_id ON proxies(pool_id);
CREATE INDEX IF NOT EXISTS idx_proxies_pool_health ON proxies(pool_health);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS pool_id BIGINT REFERENCES proxy_pools(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_pool_id ON accounts(pool_id);

CREATE TABLE IF NOT EXISTS proxy_pool_rebind_logs (
    id            BIGSERIAL PRIMARY KEY,
    pool_id       BIGINT REFERENCES proxy_pools(id) ON DELETE CASCADE,
    from_proxy_id BIGINT,
    to_proxy_id   BIGINT,
    account_count INT NOT NULL DEFAULT 0,
    reason        VARCHAR(50) NOT NULL DEFAULT 'unhealthy',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proxy_pool_rebind_logs_pool_created
    ON proxy_pool_rebind_logs(pool_id, created_at DESC);
