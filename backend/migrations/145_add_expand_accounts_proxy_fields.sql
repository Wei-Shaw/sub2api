ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS proxy_id BIGINT REFERENCES proxies(id) ON DELETE SET NULL;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS proxy_info JSONB;

CREATE INDEX IF NOT EXISTS idx_expand_accounts_proxy_id ON expand_accounts(proxy_id);
