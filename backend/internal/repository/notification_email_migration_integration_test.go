//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationAndOpsAlertMigrationsRemainApplied(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"notification_email_deliveries", "ops_alert_rule_evaluations", "ops_alert_rule_states"} {
		var exists bool
		err := integrationDB.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists)
		require.NoError(t, err)
		require.Truef(t, exists, "expected migration table %s", table)
	}

	for _, column := range []struct {
		table string
		name  string
	}{
		{table: "notification_email_deliveries", name: "sensitive_variables_ciphertext"},
		{table: "ops_alert_events", name: "email_queued"},
		{table: "ops_alert_rules", name: "minimum_samples"},
		{table: "ops_error_logs", name: "final_outcome"},
		{table: "ops_error_logs", name: "counts_toward_sla"},
		{table: "ops_error_logs", name: "classification_version"},
		{table: "ops_metrics_hourly", name: "metric_definition_version"},
		{table: "ops_metrics_daily", name: "metric_definition_version"},
	} {
		var exists bool
		err := integrationDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
			)
		`, column.table, column.name).Scan(&exists)
		require.NoError(t, err)
		require.Truef(t, exists, "expected migration column %s.%s", column.table, column.name)
	}

	for _, index := range []string{
		"idx_ops_error_logs_sla_outcome_time",
		"idx_ops_metrics_hourly_v2_bucket",
		"idx_ops_metrics_daily_v2_bucket",
	} {
		var exists bool
		err := integrationDB.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, index).Scan(&exists)
		require.NoError(t, err)
		require.Truef(t, exists, "expected migration index %s", index)
	}
}
