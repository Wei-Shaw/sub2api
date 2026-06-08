-- 101_create_plugins.sql
-- 插件注册表与插件迁移追踪表
-- plugins: 记录已安装/注册的插件元数据与启用状态
-- plugin_migrations: 与核心 schema_migrations 隔离，按插件命名空间追踪各插件自带的迁移

CREATE TABLE IF NOT EXISTS plugins (
    name            VARCHAR(64) PRIMARY KEY,
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    version         VARCHAR(32) NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order      INT NOT NULL DEFAULT 0,
    config          JSONB NOT NULL DEFAULT '{}',
    installed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plugin_migrations (
    plugin_name     VARCHAR(64) NOT NULL,
    filename        TEXT NOT NULL,
    checksum        TEXT NOT NULL,
    applied_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plugin_name, filename)
);
