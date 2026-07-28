package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestOpsV2ReleaseMigrationsRemainBlueGreenCompatible(t *testing.T) {
	t.Helper()

	alertMigration, err := migrations.FS.ReadFile("193_ops_alert_evaluation_v2.sql")
	require.NoError(t, err)
	classificationMigration, err := migrations.FS.ReadFile("195_ops_error_classification_v2.sql")
	require.NoError(t, err)
	indexMigration, err := migrations.FS.ReadFile("196_ops_v2_indexes_notx.sql")
	require.NoError(t, err)

	alertSQL := strings.ToLower(string(alertMigration))
	classificationSQL := strings.ToLower(string(classificationMigration))
	indexSQL := strings.ToLower(string(indexMigration))

	for _, sqlText := range []string{alertSQL, classificationSQL} {
		require.NotContains(t, sqlText, "\nset ", "transaction policy belongs to the migration runner")
		require.NotContains(t, sqlText, "\nupdate ", "release migrations must not rewrite historical rows")
		require.NotContains(t, sqlText, "create index concurrently", "concurrent indexes belong in a _notx migration")
	}

	for _, fragment := range []string{
		"add column if not exists incident_family varchar(64);",
		"add column if not exists email_queued boolean;",
		"add column if not exists final_outcome varchar(32);",
		"add column if not exists counts_toward_sla boolean;",
		"add column if not exists metric_definition_version integer;",
	} {
		require.Contains(t, alertSQL+classificationSQL, fragment)
	}

	require.Equal(t, 4, strings.Count(classificationSQL, "on conflict (name) do nothing"))
	require.Equal(t, 5, strings.Count(indexSQL, "create index concurrently if not exists"))
	require.NotContains(t, indexSQL, "\nupdate ")
}
