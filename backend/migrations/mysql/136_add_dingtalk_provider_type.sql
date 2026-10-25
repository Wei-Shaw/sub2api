-- MySQL 方言版本（翻译自 ../136_add_dingtalk_provider_type.sql）
-- 翻译说明：
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK（MySQL 8.0.16+
--     命名 CHECK；IF EXISTS 按迁移执行器单次执行语义去掉）。
--   - 四个 CHECK 约束由 108/110 迁移引入、135_allow_email_oauth_provider_types 以同名重建
--     （不含 dingtalk），本文件以同名 ADD CONSTRAINT 替换为包含 dingtalk 的版本。

ALTER TABLE users
    DROP CHECK users_signup_source_check;

ALTER TABLE users
    ADD CONSTRAINT users_signup_source_check
    CHECK (signup_source IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'));

ALTER TABLE auth_identities
    DROP CHECK auth_identities_provider_type_check;

ALTER TABLE auth_identities
    ADD CONSTRAINT auth_identities_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'));

ALTER TABLE auth_identity_channels
    DROP CHECK auth_identity_channels_provider_type_check;

ALTER TABLE auth_identity_channels
    ADD CONSTRAINT auth_identity_channels_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'));

ALTER TABLE pending_auth_sessions
    DROP CHECK pending_auth_sessions_provider_type_check;

ALTER TABLE pending_auth_sessions
    ADD CONSTRAINT pending_auth_sessions_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'));
