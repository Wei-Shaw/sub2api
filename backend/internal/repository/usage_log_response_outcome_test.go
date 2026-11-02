package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrepareUsageLogInsertResponseOutcomeNullableAndPositioned(t *testing.T) {
	legacy := &service.UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "client:legacy", Model: "gpt-5.5", CreatedAt: time.Now()}
	prepared := prepareUsageLogInsert(legacy)
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	// account_stats_cost sits five slots before the end: ..., account_stats_cost, response_outcome, upstream_request_id, session_id, native_compaction_v2, created_at
	outcomeIdx := len(prepared.args) - 5
	require.Equal(t, "text", usageLogInsertArgTypes[outcomeIdx])
	require.Equal(t, sql.NullString{}, prepared.args[outcomeIdx], "legacy rows persist NULL")

	outcome := string(service.ResponseOutcomeClientCancelled)
	failed := &service.UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "client:failed", Model: "gpt-5.5", ResponseOutcome: &outcome, CreatedAt: time.Now()}
	prepared = prepareUsageLogInsert(failed)
	require.Equal(t, sql.NullString{String: outcome, Valid: true}, prepared.args[outcomeIdx])
	require.Contains(t, usageLogSelectColumns, "account_stats_cost, response_outcome, upstream_request_id")
}

func TestOpsUsageCountsFilterSuccessButKeepFailedConsumptionTokens(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	// One failed-consumption row (client_cancelled, 900 tokens): success 0, tokens retained.
	mock.ExpectQuery(`COUNT\(\*\) FILTER \(WHERE \(ul\.response_outcome IS NULL OR ul\.response_outcome = 'response_written'\)\), 0\) AS success_count,\s+COALESCE\(SUM\(input_tokens \+ output_tokens \+ cache_creation_tokens \+ cache_read_tokens\), 0\) AS token_consumed`).
		WillReturnRows(sqlmock.NewRows([]string{"success_count", "token_consumed"}).AddRow(0, 900))

	success, tokens, err := repo.queryUsageCounts(context.Background(), &service.OpsDashboardFilter{}, start, start.Add(time.Hour))

	require.NoError(t, err)
	require.Zero(t, success)
	require.Equal(t, int64(900), tokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRequestDetailsDedupeFailedConsumptionByExactAPIKeyAndBillingIdentity(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	dedupe := regexp.QuoteMeta(`AND ((ul.response_outcome IS NULL OR ul.response_outcome = 'response_written') OR NOT EXISTS (`) +
		`.*` + regexp.QuoteMeta(`WHERE oe.api_key_id = ul.api_key_id`) +
		`.*` + regexp.QuoteMeta(`ul.request_id = 'client:' || oe.client_request_id`) +
		`.*` + regexp.QuoteMeta(`ul.request_id = 'local:' || oe.request_id`)
	kind := regexp.QuoteMeta(`CASE WHEN (ul.response_outcome IS NULL OR ul.response_outcome = 'response_written') THEN 'success' ELSE 'error' END::TEXT AS kind`)
	mock.ExpectQuery(`(?s)` + kind + `.*` + dedupe + `.*SELECT COUNT\(1\) FROM combined`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)` + dedupe + `.*FROM combined`).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "created_at", "request_id", "platform", "model", "duration_ms", "first_token_ms",
			"status_code", "error_id", "phase", "severity", "message",
			"user_id", "api_key_id", "account_id", "group_id", "stream",
		}).AddRow("error", start, "client-abc", "openai", "gpt-5.5", nil, nil, 499, 1, "request", "P3", "downstream delivery client_cancelled during streaming; consumption=observed", 1, 2, 3, 4, true))

	items, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{StartTime: &start, EndTime: &end, Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Equal(t, int64(1), total, "one request: the ops error row, not error+usage twice")
	require.Len(t, items, 1)
	require.Equal(t, service.OpsRequestKindError, items[0].Kind)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageStatsSuccessCountersFilterButKeepConsumption(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	// Only a failed-consumption row exists: 0 successful requests, all tokens/cost kept.
	mock.ExpectQuery(regexp.QuoteMeta(`COUNT(*) FILTER (WHERE (response_outcome IS NULL OR response_outcome = 'response_written')) as total_requests,`)+
		`\s+`+regexp.QuoteMeta(`COALESCE(SUM(input_tokens), 0) as total_input_tokens,`)+
		`(?s).*`+regexp.QuoteMeta(`COALESCE(SUM(actual_cost), 0) as total_actual_cost,`)+
		`\s+`+regexp.QuoteMeta(`COALESCE(AVG(COALESCE(duration_ms, 0)) FILTER (WHERE (response_outcome IS NULL OR response_outcome = 'response_written')), 0) as avg_duration_ms`)).
		WithArgs(int64(9), start, start.Add(time.Hour)).
		WillReturnRows(sqlmock.NewRows([]string{"total_requests", "total_input_tokens", "total_output_tokens", "total_cache_tokens",
			"total_cache_creation_tokens", "total_cache_read_tokens", "total_cost", "total_actual_cost", "avg_duration_ms"}).
			AddRow(0, 800, 100, 0, 0, 0, 0.5, 0.5, 0))

	stats, err := repo.GetUserStatsAggregated(context.Background(), 9, start, start.Add(time.Hour))

	require.NoError(t, err)
	require.Zero(t, stats.TotalRequests)
	require.Equal(t, int64(800), stats.TotalInputTokens)
	require.InDelta(t, 0.5, stats.TotalActualCost, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
}
