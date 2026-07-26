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
			delivery.SourceID, delivery.ReminderKey, delivery.Locale, sqlmock.AnyArg(), sqlmock.AnyArg(), 5).
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
			delivery.SourceID, delivery.ReminderKey, delivery.Locale, sqlmock.AnyArg(), sqlmock.AnyArg(), 5).
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
	columns := []string{"id", "dedup_key", "event", "channel", "recipient_email", "recipient_hash", "recipient_name", "user_id", "source_type", "source_id", "reminder_key", "locale", "variables", "raw_html_variables", "status", "attempt_count", "max_attempts", "next_attempt_at", "last_error_category", "last_error", "created_at", "updated_at", "sent_at"}
	mock.ExpectQuery("(?s)attempt_count >= max_attempts.*lease_expires_at < NOW\\(\\).*attempt_count < max_attempts.*FOR UPDATE SKIP LOCKED.*RETURNING").
		WithArgs("worker-a", 4, int64(120)).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			int64(1), strings.Repeat("a", 64), service.NotificationEmailEventOpsAlert,
			service.NotificationEmailChannelOpsAlert, "ops@example.com", strings.Repeat("b", 64),
			"ops", int64(0), "ops_incident", "incident-1", "firing", "en", []byte(`{"rule_name":"errors"}`),
			[]byte(`{}`), "processing", 1, 5, now, nil, nil, now, now, nil,
		))
	repo := NewNotificationEmailDeliveryRepository(db)
	items, err := repo.Claim(context.Background(), "worker-a", 4, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, items[0].AttemptCount)
	require.Equal(t, "errors", items[0].Variables["rule_name"])
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

func testNotificationEmailDelivery() service.NotificationEmailDelivery {
	return service.NotificationEmailDelivery{
		DedupKey: strings.Repeat("a", 64), Event: service.NotificationEmailEventOpsAlert,
		Channel: service.NotificationEmailChannelOpsAlert, RecipientEmail: "ops@example.com",
		RecipientHash: strings.Repeat("b", 64), RecipientName: "ops", SourceType: "ops_incident",
		SourceID: "incident-1", ReminderKey: "firing", Locale: "en", Variables: map[string]string{},
		RawHTMLVariables: map[string]string{}, MaxAttempts: 5,
	}
}
