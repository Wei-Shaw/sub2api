package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestSetModelCatalogManagedRejectsIncompleteAccountCoverage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &upstreamPriceMonitorRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT m\.seen_account_count,m\.expected_account_count`).
		WithArgs("deepseek-new-v1").
		WillReturnRows(sqlmock.NewRows([]string{"seen", "expected", "complete"}).AddRow(1, 2, true))
	mock.ExpectRollback()

	_, err = repo.SetModelCatalogStatus(
		context.Background(), "deepseek-new-v1", domain.UpstreamPriceModelStatusManaged,
	)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
