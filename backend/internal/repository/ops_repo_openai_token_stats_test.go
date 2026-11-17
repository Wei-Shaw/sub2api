package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetOpenAITokenStats_PlatformScope(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	groupID := int64(9)
	tests := []struct {
		name     string
		platform string
		groupID  *int64
		models   []string
	}{
		{name: "all platforms", models: []string{"claude-sonnet-4", "deepseek-chat", "gemini-2.5-pro", "gpt-4o", "o3"}},
		{name: "anthropic group", platform: "anthropic", groupID: &groupID, models: []string{"claude-sonnet-4"}},
		{name: "gemini", platform: "gemini", models: []string{"gemini-2.5-pro"}},
		{name: "openai reasoning model", platform: "openai", models: []string{"o3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &opsRepository{db: db}
			filter := &service.OpsOpenAITokenStatsFilter{
				TimeRange: "1h",
				StartTime: start,
				EndTime:   end,
				Platform:  tt.platform,
				GroupID:   tt.groupID,
				TopN:      20,
			}

			// Match the complete filtering clause in both queries so an implicit
			// model restriction cannot silently narrow the selected platform scope.
			where := "WHERE ul.created_at >= $1 AND ul.created_at < $2"
			args := []driver.Value{start, end}
			if tt.groupID != nil {
				where += " AND ul.group_id = $3"
				args = append(args, *tt.groupID)
			}
			if tt.platform != "" {
				placeholder := "$3"
				if tt.groupID != nil {
					placeholder = "$4"
				}
				where += " AND COALESCE(NULLIF(g.platform,''), a.platform) = " + placeholder
				args = append(args, tt.platform)
			}
			queryPrefix := regexp.QuoteMeta(where) + `\s+GROUP BY ul.model\s+\)`
			mock.ExpectQuery(queryPrefix + `\s+SELECT COUNT\(\*\) FROM stats`).
				WithArgs(args...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(len(tt.models)))

			rows := sqlmock.NewRows([]string{
				"model", "request_count", "avg_tokens_per_sec", "avg_first_token_ms",
				"total_output_tokens", "avg_duration_ms", "requests_with_first_token",
			})
			for _, model := range tt.models {
				rows.AddRow(model, int64(2), 10.0, 100.0, int64(20), int64(1000), int64(2))
			}
			mock.ExpectQuery(queryPrefix + `\s+SELECT model,`).
				WithArgs(append(args, 20)...).
				WillReturnRows(rows)

			resp, err := repo.GetOpenAITokenStats(context.Background(), filter)
			require.NoError(t, err)
			require.Equal(t, tt.platform, resp.Platform)
			require.Equal(t, tt.groupID, resp.GroupID)
			require.Equal(t, int64(len(tt.models)), resp.Total)
			require.Len(t, resp.Items, len(tt.models))
			for i, model := range tt.models {
				require.Equal(t, model, resp.Items[i].Model)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpsRepositoryGetOpenAITokenStats_PaginationMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	groupID := int64(9)

	filter := &service.OpsOpenAITokenStatsFilter{
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
		"model",
		"request_count",
		"avg_tokens_per_sec",
		"avg_first_token_ms",
		"total_output_tokens",
		"avg_duration_ms",
		"requests_with_first_token",
	}).
		AddRow("gpt-4o-mini", int64(20), 21.56, 120.34, int64(3000), int64(850), int64(18)).
		AddRow("gpt-4.1", int64(20), 10.2, 240.0, int64(2500), int64(900), int64(20))

	mock.ExpectQuery(`ORDER BY request_count DESC, model ASC\s+LIMIT \$5 OFFSET \$6`).
		WithArgs(start, end, groupID, "openai", 10, 10).
		WillReturnRows(rows)

	resp, err := repo.GetOpenAITokenStats(context.Background(), filter)
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
	require.Equal(t, "gpt-4o-mini", resp.Items[0].Model)
	require.NotNil(t, resp.Items[0].AvgTokensPerSec)
	require.InDelta(t, 21.56, *resp.Items[0].AvgTokensPerSec, 0.0001)
	require.NotNil(t, resp.Items[0].AvgFirstTokenMs)
	require.InDelta(t, 120.34, *resp.Items[0].AvgFirstTokenMs, 0.0001)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetOpenAITokenStats_TopNMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsOpenAITokenStatsFilter{
		TimeRange: "1h",
		StartTime: start,
		EndTime:   end,
		TopN:      5,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"model",
		"request_count",
		"avg_tokens_per_sec",
		"avg_first_token_ms",
		"total_output_tokens",
		"avg_duration_ms",
		"requests_with_first_token",
	}).
		AddRow("gpt-4o", int64(5), nil, nil, int64(0), int64(0), int64(0))

	mock.ExpectQuery(`ORDER BY request_count DESC, model ASC\s+LIMIT \$3`).
		WithArgs(start, end, 5).
		WillReturnRows(rows)

	resp, err := repo.GetOpenAITokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.TopN)
	require.Equal(t, 5, *resp.TopN)
	require.Equal(t, 0, resp.Page)
	require.Equal(t, 0, resp.PageSize)
	require.Len(t, resp.Items, 1)
	require.Nil(t, resp.Items[0].AvgTokensPerSec)
	require.Nil(t, resp.Items[0].AvgFirstTokenMs)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetOpenAITokenStats_EmptyResult(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	filter := &service.OpsOpenAITokenStatsFilter{
		TimeRange: "30m",
		StartTime: start,
		EndTime:   end,
		Page:      1,
		PageSize:  20,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	mock.ExpectQuery(`ORDER BY request_count DESC, model ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"model",
			"request_count",
			"avg_tokens_per_sec",
			"avg_first_token_ms",
			"total_output_tokens",
			"avg_duration_ms",
			"requests_with_first_token",
		}))

	resp, err := repo.GetOpenAITokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(0), resp.Total)
	require.Len(t, resp.Items, 0)
	require.Equal(t, 1, resp.Page)
	require.Equal(t, 20, resp.PageSize)

	require.NoError(t, mock.ExpectationsWereMet())
}
