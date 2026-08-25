package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginsMigrationKeepsAccountSchemaUnchanged(t *testing.T) {
	content, err := FS.ReadFile("229_plugins.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS sub2api_plugin_installations")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS sub2api_plugin_bindings")
	require.Contains(t, sql, "config_encrypted TEXT NOT NULL DEFAULT ''")
	require.Contains(t, sql, "REFERENCES sub2api_plugin_installations(id)")
	require.NotContains(t, sql, "CREATE TABLE IF NOT EXISTS plugin_installations")
	require.NotContains(t, sql, "CREATE TABLE IF NOT EXISTS plugin_bindings")
	require.NotContains(t, strings.ToUpper(sql), "ALTER TABLE ACCOUNTS")
	require.NotContains(t, sql, "account_id")

	indexContent, err := FS.ReadFile("260_plugin_and_monitor_indexes_notx.sql")
	require.NoError(t, err)
	indexSQL := strings.Join(strings.Fields(string(indexContent)), " ")
	require.Contains(t, indexSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_sub2api_plugin_bindings_one_enabled_scope")
	require.Contains(t, indexSQL, "WHERE enabled = TRUE")
}

func TestPluginArtifactMigrationSupportsExistingInstallations(t *testing.T) {
	content, err := FS.ReadFile("230_plugin_artifacts.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE sub2api_plugin_installations")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS artifact_data BYTEA")
	require.NotContains(t, strings.ToUpper(sql), "ALTER TABLE ACCOUNTS")
}
