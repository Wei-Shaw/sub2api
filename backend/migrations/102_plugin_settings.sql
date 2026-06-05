-- 102_plugin_settings.sql
-- V5/W3 SettingsExtension capability: plugin-scoped admin-tunable settings.
--
-- Each plugin owns a flat key/value namespace. Values are JSON. Schema/defaults
-- live in the plugin manifest and are applied by the host on plugin start.
-- The schema_json column on plugin_settings_schemas is the most recent schema
-- the plugin reported via GetManifest; the host uses it for write-time
-- validation and for rendering the admin form.

CREATE TABLE IF NOT EXISTS plugin_settings (
    plugin_name VARCHAR(64) NOT NULL,
    key         TEXT NOT NULL,
    value_json  JSONB NOT NULL,
    revision    BIGINT NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plugin_name, key)
);

CREATE INDEX IF NOT EXISTS idx_plugin_settings_updated_at
    ON plugin_settings(updated_at);

CREATE TABLE IF NOT EXISTS plugin_settings_schemas (
    plugin_name   VARCHAR(64) PRIMARY KEY,
    schema_json   JSONB NOT NULL,
    defaults_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
