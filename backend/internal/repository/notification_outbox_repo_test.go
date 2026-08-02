//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNotificationOutboxClaimStoresLeaseOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &notificationOutboxRepository{db: db}
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE notification_outbox o")).WithArgs(50, 10, int64(60), "worker-a").WillReturnRows(sqlmock.NewRows([]string{"id", "event", "recipient", "locale", "variables", "attempt_count"}).AddRow(1, "company_upgrade_submitted", "admin@example.com", "en-US", []byte(`{"company_name":"Acme"}`), 1))
	messages, err := repo.Claim(context.Background(), "worker-a", 50, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "Acme", messages[0].Variables["company_name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationOutboxAckRequiresLeaseOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &notificationOutboxRepository{db: db}
	mock.ExpectExec("UPDATE notification_outbox SET status='delivered'").WithArgs(int64(9), "worker-a").WillReturnResult(sqlmock.NewResult(0, 0))
	require.Error(t, repo.MarkDelivered(context.Background(), 9, "worker-a"))
	require.NoError(t, mock.ExpectationsWereMet())
}
