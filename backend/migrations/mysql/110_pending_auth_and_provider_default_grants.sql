-- MySQL 方言版本（翻译自 ../110_pending_auth_and_provider_default_grants.sql）
--
-- 翻译说明：
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；TIMESTAMPTZ/NOW()
--     → DATETIME(3)/CURRENT_TIMESTAMP(3)。
--   - 内联 REFERENCES → 显式命名的 CONSTRAINT fk_xxx FOREIGN KEY。
--   - CHECK 约束同名保留（与 PG 一致；后续 135 说明中提到的 110 引入的约束即此两处）。
--   - ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id（settings 有 id 列）。

CREATE TABLE IF NOT EXISTS user_provider_default_grants (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    provider_type VARCHAR(20) NOT NULL,
    grant_reason VARCHAR(20) NOT NULL DEFAULT 'first_bind',
    granted_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT user_provider_default_grants_provider_type_check
        CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc')),
    CONSTRAINT user_provider_default_grants_reason_check
        CHECK (grant_reason IN ('signup', 'first_bind')),
    CONSTRAINT fk_user_provider_default_grants_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX user_provider_default_grants_user_provider_reason_key
    ON user_provider_default_grants (user_id, provider_type, grant_reason);

CREATE INDEX user_provider_default_grants_user_id_idx
    ON user_provider_default_grants (user_id);

CREATE TABLE IF NOT EXISTS user_avatars (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    storage_provider VARCHAR(20) NOT NULL DEFAULT 'database',
    storage_key TEXT NOT NULL DEFAULT (''),
    url TEXT NOT NULL DEFAULT (''),
    content_type VARCHAR(100) NOT NULL DEFAULT '',
    byte_size INT NOT NULL DEFAULT 0,
    sha256 VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_user_avatars_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX user_avatars_user_id_key
    ON user_avatars (user_id);

INSERT INTO settings (`key`, value)
VALUES
    ('auth_source_default_email_balance', '0'),
    ('auth_source_default_email_concurrency', '5'),
    ('auth_source_default_email_subscriptions', '[]'),
    ('auth_source_default_email_grant_on_signup', 'false'),
    ('auth_source_default_email_grant_on_first_bind', 'false'),
    ('auth_source_default_linuxdo_balance', '0'),
    ('auth_source_default_linuxdo_concurrency', '5'),
    ('auth_source_default_linuxdo_subscriptions', '[]'),
    ('auth_source_default_linuxdo_grant_on_signup', 'false'),
    ('auth_source_default_linuxdo_grant_on_first_bind', 'false'),
    ('auth_source_default_oidc_balance', '0'),
    ('auth_source_default_oidc_concurrency', '5'),
    ('auth_source_default_oidc_subscriptions', '[]'),
    ('auth_source_default_oidc_grant_on_signup', 'false'),
    ('auth_source_default_oidc_grant_on_first_bind', 'false'),
    ('auth_source_default_wechat_balance', '0'),
    ('auth_source_default_wechat_concurrency', '5'),
    ('auth_source_default_wechat_subscriptions', '[]'),
    ('auth_source_default_wechat_grant_on_signup', 'false'),
    ('auth_source_default_wechat_grant_on_first_bind', 'false'),
    ('force_email_on_third_party_signup', 'false')
ON DUPLICATE KEY UPDATE id = id;
