package repository

import (
	"context"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestBootstrapCompleteRequiresDurableCompletionRecord(t *testing.T) {
	t.Run("not complete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM installation_state WHERE id = 1)")).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		complete, err := BootstrapComplete(context.Background(), db)
		require.NoError(t, err)
		require.False(t, complete)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("state table has not been migrated", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM installation_state WHERE id = 1)")).
			WillReturnError(&pq.Error{Code: "42P01"})

		complete, err := BootstrapComplete(context.Background(), db)
		require.NoError(t, err)
		require.False(t, complete)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("complete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM installation_state WHERE id = 1)")).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		complete, err := BootstrapComplete(context.Background(), db)
		require.NoError(t, err)
		require.True(t, complete)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRequireBootstrapCompleteRejectsIncompleteInstallation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM installation_state WHERE id = 1)")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	err = RequireBootstrapComplete(context.Background(), db)
	require.ErrorContains(t, err, "bootstrap has not completed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequireMigrationsAppliedFS(t *testing.T) {
	migrations := fstest.MapFS{
		"001_schema.sql": {Data: []byte("CREATE TABLE widgets (id bigint);")},
	}

	t.Run("rejects missing migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
			WithArgs("001_schema.sql").
			WillReturnRows(sqlmock.NewRows([]string{"checksum"}))

		err = requireMigrationsAppliedFS(context.Background(), db, migrations)
		require.ErrorContains(t, err, "migration gate incomplete: 001_schema.sql")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("accepts applied migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		checksums, err := migrationFilesWithChecksums(migrations)
		require.NoError(t, err)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
			WithArgs("001_schema.sql").
			WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksums["001_schema.sql"]))

		require.NoError(t, requireMigrationsAppliedFS(context.Background(), db, migrations))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
