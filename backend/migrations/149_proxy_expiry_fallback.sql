-- proxies: 有效期 + 失败回退
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS expires_at timestamptz;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS fallback_mode varchar(20) NOT NULL DEFAULT 'none';
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS backup_proxy_id BIGINT REFERENCES proxies(id) ON DELETE SET NULL;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS expiry_warn_days INT NOT NULL DEFAULT 7;
CREATE INDEX IF NOT EXISTS proxies_expires_at_idx ON proxies (expires_at);
CREATE INDEX IF NOT EXISTS proxies_backup_proxy_id_idx ON proxies (backup_proxy_id);

-- accounts: fallback 来源（手动回切用）
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_fallback_origin_id BIGINT;
CREATE INDEX IF NOT EXISTS accounts_proxy_fallback_origin_id_idx ON accounts (proxy_fallback_origin_id);
