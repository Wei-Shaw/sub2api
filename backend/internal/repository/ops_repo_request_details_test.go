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
		"routing_schedule_layer",
		"routing_selected_account_id",
		"routing_selected_account_name",
		"routing_effective_model",
		"routing_failover_count",
		"routing_failover_final_reason",
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
		"load_balance",
		int64(66),
		"acc-66",
		"gpt-5.4",
		int64(1),
		"upstream_502",
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
	require.Equal(t, "load_balance", items[0].RoutingScheduleLayer)
	require.NotNil(t, items[0].RoutingSelectedAccountID)
	require.Equal(t, int64(66), *items[0].RoutingSelectedAccountID)
	require.NotNil(t, items[0].RoutingSelectedAccountName)
	require.Equal(t, "acc-66", *items[0].RoutingSelectedAccountName)
	require.Equal(t, "gpt-5.4", items[0].RoutingEffectiveModel)
	require.NotNil(t, items[0].RoutingFailoverCount)
	require.Equal(t, 1, *items[0].RoutingFailoverCount)
	require.Equal(t, "upstream_502", items[0].RoutingFailoverFinalReason)
	require.NoError(t, mock.ExpectationsWereMet())
}
