package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryCreateSyncRequestTypeAndLegacyFields(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	createdAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	log := &service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-1",
		Model:          "gpt-5",
		RequestedModel: "gpt-5",
		InputTokens:    10,
		OutputTokens:   20,
		TotalCost:      1,
		ActualCost:     1,
		BillingType:    service.BillingTypeBalance,
		RequestType:    service.RequestTypeWSV2,
		Stream:         false,
		OpenAIWSMode:   false,
		CreatedAt:      createdAt,
	}
	prepared := prepareUsageLogInsert(log)

	mock.ExpectQuery("INSERT INTO usage_logs").
		WithArgs(anySliceToDriverValues(prepared.args)...).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(99), createdAt))

	inserted, err := repo.Create(context.Background(), log)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, int64(99), log.ID)
	require.Nil(t, log.ServiceTier)
	require.Equal(t, service.RequestTypeWSV2, log.RequestType)
	require.True(t, log.Stream)
	require.True(t, log.OpenAIWSMode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryCreate_PersistsServiceTier(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	createdAt := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	serviceTier := "priority"
	log := &service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-service-tier",
		Model:          "gpt-5.4",
		RequestedModel: "gpt-5.4",
		ServiceTier:    &serviceTier,
		CreatedAt:      createdAt,
	}
	prepared := prepareUsageLogInsert(log)

	mock.ExpectQuery("INSERT INTO usage_logs").
		WithArgs(anySliceToDriverValues(prepared.args)...).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(100), createdAt))

	inserted, err := repo.Create(context.Background(), log)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildUsageLogBestEffortInsertQuery_IncludesRequestedModelColumn(t *testing.T) {
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-best-effort-query",
		Model:          "gpt-5",
		RequestedModel: "gpt-5",
		CreatedAt:      time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC),
	})

	query, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})

	require.Contains(t, query, "INSERT INTO usage_logs (")
	require.Contains(t, query, "WITH input (\n\t\t\tuser_id,\n\t\t\tapi_key_id,\n\t\t\taccount_id,\n\t\t\trequest_id,\n\t\t\tmodel,\n\t\t\trequested_model,\n\t\t\tupstream_model,\n\t\t\tchannel_id,\n\t\t\tmodel_mapping_chain,\n\t\t\tbilling_tier,\n\t\t\tbilling_mode,")
	require.Contains(t, query, "\n\t\t\tcache_creation_5m_tokens,\n\t\t\tcache_creation_1h_tokens,\n\t\t\timage_output_tokens,\n\t\t\timage_output_cost,")
	require.Contains(t, query, "\n\t\t\tactual_cost,\n\t\t\trate_multiplier,\n\t\t\taccount_rate_multiplier,\n\t\t\tpriority_account_multiplier,\n\t\t\teffective_multiplier,\n\t\t\teffective_input_unit_price,\n\t\t\teffective_output_unit_price,\n\t\t\teffective_cache_read_unit_price,\n\t\t\tpricing_source,\n\t\t\tbilling_type,")
	require.Contains(t, query, "\n\t\t\taccount_rate_multiplier,\n\t\t\tpriority_account_multiplier,\n\t\t\teffective_multiplier,")
	require.Contains(t, query, "\n\t\t\teffective_input_unit_price,\n\t\t\teffective_output_unit_price,\n\t\t\teffective_cache_read_unit_price,\n\t\t\tpricing_source,")
	require.Contains(t, query, "\n\t\t\tmodel,\n\t\t\trequested_model,\n\t\t\tupstream_model,\n\t\t\tchannel_id,")
	require.Len(t, args, len(prepared.args))
	require.Equal(t, prepared.args[5], args[5])
}

func TestExecUsageLogInsertNoResult_PersistsRequestedModel(t *testing.T) {
	db, mock := newSQLMock(t)
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-best-effort-exec",
		Model:          "gpt-5",
		RequestedModel: "gpt-5",
		CreatedAt:      time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC),
	})

	mock.ExpectExec("INSERT INTO usage_logs[\\s\\S]*actual_cost,[\\s\\S]*rate_multiplier,[\\s\\S]*account_rate_multiplier,[\\s\\S]*priority_account_multiplier,[\\s\\S]*effective_multiplier,[\\s\\S]*effective_input_unit_price,[\\s\\S]*effective_output_unit_price,[\\s\\S]*effective_cache_read_unit_price,[\\s\\S]*pricing_source,[\\s\\S]*billing_type").
		WithArgs(anySliceToDriverValues(prepared.args)...).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := execUsageLogInsertNoResult(context.Background(), db, prepared)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExecUsageLogInsertNoResult_IncludesBillingBreakdownColumns(t *testing.T) {
	db, mock := newSQLMock(t)
	priorityAccountMultiplier := 100.0
	effectiveMultiplier := 150.0
	effectiveInputUnitPrice := 5e-6
	effectiveOutputUnitPrice := 30e-6
	effectiveCacheReadUnitPrice := 0.5e-6
	pricingSource := "priority_pricing,priority_account_multiplier"

	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:                      1,
		APIKeyID:                    2,
		AccountID:                   3,
		RequestID:                   "req-best-effort-billing-columns",
		Model:                       "gpt-5.4",
		RequestedModel:              "gpt-5.4",
		PriorityAccountMultiplier:   &priorityAccountMultiplier,
		EffectiveMultiplier:         &effectiveMultiplier,
		EffectiveInputUnitPrice:     &effectiveInputUnitPrice,
		EffectiveOutputUnitPrice:    &effectiveOutputUnitPrice,
		EffectiveCacheReadUnitPrice: &effectiveCacheReadUnitPrice,
		PricingSource:               &pricingSource,
		CreatedAt:                   time.Date(2025, 1, 4, 12, 30, 0, 0, time.UTC),
	})

	mock.ExpectExec("INSERT INTO usage_logs").
		WithArgs(anySliceToDriverValues(prepared.args)...).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := execUsageLogInsertNoResult(context.Background(), db, prepared)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareUsageLogInsert_ArgCountMatchesTypes(t *testing.T) {
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-arg-count",
		Model:          "gpt-5",
		RequestedModel: "gpt-5",
		CreatedAt:      time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC),
	})

	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
}

func TestPrepareUsageLogInsert_IncludesOpenAIRoutingFields(t *testing.T) {
	routingTargetGroup := "exhausted"
	routingScheduleLayer := "load_balance"
	routingAccountID := int64(66)
	routingAccountName := "acc-66"
	routingEffectiveModel := "gpt-5.4"
	routingFailoverCount := 1
	routingFailoverFinalReason := "selected_exhausted_fallback"

	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:                     1,
		APIKeyID:                   2,
		AccountID:                  3,
		RequestID:                  "req-routing-fields",
		Model:                      "gpt-5.4-Sys",
		RequestedModel:             "gpt-5.4-Sys",
		RoutingTargetGroup:         &routingTargetGroup,
		RoutingScheduleLayer:       &routingScheduleLayer,
		RoutingSelectedAccountID:   &routingAccountID,
		RoutingSelectedAccountName: &routingAccountName,
		RoutingEffectiveModel:      &routingEffectiveModel,
		RoutingFailoverCount:       &routingFailoverCount,
		RoutingFailoverFinalReason: &routingFailoverFinalReason,
		CreatedAt:                  time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC),
	})

	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	require.Equal(t, sql.NullString{String: routingTargetGroup, Valid: true}, prepared.args[49])
	require.Equal(t, sql.NullString{String: routingScheduleLayer, Valid: true}, prepared.args[50])
	require.Equal(t, sql.NullInt64{Int64: routingAccountID, Valid: true}, prepared.args[51])
	require.Equal(t, sql.NullString{String: routingAccountName, Valid: true}, prepared.args[52])
	require.Equal(t, sql.NullString{String: routingEffectiveModel, Valid: true}, prepared.args[53])
	require.Equal(t, sql.NullInt64{Int64: int64(routingFailoverCount), Valid: true}, prepared.args[54])
	require.Equal(t, sql.NullString{String: routingFailoverFinalReason, Valid: true}, prepared.args[55])
}

func TestPrepareUsageLogInsert_IncludesBillingBreakdownFields(t *testing.T) {
	priorityAccountMultiplier := 100.0
	effectiveMultiplier := 150.0
	effectiveInputUnitPrice := 5e-6
	effectiveOutputUnitPrice := 30e-6
	effectiveCacheReadUnitPrice := 0.5e-6
	pricingSource := "priority_pricing,priority_account_multiplier"

	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:                      1,
		APIKeyID:                    2,
		AccountID:                   3,
		RequestID:                   "req-billing-breakdown",
		Model:                       "gpt-5.4-Sys",
		RequestedModel:              "gpt-5.4-Sys",
		PriorityAccountMultiplier:   &priorityAccountMultiplier,
		EffectiveMultiplier:         &effectiveMultiplier,
		EffectiveInputUnitPrice:     &effectiveInputUnitPrice,
		EffectiveOutputUnitPrice:    &effectiveOutputUnitPrice,
		EffectiveCacheReadUnitPrice: &effectiveCacheReadUnitPrice,
		PricingSource:               &pricingSource,
		CreatedAt:                   time.Date(2025, 1, 6, 13, 0, 0, 0, time.UTC),
	})

	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	require.Equal(t, sql.NullFloat64{Float64: priorityAccountMultiplier, Valid: true}, prepared.args[29])
	require.Equal(t, sql.NullFloat64{Float64: effectiveMultiplier, Valid: true}, prepared.args[30])
	require.Equal(t, sql.NullFloat64{Float64: effectiveInputUnitPrice, Valid: true}, prepared.args[31])
	require.Equal(t, sql.NullFloat64{Float64: effectiveOutputUnitPrice, Valid: true}, prepared.args[32])
	require.Equal(t, sql.NullFloat64{Float64: effectiveCacheReadUnitPrice, Valid: true}, prepared.args[33])
	require.Equal(t, sql.NullString{String: pricingSource, Valid: true}, prepared.args[34])
}

func TestPrepareUsageLogInsert_IncludesChannelAndImageOutputFields(t *testing.T) {
	channelID := int64(55)
	modelMappingChain := "alias->mapped"
	billingTier := "channel_mapped"
	billingMode := "image"

	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:            1,
		APIKeyID:          2,
		AccountID:         3,
		RequestID:         "req-channel-image",
		Model:             "gpt-image-1",
		RequestedModel:    "gpt-image-1",
		ImageOutputTokens: 42,
		ImageOutputCost:   0.42,
		ChannelID:         &channelID,
		ModelMappingChain: &modelMappingChain,
		BillingTier:       &billingTier,
		BillingMode:       &billingMode,
		CreatedAt:         time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC),
	})

	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	require.Equal(t, sql.NullInt64{Valid: true, Int64: channelID}, prepared.args[7])
	require.Equal(t, sql.NullString{Valid: true, String: modelMappingChain}, prepared.args[8])
	require.Equal(t, sql.NullString{Valid: true, String: billingTier}, prepared.args[9])
	require.Equal(t, sql.NullString{Valid: true, String: billingMode}, prepared.args[10])
	require.Equal(t, 42, prepared.args[19])
	require.Equal(t, 0.42, prepared.args[20])
}

func TestCoalesceTrimmedString(t *testing.T) {
	require.Equal(t, "fallback", coalesceTrimmedString(sql.NullString{}, "fallback"))
	require.Equal(t, "fallback", coalesceTrimmedString(sql.NullString{Valid: true, String: "   "}, "fallback"))
	require.Equal(t, "value", coalesceTrimmedString(sql.NullString{Valid: true, String: "value"}, "fallback"))
}

func anySliceToDriverValues(values []any) []driver.Value {
	out := make([]driver.Value, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func TestUsageLogRepositoryListWithFiltersRequestTypePriority(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	requestType := int16(service.RequestTypeWSV2)
	stream := false
	filters := usagestats.UsageLogFilters{
		RequestType: &requestType,
		Stream:      &stream,
		ExactTotal:  true,
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_logs WHERE \\(request_type = \\$1 OR \\(request_type = 0 AND openai_ws_mode = TRUE\\)\\)").
		WithArgs(requestType).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT .* FROM usage_logs WHERE \\(request_type = \\$1 OR \\(request_type = 0 AND openai_ws_mode = TRUE\\)\\) ORDER BY id DESC LIMIT \\$2 OFFSET \\$3").
		WithArgs(requestType, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, page, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, filters)
	require.NoError(t, err)
	require.Empty(t, logs)
	require.NotNil(t, page)
	require.Equal(t, int64(0), page.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryListWithFilters_RoutingFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	filters := usagestats.UsageLogFilters{
		RoutingTargetGroup:   "exhausted",
		RoutingScheduleLayer: "load_balance",
		ExactTotal:           true,
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_logs WHERE routing_target_group = \\$1 AND routing_schedule_layer = \\$2").
		WithArgs("exhausted", "load_balance").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT .* FROM usage_logs WHERE routing_target_group = \\$1 AND routing_schedule_layer = \\$2 ORDER BY id DESC LIMIT \\$3 OFFSET \\$4").
		WithArgs("exhausted", "load_balance", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, page, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, filters)
	require.NoError(t, err)
	require.Empty(t, logs)
	require.NotNil(t, page)
	require.Equal(t, int64(0), page.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryListWithFilters_BillingMode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	filters := usagestats.UsageLogFilters{
		BillingMode: "image",
		ExactTotal:  true,
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM usage_logs WHERE billing_mode = \$1`).
		WithArgs("image").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`SELECT .* FROM usage_logs WHERE billing_mode = \$1 ORDER BY id DESC LIMIT \$2 OFFSET \$3`).
		WithArgs("image", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, page, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, filters)
	require.NoError(t, err)
	require.Empty(t, logs)
	require.NotNil(t, page)
	require.Equal(t, int64(0), page.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryGetUsageTrendWithFiltersRequestTypePriority(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)
	stream := true

	mock.ExpectQuery("AND \\(request_type = \\$3 OR \\(request_type = 0 AND stream = TRUE AND openai_ws_mode = FALSE\\)\\)").
		WithArgs(start, end, requestType).
		WillReturnRows(sqlmock.NewRows([]string{"date", "requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "cost", "actual_cost"}))

	trend, err := repo.GetUsageTrendWithFilters(context.Background(), start, end, "day", 0, 0, 0, 0, "", &requestType, &stream, nil)
	require.NoError(t, err)
	require.Empty(t, trend)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryGetModelStatsWithFiltersRequestTypePriority(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeWSV2)
	stream := false

	mock.ExpectQuery("AND \\(request_type = \\$3 OR \\(request_type = 0 AND openai_ws_mode = TRUE\\)\\)").
		WithArgs(start, end, requestType).
		WillReturnRows(sqlmock.NewRows([]string{"model", "requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "cost", "actual_cost"}))

	stats, err := repo.GetModelStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, &requestType, &stream, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryGetStatsWithFiltersRequestTypePriority(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	requestType := int16(service.RequestTypeSync)
	stream := true
	filters := usagestats.UsageLogFilters{
		RequestType: &requestType,
		Stream:      &stream,
	}

	mock.ExpectQuery("FROM usage_logs\\s+WHERE \\(request_type = \\$1 OR \\(request_type = 0 AND stream = FALSE AND openai_ws_mode = FALSE\\)\\)").
		WithArgs(requestType).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_requests",
			"total_input_tokens",
			"total_output_tokens",
			"total_cache_tokens",
			"total_cost",
			"total_actual_cost",
			"total_account_cost",
			"avg_duration_ms",
		}).AddRow(int64(1), int64(2), int64(3), int64(4), 1.2, 1.0, 1.2, 20.0))
	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(inbound_endpoint").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), requestType).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))
	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(upstream_endpoint").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), requestType).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))
	mock.ExpectQuery("SELECT CONCAT\\(").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), requestType).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))

	stats, err := repo.GetStatsWithFilters(context.Background(), filters)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalRequests)
	require.Equal(t, int64(9), stats.TotalTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryGetUserSpendingRanking(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"user_id", "email", "actual_cost", "requests", "tokens", "total_actual_cost", "total_requests", "total_tokens"}).
		AddRow(int64(2), "beta@example.com", 12.5, int64(9), int64(900), 40.0, int64(30), int64(2600)).
		AddRow(int64(1), "alpha@example.com", 12.5, int64(8), int64(800), 40.0, int64(30), int64(2600)).
		AddRow(int64(3), "gamma@example.com", 4.25, int64(5), int64(300), 40.0, int64(30), int64(2600))

	mock.ExpectQuery("WITH user_spend AS \\(").
		WithArgs(start, end, 12).
		WillReturnRows(rows)

	got, err := repo.GetUserSpendingRanking(context.Background(), start, end, 12)
	require.NoError(t, err)
	require.Equal(t, &usagestats.UserSpendingRankingResponse{
		Ranking: []usagestats.UserSpendingRankingItem{
			{UserID: 2, Email: "beta@example.com", ActualCost: 12.5, Requests: 9, Tokens: 900},
			{UserID: 1, Email: "alpha@example.com", ActualCost: 12.5, Requests: 8, Tokens: 800},
			{UserID: 3, Email: "gamma@example.com", ActualCost: 4.25, Requests: 5, Tokens: 300},
		},
		TotalActualCost: 40.0,
		TotalRequests:   30,
		TotalTokens:     2600,
	}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildRequestTypeFilterConditionLegacyFallback(t *testing.T) {
	tests := []struct {
		name      string
		request   int16
		wantWhere string
		wantArg   int16
	}{
		{
			name:      "sync_with_legacy_fallback",
			request:   int16(service.RequestTypeSync),
			wantWhere: "(request_type = $3 OR (request_type = 0 AND stream = FALSE AND openai_ws_mode = FALSE))",
			wantArg:   int16(service.RequestTypeSync),
		},
		{
			name:      "stream_with_legacy_fallback",
			request:   int16(service.RequestTypeStream),
			wantWhere: "(request_type = $3 OR (request_type = 0 AND stream = TRUE AND openai_ws_mode = FALSE))",
			wantArg:   int16(service.RequestTypeStream),
		},
		{
			name:      "ws_v2_with_legacy_fallback",
			request:   int16(service.RequestTypeWSV2),
			wantWhere: "(request_type = $3 OR (request_type = 0 AND openai_ws_mode = TRUE))",
			wantArg:   int16(service.RequestTypeWSV2),
		},
		{
			name:      "invalid_request_type_normalized_to_unknown",
			request:   int16(99),
			wantWhere: "request_type = $3",
			wantArg:   int16(service.RequestTypeUnknown),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := buildRequestTypeFilterCondition(3, tt.request)
			require.Equal(t, tt.wantWhere, where)
			require.Equal(t, []any{tt.wantArg}, args)
		})
	}
}

type usageLogScannerStub struct {
	values []any
}

func (s usageLogScannerStub) Scan(dest ...any) error {
	if len(dest) != len(s.values) {
		return fmt.Errorf("scan arg count mismatch: got %d want %d", len(dest), len(s.values))
	}
	for i := range dest {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Ptr {
			return fmt.Errorf("dest[%d] is not pointer", i)
		}
		dv.Elem().Set(reflect.ValueOf(s.values[i]))
	}
	return nil
}

func TestScanUsageLogRequestTypeAndLegacyFallback(t *testing.T) {
	t.Run("request_type_ws_v2_overrides_legacy", func(t *testing.T) {
		now := time.Now().UTC()
		log, err := scanUsageLog(usageLogScannerStub{values: []any{
			int64(1),  // id
			int64(10), // user_id
			int64(20), // api_key_id
			int64(30), // account_id
			sql.NullString{Valid: true, String: "req-1"},
			"gpt-5", // model
			sql.NullString{Valid: true, String: "gpt-5"}, // requested_model
			sql.NullString{},  // upstream_model
			sql.NullInt64{},   // channel_id
			sql.NullString{},  // model_mapping_chain
			sql.NullString{},  // billing_tier
			sql.NullString{},  // billing_mode
			sql.NullInt64{},   // group_id
			sql.NullInt64{},   // subscription_id
			1,                 // input_tokens
			2,                 // output_tokens
			3,                 // cache_creation_tokens
			4,                 // cache_read_tokens
			5,                 // cache_creation_5m_tokens
			6,                 // cache_creation_1h_tokens
			0,                 // image_output_tokens
			0.0,               // image_output_cost
			0.1,               // input_cost
			0.2,               // output_cost
			0.3,               // cache_creation_cost
			0.4,               // cache_read_cost
			1.0,               // total_cost
			0.9,               // actual_cost
			1.0,               // rate_multiplier
			sql.NullFloat64{}, // account_rate_multiplier
			sql.NullFloat64{}, // priority_account_multiplier
			sql.NullFloat64{}, // effective_multiplier
			sql.NullFloat64{}, // effective_input_unit_price
			sql.NullFloat64{}, // effective_output_unit_price
			sql.NullFloat64{}, // effective_cache_read_unit_price
			sql.NullString{},  // pricing_source
			int16(service.BillingTypeBalance),
			int16(service.RequestTypeWSV2),
			false, // legacy stream
			false, // legacy openai ws
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			0,
			sql.NullString{},
			sql.NullString{Valid: true, String: "priority"},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			false,
			now,
		}})
		require.NoError(t, err)
		require.NotNil(t, log.ServiceTier)
		require.Equal(t, "priority", *log.ServiceTier)
		require.Equal(t, service.RequestTypeWSV2, log.RequestType)
		require.True(t, log.Stream)
		require.True(t, log.OpenAIWSMode)
	})

	t.Run("request_type_unknown_falls_back_to_legacy", func(t *testing.T) {
		now := time.Now().UTC()
		log, err := scanUsageLog(usageLogScannerStub{values: []any{
			int64(2),
			int64(11),
			int64(21),
			int64(31),
			sql.NullString{Valid: true, String: "req-2"},
			"gpt-5",
			sql.NullString{Valid: true, String: "gpt-5"},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullInt64{},
			1, 2, 3, 4, 5, 6,
			0, 0.0,
			0.1, 0.2, 0.3, 0.4, 1.0, 0.9,
			1.0,
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullString{},
			int16(service.BillingTypeBalance),
			int16(service.RequestTypeUnknown),
			true,
			false,
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			0,
			sql.NullString{},
			sql.NullString{Valid: true, String: "flex"},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			false,
			now,
		}})
		require.NoError(t, err)
		require.NotNil(t, log.ServiceTier)
		require.Equal(t, "flex", *log.ServiceTier)
		require.Equal(t, service.RequestTypeStream, log.RequestType)
		require.True(t, log.Stream)
		require.False(t, log.OpenAIWSMode)
	})

	t.Run("service_tier_is_scanned", func(t *testing.T) {
		now := time.Now().UTC()
		log, err := scanUsageLog(usageLogScannerStub{values: []any{
			int64(3),
			int64(12),
			int64(22),
			int64(32),
			sql.NullString{Valid: true, String: "req-3"},
			"gpt-5.4",
			sql.NullString{Valid: true, String: "gpt-5.4"},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullInt64{},
			1, 2, 3, 4, 5, 6,
			0, 0.0,
			0.1, 0.2, 0.3, 0.4, 1.0, 0.9,
			1.0,
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullString{},
			int16(service.BillingTypeBalance),
			int16(service.RequestTypeSync),
			false,
			false,
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			0,
			sql.NullString{},
			sql.NullString{Valid: true, String: "priority"},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			sql.NullInt64{},
			sql.NullString{},
			false,
			now,
		}})
		require.NoError(t, err)
		require.NotNil(t, log.ServiceTier)
		require.Equal(t, "priority", *log.ServiceTier)
	})

}

func TestScanUsageLog_PreservesBillingBreakdownFields(t *testing.T) {
	now := time.Now().UTC()
	log, err := scanUsageLog(usageLogScannerStub{values: []any{
		int64(4),
		int64(13),
		int64(23),
		int64(33),
		sql.NullString{Valid: true, String: "req-breakdown"},
		"gpt-5.4",
		sql.NullString{Valid: true, String: "gpt-5.4"},
		sql.NullString{},
		sql.NullInt64{},
		sql.NullString{},
		sql.NullString{},
		sql.NullString{},
		sql.NullInt64{},
		sql.NullInt64{},
		1, 2, 3, 4, 5, 6,
		0, 0.0,
		0.1, 0.2, 0.3, 0.4, 1.0, 90.0,
		1.5,
		sql.NullFloat64{Valid: true, Float64: 2.0},
		sql.NullFloat64{Valid: true, Float64: 100.0},
		sql.NullFloat64{Valid: true, Float64: 300.0},
		sql.NullFloat64{Valid: true, Float64: 5e-6},
		sql.NullFloat64{Valid: true, Float64: 30e-6},
		sql.NullFloat64{Valid: true, Float64: 0.5e-6},
		sql.NullString{Valid: true, String: "priority_pricing,priority_account_multiplier"},
		int16(service.BillingTypeBalance),
		int16(service.RequestTypeSync),
		false,
		false,
		sql.NullInt64{},
		sql.NullInt64{},
			sql.NullString{},
			sql.NullString{},
			0,
			sql.NullString{},
			sql.NullString{Valid: true, String: "priority"},
		sql.NullString{},
		sql.NullString{},
		sql.NullString{},
		sql.NullString{},
		sql.NullString{},
		sql.NullInt64{},
		sql.NullString{},
		sql.NullString{},
		sql.NullInt64{},
		sql.NullString{},
		false,
		now,
	}})
	require.NoError(t, err)
	require.NotNil(t, log.PriorityAccountMultiplier)
	require.Equal(t, 100.0, *log.PriorityAccountMultiplier)
	require.NotNil(t, log.EffectiveMultiplier)
	require.Equal(t, 300.0, *log.EffectiveMultiplier)
	require.NotNil(t, log.EffectiveInputUnitPrice)
	require.Equal(t, 5e-6, *log.EffectiveInputUnitPrice)
	require.NotNil(t, log.EffectiveOutputUnitPrice)
	require.Equal(t, 30e-6, *log.EffectiveOutputUnitPrice)
	require.NotNil(t, log.EffectiveCacheReadUnitPrice)
	require.Equal(t, 0.5e-6, *log.EffectiveCacheReadUnitPrice)
	require.NotNil(t, log.PricingSource)
	require.Equal(t, "priority_pricing,priority_account_multiplier", *log.PricingSource)
}

func TestScanUsageLog_PreservesChannelAndImageOutputFields(t *testing.T) {
	now := time.Now().UTC()
	log, err := scanUsageLog(usageLogScannerStub{values: []any{
		int64(4),  // id
		int64(13), // user_id
		int64(23), // api_key_id
		int64(33), // account_id
		sql.NullString{Valid: true, String: "req-channel-image"},
		"gpt-image-1", // model
		sql.NullString{Valid: true, String: "gpt-image-1"},
		sql.NullString{},                      // upstream_model
		sql.NullInt64{Valid: true, Int64: 55}, // channel_id
		sql.NullString{Valid: true, String: "alias->mapped"},
		sql.NullString{Valid: true, String: "channel_mapped"},
		sql.NullString{Valid: true, String: "image"},
		sql.NullInt64{},  // group_id
		sql.NullInt64{},  // subscription_id
		1, 2, 3, 4, 5, 6, // token counters
		42,                           // image_output_tokens
		0.42,                         // image_output_cost
		0.1, 0.2, 0.3, 0.4, 1.0, 1.0, // cost fields
		1.5,                               // rate_multiplier
		sql.NullFloat64{},                 // account_rate_multiplier
		sql.NullFloat64{},                 // priority_account_multiplier
		sql.NullFloat64{},                 // effective_multiplier
		sql.NullFloat64{},                 // effective_input_unit_price
		sql.NullFloat64{},                 // effective_output_unit_price
		sql.NullFloat64{},                 // effective_cache_read_unit_price
		sql.NullString{},                  // pricing_source
		int16(service.BillingTypeBalance), // billing_type
		int16(service.RequestTypeSync),    // request_type
		false,                             // stream
		false,                             // openai_ws_mode
		sql.NullInt64{},                   // duration_ms
		sql.NullInt64{},                   // first_token_ms
		sql.NullString{},                  // user_agent
		sql.NullString{},                  // ip_address
		0,                                 // image_count
		sql.NullString{},                  // image_size
		sql.NullString{},                  // service_tier
		sql.NullString{},                  // reasoning_effort
		sql.NullString{},                  // inbound_endpoint
		sql.NullString{},                  // upstream_endpoint
		sql.NullString{},                  // routing_target_group
		sql.NullString{},                  // routing_schedule_layer
		sql.NullInt64{},                   // routing_selected_account_id
		sql.NullString{},                  // routing_selected_account_name
		sql.NullString{},                  // routing_effective_model
		sql.NullInt64{},                   // routing_failover_count
		sql.NullString{},                  // routing_failover_final_reason
		false,                             // cache_ttl_overridden
		now,                               // created_at
	}})
	require.NoError(t, err)
	require.Equal(t, 42, log.ImageOutputTokens)
	require.Equal(t, 0.42, log.ImageOutputCost)
	require.NotNil(t, log.ChannelID)
	require.Equal(t, int64(55), *log.ChannelID)
	require.NotNil(t, log.ModelMappingChain)
	require.Equal(t, "alias->mapped", *log.ModelMappingChain)
	require.NotNil(t, log.BillingTier)
	require.Equal(t, "channel_mapped", *log.BillingTier)
	require.NotNil(t, log.BillingMode)
	require.Equal(t, "image", *log.BillingMode)
}
