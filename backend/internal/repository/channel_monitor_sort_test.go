//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorUpdateSortOrdersCommitsCompleteOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE channel_monitors IN SHARE ROW EXCLUSIVE MODE")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM channel_monitors")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec("UPDATE channel_monitors AS monitors").
		WithArgs(int64(8), 0, int64(3), 10).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	repo := &channelMonitorRepository{db: db}
	err = repo.UpdateSortOrders(context.Background(), []service.ChannelMonitorSortOrderUpdate{
		{ID: 8, SortOrder: 0},
		{ID: 3, SortOrder: 10},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorUpdateSortOrdersRejectsStaleList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE channel_monitors IN SHARE ROW EXCLUSIVE MODE")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM channel_monitors")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectRollback()

	repo := &channelMonitorRepository{db: db}
	err = repo.UpdateSortOrders(context.Background(), []service.ChannelMonitorSortOrderUpdate{
		{ID: 8, SortOrder: 0},
		{ID: 3, SortOrder: 10},
	})

	require.ErrorContains(t, err, "list changed")
	require.NoError(t, mock.ExpectationsWereMet())
}
