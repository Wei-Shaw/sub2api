package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestCompareAndSwapExtraUsesExpectedJSONValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+regexp.QuoteMeta("COALESCE(extra -> $3::text, 'null'::jsonb) = $4::jsonb")).
		WithArgs(`{"plan":null,"state":{"status":"done"}}`, int64(27), "plan", `{"plan_id":"old"}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := newAccountRepositoryWithSQL(client, db, nil)

	swapped, err := repo.CompareAndSwapExtra(context.Background(), 27, "plan", map[string]any{"plan_id": "old"}, map[string]any{
		"plan":  nil,
		"state": map[string]any{"status": "done"},
	})

	require.NoError(t, err)
	require.True(t, swapped)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompareAndSwapExtraReportsConflictWithoutOverwriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")+`.*`+regexp.QuoteMeta("COALESCE(extra -> $3::text, 'null'::jsonb) = $4::jsonb")).
		WithArgs(`{"plan":null}`, int64(27), "plan", `{"plan_id":"old"}`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT .*accounts.*id.*FROM .*accounts.*WHERE .*accounts.*id.*LIMIT 1`).
		WithArgs(int64(27)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(27)))
	repo := newAccountRepositoryWithSQL(client, db, nil)

	swapped, err := repo.CompareAndSwapExtra(context.Background(), 27, "plan", map[string]any{"plan_id": "old"}, map[string]any{"plan": nil})

	require.NoError(t, err)
	require.False(t, swapped)
	require.NoError(t, mock.ExpectationsWereMet())
}
