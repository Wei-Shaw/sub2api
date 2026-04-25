package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetails_IncludesRoutingFields(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	filter := &service.OpsRequestDetailFilter{Page: 1, PageSize: 10}

	mock.ExpectQuery("SELECT COUNT\\(1\\) FROM combined").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"kind",
		"created_at",
		"request_id",
		"platform",
		"model",
		"routing_target_group",
		"routing_selected_group",
		"routing_schedule_layer",
		"routing_selected_account_id",
		"routing_selected_account_name",
		"routing_effective_model",
		"routing_failover_count",
		"routing_failover_final_reason",
		"sticky_session_source",
		"sticky_session_hash_present",
		"sticky_eval_result",
		"sticky_selected_account_changed",
		"sticky_parent_session_present",
		"sticky_parent_session_key",
		"duration_ms",
		"status_code",
		"error_id",
		"phase",
		"severity",
		"message",
		"user_id",
		"api_key_id",
		"account_id",
		"group_id",
		"stream",
	}).AddRow(
		"error",
		time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		"req-routing-1",
		"openai",
		"gpt-5.4-Sys",
		"exhausted",
		"reserve",
		"load_balance",
		int64(66),
		"acc-66",
		"gpt-5.4",
		int64(1),
		"upstream_502",
		"header_x_session_affinity",
		true,
		"hit",
		false,
		true,
		"parent_abc",
		int64(1800),
		502,
		int64(9),
		"upstream",
		"error",
		"boom",
		int64(1),
		int64(2),
		int64(66),
		int64(3),
		false,
	)

	mock.ExpectQuery("SELECT[\\s\\S]*routing_target_group[\\s\\S]*FROM combined").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "exhausted", items[0].RoutingTargetGroup)
	require.Equal(t, "reserve", items[0].RoutingSelectedGroup)
	require.Equal(t, "load_balance", items[0].RoutingScheduleLayer)
	require.NotNil(t, items[0].RoutingSelectedAccountID)
	require.Equal(t, int64(66), *items[0].RoutingSelectedAccountID)
	require.NotNil(t, items[0].RoutingSelectedAccountName)
	require.Equal(t, "acc-66", *items[0].RoutingSelectedAccountName)
	require.Equal(t, "gpt-5.4", items[0].RoutingEffectiveModel)
	require.NotNil(t, items[0].RoutingFailoverCount)
	require.Equal(t, 1, *items[0].RoutingFailoverCount)
	require.Equal(t, "upstream_502", items[0].RoutingFailoverFinalReason)
	require.NotNil(t, items[0].StickySessionSource)
	require.Equal(t, "header_x_session_affinity", *items[0].StickySessionSource)
	require.NotNil(t, items[0].StickySessionHashPresent)
	require.True(t, *items[0].StickySessionHashPresent)
	require.NotNil(t, items[0].StickyEvalResult)
	require.Equal(t, "hit", *items[0].StickyEvalResult)
	require.NotNil(t, items[0].StickySelectedAccountChanged)
	require.False(t, *items[0].StickySelectedAccountChanged)
	require.NotNil(t, items[0].StickyParentSessionPresent)
	require.True(t, *items[0].StickyParentSessionPresent)
	require.NotNil(t, items[0].StickyParentSessionKey)
	require.Equal(t, "parent_abc", *items[0].StickyParentSessionKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetails_FiltersRoutingSelectedGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	filter := &service.OpsRequestDetailFilter{
		Page:                 1,
		PageSize:             10,
		RoutingSelectedGroup: "reserve",
	}

	mock.ExpectQuery("SELECT COUNT\\(1\\) FROM combined WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_selected_group,''\\), routing_target_group\\)\\) = \\$3").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "reserve").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM combined[\\s\\S]*WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_selected_group,''\\), routing_target_group\\)\\) = \\$3").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "reserve", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind",
			"created_at",
			"request_id",
			"platform",
			"model",
			"routing_target_group",
			"routing_selected_group",
			"routing_schedule_layer",
			"routing_selected_account_id",
			"routing_selected_account_name",
			"routing_effective_model",
			"routing_failover_count",
			"routing_failover_final_reason",
			"sticky_session_source",
			"sticky_session_hash_present",
			"sticky_eval_result",
			"sticky_selected_account_changed",
			"sticky_parent_session_present",
			"sticky_parent_session_key",
			"duration_ms",
			"status_code",
			"error_id",
			"phase",
			"severity",
			"message",
			"user_id",
			"api_key_id",
			"account_id",
			"group_id",
			"stream",
		}))

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(0), total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetails_ReserveRoutingOnlyIncludesAnyTarget(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	filter := &service.OpsRequestDetailFilter{Page: 1, PageSize: 10, OpenAIRoutingOnly: true}
	createdAt := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(1\\) FROM combined WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_target_group,''\\), 'any'\\)\\) IN \\('active','exhausted','any'\\) AND \\(NULLIF\\(routing_target_group,''\\) IS NOT NULL OR NULLIF\\(routing_selected_group,''\\) IS NOT NULL\\)").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	rows := newOpsRequestDetailsRows().
		AddRow("success", createdAt, "req-active-reserve", "openai", "gpt-5.5", "active", "reserve", "load_balance", int64(66), "acc-66", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(66), int64(3), false).
		AddRow("success", createdAt.Add(time.Second), "req-any-reserve", "openai", "gpt-5.5", "any", "reserve", "load_balance", int64(67), "acc-67", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(67), int64(3), false).
		AddRow("success", createdAt.Add(2*time.Second), "req-exhausted-reserve", "openai", "gpt-5.5-Sys", "exhausted", "reserve", "load_balance", int64(68), "acc-68", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(68), int64(3), false)

	mock.ExpectQuery("SELECT[\\s\\S]*CASE WHEN NULLIF\\(ul\\.routing_target_group, ''\\) IS NULL AND NULLIF\\(ul\\.routing_selected_group, ''\\) IS NOT NULL THEN 'any'[\\s\\S]*CASE WHEN NULLIF\\(o\\.routing_target_group, ''\\) IS NULL AND NULLIF\\(o\\.routing_selected_group, ''\\) IS NOT NULL THEN 'any'[\\s\\S]*FROM combined[\\s\\S]*WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_target_group,''\\), 'any'\\)\\) IN \\('active','exhausted','any'\\) AND \\(NULLIF\\(routing_target_group,''\\) IS NOT NULL OR NULLIF\\(routing_selected_group,''\\) IS NOT NULL\\)").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	require.Equal(t, "active", items[0].RoutingTargetGroup)
	require.Equal(t, "reserve", items[0].RoutingSelectedGroup)
	require.Equal(t, "any", items[1].RoutingTargetGroup)
	require.Equal(t, "reserve", items[1].RoutingSelectedGroup)
	require.Equal(t, "exhausted", items[2].RoutingTargetGroup)
	require.Equal(t, "reserve", items[2].RoutingSelectedGroup)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetails_ReserveRoutingOnlyNormalizesBlankAnyTarget(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	filter := &service.OpsRequestDetailFilter{Page: 1, PageSize: 10, OpenAIRoutingOnly: true}
	createdAt := time.Date(2026, 4, 25, 13, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(1\\) FROM combined WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_target_group,''\\), 'any'\\)\\) IN \\('active','exhausted','any'\\) AND \\(NULLIF\\(routing_target_group,''\\) IS NOT NULL OR NULLIF\\(routing_selected_group,''\\) IS NOT NULL\\)").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	rows := newOpsRequestDetailsRows().
		AddRow("success", createdAt, "req-null-any-reserve", "openai", "gpt-5.5", "any", "reserve", "load_balance", int64(70), "acc-70", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(70), int64(3), false).
		AddRow("success", createdAt.Add(time.Second), "req-empty-any-reserve", "openai", "gpt-5.5", "any", "reserve", "load_balance", int64(71), "acc-71", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(71), int64(3), false).
		AddRow("success", createdAt.Add(2*time.Second), "req-explicit-any-reserve", "openai", "gpt-5.5", "any", "reserve", "load_balance", int64(72), "acc-72", "gpt-5.5", int64(0), "", nil, nil, nil, nil, nil, nil, int64(100), nil, nil, nil, nil, nil, int64(1), int64(2), int64(72), int64(3), false)

	mock.ExpectQuery("SELECT[\\s\\S]*CASE WHEN NULLIF\\(ul\\.routing_target_group, ''\\) IS NULL AND NULLIF\\(ul\\.routing_selected_group, ''\\) IS NOT NULL THEN 'any'[\\s\\S]*FROM combined[\\s\\S]*WHERE LOWER\\(COALESCE\\(NULLIF\\(routing_target_group,''\\), 'any'\\)\\) IN \\('active','exhausted','any'\\) AND \\(NULLIF\\(routing_target_group,''\\) IS NOT NULL OR NULLIF\\(routing_selected_group,''\\) IS NOT NULL\\)").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	for _, item := range items {
		require.Equal(t, "any", item.RoutingTargetGroup)
		require.Equal(t, "reserve", item.RoutingSelectedGroup)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func newOpsRequestDetailsRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"kind",
		"created_at",
		"request_id",
		"platform",
		"model",
		"routing_target_group",
		"routing_selected_group",
		"routing_schedule_layer",
		"routing_selected_account_id",
		"routing_selected_account_name",
		"routing_effective_model",
		"routing_failover_count",
		"routing_failover_final_reason",
		"sticky_session_source",
		"sticky_session_hash_present",
		"sticky_eval_result",
		"sticky_selected_account_changed",
		"sticky_parent_session_present",
		"sticky_parent_session_key",
		"duration_ms",
		"status_code",
		"error_id",
		"phase",
		"severity",
		"message",
		"user_id",
		"api_key_id",
		"account_id",
		"group_id",
		"stream",
	})
}
