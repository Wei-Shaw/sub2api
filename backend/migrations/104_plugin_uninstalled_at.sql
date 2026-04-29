-- 104_plugin_uninstalled_at.sql
-- P13/C-1: track soft-uninstalled plugins. NULL = active (installed/disabled/enabled),
-- non-null = soft-uninstalled (data preserved, hidden from sidebar/list,
-- can be restored via POST /admin/plugins/:name/install).
--
-- The partial index keeps the common "list active plugins" query fast — most
-- rows have uninstalled_at IS NULL, so we only index the active subset.

ALTER TABLE plugins ADD COLUMN IF NOT EXISTS uninstalled_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_plugins_uninstalled_at
    ON plugins (uninstalled_at)
    WHERE uninstalled_at IS NULL;
