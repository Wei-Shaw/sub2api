//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 这些用例复现生产写入路径对一次 cyber_policy 命中产生的两条记录：
//   - usage_logs 一行 request_type=4（RecordCyberPolicyUsageLog / RecordUsage(CyberBlocked=true)）
//   - ops_error_logs 一行 error_type=cyber_policy（buildCyberPolicyOpsErrorEntry），
//     非流式 status=400，流式/WS 为 200，is_business_limited=true，error_owner=provider。
// 期望口径：cyber 请求只以错误身份出现一次；token 仍计入吞吐；不参与成功数与延迟分位。

type opsCyberFixture struct {
	userID    int64
	apiKeyID  int64
	accountID int64
	groupID   int64
}

func newOpsCyberFixture(t *testing.T, label string) opsCyberFixture {
	t.Helper()
	client := integrationEntClient
	suffix := fmt.Sprintf("%s-%d", label, time.Now().UnixNano())
	group := mustCreateGroup(t, client, &service.Group{Name: "ops-cyber-" + suffix, Platform: service.PlatformOpenAI})
	user := mustCreateUser(t, client, &service.User{Email: "ops-cyber-" + suffix + "@example.com"})
	gid := group.ID
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-ops-cyber-" + suffix, Name: "k", GroupID: &gid})
	account := mustCreateAccount(t, client, &service.Account{Name: "ops-cyber-" + suffix, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	return opsCyberFixture{userID: user.ID, apiKeyID: key.ID, accountID: account.ID, groupID: group.ID}
}

type opsCyberUsageRow struct {
	requestID    string
	requestType  service.RequestType
	stream       bool
	durationMs   *int
	firstTokenMs *int
	inputTokens  int
	outputTokens int
	offset       time.Duration
}

type opsCyberErrorRow struct {
	requestID         string
	errorType         string
	errorPhase        string
	statusCode        int
	stream            bool
	isBusinessLimited bool
	owner             string
	source            string
	requestType       *int16
	offset            time.Duration
}

func opsCyberIntPtr(v int) *int { return &v }

func opsCyberInsert(t *testing.T, fx opsCyberFixture, base time.Time, usage []opsCyberUsageRow, errs []opsCyberErrorRow) {
	t.Helper()
	ctx := context.Background()
	usageRepo := newUsageLogRepositoryWithSQL(integrationEntClient, integrationDB)
	opsRepo := NewOpsRepository(integrationDB)
	gid := fx.groupID
	for _, u := range usage {
		inserted, err := usageRepo.Create(ctx, &service.UsageLog{
			UserID:       fx.userID,
			APIKeyID:     fx.apiKeyID,
			AccountID:    fx.accountID,
			GroupID:      &gid,
			RequestID:    u.requestID,
			Model:        "gpt-5.1",
			InputTokens:  u.inputTokens,
			OutputTokens: u.outputTokens,
			RequestType:  u.requestType,
			Stream:       u.stream,
			DurationMs:   u.durationMs,
			FirstTokenMs: u.firstTokenMs,
			CreatedAt:    base.Add(u.offset),
		})
		require.NoError(t, err)
		require.True(t, inserted)
	}
	for _, e := range errs {
		userID, apiKeyID, accountID := fx.userID, fx.apiKeyID, fx.accountID
		_, err := opsRepo.InsertErrorLog(ctx, &service.OpsInsertErrorLogInput{
			RequestID:         e.requestID,
			UserID:            &userID,
			APIKeyID:          &apiKeyID,
			AccountID:         &accountID,
			GroupID:           &gid,
			Platform:          service.PlatformOpenAI,
			Model:             "gpt-5.1",
			Stream:            e.stream,
			RequestType:       e.requestType,
			ErrorPhase:        e.errorPhase,
			ErrorType:         e.errorType,
			Severity:          "P3",
			StatusCode:        e.statusCode,
			IsBusinessLimited: e.isBusinessLimited,
			ErrorMessage:      e.errorType,
			ErrorSource:       e.source,
			ErrorOwner:        e.owner,
			CreatedAt:         base.Add(e.offset),
		})
		require.NoError(t, err)
	}
}

// opsCyberCleanWindow 清空窗口内的数据，并在用例结束后再清一次：这些行不在事务里，
// 残留会污染同包其他按"今天"统计的集成用例。
func opsCyberCleanWindow(t *testing.T, start, end time.Time) {
	t.Helper()
	clean := func() error {
		ctx := context.Background()
		if _, err := integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE created_at >= $1 AND created_at < $2`, start, end); err != nil {
			return err
		}
		if _, err := integrationDB.ExecContext(ctx, `DELETE FROM ops_error_logs WHERE created_at >= $1 AND created_at < $2`, start, end); err != nil {
			return err
		}
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM ops_metrics_hourly WHERE bucket_start >= $1 AND bucket_start < $2`, start.Truncate(time.Hour), end)
		return err
	}
	require.NoError(t, clean())
	t.Cleanup(func() { require.NoError(t, clean()) })
}

// 两条正常成功请求：duration 100/300，ttft 40/80，tokens 30+40。
func opsCyberNormalUsage(prefix string) []opsCyberUsageRow {
	return []opsCyberUsageRow{
		{requestID: prefix + "-ok-sync", requestType: service.RequestTypeSync, durationMs: opsCyberIntPtr(100), firstTokenMs: opsCyberIntPtr(40), inputTokens: 10, outputTokens: 20, offset: 1 * time.Second},
		{requestID: prefix + "-ok-stream", requestType: service.RequestTypeStream, stream: true, durationMs: opsCyberIntPtr(300), firstTokenMs: opsCyberIntPtr(80), inputTokens: 10, outputTokens: 30, offset: 2 * time.Second},
	}
}

func opsCyberRequestType() *int16 {
	rt := int16(service.RequestTypeCyberBlocked)
	return &rt
}

type opsHourlyRow struct {
	success, errTotal, bizLimited, errSLA, tokens, ttftSamples int64
	durationMax, ttftMax                                       sql.NullInt64
	durationAvg                                                sql.NullFloat64
}

func opsCyberReadHourlyOverall(t *testing.T, bucket time.Time) opsHourlyRow {
	t.Helper()
	var row opsHourlyRow
	err := integrationDB.QueryRowContext(context.Background(), `
SELECT success_count, error_count_total, business_limited_count, error_count_sla, token_consumed, ttft_sample_count,
       duration_max_ms, ttft_max_ms, duration_avg_ms
FROM ops_metrics_hourly
WHERE bucket_start = $1 AND platform IS NULL AND group_id IS NULL`, bucket).Scan(
		&row.success, &row.errTotal, &row.bizLimited, &row.errSLA, &row.tokens, &row.ttftSamples,
		&row.durationMax, &row.ttftMax, &row.durationAvg,
	)
	require.NoError(t, err)
	return row
}

func opsCyberRequestDetailKinds(t *testing.T, repo *opsRepository, start, end time.Time) (success []string, errs []string) {
	t.Helper()
	items, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Page:      1,
		PageSize:  100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(len(items)), total)
	for _, it := range items {
		if it.Kind == service.OpsRequestKindSuccess {
			success = append(success, it.RequestID)
		} else {
			errs = append(errs, it.RequestID)
		}
	}
	sort.Strings(success)
	sort.Strings(errs)
	return success, errs
}

// 场景 A：非流式 cyber（客户端收到 400）。修复前同一请求既算成功又算错误。
func TestOpsCyberDoubleCount_NonStreamRepoViews(t *testing.T) {
	ctx := context.Background()
	fx := newOpsCyberFixture(t, "a")
	base := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	hourEnd := base.Add(time.Hour)
	opsCyberCleanWindow(t, base, hourEnd)

	usage := append(opsCyberNormalUsage("a"),
		// RecordCyberPolicyUsageLog：result.Duration 为零 → duration_ms=0，无 first_token。
		opsCyberUsageRow{requestID: "a-cyber", requestType: service.RequestTypeCyberBlocked, durationMs: opsCyberIntPtr(0), inputTokens: 5, outputTokens: 0, offset: 3 * time.Second},
	)
	errs := []opsCyberErrorRow{
		{requestID: "a-cyber", errorType: "cyber_policy", errorPhase: "request", statusCode: 400, isBusinessLimited: true, owner: "provider", source: "upstream_http", requestType: opsCyberRequestType(), offset: 3 * time.Second},
		// 对照组：一条普通 502 上游错误，没有 usage 行。
		{requestID: "a-upstream-502", errorType: "upstream_error", errorPhase: "upstream", statusCode: 502, owner: "provider", source: "upstream_http", offset: 4 * time.Second},
	}
	opsCyberInsert(t, fx, base, usage, errs)

	repo := NewOpsRepository(integrationDB).(*opsRepository)

	t.Run("dashboard_overview_raw", func(t *testing.T) {
		ov, err := repo.GetDashboardOverview(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd, QueryMode: service.OpsQueryModeRaw})
		require.NoError(t, err)
		require.Equal(t, int64(2), ov.SuccessCount, "cyber usage row must not be a success")
		require.Equal(t, int64(2), ov.ErrorCountTotal)
		require.Equal(t, int64(4), ov.RequestCountTotal, "each request counted once")
		require.Equal(t, int64(75), ov.TokenConsumed, "cyber tokens stay in throughput")
		require.NotNil(t, ov.Duration.Avg)
		require.Equal(t, 200, *ov.Duration.Avg, "cyber duration must not enter latency")
	})

	t.Run("hourly_preagg", func(t *testing.T) {
		require.NoError(t, repo.UpsertHourlyMetrics(ctx, base, hourEnd))
		row := opsCyberReadHourlyOverall(t, base)
		require.Equal(t, int64(2), row.success)
		require.Equal(t, int64(2), row.errTotal)
		require.Equal(t, int64(75), row.tokens)
		require.True(t, row.durationAvg.Valid)
		require.InDelta(t, 200.0, row.durationAvg.Float64, 0.001)
	})

	t.Run("throughput_trend", func(t *testing.T) {
		trend, err := repo.GetThroughputTrend(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd}, 60)
		require.NoError(t, err)
		var reqs, tokens int64
		for _, p := range trend.Points {
			reqs += p.RequestCount
			tokens += p.TokenConsumed
		}
		require.Equal(t, int64(4), reqs)
		require.Equal(t, int64(75), tokens)
		var openaiReqs int64
		for _, item := range trend.ByPlatform {
			if item.Platform == service.PlatformOpenAI {
				openaiReqs = item.RequestCount
			}
		}
		require.Equal(t, int64(4), openaiReqs, "platform breakdown")

		top, err := repo.GetThroughputTrend(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd, Platform: service.PlatformOpenAI}, 60)
		require.NoError(t, err)
		require.Len(t, top.TopGroups, 1)
		require.Equal(t, int64(4), top.TopGroups[0].RequestCount, "top groups breakdown")
	})

	t.Run("realtime_traffic", func(t *testing.T) {
		sum, err := repo.GetRealtimeTrafficSummary(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: base.Add(10 * time.Second)})
		require.NoError(t, err)
		require.InDelta(t, 0.4, sum.QPS.Avg, 0.0001, "4 requests / 10s")
		require.InDelta(t, 7.5, sum.TPS.Avg, 0.0001, "75 tokens / 10s")
	})

	t.Run("latency_histogram", func(t *testing.T) {
		h, err := repo.GetLatencyHistogram(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd})
		require.NoError(t, err)
		require.Equal(t, int64(2), h.TotalRequests)
	})

	t.Run("openai_token_stats", func(t *testing.T) {
		stats, err := repo.GetOpenAITokenStats(ctx, &service.OpsOpenAITokenStatsFilter{StartTime: base, EndTime: hourEnd, Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.Len(t, stats.Items, 1)
		item := stats.Items[0]
		require.Equal(t, int64(2), item.RequestCount)
		require.Equal(t, int64(200), item.AvgDurationMs)
		require.Equal(t, int64(50), item.TotalOutputTokens)
	})

	t.Run("request_details", func(t *testing.T) {
		success, errsList := opsCyberRequestDetailKinds(t, repo, base, hourEnd)
		require.Equal(t, []string{"a-ok-stream", "a-ok-sync"}, success)
		require.Equal(t, []string{"a-cyber", "a-upstream-502"}, errsList)
	})
}

// 场景 B：流式 cyber（SSE 已提交 200，上游随后 response.failed）。成功侧与请求明细。
func TestOpsCyberDoubleCount_StreamingSuccessSideAndDetails(t *testing.T) {
	ctx := context.Background()
	fx := newOpsCyberFixture(t, "b")
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	hourEnd := base.Add(time.Hour)
	opsCyberCleanWindow(t, base, hourEnd)
	insertOpsCyberStreamingScenario(t, fx, base)

	repo := NewOpsRepository(integrationDB).(*opsRepository)

	t.Run("dashboard_overview_raw_success_side", func(t *testing.T) {
		ov, err := repo.GetDashboardOverview(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd, QueryMode: service.OpsQueryModeRaw})
		require.NoError(t, err)
		require.Equal(t, int64(2), ov.SuccessCount)
		require.Equal(t, int64(80), ov.TokenConsumed)
		require.NotNil(t, ov.Duration.Max)
		require.Equal(t, 300, *ov.Duration.Max)
		require.NotNil(t, ov.TTFT.Max)
		require.Equal(t, 80, *ov.TTFT.Max)
	})

	t.Run("hourly_preagg_success_side", func(t *testing.T) {
		require.NoError(t, repo.UpsertHourlyMetrics(ctx, base, hourEnd))
		row := opsCyberReadHourlyOverall(t, base)
		require.Equal(t, int64(2), row.success)
		require.Equal(t, int64(2), row.ttftSamples)
		require.Equal(t, int64(80), row.tokens)
		require.True(t, row.durationMax.Valid)
		require.Equal(t, int64(300), row.durationMax.Int64)
		require.True(t, row.ttftMax.Valid)
		require.Equal(t, int64(80), row.ttftMax.Int64)
	})

	t.Run("openai_token_stats", func(t *testing.T) {
		stats, err := repo.GetOpenAITokenStats(ctx, &service.OpsOpenAITokenStatsFilter{StartTime: base, EndTime: hourEnd, Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.Len(t, stats.Items, 1)
		item := stats.Items[0]
		require.Equal(t, int64(2), item.RequestCount)
		require.Equal(t, int64(2), item.RequestsWithFirstToken)
		require.NotNil(t, item.AvgFirstTokenMs)
		require.InDelta(t, 60.0, *item.AvgFirstTokenMs, 0.001)
	})

	t.Run("request_details", func(t *testing.T) {
		success, errsList := opsCyberRequestDetailKinds(t, repo, base, hourEnd)
		require.Equal(t, []string{"b-ok-stream", "b-ok-sync"}, success)
		require.Equal(t, []string{"b-cyber-stream"}, errsList, "streaming cyber (status 200) is a client-visible error")
	})
}

// 流式 cyber：usage 行走正常 RecordUsage(CyberBlocked=true)，带真实耗时；ops 行 status=200。
func insertOpsCyberStreamingScenario(t *testing.T, fx opsCyberFixture, base time.Time) {
	t.Helper()
	usage := append(opsCyberNormalUsage("b"),
		opsCyberUsageRow{requestID: "b-cyber-stream", requestType: service.RequestTypeCyberBlocked, stream: true, durationMs: opsCyberIntPtr(9000), firstTokenMs: opsCyberIntPtr(5000), inputTokens: 7, outputTokens: 3, offset: 3 * time.Second},
	)
	errs := []opsCyberErrorRow{
		{requestID: "b-cyber-stream", errorType: "cyber_policy", errorPhase: "request", statusCode: 200, stream: true, isBusinessLimited: true, owner: "provider", source: "upstream_http", requestType: opsCyberRequestType(), offset: 3 * time.Second},
	}
	opsCyberInsert(t, fx, base, usage, errs)
}

// 场景 B 的错误侧：流式 cyber（status=200）在看板/预聚合/趋势/实时里应作为错误计一次，
// 否则成功侧排除它之后，这次请求在这些视图里既不算成功也不算错误。
func TestOpsCyberDoubleCount_StreamingErrorSideRepoViews(t *testing.T) {
	ctx := context.Background()
	fx := newOpsCyberFixture(t, "gap")
	base := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)
	hourEnd := base.Add(time.Hour)
	opsCyberCleanWindow(t, base, hourEnd)
	insertOpsCyberStreamingScenario(t, fx, base)

	repo := NewOpsRepository(integrationDB).(*opsRepository)

	t.Run("dashboard_overview_raw", func(t *testing.T) {
		ov, err := repo.GetDashboardOverview(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd, QueryMode: service.OpsQueryModeRaw})
		require.NoError(t, err)
		require.Equal(t, int64(2), ov.SuccessCount, "success")
		require.Equal(t, int64(1), ov.ErrorCountTotal, "error_total")
		require.Equal(t, int64(3), ov.RequestCountTotal, "request_total")
	})
	t.Run("hourly_preagg", func(t *testing.T) {
		require.NoError(t, repo.UpsertHourlyMetrics(ctx, base, hourEnd))
		row := opsCyberReadHourlyOverall(t, base)
		require.Equal(t, int64(2), row.success, "success")
		require.Equal(t, int64(1), row.errTotal, "error_total")
	})
	t.Run("throughput_trend", func(t *testing.T) {
		trend, err := repo.GetThroughputTrend(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: hourEnd}, 60)
		require.NoError(t, err)
		var reqs int64
		for _, p := range trend.Points {
			reqs += p.RequestCount
		}
		require.Equal(t, int64(3), reqs, "request_total")
	})
	t.Run("realtime_traffic", func(t *testing.T) {
		sum, err := repo.GetRealtimeTrafficSummary(ctx, &service.OpsDashboardFilter{StartTime: base, EndTime: base.Add(10 * time.Second)})
		require.NoError(t, err)
		require.InDelta(t, 0.3, sum.QPS.Avg, 0.0001, "3 requests / 10s")
	})
}

// 实时采集器（每分钟写 ops_system_metrics）：通过导出入口 Start() 驱动一次真实采集。
func TestOpsCyberDoubleCount_MetricsCollector(t *testing.T) {
	ctx := context.Background()
	fx := newOpsCyberFixture(t, "collector")

	now := time.Now().UTC()
	if now.Second() >= 45 {
		time.Sleep(now.Truncate(time.Minute).Add(time.Minute + time.Second).Sub(now))
		now = time.Now().UTC()
	}
	windowEnd := now.Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Minute)
	opsCyberCleanWindow(t, windowStart, windowEnd)
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM ops_system_metrics WHERE created_at = $1`, windowEnd)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM ops_system_metrics WHERE created_at = $1`, windowEnd)
	})

	base := windowStart.Add(10 * time.Second)
	usage := append(opsCyberNormalUsage("c"),
		opsCyberUsageRow{requestID: "c-cyber", requestType: service.RequestTypeCyberBlocked, durationMs: opsCyberIntPtr(0), inputTokens: 5, offset: 3 * time.Second},
		opsCyberUsageRow{requestID: "c-cyber-stream", requestType: service.RequestTypeCyberBlocked, stream: true, durationMs: opsCyberIntPtr(9000), firstTokenMs: opsCyberIntPtr(5000), inputTokens: 7, outputTokens: 3, offset: 4 * time.Second},
	)
	errs := []opsCyberErrorRow{
		{requestID: "c-cyber", errorType: "cyber_policy", errorPhase: "request", statusCode: 400, isBusinessLimited: true, owner: "provider", source: "upstream_http", requestType: opsCyberRequestType(), offset: 3 * time.Second},
		{requestID: "c-cyber-stream", errorType: "cyber_policy", errorPhase: "request", statusCode: 200, stream: true, isBusinessLimited: true, owner: "provider", source: "upstream_http", requestType: opsCyberRequestType(), offset: 4 * time.Second},
		{requestID: "c-upstream-502", errorType: "upstream_error", errorPhase: "upstream", statusCode: 502, owner: "provider", source: "upstream_http", offset: 5 * time.Second},
	}
	opsCyberInsert(t, fx, base, usage, errs)

	collector := service.NewOpsMetricsCollector(NewOpsRepository(integrationDB), nil, nil, nil, integrationDB, nil, nil)
	collector.Start()
	defer collector.Stop()

	var (
		success, errTotal, bizLimited, errSLA, tokens int64
		durationMax, ttftMax                          sql.NullInt64
	)
	deadline := time.Now().Add(20 * time.Second)
	for {
		err = integrationDB.QueryRowContext(ctx, `
SELECT success_count, error_count_total, business_limited_count, error_count_sla, token_consumed, duration_max_ms, ttft_max_ms
FROM ops_system_metrics
WHERE created_at = $1 AND window_minutes = 1
ORDER BY id DESC LIMIT 1`, windowEnd).Scan(&success, &errTotal, &bizLimited, &errSLA, &tokens, &durationMax, &ttftMax)
		if err == nil {
			break
		}
		require.ErrorIs(t, err, sql.ErrNoRows)
		require.True(t, time.Now().Before(deadline), "collector did not persist window %s", windowEnd)
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("collector window=%s success=%d error_total=%d business_limited=%d error_sla=%d tokens=%d duration_max=%v ttft_max=%v",
		windowEnd.Format(time.RFC3339), success, errTotal, bizLimited, errSLA, tokens, durationMax, ttftMax)
	require.Equal(t, int64(2), success, "cyber usage rows must not be successes")
	require.Equal(t, int64(3), errTotal, "both cyber hits (400 and streaming 200) plus the 502 are errors")
	require.Equal(t, int64(2), bizLimited)
	require.Equal(t, int64(1), errSLA)
	require.Equal(t, int64(85), tokens, "cyber tokens stay in throughput")
	require.True(t, durationMax.Valid)
	require.Equal(t, int64(300), durationMax.Int64)
	require.True(t, ttftMax.Valid)
	require.Equal(t, int64(80), ttftMax.Int64)
}
