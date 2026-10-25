-- MySQL 方言版本（翻译自 ../135_allow_email_oauth_provider_types.sql）
-- 翻译说明：
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK（MySQL 8.0.16+
--     命名 CHECK；IF EXISTS 按迁移执行器单次执行语义去掉）。
--   - ADD CONSTRAINT ... CHECK 语法同名保留（MySQL 8.0.16+ 强制执行并校验存量行）。
--   - 依赖前置：四个 CHECK 约束由 ../108_auth_identity_foundation_core.sql（及 110）引入，
--     其 MySQL 翻译须以同名 ADD CONSTRAINT 建立，本文件才能 DROP CHECK 成功。

ALTER TABLE users
    DROP CHECK users_signup_source_check;

ALTER TABLE users
    ADD CONSTRAINT users_signup_source_check
    CHECK (signup_source IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google'));

ALTER TABLE auth_identities
    DROP CHECK auth_identities_provider_type_check;

ALTER TABLE auth_identities
    ADD CONSTRAINT auth_identities_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google'));

ALTER TABLE auth_identity_channels
    DROP CHECK auth_identity_channels_provider_type_check;

ALTER TABLE auth_identity_channels
    ADD CONSTRAINT auth_identity_channels_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google'));

ALTER TABLE pending_auth_sessions
    DROP CHECK pending_auth_sessions_provider_type_check;

ALTER TABLE pending_auth_sessions
    ADD CONSTRAINT pending_auth_sessions_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google'));
