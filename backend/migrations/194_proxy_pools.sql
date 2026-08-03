-- Proxy Pools: 代理池（健康检测 + 周期自动重绑）
-- 池是一组代理的集合；池服务周期探测池内代理健康度，并将绑定到不健康代理的
-- 账号自动改投到池内健康代理（auto_rebind）。请求热路径不变，仅改数据库绑定。

CREATE TABLE IF NOT EXISTS proxy_pools (
    id                      BIGSERIAL PRIMARY KEY,
    name                    VARCHAR(100) NOT NULL UNIQUE,
    description             TEXT,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled
    health_interval_seconds INT NOT NULL DEFAULT 300,              -- 健康探测间隔（秒）
    failure_threshold       INT NOT NULL DEFAULT 2,                -- 连续失败多少次判为不健康
    auto_rebind             BOOLEAN NOT NULL DEFAULT TRUE,         -- 是否自动把账号从不健康代理改投到健康代理
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_proxy_pools_status ON proxy_pools(status);
CREATE INDEX IF NOT EXISTS idx_proxy_pools_deleted_at ON proxy_pools(deleted_at);

-- proxies: 归属池 + 池内健康状态
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_id BIGINT REFERENCES proxy_pools(id) ON DELETE SET NULL;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_health VARCHAR(20) NOT NULL DEFAULT 'unknown'; -- unknown/healthy/unhealthy
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_checked_at TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS pool_failures INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_proxies_pool_id ON proxies(pool_id);
CREATE INDEX IF NOT EXISTS idx_proxies_pool_health ON proxies(pool_health);

COMMENT ON COLUMN proxies.pool_id IS '所属代理池（NULL 表示独立代理）';
COMMENT ON COLUMN proxies.pool_health IS '池内健康状态：unknown/healthy/unhealthy';
COMMENT ON COLUMN proxies.pool_failures IS '池健康探测连续失败次数';

-- 池重绑变更日志（管理端展示 + 审计）
CREATE TABLE IF NOT EXISTS proxy_pool_rebind_logs (
    id            BIGSERIAL PRIMARY KEY,
    pool_id       BIGINT REFERENCES proxy_pools(id) ON DELETE CASCADE,
    from_proxy_id BIGINT,
    to_proxy_id   BIGINT,
    account_count INT NOT NULL DEFAULT 0,
    reason        VARCHAR(50) NOT NULL DEFAULT 'unhealthy',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proxy_pool_rebind_logs_pool_created ON proxy_pool_rebind_logs(pool_id, created_at DESC);

COMMENT ON TABLE proxy_pool_rebind_logs IS '代理池自动/手动重绑操作日志';

-- accounts: 可选代理池绑定。绑定池后由代理池服务维护池内健康代理分配
-- （写入 accounts.proxy_id）；绑定具体代理则 pool_id 为 NULL。池删除时 ON DELETE SET NULL。
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS pool_id BIGINT REFERENCES proxy_pools(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_pool_id ON accounts(pool_id);
