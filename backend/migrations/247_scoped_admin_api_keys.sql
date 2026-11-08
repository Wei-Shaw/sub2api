-- 管理密钥独立存储，仅保存不可逆摘要，保留撤销记录用于审计。
CREATE TABLE IF NOT EXISTS scoped_admin_api_keys (
 id VARCHAR(24) PRIMARY KEY,
 name VARCHAR(100) NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 key_prefix VARCHAR(64) NOT NULL,
 key_hash VARCHAR(64) NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 expires_at TIMESTAMPTZ,
 allowed_ips JSONB NOT NULL DEFAULT '[]',
 policy JSONB NOT NULL DEFAULT '{}',
 created_by BIGINT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 last_used_at TIMESTAMPTZ,
 last_used_ip VARCHAR(64) NOT NULL DEFAULT '',
 revoked_at TIMESTAMPTZ
);

-- 仅将已撤销管理密钥从列表移除，保留原记录与历史审计。
ALTER TABLE scoped_admin_api_keys
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
