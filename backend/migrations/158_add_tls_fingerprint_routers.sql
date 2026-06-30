-- 创建 TLS 指纹路由器，用于按入站 User-Agent 选择 TLS 指纹模板。
-- 合并自 TokenRouter 的 3 个迁移：151(建表)+ 153(chatgpt_oauth_token 两列)+ 161(codex_invite_reset 两列)。
-- 全部 IF NOT EXISTS，对现有库零影响(全新表)；列类型与 ent schema 生成结果一致。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS tls_fingerprint_routers (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    enabled     BOOLEAN      NOT NULL DEFAULT true,
    rules       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tls_fingerprint_routers_enabled ON tls_fingerprint_routers (enabled);

-- ChatGPT OAuth token exchange/refresh 请求专用 UA 与 TLS 模板配置(来自 153)。
ALTER TABLE tls_fingerprint_routers
    ADD COLUMN IF NOT EXISTS chatgpt_oauth_token_user_agent VARCHAR(512) NOT NULL DEFAULT '';

ALTER TABLE tls_fingerprint_routers
    ADD COLUMN IF NOT EXISTS chatgpt_oauth_token_tls_fingerprint_profile_id BIGINT;

-- Codex 邀请重置等 Codex Desktop 后台请求专用 UA 与 TLS 模板配置(来自 161)。
ALTER TABLE tls_fingerprint_routers
    ADD COLUMN IF NOT EXISTS codex_invite_reset_user_agent VARCHAR(512) NOT NULL DEFAULT '';

ALTER TABLE tls_fingerprint_routers
    ADD COLUMN IF NOT EXISTS codex_invite_reset_tls_fingerprint_profile_id BIGINT;

COMMENT ON TABLE tls_fingerprint_routers IS 'TLS 指纹路由器，用于按入站 User-Agent 选择 TLS 指纹模板';
COMMENT ON COLUMN tls_fingerprint_routers.rules IS '有序 User-Agent 匹配规则，第一条命中规则决定使用的 TLS 指纹模板';
COMMENT ON COLUMN tls_fingerprint_routers.chatgpt_oauth_token_user_agent IS
    'ChatGPT OAuth token exchange/refresh 请求使用的 User-Agent，空值使用系统兜底';
COMMENT ON COLUMN tls_fingerprint_routers.chatgpt_oauth_token_tls_fingerprint_profile_id IS
    'ChatGPT OAuth token exchange/refresh 请求使用的 TLS 模板：NULL=不启用，0=内置默认，-1=随机，正数=指定模板';
COMMENT ON COLUMN tls_fingerprint_routers.codex_invite_reset_user_agent IS
    'Codex 邀请重置等 Codex Desktop 后台请求使用的 User-Agent，空值使用系统兜底';
COMMENT ON COLUMN tls_fingerprint_routers.codex_invite_reset_tls_fingerprint_profile_id IS
    'Codex 邀请重置请求使用的 TLS 模板：NULL=沿用账号配置，0=内置默认，-1=随机，正数=指定模板';
