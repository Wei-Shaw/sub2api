package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetOpenAIRoutingStats_GroupsByTargetGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	groupID := int64(9)

	filter := &service.OpsOpenAIRoutingStatsFilter{
		TimeRange: "1d",
		StartTime: start,
		EndTime:   end,
		Platform:  " OpenAI ",
		GroupID:   &groupID,
	}

	rows := sqlmock.NewRows([]string{
		"routing_target_group",
		"request_count",
		"total_tokens",
		"input_tokens",
		"output_tokens",
	}).
		AddRow("active", int64(10), int64(1200), int64(700), int64(500)).
		AddRow("exhausted", int64(4), int64(320), int64(200), int64(120))

	mock.ExpectQuery(`SELECT\s+LOWER\(ul\.routing_target_group\) AS routing_target_group`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(rows)

	resp, err := repo.GetOpenAIRoutingStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "1d", resp.TimeRange)
	require.Equal(t, "openai", resp.Platform)
	require.NotNil(t, resp.GroupID)
	require.Equal(t, groupID, *resp.GroupID)
	require.Equal(t, int64(10), resp.RequestCountByGroup["active"])
	require.Equal(t, int64(4), resp.RequestCountByGroup["exhausted"])
	require.Equal(t, int64(1200), resp.TotalTokensByGroup["active"])
	require.Equal(t, int64(320), resp.TotalTokensByGroup["exhausted"])
	require.Equal(t, int64(700), resp.InputTokensByGroup["active"])
	require.Equal(t, int64(200), resp.InputTokensByGroup["exhausted"])
	require.Equal(t, int64(500), resp.OutputTokensByGroup["active"])
	require.Equal(t, int64(120), resp.OutputTokensByGroup["exhausted"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetOpenAIRoutingStats_EmptyResult(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	filter := &service.OpsOpenAIRoutingStatsFilter{
		TimeRange: "30m",
		StartTime: start,
		EndTime:   end,
	}

	mock.ExpectQuery(`SELECT\s+LOWER\(ul\.routing_target_group\) AS routing_target_group`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"routing_target_group",
			"request_count",
			"total_tokens",
			"input_tokens",
			"output_tokens",
		}))

	resp, err := repo.GetOpenAIRoutingStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(0), resp.RequestCountByGroup["active"])
	require.Equal(t, int64(0), resp.RequestCountByGroup["exhausted"])
	require.Equal(t, int64(0), resp.TotalTokensByGroup["active"])
	require.Equal(t, int64(0), resp.TotalTokensByGroup["exhausted"])

	require.NoError(t, mock.ExpectationsWereMet())
}
