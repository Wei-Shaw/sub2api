-- OIDC Provider 相关 6 张表 (任务 1.1-1.7)
-- forward-only; 端点默认 disabled，schema 落地不影响现有功能。

-- 1. oidc_clients：admin 注册的第三方 RP
CREATE TABLE IF NOT EXISTS oidc_clients (
    id                  BIGSERIAL PRIMARY KEY,
    client_id           VARCHAR(64)  NOT NULL,
    client_secret_hash  TEXT         NOT NULL,
    client_name         VARCHAR(100) NOT NULL,
    redirect_uris       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    allowed_scopes      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    grant_types         JSONB        NOT NULL DEFAULT '["authorization_code","refresh_token"]'::jsonb,
    consent_required    BOOLEAN      NOT NULL DEFAULT TRUE,
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS oidcclient_client_id_uq ON oidc_clients (client_id);
CREATE INDEX IF NOT EXISTS oidcclient_enabled_idx ON oidc_clients (enabled);

-- 2. oidc_authorization_codes：一次性短生命周期授权码
CREATE TABLE IF NOT EXISTS oidc_authorization_codes (
    id                     BIGSERIAL PRIMARY KEY,
    code                   VARCHAR(128) NOT NULL,
    client_id              VARCHAR(64)  NOT NULL,
    user_id                BIGINT       NOT NULL,
    redirect_uri           TEXT         NOT NULL,
    scopes                 JSONB        NOT NULL DEFAULT '[]'::jsonb,
    code_challenge         VARCHAR(128) NOT NULL,
    code_challenge_method  VARCHAR(10)  NOT NULL DEFAULT 'S256',
    nonce                  TEXT         NOT NULL DEFAULT '',
    expires_at             TIMESTAMPTZ  NOT NULL,
    consumed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS oidcauthcode_code_uq ON oidc_authorization_codes (code);
CREATE INDEX IF NOT EXISTS oidcauthcode_expires_at_idx ON oidc_authorization_codes (expires_at);
CREATE INDEX IF NOT EXISTS oidcauthcode_user_id_idx ON oidc_authorization_codes (user_id);
CREATE INDEX IF NOT EXISTS oidcauthcode_client_id_idx ON oidc_authorization_codes (client_id);

-- 3. oidc_refresh_tokens：opaque refresh token，family rotation
CREATE TABLE IF NOT EXISTS oidc_refresh_tokens (
    id                  BIGSERIAL PRIMARY KEY,
    token               VARCHAR(128) NOT NULL,
    family_id           VARCHAR(64)  NOT NULL,
    client_id           VARCHAR(64)  NOT NULL,
    user_id             BIGINT       NOT NULL,
    scopes              JSONB        NOT NULL DEFAULT '[]'::jsonb,
    expires_at          TIMESTAMPTZ  NOT NULL,
    revoked_at          TIMESTAMPTZ,
    parent_token_hash   TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS oidcrefreshtoken_token_uq ON oidc_refresh_tokens (token);
CREATE INDEX IF NOT EXISTS oidcrefreshtoken_family_id_idx ON oidc_refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS oidcrefreshtoken_user_id_idx ON oidc_refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS oidcrefreshtoken_client_id_idx ON oidc_refresh_tokens (client_id);
CREATE INDEX IF NOT EXISTS oidcrefreshtoken_expires_at_idx ON oidc_refresh_tokens (expires_at);

-- 4. oidc_access_tokens：opaque access token (Stage1B 决策 B1，独立表)
CREATE TABLE IF NOT EXISTS oidc_access_tokens (
    id                  BIGSERIAL PRIMARY KEY,
    token               VARCHAR(128) NOT NULL,
    client_id           VARCHAR(64)  NOT NULL,
    user_id             BIGINT       NOT NULL,
    scopes              JSONB        NOT NULL DEFAULT '[]'::jsonb,
    refresh_family_id   VARCHAR(64)  NOT NULL DEFAULT '',
    expires_at          TIMESTAMPTZ  NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS oidcaccesstoken_token_uq ON oidc_access_tokens (token);
CREATE INDEX IF NOT EXISTS oidcaccesstoken_user_id_idx ON oidc_access_tokens (user_id);
CREATE INDEX IF NOT EXISTS oidcaccesstoken_client_id_idx ON oidc_access_tokens (client_id);
CREATE INDEX IF NOT EXISTS oidcaccesstoken_expires_at_idx ON oidc_access_tokens (expires_at);
CREATE INDEX IF NOT EXISTS oidcaccesstoken_refresh_family_id_idx ON oidc_access_tokens (refresh_family_id);

-- 5. oidc_consents：(user_id, client_id) 维度的同意 scope 集合
CREATE TABLE IF NOT EXISTS oidc_consents (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT      NOT NULL,
    client_id       VARCHAR(64) NOT NULL,
    granted_scopes  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    granted_at      TIMESTAMPTZ NOT NULL,
    last_used_at    TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS oidcconsent_user_client_uq ON oidc_consents (user_id, client_id);
CREATE INDEX IF NOT EXISTS oidcconsent_client_id_idx ON oidc_consents (client_id);

-- 6. sso_sessions：HttpOnly SSO cookie 后端持久化
-- user_id 直接做 FK + ON DELETE CASCADE，与 ent edge `from User`  cascade 注解对齐
CREATE TABLE IF NOT EXISTS sso_sessions (
    id                BIGSERIAL PRIMARY KEY,
    session_id        VARCHAR(128) NOT NULL,
    user_id           BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at         TIMESTAMPTZ  NOT NULL,
    last_seen_at      TIMESTAMPTZ  NOT NULL,
    expires_at        TIMESTAMPTZ  NOT NULL,
    revoked_at        TIMESTAMPTZ,
    totp_verified_at  TIMESTAMPTZ,
    user_agent        TEXT         NOT NULL DEFAULT '',
    ip_address        TEXT         NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS ssosession_session_id_uq ON sso_sessions (session_id);
CREATE INDEX IF NOT EXISTS ssosession_user_id_idx ON sso_sessions (user_id);
CREATE INDEX IF NOT EXISTS ssosession_expires_at_idx ON sso_sessions (expires_at);
