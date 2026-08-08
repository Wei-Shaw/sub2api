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

func TestTTFTAlertRuleNormalizationMigrationIsStrictlyScoped(t *testing.T) {
	migration, err := migrations.FS.ReadFile("203_normalize_ttft_alert_minimum_bad_count.sql")
	require.NoError(t, err)
	sqlText := strings.ToLower(string(migration))

	require.Contains(t, sqlText, "sub2api-managed-update: reviewed-compatible")
	require.Contains(t, sqlText, "update ops_alert_rules")
	require.Contains(t, sqlText, "set minimum_bad_count = 0")
	require.Contains(t, sqlText, "'ttft_p95_seconds'")
	require.Contains(t, sqlText, "'ttft_p99_seconds'")
	require.Contains(t, sqlText, "'ttft_max_seconds'")
	require.Contains(t, sqlText, "and minimum_bad_count <> 0")
	require.NotContains(t, sqlText, "threshold =")
	require.NotContains(t, sqlText, "notify_email =")
	require.NotContains(t, sqlText, "enabled =")
}

func TestTTFTAlertEvaluationStatusMigrationKeepsV2AndV3WritersCompatible(t *testing.T) {
	addMigration, err := migrations.FS.ReadFile("204_expand_ops_alert_evaluation_status.sql")
	require.NoError(t, err)
	validateMigration, err := migrations.FS.ReadFile("205_validate_ops_alert_evaluation_status.sql")
	require.NoError(t, err)
	dropMigration, err := migrations.FS.ReadFile("206_drop_ops_alert_evaluation_status_v2.sql")
	require.NoError(t, err)
	addSQL := strings.ToLower(string(addMigration))
	validateSQL := strings.ToLower(string(validateMigration))
	dropSQL := strings.ToLower(string(dropMigration))

	require.Contains(t, addSQL, "add constraint ops_alert_rule_evaluations_status_check_v3")
	require.Contains(t, addSQL, "'insufficient_samples'")
	require.Contains(t, addSQL, "'insufficient_bad_count'")
	require.Contains(t, addSQL, "not valid")
	require.NotContains(t, addSQL, "validate constraint")
	require.NotContains(t, addSQL, "drop constraint")
	require.Contains(t, validateSQL, "validate constraint ops_alert_rule_evaluations_status_check_v3")
	require.NotContains(t, validateSQL, "add constraint")
	require.NotContains(t, validateSQL, "drop constraint")
	require.Contains(t, dropSQL, "drop constraint if exists ops_alert_rule_evaluations_status_check")
	require.NotContains(t, dropSQL, "validate constraint")
}

func TestOpsClassificationV3TriggerAndBackfillUseSeparateTransactions(t *testing.T) {
	triggerMigration, err := migrations.FS.ReadFile("209_reclassify_confirmed_upstream_failures.sql")
	require.NoError(t, err)
	backfillMigration, err := migrations.FS.ReadFile("210_backfill_ops_error_classification_v3.sql")
	require.NoError(t, err)

	triggerSQL := strings.ToLower(string(triggerMigration))
	backfillSQL := strings.ToLower(string(backfillMigration))
	require.Contains(t, triggerSQL, "create trigger ops_error_logs_normalize_v3_mixed_writer")
	require.Contains(t, triggerSQL, "before insert on ops_error_logs")
	require.NotContains(t, triggerSQL, "update ops_error_logs",
		"the trigger lock must commit before the historical backfill starts")
	require.Equal(t, 15, strings.Count(backfillSQL, "update ops_error_logs"))
	require.NotContains(t, backfillSQL, "create trigger")
	require.NotContains(t, backfillSQL, "create or replace function")
}
