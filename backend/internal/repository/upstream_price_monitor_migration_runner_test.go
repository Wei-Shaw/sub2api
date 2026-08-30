package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPrepareUpstreamPriceUsageWindowMigrationDropsInvalidConcurrentIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs(upstreamPriceUsageWindowIndex).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_account_id_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, prepareNonTransactionalMigration(
		context.Background(), db, upstreamPriceUsageWindowIndexMigration,
	))
	require.NoError(t, mock.ExpectationsWereMet())
}
