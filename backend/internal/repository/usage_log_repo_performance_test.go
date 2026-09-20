package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var performanceColumns = []string{
	"account_id", "request_count", "ttft_ms", "tps", "cache_rate",
	"ttft_samples", "tps_samples", "cache_samples", "last_request_at",
}

func TestAccountPerformanceRepositoryBatchSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, query string) error {
		for _, clause := range []string{
			"WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3",
			"GROUP BY account_id", "SELECT account_id, COUNT(*)", "MAX(created_at)",
			"AVG(first_token_ms) FILTER (WHERE ttft_eligible)",
			"SUM(output_tokens::double precision) FILTER (WHERE tps_eligible)",
			"NULLIF(SUM(duration_ms::double precision - first_token_ms) FILTER (WHERE tps_eligible), 0)",
			"SUM(cache_read_tokens::double precision) FILTER (WHERE cache_eligible)",
			"NULLIF(SUM(input_total::double precision) FILTER (WHERE cache_eligible), 0)",
			"input_tokens::bigint + cache_creation_tokens::bigint + cache_read_tokens::bigint",
			"AND first_token_ms >= 0 AND duration_ms > 0",
			"AND duration_ms >= first_token_ms AS ttft_eligible",
			"AND first_token_ms >= 0 AND duration_ms > first_token_ms AS tps_eligible",
			"text_eligible AND input_total > 0 AS cache_eligible",
			"WHEN request_type IN (2, 3) THEN TRUE",
			"WHEN request_type = 1 THEN FALSE",
			"ELSE COALESCE(stream, FALSE) OR COALESCE(openai_ws_mode, FALSE)",
			"COALESCE(image_count, 0) = 0 AND COALESCE(video_count, 0) = 0",
			"AND COALESCE(image_output_tokens, 0) = 0",
			"NOT COALESCE(native_compaction_v2, FALSE)",
			"COALESCE(request_type, 0) NOT IN (4, 5)",
			"AND input_tokens >= 0 AND output_tokens >= 0",
			"AND cache_creation_tokens >= 0 AND cache_read_tokens >= 0",
		} {
			require.Contains(t, query, clause)
		}
		require.NotContains(t, query, "media_type")
		require.NotContains(t, strings.ToUpper(query), "ROUND(")
		return nil
	})))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &usageLogRepository{sql: db}
	end := time.Date(2026, 9, 16, 11, 37, 23, 123456000, time.UTC)
	start := end.Add(-time.Hour)
	last := end.Add(-time.Microsecond)
	mock.ExpectQuery("performance batch").WithArgs("{11,22,33}", start, end).
		WillReturnRows(sqlmock.NewRows(performanceColumns).
			AddRow(int64(11), int64(7), 12.3456789, 200.1234567, 0.625, 3, 2, 4, last).
			AddRow(int64(22), int64(1), nil, nil, nil, 0, 0, 0, start)).
		RowsWillBeClosed()
	stats, err := repo.GetAccountPerformanceStatsBatch(context.Background(), []int64{11, 22, 33}, start, end)
	require.NoError(t, err)
	require.Len(t, stats, 3)
	require.Equal(t, 12.3456789, *stats[11].TTFTMs)
	require.Equal(t, 200.1234567, *stats[11].TPS)
	require.Equal(t, 0.625, *stats[11].CacheRate)
	require.Equal(t, last, *stats[11].LastRequestAt)
	require.Equal(t, int64(7), stats[11].RequestCount)
	require.Equal(t, &service.AccountPerformanceStats{RequestCount: 1, LastRequestAt: &start}, stats[22])
	require.Equal(t, &service.AccountPerformanceStats{}, stats[33])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountPerformanceRepositoryErrors(t *testing.T) {
	expected := errors.New("read failed")
	end := time.Now().UTC()
	for _, failure := range []string{"query", "scan", "iteration"} {
		t.Run(failure, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &usageLogRepository{sql: db}
			query := mock.ExpectQuery(regexp.QuoteMeta("WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3")).
				WithArgs("{1}", end.Add(-time.Hour), end)
			switch failure {
			case "query":
				query.WillReturnError(expected)
			case "scan":
				query.WillReturnRows(sqlmock.NewRows(performanceColumns).
					AddRow(1, "invalid count", nil, nil, nil, 0, 0, 0, end)).RowsWillBeClosed()
			case "iteration":
				query.WillReturnRows(sqlmock.NewRows(performanceColumns).
					AddRow(1, 1, nil, nil, nil, 0, 0, 0, end).RowError(0, expected)).RowsWillBeClosed()
			}
			result, err := repo.GetAccountPerformanceStatsBatch(context.Background(), []int64{1}, end.Add(-time.Hour), end)
			require.Error(t, err)
			require.Nil(t, result)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountPerformanceRepositoryEmpty(t *testing.T) {
	repo := &usageLogRepository{}
	stats, err := repo.GetAccountPerformanceStatsBatch(context.Background(), nil, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Empty(t, stats)
}
