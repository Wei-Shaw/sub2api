package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetOpenAIStickyStats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	filter := &service.OpsOpenAIStickyStatsFilter{
		TimeRange: "24h",
		StartTime: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		Platform:  "openai",
	}

	rows := sqlmock.NewRows([]string{
		"sticky_session_source",
		"sticky_eval_result",
		"sticky_selected_account_changed",
		"request_count",
	}).
		AddRow("header_x_session_affinity", "hit", false, int64(8)).
		AddRow("header_x_session_affinity", "miss_binding_invalid", true, int64(2)).
		AddRow("content_fallback", "miss_no_binding", false, int64(3)).
		AddRow("", "no_session_signal", false, int64(5)).
		AddRow("header_x_session_affinity", "bypassed_previous_response_id", false, int64(4))

	mock.ExpectQuery(`SELECT[\s\S]*sticky_session_source[\s\S]*sticky_eval_result[\s\S]*FROM combined`).
		WithArgs(filter.StartTime, filter.EndTime, filter.Platform).
		WillReturnRows(rows)

	resp, err := repo.GetOpenAIStickyStats(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(13), resp.EvaluatedRequestCount)
	require.Equal(t, int64(8), resp.StickyHitCount)
	require.Equal(t, int64(2), resp.StickyAccountSwitchCount)
	require.InDelta(t, 8.0/13.0, resp.StickyHitRate, 1e-12)
	require.InDelta(t, 2.0/13.0, resp.StickyAccountSwitchRate, 1e-12)
	require.Equal(t, int64(8), resp.EvalResultCount["hit"])
	require.Equal(t, int64(2), resp.EvalResultCount["miss_binding_invalid"])
	require.Equal(t, int64(3), resp.EvalResultCount["miss_no_binding"])
	require.Equal(t, int64(5), resp.EvalResultCount["no_session_signal"])
	require.Equal(t, int64(4), resp.EvalResultCount["bypassed_previous_response_id"])
	require.Equal(t, int64(14), resp.SessionSourceCount["header_x_session_affinity"])
	require.Equal(t, int64(3), resp.SessionSourceCount["content_fallback"])
	require.NoError(t, mock.ExpectationsWereMet())
}
