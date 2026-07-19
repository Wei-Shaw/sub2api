package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogRepositoryStageRevisionKeepsOperatorPolicyAndStagesAliasesAtomically(t *testing.T) {
	db, mock, cleanup := newCatalogSQLMock(t)
	defer cleanup()
	repo := NewModelCatalogRepository(db)

	stage := service.CatalogRevisionStage{
		Revision:       7,
		NormalizedHash: "normalized-hash-7",
		Normalizer:     "catalog-normalizer-v1",
		SyncRun: service.CatalogSyncRunSpec{
			SourceSet:       "legacy-pricing",
			Trigger:         "manual",
			Normalizer:      "catalog-normalizer-v1",
			SourceCount:     1,
			NormalizedCount: 1,
			AddedCount:      1,
		},
		Models: []service.CatalogSnapshotModelSpec{
			{
				ID:            1,
				RevisionID:    7001,
				CanonicalKey:  "gpt-5.6-sol",
				OperatorState: service.CatalogOperatorStateEnabled,
				SourceState:   service.CatalogSourceStatePresent,
				Provider:      "openai",
				Platform:      "openai",
				Mode:          "chat",
				PricingValid:  true,
				PricingSource: "legacy-lite-llm",
				SourceHash:    "source-hash-1",
				Pricing:       &service.LiteLLMModelPricing{InputCostPerToken: 5e-6},
				Aliases:       []service.CatalogAliasSpec{{Alias: "gpt-5.6", PlatformScope: "openai"}},
			},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO catalog_sync_runs")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(501)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO catalog_revisions")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(701)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO catalog_models")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "operator_state", "operator_reason", "operator_version"}).AddRow(int64(1), "disabled", "maintenance", int64(3)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO catalog_model_revisions")).
		WillReturnResult(sqlmock.NewResult(7001, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO catalog_model_aliases")).
		WillReturnResult(sqlmock.NewResult(8001, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT model_id")).
		WithArgs("openai", "gpt-5.6").
		WillReturnRows(sqlmock.NewRows([]string{"model_id"}).AddRow(int64(1)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO catalog_model_revisions")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE catalog_sync_runs")).
		WithArgs(int64(501), "normalized-hash-7").
		WillReturnResult(sqlmock.NewResult(0, 1))

	revisionID, err := repo.StageRevision(context.Background(), stage)
	require.NoError(t, err)
	require.Equal(t, int64(701), revisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}
