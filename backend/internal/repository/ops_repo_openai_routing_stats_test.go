package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetOpenAIRoutingStats_GroupsBySelectedGroupIncludingReserve(t *testing.T) {
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
		"routing_selected_group",
		"request_count",
		"total_tokens",
		"input_tokens",
		"output_tokens",
		"retried_request_count",
		"retry_count",
	}).
		AddRow("active", "active", int64(10), int64(1200), int64(700), int64(500), int64(3), int64(5)).
		AddRow("exhausted", "exhausted", int64(4), int64(320), int64(200), int64(120), int64(2), int64(7)).
		AddRow("exhausted", "reserve", int64(6), int64(540), int64(300), int64(240), int64(4), int64(9))

	mock.ExpectQuery(`SELECT\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) AS routing_target_group,\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_selected_group, ''\), COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\)\) AS routing_selected_group[\s\S]*LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) IN \('active','exhausted','any'\)[\s\S]*\(NULLIF\(ul\.routing_target_group, ''\) IS NOT NULL OR NULLIF\(ul\.routing_selected_group, ''\) IS NOT NULL\)`).
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
	require.Equal(t, int64(6), resp.RequestCountByGroup["reserve"])
	require.Equal(t, int64(1200), resp.TotalTokensByGroup["active"])
	require.Equal(t, int64(320), resp.TotalTokensByGroup["exhausted"])
	require.Equal(t, int64(540), resp.TotalTokensByGroup["reserve"])
	require.Equal(t, int64(700), resp.InputTokensByGroup["active"])
	require.Equal(t, int64(200), resp.InputTokensByGroup["exhausted"])
	require.Equal(t, int64(300), resp.InputTokensByGroup["reserve"])
	require.Equal(t, int64(500), resp.OutputTokensByGroup["active"])
	require.Equal(t, int64(120), resp.OutputTokensByGroup["exhausted"])
	require.Equal(t, int64(240), resp.OutputTokensByGroup["reserve"])
	require.Equal(t, int64(3), resp.RetriedRequestCountByGroup["active"])
	require.Equal(t, int64(2), resp.RetriedRequestCountByGroup["exhausted"])
	require.Equal(t, int64(4), resp.RetriedRequestCountByGroup["reserve"])
	require.Equal(t, int64(5), resp.RetryCountByGroup["active"])
	require.Equal(t, int64(7), resp.RetryCountByGroup["exhausted"])
	require.Equal(t, int64(9), resp.RetryCountByGroup["reserve"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetOpenAIRoutingStats_ReserveRoutingBreakdownIncludesAnyTarget(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	filter := &service.OpsOpenAIRoutingStatsFilter{
		TimeRange: "1h",
		StartTime: start,
		EndTime:   end,
		Platform:  "openai",
	}

	rows := sqlmock.NewRows([]string{
		"routing_target_group",
		"routing_selected_group",
		"request_count",
		"total_tokens",
		"input_tokens",
		"output_tokens",
		"retried_request_count",
		"retry_count",
	}).
		AddRow("active", "reserve", int64(3), int64(300), int64(180), int64(120), int64(1), int64(1)).
		AddRow("any", "reserve", int64(2), int64(200), int64(120), int64(80), int64(1), int64(2)).
		AddRow("exhausted", "reserve", int64(4), int64(400), int64(240), int64(160), int64(2), int64(3))

	mock.ExpectQuery(`SELECT\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) AS routing_target_group,\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_selected_group, ''\), COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\)\) AS routing_selected_group[\s\S]*LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) IN \('active','exhausted','any'\)[\s\S]*\(NULLIF\(ul\.routing_target_group, ''\) IS NOT NULL OR NULLIF\(ul\.routing_selected_group, ''\) IS NOT NULL\)`).
		WithArgs(start, end, "openai").
		WillReturnRows(rows)

	resp, err := repo.GetOpenAIRoutingStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(9), resp.RequestCountByGroup["reserve"])
	require.Len(t, resp.Breakdown, 3)
	require.Equal(t, service.OpsOpenAIRoutingStatsBreakdown{TargetGroup: "active", SelectedGroup: "reserve", RequestCount: 3, TotalTokens: 300, InputTokens: 180, OutputTokens: 120, RetriedRequestCount: 1, RetryCount: 1}, resp.Breakdown[0])
	require.Equal(t, service.OpsOpenAIRoutingStatsBreakdown{TargetGroup: "any", SelectedGroup: "reserve", RequestCount: 2, TotalTokens: 200, InputTokens: 120, OutputTokens: 80, RetriedRequestCount: 1, RetryCount: 2}, resp.Breakdown[1])
	require.Equal(t, service.OpsOpenAIRoutingStatsBreakdown{TargetGroup: "exhausted", SelectedGroup: "reserve", RequestCount: 4, TotalTokens: 400, InputTokens: 240, OutputTokens: 160, RetriedRequestCount: 2, RetryCount: 3}, resp.Breakdown[2])

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

	mock.ExpectQuery(`SELECT\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) AS routing_target_group,\s+LOWER\(COALESCE\(NULLIF\(ul\.routing_selected_group, ''\), COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\)\) AS routing_selected_group[\s\S]*LOWER\(COALESCE\(NULLIF\(ul\.routing_target_group, ''\), 'any'\)\) IN \('active','exhausted','any'\)[\s\S]*\(NULLIF\(ul\.routing_target_group, ''\) IS NOT NULL OR NULLIF\(ul\.routing_selected_group, ''\) IS NOT NULL\)`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"routing_target_group",
			"routing_selected_group",
			"request_count",
			"total_tokens",
			"input_tokens",
			"output_tokens",
			"retried_request_count",
			"retry_count",
		}))

	resp, err := repo.GetOpenAIRoutingStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Contains(t, resp.RequestCountByGroup, "reserve")
	require.Contains(t, resp.TotalTokensByGroup, "reserve")
	require.Contains(t, resp.InputTokensByGroup, "reserve")
	require.Contains(t, resp.OutputTokensByGroup, "reserve")
	require.Contains(t, resp.RetriedRequestCountByGroup, "reserve")
	require.Contains(t, resp.RetryCountByGroup, "reserve")
	require.Equal(t, int64(0), resp.RequestCountByGroup["active"])
	require.Equal(t, int64(0), resp.RequestCountByGroup["exhausted"])
	require.Equal(t, int64(0), resp.RequestCountByGroup["reserve"])
	require.Equal(t, int64(0), resp.TotalTokensByGroup["active"])
	require.Equal(t, int64(0), resp.TotalTokensByGroup["exhausted"])
	require.Equal(t, int64(0), resp.TotalTokensByGroup["reserve"])
	require.Equal(t, int64(0), resp.RetriedRequestCountByGroup["active"])
	require.Equal(t, int64(0), resp.RetriedRequestCountByGroup["exhausted"])
	require.Equal(t, int64(0), resp.RetriedRequestCountByGroup["reserve"])
	require.Equal(t, int64(0), resp.RetryCountByGroup["active"])
	require.Equal(t, int64(0), resp.RetryCountByGroup["exhausted"])
	require.Equal(t, int64(0), resp.RetryCountByGroup["reserve"])

	require.NoError(t, mock.ExpectationsWereMet())
}
