-- 余额 RPC：接入方（扣费 app）注册表。
-- 鉴权采用无状态 token（app 的 secret = AES-256-GCM(本地密钥, payload{app_id})），
-- DB 不存任何密文/hash；本表仅做接入方注册（app_id / 名称 / 启停）与审计。
-- app_id 为对外业务主键（如 "bapp_<base32>"）。
CREATE TABLE IF NOT EXISTS billing_apps (
    id            BIGSERIAL PRIMARY KEY,
    app_id        VARCHAR(64)  NOT NULL,
    app_name      VARCHAR(100) NOT NULL,
    enabled       BOOLEAN      NOT NULL DEFAULT TRUE,
    token_version INTEGER      NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS billing_apps_app_id_key ON billing_apps (app_id);
CREATE INDEX IF NOT EXISTS billing_apps_enabled_idx ON billing_apps (enabled);
