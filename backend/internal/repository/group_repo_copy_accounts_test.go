package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGroupRepositoryGetAccountIDsByGroupIDsReturnsDistinctUnion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT DISTINCT account_id FROM account_groups WHERE group_id = ANY($1) ORDER BY account_id",
	)).
		WithArgs("{1,2}").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).
			AddRow(int64(101)).
			AddRow(int64(102)).
			AddRow(int64(103)))

	repo := newGroupRepositoryWithSQL(nil, db)
	accountIDs, err := repo.GetAccountIDsByGroupIDs(context.Background(), []int64{1, 2})

	require.NoError(t, err)
	require.Equal(t, []int64{101, 102, 103}, accountIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
