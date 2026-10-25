-- MySQL 方言版本（翻译自 ../229_plugins.sql）
-- 管理员手动上传的本地进程插件。
-- 插件默认停用；账号表不保存任何插件字段。
-- 表名使用 sub2api 前缀，避免与部署数据库中已有的通用插件表冲突。
--
-- 说明：manifest JSONB DEFAULT '{}'::jsonb → JSON DEFAULT ('{}')；
--       TEXT 列默认值用表达式默认值；TEXT[] 无；部分索引见下方各索引注释。

CREATE TABLE IF NOT EXISTS sub2api_plugin_installations (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    plugin_key VARCHAR(160) NOT NULL UNIQUE,
    name VARCHAR(160) NOT NULL,
    version VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT (''),
    author VARCHAR(160) NOT NULL DEFAULT '',
    manifest JSON NOT NULL DEFAULT ('{}'),
    artifact_path TEXT NOT NULL,
    install_path TEXT NOT NULL,
    binary_path TEXT NOT NULL,
    binary_sha256 VARCHAR(64) NOT NULL,
    signature_status VARCHAR(32) NOT NULL DEFAULT 'unsigned',
    state VARCHAR(32) NOT NULL DEFAULT 'disabled',
    config_encrypted TEXT NOT NULL DEFAULT (''),
    last_error TEXT NOT NULL DEFAULT (''),
    installed_by BIGINT NULL,
    installed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    enabled_at DATETIME(3) NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT sub2api_plugin_installations_state_check
        CHECK (state IN ('disabled', 'starting', 'enabled', 'error', 'incompatible')),
    CONSTRAINT sub2api_plugin_installations_signature_status_check
        CHECK (signature_status IN ('trusted', 'unsigned')),
    CONSTRAINT fk_sub2api_plugin_installations_installed_by FOREIGN KEY (installed_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS sub2api_plugin_bindings (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    plugin_id BIGINT NOT NULL,
    capability VARCHAR(160) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    account_type VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    rollout_percent INT NOT NULL DEFAULT 100,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT sub2api_plugin_bindings_rollout_check CHECK (rollout_percent BETWEEN 0 AND 100),
    CONSTRAINT sub2api_plugin_bindings_scope_unique UNIQUE (plugin_id, capability, platform, account_type),
    CONSTRAINT fk_sub2api_plugin_bindings_plugin_id FOREIGN KEY (plugin_id) REFERENCES sub2api_plugin_installations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_sub2api_plugin_bindings_plugin_id
    ON sub2api_plugin_bindings(plugin_id);

-- （PG 版为部分索引 WHERE enabled = TRUE；非唯一 → 去掉 WHERE）
CREATE INDEX idx_sub2api_plugin_bindings_enabled_scope
    ON sub2api_plugin_bindings(platform, account_type, capability);

-- （PG 版为部分唯一索引 WHERE enabled = TRUE → 函数键部分）
CREATE UNIQUE INDEX idx_sub2api_plugin_bindings_one_enabled_scope
    ON sub2api_plugin_bindings(capability, platform, account_type, (IF(enabled, 0, NULL)));
