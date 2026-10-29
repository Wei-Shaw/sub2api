-- 修复：user_provider_default_grants 表的 provider_type check 约束
-- 与 auth_identities / auth_identity_channels / pending_auth_sessions 保持一致，
-- 否则启用了 auth_source_default_{github,google,dingtalk}_grant_on_first_bind
-- 之后，OAuth 首次绑定流程会因约束违反而失败。
-- 参见 migrations 135、136 漏改本表。

ALTER TABLE user_provider_default_grants
    DROP CONSTRAINT IF EXISTS user_provider_default_grants_provider_type_check;

ALTER TABLE user_provider_default_grants
    ADD CONSTRAINT user_provider_default_grants_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk'));
