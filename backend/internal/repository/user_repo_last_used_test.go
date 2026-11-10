//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_GetLatestUsedAtByUserIDs_BoundedLookup(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &userRepository{sql: db}
	latest := time.Date(2026, 9, 21, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	mock.ExpectQuery(`SELECT requested.user_id, latest.created_at AS last_used_at.*SELECT DISTINCT unnest\(\$1::bigint\[\]\).*CROSS JOIN LATERAL.*WHERE user_id >= requested.user_id AND user_id <= requested.user_id.*ORDER BY user_id DESC, created_at DESC LIMIT 1`).
		WithArgs(pq.Array([]int64{101, 202, 101})).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "last_used_at"}).AddRow(101, latest))
	got, err := repo.GetLatestUsedAtByUserIDs(context.Background(), []int64{101, 202, 101})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, latest.Equal(*got[101]))
	require.Equal(t, time.UTC, got[101].Location())
	require.NotContains(t, got, int64(202))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_GetLatestUsedAtByUserIDs_Empty(t *testing.T) {
	repo := &userRepository{}
	got, err := repo.GetLatestUsedAtByUserIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestUserRepository_GetLatestUsedAtByUserIDs_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &userRepository{sql: db}
	wantErr := errors.New("query canceled")
	mock.ExpectQuery(`SELECT requested.user_id`).WithArgs(pq.Array([]int64{101})).WillReturnError(wantErr)
	_, err = repo.GetLatestUsedAtByUserIDs(context.Background(), []int64{101})
	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
