-- proxy_groups: 代理池分组
CREATE TABLE IF NOT EXISTS proxy_groups (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    strategy VARCHAR(20) NOT NULL DEFAULT 'round_robin',
    sticky_by_account BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS proxy_groups_status_idx ON proxy_groups (status);
CREATE INDEX IF NOT EXISTS proxy_groups_deleted_at_idx ON proxy_groups (deleted_at);

-- 软删除后允许重用同名：仅对未删除行强制 name 唯一
CREATE UNIQUE INDEX IF NOT EXISTS proxy_groups_name_alive_uidx
    ON proxy_groups (name)
    WHERE deleted_at IS NULL;

-- proxies: 归属组（一对多，一个代理最多属于一个组）
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES proxy_groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS proxies_group_id_idx ON proxies (group_id);

-- accounts: 绑定组（与 proxy_id 共存；proxy_id 优先）
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_group_id BIGINT REFERENCES proxy_groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS accounts_proxy_group_id_idx ON accounts (proxy_group_id);
