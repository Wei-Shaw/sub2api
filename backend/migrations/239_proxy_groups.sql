-- Proxy groups allow an account to select one active endpoint per request.
CREATE TABLE IF NOT EXISTS proxy_groups (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT proxy_groups_status_check CHECK (status IN ('active', 'inactive'))
);

CREATE INDEX IF NOT EXISTS proxy_groups_status_idx ON proxy_groups(status);
CREATE INDEX IF NOT EXISTS proxy_groups_deleted_at_idx ON proxy_groups(deleted_at);

CREATE TABLE IF NOT EXISTS proxy_group_proxies (
    proxy_group_id BIGINT NOT NULL REFERENCES proxy_groups(id) ON DELETE CASCADE,
    proxy_id BIGINT NOT NULL REFERENCES proxies(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (proxy_group_id, proxy_id)
);

CREATE INDEX IF NOT EXISTS proxy_group_proxies_proxy_id_idx ON proxy_group_proxies(proxy_id);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_group_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'accounts_proxy_group_id_fkey'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_proxy_group_id_fkey
            FOREIGN KEY (proxy_group_id) REFERENCES proxy_groups(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'accounts_proxy_binding_exclusive'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_proxy_binding_exclusive
            CHECK (proxy_id IS NULL OR proxy_group_id IS NULL);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS accounts_proxy_group_id_idx ON accounts(proxy_group_id);
