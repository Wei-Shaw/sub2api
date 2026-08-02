//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetTTFTPercentilesUsesDedicatedQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	mock.ExpectQuery(`(?s)SELECT\s+percentile_cont\(0\.95\).*COUNT\(first_token_ms\).*FROM usage_logs ul`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"ttft_p95", "ttft_p99", "ttft_max", "ttft_sample_count",
		}).AddRow(1200.4, 3400.6, int64(9000), int64(27)))

	result, err := repo.GetTTFTPercentiles(context.Background(), &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		QueryMode: service.OpsQueryModeRaw,
	})

	require.NoError(t, err)
	require.Equal(t, int64(27), result.SampleCount)
	require.Equal(t, 1200, *result.TTFT.P95)
	require.Equal(t, 3401, *result.TTFT.P99)
	require.Equal(t, 9000, *result.TTFT.Max)
	require.NoError(t, mock.ExpectationsWereMet())
}
