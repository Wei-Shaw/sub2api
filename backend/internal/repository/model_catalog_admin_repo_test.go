package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestStageAdminRevisionTxUsesTypedRevisionIDForSourceMissingRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO catalog_sync_runs")).
		WithArgs("admin", "admin_operator_state_bulk", nil, nil, nil, nil, "catalog-v3.1", 0, 0, 0, 0, 0, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO catalog_revisions")).
		WithArgs(int64(1), int64(10), "normalized-hash", "catalog-v3.1", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))
	mock.ExpectExec(`(?s)INSERT INTO catalog_model_revisions.*SELECT \$1::bigint.*catalog_revision_id = \$1::bigint`).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE catalog_sync_runs SET status = 'staged'")).
		WithArgs(int64(10), "normalized-hash").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	_, err = stageAdminRevisionTx(context.Background(), tx, service.CatalogRevisionStage{
		Revision:       1,
		NormalizedHash: "normalized-hash",
		Normalizer:     "catalog-v3.1",
		SyncRun: service.CatalogSyncRunSpec{
			SourceSet:  "admin",
			Trigger:    "admin_operator_state_bulk",
			Normalizer: "catalog-v3.1",
		},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
