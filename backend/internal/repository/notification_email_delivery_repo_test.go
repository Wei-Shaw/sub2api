package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestNotificationEmailDeliveryRepositoryEnqueueDeduplicates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewNotificationEmailDeliveryRepository(db)
	delivery := testNotificationEmailDelivery()

	mock.ExpectQuery("INSERT INTO notification_email_deliveries").
		WithArgs(delivery.DedupKey, delivery.Event, delivery.Channel, delivery.RecipientEmail,
			delivery.RecipientHash, delivery.RecipientName, delivery.UserID, delivery.SourceType,
			delivery.SourceID, delivery.ReminderKey, delivery.Locale, sqlmock.AnyArg(), sqlmock.AnyArg(), 5, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	id, created, err := repo.Enqueue(context.Background(), delivery)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(9), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationEmailDeliveryRepositoryEnqueueReusesEntTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	defer func() { _ = client.Close() }()
	repo := NewNotificationEmailDeliveryRepository(db)
	delivery := testNotificationEmailDelivery()

	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(context.Background(), tx)
	exec := notificationEmailDeliveryExecutor(txCtx, nil)
	require.NotNil(t, exec)
	require.Same(t, tx.Client().Driver(), exec)
	mock.ExpectQuery("INSERT INTO notification_email_deliveries").
		WithArgs(delivery.DedupKey, delivery.Event, delivery.Channel, delivery.RecipientEmail,
			delivery.RecipientHash, delivery.RecipientName, delivery.UserID, delivery.SourceType,
			delivery.SourceID, delivery.ReminderKey, delivery.Locale, sqlmock.AnyArg(), sqlmock.AnyArg(), 5, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	id, created, err := repo.Enqueue(txCtx, delivery)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(11), id)
	mock.ExpectCommit()
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationEmailDeliveryRepositoryClaimUsesCrossInstanceLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	now := time.Now().UTC()
	columns := []string{"id", "dedup_key", "event", "channel", "recipient_email", "recipient_hash", "recipient_name", "user_id", "source_type", "source_id", "reminder_key", "locale", "variables", "raw_html_variables", "status", "attempt_count", "max_attempts", "next_attempt_at", "last_error_category", "last_error", "created_at", "updated_at", "sent_at", "sensitive_variables_ciphertext"}
	mock.ExpectQuery("(?s)attempt_count >= max_attempts.*lease_expires_at < NOW\\(\\).*attempt_count < max_attempts.*FOR UPDATE SKIP LOCKED.*RETURNING").
		WithArgs("worker-a", 4, int64(120)).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			int64(1), strings.Repeat("a", 64), service.NotificationEmailEventOpsAlert,
			service.NotificationEmailChannelOpsAlert, "ops@example.com", strings.Repeat("b", 64),
			"ops", int64(0), "ops_incident", "incident-1", "firing", "en", []byte(`{"rule_name":"errors"}`),
			[]byte(`{}`), "processing", 1, 5, now, nil, nil, now, now, nil, "encrypted-payload",
		))
	repo := NewNotificationEmailDeliveryRepository(db)
	items, err := repo.Claim(context.Background(), "worker-a", 4, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, items[0].AttemptCount)
	require.Equal(t, "errors", items[0].Variables["rule_name"])
	require.Equal(t, "encrypted-payload", items[0].SensitiveVariablesCiphertext)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationEmailDeliveryRepositoryCleansOnlyTerminalRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	successfulBefore := time.Now().UTC().Add(-30 * 24 * time.Hour)
	failedBefore := time.Now().UTC().Add(-90 * 24 * time.Hour)
	mock.ExpectExec("(?s)status IN \\('sent', 'suppressed'\\).*status = 'failed'.*FOR UPDATE SKIP LOCKED.*DELETE FROM notification_email_deliveries").
		WithArgs(successfulBefore, failedBefore, 1000).
		WillReturnResult(sqlmock.NewResult(0, 12))
	repo := NewNotificationEmailDeliveryRepository(db)
	deleted, err := repo.DeleteTerminalBefore(context.Background(), successfulBefore, failedBefore, 1000)
	require.NoError(t, err)
	require.Equal(t, int64(12), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationEmailDeliveryRepositoryRejectsLostClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectExec("UPDATE notification_email_deliveries").
		WithArgs(int64(3), "old-worker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	repo := NewNotificationEmailDeliveryRepository(db)
	err = repo.MarkSent(context.Background(), 3, "old-worker")
	require.ErrorContains(t, err, "no longer owned")
}

func TestNotificationEmailDeliveryRepositoryRetryOnlyAllowsTransientFailures(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectExec("(?s)status IN \\('failed', 'retry_wait'\\).*last_error_category.*'transport', 'internal', 'configuration', 'template'").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := NewNotificationEmailDeliveryRepository(db)
	retried, err := repo.Retry(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, retried)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationEmailDeliveryListWhereScopesOpsRateAndMergeWindows(t *testing.T) {
	createdAfter := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	where, args := notificationEmailDeliveryListWhere(service.NotificationEmailDeliveryListFilter{
		Event:         service.NotificationEmailEventOpsAlert,
		SourceType:    "ops_alert_event",
		RecipientHash: strings.Repeat("b", 64),
		ReminderKey:   "firing:availability",
		CreatedAfter:  &createdAfter,
	})

	require.Equal(t,
		" WHERE event = $1 AND source_type = $2 AND recipient_hash = $3 AND reminder_key = $4 AND created_at >= $5",
		where,
	)
	require.Equal(t, []any{
		service.NotificationEmailEventOpsAlert,
		"ops_alert_event",
		strings.Repeat("b", 64),
		"firing:availability",
		createdAfter,
	}, args)
}

func TestNotificationEmailDeliveryMigrationHasLeasePrivacyAndIndexGuards(t *testing.T) {
	content, err := migrations.FS.ReadFile("188_notification_email_deliveries.sql")
	require.NoError(t, err)
	sqlText := string(content)
	for _, required := range []string{
		"dedup_key", "recipient_hash", "lease_owner", "lease_expires_at",
		"retry_wait", "suppressed", "max_attempts", "raw_html_variables",
		"idx_notification_email_deliveries_claim", "idx_notification_email_deliveries_source",
		"idx_notification_email_deliveries_terminal_cleanup",
	} {
		require.Contains(t, sqlText, required)
	}
	require.NotContains(t, sqlText, "DROP TABLE")
	require.NotContains(t, sqlText, "ALTER TABLE")
}

func TestNotificationEmailSensitivePayloadMigrationIsAdditive(t *testing.T) {
	content, err := migrations.FS.ReadFile("194_notification_email_sensitive_payload.sql")
	require.NoError(t, err)
	sqlText := string(content)
	require.Contains(t, sqlText, "ADD COLUMN IF NOT EXISTS sensitive_variables_ciphertext TEXT")
	require.NotContains(t, sqlText, "DROP TABLE")
	require.NotContains(t, sqlText, "DROP COLUMN")
	require.NotContains(t, sqlText, "-- +goose Down")
}

func TestOpsAlertEvaluationMigrationIsForwardOnlyAndAdditive(t *testing.T) {
	content, err := migrations.FS.ReadFile("193_ops_alert_evaluation_v2.sql")
	require.NoError(t, err)
	sqlText := string(content)
	require.Contains(t, sqlText, "CREATE TABLE ops_alert_rule_evaluations")
	require.NotContains(t, sqlText, "CREATE TABLE IF NOT EXISTS ops_alert_rule_evaluations")
	require.Contains(t, sqlText, "ADD COLUMN IF NOT EXISTS email_queued")
	require.NotContains(t, sqlText, "DROP TABLE")
	require.NotContains(t, sqlText, "DROP COLUMN")
	require.NotContains(t, sqlText, "-- +goose Down")
}

func TestOpsAlertRuleDefaultsMigrationOnlyAddsV2AvailabilityRules(t *testing.T) {
	content, err := migrations.FS.ReadFile("197_add_ops_alert_rule_v2_defaults.sql")
	require.NoError(t, err)
	sqlText := string(content)
	for _, required := range []string{
		"'基础设施可用性缓慢下降'",
		"'基础设施可用性快速下降'",
		"'availability_failure_rate'",
		"ON CONFLICT (name) DO NOTHING",
	} {
		require.Contains(t, sqlText, required)
	}
	require.Equal(t, 2, strings.Count(sqlText, "INSERT INTO ops_alert_rules"))
	require.NotContains(t, sqlText, "UPDATE ")
	require.NotContains(t, sqlText, "DELETE ")
	require.NotContains(t, sqlText, "DROP TABLE")
	require.NotContains(t, sqlText, "DROP COLUMN")
	require.NotContains(t, sqlText, "-- +goose Down")
}

func testNotificationEmailDelivery() service.NotificationEmailDelivery {
	return service.NotificationEmailDelivery{
		DedupKey: strings.Repeat("a", 64), Event: service.NotificationEmailEventOpsAlert,
		Channel: service.NotificationEmailChannelOpsAlert, RecipientEmail: "ops@example.com",
		RecipientHash: strings.Repeat("b", 64), RecipientName: "ops", SourceType: "ops_incident",
		SourceID: "incident-1", ReminderKey: "firing", Locale: "en", Variables: map[string]string{},
		RawHTMLVariables: map[string]string{}, MaxAttempts: 5,
	}
}
