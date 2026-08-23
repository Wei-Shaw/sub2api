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

func TestUserMaterialRepositoryUpdateFileNameByIDIsOwnerScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const query = `
UPDATE user_materials
SET file_name = $3
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
`
	publicID := "550e8400-e29b-41d4-a716-446655440000"
	createdAt := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(int64(42), int64(7), "renamed.png").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_id", "user_id", "file_name", "cos_key", "cos_url",
			"content_type", "size_bytes", "kind", "source", "created_at",
		}).AddRow(42, publicID, 7, "renamed.png", "users/u/material.png", "https://cdn.example.com/material.png",
			"image/png", 123, "image", "upload", createdAt))

	repo := &userMaterialRepository{db: db}
	material, err := repo.UpdateFileNameByID(context.Background(), 7, 42, "renamed.png")
	require.NoError(t, err)
	require.Equal(t, "renamed.png", material.FileName)
	require.Equal(t, "users/u/material.png", material.CosKey, "rename must not alter the object key")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserMaterialRepositoryUpdateFileNameByPublicIDIsOwnerScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const query = `
UPDATE user_materials
SET file_name = $3
WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, public_id, user_id, file_name, cos_key, cos_url, content_type, size_bytes, kind, source, created_at
`
	publicID := "550e8400-e29b-41d4-a716-446655440000"
	createdAt := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(publicID, int64(7), "renamed.png").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_id", "user_id", "file_name", "cos_key", "cos_url",
			"content_type", "size_bytes", "kind", "source", "created_at",
		}).AddRow(42, publicID, 7, "renamed.png", "users/u/material.png", "https://cdn.example.com/material.png",
			"image/png", 123, "image", "upload", createdAt))

	repo := &userMaterialRepository{db: db}
	material, err := repo.UpdateFileNameByPublicID(context.Background(), 7, publicID, "renamed.png")
	require.NoError(t, err)
	require.Equal(t, "renamed.png", material.FileName)
	require.Equal(t, "users/u/material.png", material.CosKey, "rename must not alter the object key")
	require.NoError(t, mock.ExpectationsWereMet())
}

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
