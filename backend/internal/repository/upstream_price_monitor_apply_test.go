package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyUpstreamTokenBaseIntervalsPreservesMultiplierOnlyFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT id,input_price::text,output_price::text`).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "input_price", "output_price", "cache_write_price", "cache_read_price", "per_request_price",
			"has_input_multiplier", "has_output_multiplier", "has_cache_write_multiplier", "has_cache_read_multiplier",
		}).AddRow(int64(101), nil, "0.000002", nil, nil, nil, true, false, false, false))
	mock.ExpectExec(`UPDATE channel_pricing_intervals SET`).
		WithArgs(int64(101), nil, "0.000003", nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	snapshot := &upstreamPriceRollbackSnapshot{}
	changed, err := applyUpstreamTokenBaseIntervals(
		context.Background(), tx, 44, "0.000001", "0.000003", nil, nil, snapshot,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, snapshot.Intervals, 1)
	require.Nil(t, snapshot.Intervals[0].InputPrice)
	require.Equal(t, "0.000002", *snapshot.Intervals[0].OutputPrice)

	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCalculateDisplayMultiplierUsesHalfUpRounding(t *testing.T) {
	input := 0.0135
	multiplier, ok, err := calculateDisplayMultiplier(upstreamPriceApplyEvidence{
		Model: "domestic-model", BillingMode: service.DisplayBillingModeToken,
		Suggested: domain.UpstreamPriceVector{InputPerMillion: &input},
	}, sql.NullString{String: "1", Valid: true}, sql.NullString{}, 3)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0.014", multiplier.StringFixed(3))
}

func TestMergeMaximumCommonUpstreamPriceVectorProtectsEveryAccountMargin(t *testing.T) {
	aInput, aOutput, aCache := 0.2, 0.8, 0.03
	bInput, bOutput := 0.25, 0.7
	merged := mergeMaximumCommonUpstreamPriceVector([]domain.UpstreamPriceVector{
		{InputPerMillion: &aInput, OutputPerMillion: &aOutput, CacheReadPerMillion: &aCache},
		{InputPerMillion: &bInput, OutputPerMillion: &bOutput},
	})
	require.InDelta(t, 0.25, *merged.InputPerMillion, 1e-12)
	require.InDelta(t, 0.8, *merged.OutputPerMillion, 1e-12)
	require.Nil(t, merged.CacheReadPerMillion, "a dimension missing on any selected account must remain unchanged")
}

func TestMergeCompatibleUpstreamPriceVectorsCombinesPassiveAndActiveDimensions(t *testing.T) {
	input, output, cacheWrite, cacheRead := 0.2, 0.8, 0.4, 0.05
	merged, ok := mergeCompatibleUpstreamPriceVectors(
		domain.UpstreamPriceVector{InputPerMillion: &input, OutputPerMillion: &output, CacheReadPerMillion: &cacheRead},
		domain.UpstreamPriceVector{InputPerMillion: &input, OutputPerMillion: &output, CacheWritePerMillion: &cacheWrite},
	)
	require.True(t, ok)
	require.InDelta(t, 0.4, *merged.CacheWritePerMillion, 1e-12)
	require.InDelta(t, 0.05, *merged.CacheReadPerMillion, 1e-12)

	conflicting := 0.25
	_, ok = mergeCompatibleUpstreamPriceVectors(
		domain.UpstreamPriceVector{InputPerMillion: &input},
		domain.UpstreamPriceVector{InputPerMillion: &conflicting},
	)
	require.False(t, ok)
}
