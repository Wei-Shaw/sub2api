-- Indexes on tables that already exist in production must be built outside a transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sub2api_plugin_bindings_plugin_id
    ON sub2api_plugin_bindings(plugin_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sub2api_plugin_bindings_enabled_scope
    ON sub2api_plugin_bindings(platform, account_type, capability)
    WHERE enabled = TRUE;

-- sub2api-managed-update: reviewed-compatible
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_sub2api_plugin_bindings_one_enabled_scope
    ON sub2api_plugin_bindings(capability, platform, account_type)
    WHERE enabled = TRUE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_channel_monitors_group_id
    ON channel_monitors(group_id);
