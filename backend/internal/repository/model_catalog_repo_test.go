package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogRepositoryPublishRevisionFencesAndAtomicallyWritesSideEffects(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newCatalogSQLMock(t)
	defer cleanup()
	repo := NewModelCatalogRepository(db)
	updatedAt := time.Date(2026, 7, 18, 12, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT state FROM catalog_revisions WHERE id = $1 FOR SHARE")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("validated"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT active_revision_id, epoch")).
		WithArgs("global").
		WillReturnRows(sqlmock.NewRows([]string{"active_revision_id", "epoch"}).AddRow(int64(10), int64(3)))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE catalog_publications")).
		WithArgs("global", int64(11), int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(updatedAt))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE catalog_revisions")).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO catalog_outbox")).
		WithArgs("catalog.publication.published", "global", int64(4), int64(11), `{"catalog_revision_id":11,"publication_epoch":4,"scope":"global"}`, "global:4:11").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO catalog_lifecycle_audits")).
		WithArgs(int64(11), "operator", nil, nil, `{"catalog_revision_id":10,"publication_epoch":3,"scope":"global"}`, `{"catalog_revision_id":11,"publication_epoch":4,"scope":"global"}`, nil, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	publication, err := repo.PublishRevision(context.Background(), service.CatalogPublishRequest{
		Scope:              "global",
		CatalogRevisionID:  11,
		ExpectedEpoch:      3,
		ExpectedRevisionID: 10,
		ActorType:          "operator",
	})
	require.NoError(t, err)
	require.Equal(t, service.CatalogPublicationRecord{
		Scope:             "global",
		CatalogRevisionID: 11,
		Epoch:             4,
		UpdatedAt:         updatedAt,
	}, publication)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelCatalogRepositoryPublishRevisionRejectsStalePublicationFence(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newCatalogSQLMock(t)
	defer cleanup()
	repo := NewModelCatalogRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT state FROM catalog_revisions WHERE id = $1 FOR SHARE")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("validated"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT active_revision_id, epoch")).
		WithArgs("global").
		WillReturnRows(sqlmock.NewRows([]string{"active_revision_id", "epoch"}).AddRow(int64(12), int64(5)))
	mock.ExpectRollback()

	_, err := repo.PublishRevision(context.Background(), service.CatalogPublishRequest{
		Scope:              "global",
		CatalogRevisionID:  11,
		ExpectedEpoch:      3,
		ExpectedRevisionID: 10,
	})
	require.ErrorIs(t, err, service.ErrCatalogPublicationConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelCatalogRepositoryLoadActiveSnapshotUsesOneRepeatableReadAndValidatesProjection(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newCatalogSQLMock(t)
	defer cleanup()
	repo := NewModelCatalogRepository(db)
	publishedAt := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	verifiedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT p.epoch, r.id, r.revision, r.normalized_hash")).
		WithArgs("global").
		WillReturnRows(sqlmock.NewRows([]string{"epoch", "id", "revision", "normalized_hash", "published_at", "verified_at"}).
			AddRow(int64(7), int64(701), int64(7), "hash-7", publishedAt, verifiedAt))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT cmr.id, cmr.model_id, cm.canonical_key")).
		WithArgs(int64(701)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "model_id", "canonical_key", "operator_state", "operator_reason", "operator_version", "source_state", "provider", "platform", "mode",
			"capabilities", "context_window", "max_output_tokens", "pricing_schema_version", "pricing_json", "pricing_valid",
			"pricing_source", "source_metadata", "source_hash",
		}).AddRow(
			int64(9001), int64(1), "gpt-5.6-sol", "disabled", "maintenance", int64(3), "present", "openai", "openai", "chat",
			[]byte(`{"supports_service_tier":true}`), int64(272000), int64(128000), 1,
			[]byte(`{"input_cost_per_token":0.000005,"output_cost_per_token":0.00003,"litellm_provider":"openai","mode":"chat"}`), true,
			"legacy-lite-llm", []byte(`{"source":"fixture"}`), "source-hash-1",
		))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT alias_normalized, platform_scope, model_id")).
		WithArgs(int64(701)).
		WillReturnRows(sqlmock.NewRows([]string{"alias_normalized", "platform_scope", "model_id"}).AddRow("gpt-5.6", "openai", int64(1)))
	mock.ExpectCommit()

	spec, err := repo.LoadActiveSnapshot(context.Background(), "global")
	require.NoError(t, err)
	require.Equal(t, int64(7), spec.Epoch)
	require.Equal(t, int64(701), spec.RevisionID)
	require.Len(t, spec.Models, 1)
	require.Equal(t, "gpt-5.6-sol", spec.Models[0].CanonicalKey)
	require.Equal(t, service.CatalogOperatorStateDisabled, spec.Models[0].OperatorState)
	require.Equal(t, "maintenance", spec.Models[0].OperatorReason)
	require.Equal(t, int64(3), spec.Models[0].OperatorVersion)
	require.Equal(t, "gpt-5.6", spec.Models[0].Aliases[0].Alias)
	require.True(t, spec.Models[0].PricingValid)
	require.Equal(t, 5e-6, spec.Models[0].Pricing.InputCostPerToken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelCatalogRepositoryListOutboxAfterIsOrderedAndCopiesPayload(t *testing.T) {
	t.Parallel()

	db, mock, cleanup := newCatalogSQLMock(t)
	defer cleanup()
	repo := NewModelCatalogRepository(db)
	createdAt := time.Date(2026, 7, 18, 12, 2, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, event_type, scope, publication_epoch")).
		WithArgs("global", int64(5), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "scope", "publication_epoch", "catalog_revision_id", "model_id", "payload", "created_at"}).
			AddRow(int64(6), "catalog.publication.published", "global", int64(7), int64(701), nil, []byte(`{"epoch":7}`), createdAt))

	events, err := repo.ListOutboxAfter(context.Background(), "global", 5, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, int64(6), events[0].ID)
	require.Equal(t, jsonRaw(`{"epoch":7}`), string(events[0].Payload))
	require.NoError(t, mock.ExpectationsWereMet())
}

func jsonRaw(value string) string { return value }

func newCatalogSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	return db, mock, func() { _ = db.Close() }
}
