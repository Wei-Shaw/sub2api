-- MySQL 方言版本（翻译自 ../149_proxy_expiry_fallback.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS / CREATE INDEX IF NOT EXISTS → 去掉（按迁移执行器单次执行语义）。
--   - timestamptz → DATETIME(3) NULL。
--   - 内联 REFERENCES（backup_proxy_id → proxies.id）→ 显式命名表级外键
--     （MySQL 会忽略列内联 REFERENCES）；ADD CONSTRAINT 移到显式 CREATE INDEX 之后，
--     InnoDB 复用该索引，避免自动建重复索引。

-- proxies: 有效期 + 失败回退
ALTER TABLE proxies ADD COLUMN expires_at DATETIME(3) NULL;
ALTER TABLE proxies ADD COLUMN fallback_mode VARCHAR(20) NOT NULL DEFAULT 'none';
ALTER TABLE proxies ADD COLUMN backup_proxy_id BIGINT NULL;
ALTER TABLE proxies ADD COLUMN expiry_warn_days INT NOT NULL DEFAULT 7;
CREATE INDEX proxies_expires_at_idx ON proxies (expires_at);
CREATE INDEX proxies_backup_proxy_id_idx ON proxies (backup_proxy_id);
ALTER TABLE proxies
    ADD CONSTRAINT fk_proxies_backup_proxy_id
    FOREIGN KEY (backup_proxy_id) REFERENCES proxies(id) ON DELETE SET NULL;

-- accounts: fallback 来源（手动回切用）
ALTER TABLE accounts ADD COLUMN proxy_fallback_origin_id BIGINT NULL;
CREATE INDEX accounts_proxy_fallback_origin_id_idx ON accounts (proxy_fallback_origin_id);
