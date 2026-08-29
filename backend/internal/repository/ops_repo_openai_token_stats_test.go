package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetTokenStats_PaginationMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	groupID := int64(9)

	filter := &service.OpsTokenStatsFilter{
		TimeRange: "1d",
		StartTime: start,
		EndTime:   end,
		Platform:  " OpenAI ",
		GroupID:   &groupID,
		Page:      2,
		PageSize:  10,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	rows := sqlmock.NewRows([]string{
		"platform",
		"model",
		"request_count",
		"avg_tokens_per_sec",
		"avg_first_token_ms",
		"total_output_tokens",
		"avg_duration_ms",
		"requests_with_first_token",
	}).
		AddRow("openai", "gpt-4o-mini", int64(20), 21.56, 120.34, int64(3000), int64(850), int64(18)).
		AddRow("openai", "o3", int64(20), 10.2, 240.0, int64(2500), int64(900), int64(20))

	mock.ExpectQuery(`(?s)LOWER\(TRIM\(CASE WHEN g\.platform = 'composite' THEN a\.platform ELSE COALESCE\(NULLIF\(g\.platform,''\), a\.platform\) END\)\) = \$4.*NULLIF\(TRIM\(COALESCE\(CASE WHEN.*IS NOT NULL.*ul\.input_tokens.*> 0.*GROUP BY LOWER\(TRIM\(CASE WHEN.*COALESCE\(NULLIF\(TRIM\(ul\.requested_model\), ''\), ul\.model\).*ORDER BY request_count DESC, platform ASC, model ASC\s+LIMIT \$5 OFFSET \$6`).
		WithArgs(start, end, groupID, "openai", 10, 10).
		WillReturnRows(rows)

	resp, err := repo.GetTokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(3), resp.Total)
	require.Equal(t, 2, resp.Page)
	require.Equal(t, 10, resp.PageSize)
	require.Nil(t, resp.TopN)
	require.Equal(t, "openai", resp.Platform)
	require.NotNil(t, resp.GroupID)
	require.Equal(t, groupID, *resp.GroupID)
	require.Len(t, resp.Items, 2)
	require.Equal(t, "openai", resp.Items[0].Platform)
	require.Equal(t, "gpt-4o-mini", resp.Items[0].Model)
	require.NotNil(t, resp.Items[0].AvgTokensPerSec)
	require.InDelta(t, 21.56, *resp.Items[0].AvgTokensPerSec, 0.0001)
	require.NotNil(t, resp.Items[0].AvgFirstTokenMs)
	require.InDelta(t, 120.34, *resp.Items[0].AvgFirstTokenMs, 0.0001)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetTokenStats_TopNMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsTokenStatsFilter{
		TimeRange: "1h",
		StartTime: start,
		EndTime:   end,
		TopN:      5,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"platform",
		"model",
		"request_count",
		"avg_tokens_per_sec",
		"avg_first_token_ms",
		"total_output_tokens",
		"avg_duration_ms",
		"requests_with_first_token",
	}).
		AddRow("deepseek", "custom-chat-alias", int64(5), nil, nil, int64(7), int64(0), int64(0))

	mock.ExpectQuery(`(?s)COALESCE\(NULLIF\(TRIM\(ul\.requested_model\), ''\), ul\.model\) AS model.*NULLIF\(TRIM\(COALESCE\(CASE WHEN.*IS NOT NULL.*ORDER BY request_count DESC, platform ASC, model ASC\s+LIMIT \$3`).
		WithArgs(start, end, 5).
		WillReturnRows(rows)

	resp, err := repo.GetTokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.TopN)
	require.Equal(t, 5, *resp.TopN)
	require.Equal(t, 0, resp.Page)
	require.Equal(t, 0, resp.PageSize)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "deepseek", resp.Items[0].Platform)
	require.Equal(t, "custom-chat-alias", resp.Items[0].Model)
	require.Nil(t, resp.Items[0].AvgTokensPerSec)
	require.Nil(t, resp.Items[0].AvgFirstTokenMs)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetTokenStats_EmptyResult(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	filter := &service.OpsTokenStatsFilter{
		TimeRange: "30m",
		StartTime: start,
		EndTime:   end,
		Page:      1,
		PageSize:  20,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	mock.ExpectQuery(`ORDER BY request_count DESC, platform ASC, model ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"platform",
			"model",
			"request_count",
			"avg_tokens_per_sec",
			"avg_first_token_ms",
			"total_output_tokens",
			"avg_duration_ms",
			"requests_with_first_token",
		}))

	resp, err := repo.GetTokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(0), resp.Total)
	require.Len(t, resp.Items, 0)
	require.Equal(t, 1, resp.Page)
	require.Equal(t, 20, resp.PageSize)

	require.NoError(t, mock.ExpectationsWereMet())
}
