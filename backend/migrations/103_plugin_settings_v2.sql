-- 103_plugin_settings_v2.sql
-- V5/W6 SETTINGS-V2: extend plugin settings schema metadata to support
-- visibility / deprecated / requires_reload markers and per-row schema
-- version stamping. See docs/plugin-architecture/SETTINGS-V2-DESIGN.md §1.

-- 1. plugin_settings_schemas: per-plugin schema row.
-- schema_version mirrors Manifest.SettingsSchema.Version reported by the
-- plugin. Sentinel '0' means "plugin did not declare a version" — treated
-- as the lowest possible version when comparing against stored values.
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS schema_version TEXT NOT NULL DEFAULT '0';

-- properties_meta caches the marker extraction (visibility / deprecated /
-- requires_reload) keyed by top-level property name. Persisted so admin
-- API responses do not need to re-walk schema_json on every GET.
-- Layout is documented in SETTINGS-V2-DESIGN §1.4. NULL means
-- "schema has no declared markers"; the host writes '{}'::jsonb instead
-- so handlers never need to nil-check.
ALTER TABLE plugin_settings_schemas
    ADD COLUMN IF NOT EXISTS properties_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN plugin_settings_schemas.schema_version IS
    'Plugin-declared schema version (Manifest.SettingsSchema.Version). ''0'' means undeclared.';
COMMENT ON COLUMN plugin_settings_schemas.properties_meta IS
    'Cached marker extraction: {<prop>: {visibility,deprecated,requires_reload,...}}. See SETTINGS-V2-DESIGN §1.4.';

-- 2. plugin_settings: per-(plugin,key) value row.
-- schema_version_at_write records the schema version that was active when
-- the row was last written. Used by the host to detect stale values when
-- a plugin upgrade ships a new schema_version.
ALTER TABLE plugin_settings
    ADD COLUMN IF NOT EXISTS schema_version_at_write TEXT NOT NULL DEFAULT '0';

COMMENT ON COLUMN plugin_settings.schema_version_at_write IS
    'Schema version active when this row was last written. Compared against plugin_settings_schemas.schema_version to detect stale values.';

-- 3. Index for "find all values written under an old schema version".
-- Used by future cleanup jobs; not on the hot read path.
CREATE INDEX IF NOT EXISTS idx_plugin_settings_schema_version_at_write
    ON plugin_settings (plugin_name, schema_version_at_write);

