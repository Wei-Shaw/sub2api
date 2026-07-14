package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetUserUsageStats_TopNMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	groupID := int64(9)
	filter := &service.OpsUserUsageStatsFilter{
		TimeRange: "24h",
		StartTime: start,
		EndTime:   end,
		Platform:  " OpenAI ",
		GroupID:   &groupID,
		TopN:      20,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`ORDER BY actual_cost DESC, total_tokens DESC, user_id ASC\s+LIMIT \$5`).
		WithArgs(start, end, groupID, "openai", 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "username", "email", "request_count", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "actual_cost", "last_request_at",
		}).AddRow(
			int64(7), "alice", "alice@example.com", int64(12), int64(1000), int64(300),
			int64(200), int64(1500), 1.25, end.Add(-time.Minute),
		))

	resp, err := repo.GetUserUsageStats(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(2), resp.Total)
	require.Equal(t, "openai", resp.Platform)
	require.NotNil(t, resp.TopN)
	require.Equal(t, 20, *resp.TopN)
	require.Len(t, resp.Items, 1)
	require.Equal(t, int64(7), resp.Items[0].UserID)
	require.Equal(t, "alice@example.com", resp.Items[0].Email)
	require.Equal(t, int64(1500), resp.Items[0].TotalTokens)
	require.InDelta(t, 1.25, resp.Items[0].ActualCost, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetUserUsageStats_PaginationModeEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsUserUsageStatsFilter{
		TimeRange: "1h",
		StartTime: start,
		EndTime:   end,
		Page:      2,
		PageSize:  10,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM stats`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`ORDER BY actual_cost DESC, total_tokens DESC, user_id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "username", "email", "request_count", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "actual_cost", "last_request_at",
		}))

	resp, err := repo.GetUserUsageStats(context.Background(), filter)
	require.NoError(t, err)
	require.Empty(t, resp.Items)
	require.Equal(t, 2, resp.Page)
	require.Equal(t, 10, resp.PageSize)
	require.NoError(t, mock.ExpectationsWereMet())
}
