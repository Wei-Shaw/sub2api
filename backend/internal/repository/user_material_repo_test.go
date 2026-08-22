//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUserMaterialRepositorySoftDeleteByPublicIDsIsOwnerScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const query = `
UPDATE user_materials
SET deleted_at = NOW()
WHERE user_id = $1 AND public_id = ANY($2::uuid[]) AND deleted_at IS NULL
RETURNING public_id
`
	first := "550e8400-e29b-41d4-a716-446655440000"
	second := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"public_id"}).AddRow(second).AddRow(first))

	repo := &userMaterialRepository{db: db}
	deleted, err := repo.SoftDeleteByPublicIDs(context.Background(), 7, []string{first, second})
	require.NoError(t, err)
	require.Equal(t, []string{second, first}, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserMaterialRepositorySoftDeleteByPublicIDsSkipsEmptyInput(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &userMaterialRepository{db: db}
	deleted, err := repo.SoftDeleteByPublicIDs(context.Background(), 7, nil)
	require.NoError(t, err)
	require.Empty(t, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}
