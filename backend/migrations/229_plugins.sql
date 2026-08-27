-- 管理员手动上传的本地进程插件。
-- 插件默认停用；账号表不保存任何插件字段。
-- 表名使用 sub2api 前缀，避免与部署数据库中已有的通用插件表冲突。

CREATE TABLE IF NOT EXISTS sub2api_plugin_installations (
    id BIGSERIAL PRIMARY KEY,
    plugin_key VARCHAR(160) NOT NULL UNIQUE,
    name VARCHAR(160) NOT NULL,
    version VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author VARCHAR(160) NOT NULL DEFAULT '',
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifact_path TEXT NOT NULL,
    install_path TEXT NOT NULL,
    binary_path TEXT NOT NULL,
    binary_sha256 VARCHAR(64) NOT NULL,
    signature_status VARCHAR(32) NOT NULL DEFAULT 'unsigned',
    state VARCHAR(32) NOT NULL DEFAULT 'disabled',
    config_encrypted TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    installed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enabled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sub2api_plugin_installations_state_check
        CHECK (state IN ('disabled', 'starting', 'enabled', 'error', 'incompatible')),
    CONSTRAINT sub2api_plugin_installations_signature_status_check
        CHECK (signature_status IN ('trusted', 'unsigned'))
);

CREATE TABLE IF NOT EXISTS sub2api_plugin_bindings (
    id BIGSERIAL PRIMARY KEY,
    plugin_id BIGINT NOT NULL REFERENCES sub2api_plugin_installations(id) ON DELETE CASCADE,
    capability VARCHAR(160) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    account_type VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    rollout_percent INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sub2api_plugin_bindings_rollout_check CHECK (rollout_percent BETWEEN 0 AND 100),
    CONSTRAINT sub2api_plugin_bindings_scope_unique UNIQUE (plugin_id, capability, platform, account_type)
);

CREATE INDEX IF NOT EXISTS idx_sub2api_plugin_bindings_plugin_id
    ON sub2api_plugin_bindings(plugin_id);

CREATE INDEX IF NOT EXISTS idx_sub2api_plugin_bindings_enabled_scope
    ON sub2api_plugin_bindings(platform, account_type, capability)
    WHERE enabled = TRUE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_sub2api_plugin_bindings_one_enabled_scope
    ON sub2api_plugin_bindings(capability, platform, account_type)
    WHERE enabled = TRUE;
