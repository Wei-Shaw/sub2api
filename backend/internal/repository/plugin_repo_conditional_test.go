package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPluginRepositoryUpdateConfigIfCurrent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("UPDATE sub2api_plugin_installations").
		WithArgs(int64(8), "encrypted-next", "binary-sha", "encrypted-current").
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := &pluginRepository{db: db}
	require.NoError(t, repo.UpdateConfigIfCurrent(context.Background(), 8, "encrypted-next", "binary-sha", "encrypted-current"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPluginRepositoryUpdateConfigIfCurrentRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("UPDATE sub2api_plugin_installations").
		WithArgs(int64(8), "encrypted-next", "binary-sha", "encrypted-current").
		WillReturnResult(sqlmock.NewResult(0, 0))
	repo := &pluginRepository{db: db}
	err = repo.UpdateConfigIfCurrent(context.Background(), 8, "encrypted-next", "binary-sha", "encrypted-current")
	require.ErrorIs(t, err, service.ErrPluginConfigChanged)
	require.NoError(t, mock.ExpectationsWereMet())
}
