package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountVisibilityRepositoryListsOnlyMappedAccountIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewAccountVisibilityRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM user_visible_accounts uva JOIN accounts a ON a.id = uva.account_id WHERE uva.user_id = $1 AND a.deleted_at IS NULL AND a.platform = $2")).
		WithArgs(int64(42), "openai").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT a\\.id FROM user_visible_accounts uva JOIN accounts a ON a\\.id = uva\\.account_id WHERE .*ORDER BY a\\.name ASC").
		WithArgs(int64(42), "openai", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11).AddRow(12))

	ids, result, err := repo.ListVisibleAccountIDs(context.Background(), 42, pagination.PaginationParams{
		Page: 1, PageSize: 20, SortBy: "name", SortOrder: "asc",
	}, service.AccountVisibilityFilters{Platform: "openai"})
	require.NoError(t, err)
	require.Equal(t, []int64{11, 12}, ids)
	require.EqualValues(t, 2, result.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountVisibilityRepositoryChecksMappedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewAccountVisibilityRepository(db)

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(int64(42), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	visible, err := repo.CanViewAccount(context.Background(), 42, 11)
	require.NoError(t, err)
	require.True(t, visible)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountVisibilityRepositoryReplacesUsersAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewAccountVisibilityRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND id IN ($1,$2)")).
		WithArgs(int64(3), int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM user_visible_accounts WHERE account_id = $1")).
		WithArgs(int64(21)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_visible_accounts (user_id, account_id) VALUES ($2, $1),($3, $1)")).
		WithArgs(int64(21), int64(3), int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err = repo.ReplaceAccountVisibleUsers(context.Background(), 21, []int64{9, 3, 9})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
