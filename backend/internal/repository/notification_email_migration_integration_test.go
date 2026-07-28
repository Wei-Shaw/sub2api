//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
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

func TestOpsV2MigrationsKeepLegacyRowsReadable(t *testing.T) {
	ctx := context.Background()
	platform := "ops-v2-legacy-fallback-test"
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ops_error_logs WHERE platform = $1", platform)
	})

	for _, tableColumn := range []struct {
		table  string
		column string
	}{
		{table: "ops_alert_events", column: "email_queued"},
		{table: "ops_alert_rules", column: "incident_family"},
		{table: "ops_error_logs", column: "counts_toward_sla"},
		{table: "ops_metrics_hourly", column: "metric_definition_version"},
		{table: "ops_metrics_daily", column: "metric_definition_version"},
	} {
		var nullable string
		err := integrationDB.QueryRowContext(ctx, `
			SELECT is_nullable
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		`, tableColumn.table, tableColumn.column).Scan(&nullable)
		require.NoError(t, err)
		require.Equalf(t, "YES", nullable, "%s.%s must remain nullable for old-image writes", tableColumn.table, tableColumn.column)
	}

	for _, businessLimited := range []bool{false, true} {
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO ops_error_logs (
				platform, error_phase, error_type, status_code, is_business_limited,
				final_outcome, responsibility, error_category, counts_toward_sla,
				alert_family, classification_reason, classification_version, created_at
			) VALUES ($1, 'legacy', 'legacy', 500, $2, NULL, NULL, NULL, NULL, NULL, NULL, NULL, $3)
		`, platform, businessLimited, now)
		require.NoError(t, err)
	}

	repo := &opsRepository{db: integrationDB}
	stats, err := repo.GetErrorClassificationStats(ctx, &service.OpsDashboardFilter{
		StartTime: now.Add(-time.Minute),
		EndTime:   now.Add(time.Minute),
		Platform:  platform,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.FinalErrorCount)
	require.EqualValues(t, 1, stats.SLAFailureCount)
	require.EqualValues(t, 1, stats.UnknownFailureCount)
	require.EqualValues(t, 1, stats.BusinessLimitedCount)

	var eventID int64
	err = integrationDB.QueryRowContext(ctx, `
		INSERT INTO ops_alert_events (severity, status, fired_at, email_queued)
		VALUES ('P3', 'firing', $1, NULL)
		RETURNING id
	`, now).Scan(&eventID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ops_alert_events WHERE id = $1", eventID)
	})

	event, err := repo.GetAlertEventByID(ctx, eventID)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.False(t, event.EmailQueued)
}

func TestOpsV2MigrationLockTimeoutFailsFastWithoutBlockingLegacyWrites(t *testing.T) {
	ctx := context.Background()
	lockTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lockTx.Rollback() })
	_, err = lockTx.ExecContext(ctx, "LOCK TABLE ops_error_logs IN ACCESS SHARE MODE")
	require.NoError(t, err)

	migrationTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = migrationTx.Rollback() })
	require.NoError(t, configureMigrationTransaction(ctx, migrationTx))

	startedAt := time.Now()
	_, err = migrationTx.ExecContext(ctx, "ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS classification_version INTEGER")
	require.Error(t, err)
	require.Less(t, time.Since(startedAt), 2*time.Second)
	var pqErr *pq.Error
	require.True(t, errors.As(err, &pqErr))
	require.Equal(t, pq.ErrorCode("55P03"), pqErr.Code)

	platform := "ops-v2-lock-timeout-test"
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO ops_error_logs (platform, error_phase, error_type, status_code, is_business_limited)
		VALUES ($1, 'legacy', 'legacy', 500, false)
	`, platform)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ops_error_logs WHERE platform = $1", platform)
	})
}
