//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// opsTrendPreaggFixture 在一个远离"现在"的窗口里铺一批 usage/error 明细，
// 并把该窗口聚合进 ops_metrics_minute。
//
// 窗口刻意取 4 小时前并对齐到整点：opsMinutePreaggSafeEnd 会把 preagg 段裁到
// now-3min，只有窗口整体落在安全线之内，preagg 段才等于整个窗口，
// 从而让 preagg 与 raw 的结果可以逐点比对（否则尾段口径不同是设计使然）。
type opsTrendPreaggFixture struct {
	repo    *opsRepository
	start   time.Time
	end     time.Time
	groupID int64
}

func setupOpsTrendPreaggFixture(t *testing.T, ctx context.Context) *opsTrendPreaggFixture {
	t.Helper()

	start := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Hour)
	end := start.Add(2 * time.Hour)

	// 只清窗口内的数据：整表 TRUNCATE 会波及同包其他用例的 fixture。
	// 用例结束后必须再清一次——dashboard 那批用例按"今天"全表统计，
	// 本 fixture 的 4 小时前落在它们的窗口里，留着会把它们的计数顶高。
	cleanup := func() {
		for _, stmt := range []string{
			`DELETE FROM usage_logs WHERE created_at >= $1 AND created_at < $2`,
			`DELETE FROM ops_error_logs WHERE created_at >= $1 AND created_at < $2`,
			`DELETE FROM ops_metrics_minute WHERE bucket_start >= $1 AND bucket_start < $2`,
		} {
			_, err := integrationDB.ExecContext(context.Background(), stmt, start, end)
			require.NoError(t, err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email:    fmt.Sprintf("ops-trend-%d@example.com", time.Now().UnixNano()),
		Username: fmt.Sprintf("ops-trend-%d", time.Now().UnixNano()),
	})
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name:     fmt.Sprintf("ops-trend-group-%d", time.Now().UnixNano()),
		Platform: service.PlatformAnthropic,
	})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:     fmt.Sprintf("ops-trend-account-%d", time.Now().UnixNano()),
		Platform: service.PlatformAnthropic,
	})
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID})

	// usage 明细散落在多个分钟桶里，且跨越 5 分钟 / 1 小时边界。
	for _, offset := range []time.Duration{
		1 * time.Minute, 1 * time.Minute, 7 * time.Minute,
		33 * time.Minute, 61 * time.Minute, 119 * time.Minute,
	} {
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO usage_logs
				(user_id, api_key_id, account_id, group_id, model,
				 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
				 total_cost, actual_cost, created_at)
			VALUES ($1, $2, $3, $4, 'claude-test', 10, 20, 5, 3, 0.01, 0.01, $5)
		`, user.ID, apiKey.ID, account.ID, group.ID, start.Add(offset))
		require.NoError(t, err)
	}

	// group_id 为 NULL 的 usage 行：ops_metrics_hourly 的 INNER JOIN 会丢掉它，
	// 分钟表用 LEFT JOIN 保留，overall 行必须把它算进去。
	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_logs
			(user_id, api_key_id, account_id, group_id, model,
			 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			 total_cost, actual_cost, created_at)
		VALUES ($1, $2, $3, NULL, 'claude-test', 100, 0, 0, 0, 0.01, 0.01, $4)
	`, user.ID, apiKey.ID, account.ID, start.Add(9*time.Minute))
	require.NoError(t, err)

	repo := NewOpsRepository(integrationDB).(*opsRepository)
	upstream529 := 529
	upstream500 := 500
	// 仓储层写的是已由 service 侧脱敏序列化好的 UpstreamErrorsJSON，不是 UpstreamErrors。
	failoverEvents := `[{"kind":"failover","upstream_status_code":500},` +
		`{"kind":"retry_exhausted_failover:pool","upstream_status_code":500},` +
		`{"kind":"http_error","upstream_status_code":500}]`
	countTokensEvents := `[{"kind":"failover"}]`
	_, err = repo.BatchInsertErrorLogs(ctx, []*service.OpsInsertErrorLogInput{
		{
			RequestID: "trend-429", Platform: service.PlatformAnthropic, GroupID: &group.ID,
			ErrorPhase: "upstream", ErrorType: "upstream_error", Severity: "error",
			StatusCode: 429, ErrorOwner: "provider", IsBusinessLimited: false,
			CreatedAt: start.Add(2 * time.Minute),
		},
		{
			RequestID: "trend-429-business", Platform: service.PlatformAnthropic, GroupID: &group.ID,
			ErrorPhase: "upstream", ErrorType: "quota", Severity: "error",
			StatusCode: 429, ErrorOwner: "provider", IsBusinessLimited: true,
			CreatedAt: start.Add(2 * time.Minute),
		},
		{
			RequestID: "trend-529", Platform: service.PlatformAnthropic, GroupID: &group.ID,
			ErrorPhase: "upstream", ErrorType: "overloaded", Severity: "error",
			StatusCode: 500, UpstreamStatusCode: &upstream529, ErrorOwner: "provider",
			CreatedAt: start.Add(35 * time.Minute),
		},
		{
			// 带 failover 事件：switch_count 必须由写入侧展开出 2。
			RequestID: "trend-failover", Platform: service.PlatformAnthropic, GroupID: &group.ID,
			ErrorPhase: "upstream", ErrorType: "upstream_error", Severity: "error",
			StatusCode: 500, UpstreamStatusCode: &upstream500, ErrorOwner: "provider",
			UpstreamErrorsJSON: &failoverEvents,
			CreatedAt:          start.Add(65 * time.Minute),
		},
		{
			// count_tokens 探针：两侧都必须排除，否则错误率虚高、切换数虚高。
			RequestID: "trend-count-tokens", Platform: service.PlatformAnthropic, GroupID: &group.ID,
			ErrorPhase: "upstream", ErrorType: "upstream_error", Severity: "error",
			StatusCode: 500, ErrorOwner: "provider", IsCountTokens: true,
			UpstreamErrorsJSON: &countTokensEvents,
			CreatedAt:          start.Add(66 * time.Minute),
		},
	})
	require.NoError(t, err)

	require.NoError(t, repo.UpsertMinuteMetrics(ctx, start, end))

	return &opsTrendPreaggFixture{repo: repo, start: start, end: end, groupID: group.ID}
}

// preagg 与 raw 必须逐点相等：两条路径共用同一份口径，任何一侧漂移都会让
// 面板在 auto fallback 前后跳数字。
func TestOpsTrendPreaggMatchesRaw(t *testing.T) {
	ctx := context.Background()
	fx := setupOpsTrendPreaggFixture(t, ctx)

	filters := map[string]*service.OpsDashboardFilter{
		"overall":  {StartTime: fx.start, EndTime: fx.end},
		"platform": {StartTime: fx.start, EndTime: fx.end, Platform: service.PlatformAnthropic},
		"group":    {StartTime: fx.start, EndTime: fx.end, Platform: service.PlatformAnthropic, GroupID: &fx.groupID},
	}

	for _, bucketSeconds := range []int{60, 300, 3600} {
		for name, base := range filters {
			t.Run(fmt.Sprintf("%s/%ds", name, bucketSeconds), func(t *testing.T) {
				rawFilter := *base
				rawFilter.QueryMode = service.OpsQueryModeRaw
				preaggFilter := *base
				preaggFilter.QueryMode = service.OpsQueryModePreagg

				rawThroughput, err := fx.repo.GetThroughputTrend(ctx, &rawFilter, bucketSeconds)
				require.NoError(t, err)
				preaggThroughput, err := fx.repo.GetThroughputTrend(ctx, &preaggFilter, bucketSeconds)
				require.NoError(t, err)
				require.Equal(t, rawThroughput.Bucket, preaggThroughput.Bucket)
				require.Equal(t, rawThroughput.Points, preaggThroughput.Points)
				require.Equal(t, rawThroughput.ByPlatform, preaggThroughput.ByPlatform)
				require.Equal(t, rawThroughput.TopGroups, preaggThroughput.TopGroups)

				rawError, err := fx.repo.GetErrorTrend(ctx, &rawFilter, bucketSeconds)
				require.NoError(t, err)
				preaggError, err := fx.repo.GetErrorTrend(ctx, &preaggFilter, bucketSeconds)
				require.NoError(t, err)
				require.Equal(t, rawError.Points, preaggError.Points)
			})
		}
	}
}

// switch_count 从查询期的 JSONB 展开挪到了写入期，口径必须原样保留。
func TestOpsMinuteMetricsSwitchCountMatchesRawExpansion(t *testing.T) {
	ctx := context.Background()
	fx := setupOpsTrendPreaggFixture(t, ctx)

	var preaggSwitch int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(switch_count), 0) FROM ops_metrics_minute
		WHERE bucket_start >= $1 AND bucket_start < $2 AND platform IS NULL AND group_id IS NULL
	`, fx.start, fx.end).Scan(&preaggSwitch))

	// 与改造前 queryAccountSwitchCount 的展开逻辑逐字一致。
	var rawSwitch int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE
		  WHEN split_part(ev->>'kind', ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
		  ELSE 0
		END), 0)
		FROM ops_error_logs
		CROSS JOIN LATERAL jsonb_array_elements(
		  COALESCE(NULLIF(upstream_errors, 'null'::jsonb), '[]'::jsonb)
		) AS ev
		WHERE created_at >= $1 AND created_at < $2 AND is_count_tokens = FALSE
	`, fx.start, fx.end).Scan(&rawSwitch))

	require.Equal(t, int64(2), rawSwitch, "fixture 应产生 2 次切换（count_tokens 那条不计）")
	require.Equal(t, rawSwitch, preaggSwitch)
}

// 分钟表尚未覆盖但明细有数据时，preagg 必须报 ErrOpsPreaggregatedNotPopulated，
// auto 据此回落到 raw —— 否则面板会显示成"这段时间没有流量"。
func TestOpsTrendAutoFallsBackWhenMinutePreaggEmpty(t *testing.T) {
	ctx := context.Background()
	fx := setupOpsTrendPreaggFixture(t, ctx)

	_, err := integrationDB.ExecContext(ctx,
		`DELETE FROM ops_metrics_minute WHERE bucket_start >= $1 AND bucket_start < $2`, fx.start, fx.end)
	require.NoError(t, err)

	rawFilter := &service.OpsDashboardFilter{StartTime: fx.start, EndTime: fx.end, QueryMode: service.OpsQueryModeRaw}
	preaggFilter := &service.OpsDashboardFilter{StartTime: fx.start, EndTime: fx.end, QueryMode: service.OpsQueryModePreagg}
	autoFilter := &service.OpsDashboardFilter{StartTime: fx.start, EndTime: fx.end, QueryMode: service.OpsQueryModeAuto}

	_, err = fx.repo.GetThroughputTrend(ctx, preaggFilter, 60)
	require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)
	_, err = fx.repo.GetErrorTrend(ctx, preaggFilter, 60)
	require.ErrorIs(t, err, service.ErrOpsPreaggregatedNotPopulated)

	want, err := fx.repo.GetThroughputTrend(ctx, rawFilter, 60)
	require.NoError(t, err)
	got, err := fx.repo.GetThroughputTrend(ctx, autoFilter, 60)
	require.NoError(t, err)
	require.Equal(t, want.Points, got.Points)

	wantErrTrend, err := fx.repo.GetErrorTrend(ctx, rawFilter, 60)
	require.NoError(t, err)
	gotErrTrend, err := fx.repo.GetErrorTrend(ctx, autoFilter, 60)
	require.NoError(t, err)
	require.Equal(t, wantErrTrend.Points, gotErrTrend.Points)
}

// 重复聚合同一窗口必须幂等：回填与增量聚合会大量重叠，
// ON CONFLICT 若写成累加，重叠区的数字就会翻倍。
func TestUpsertMinuteMetricsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	fx := setupOpsTrendPreaggFixture(t, ctx)

	snapshot := func() []int64 {
		var rows, requests, tokens, switches int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT COUNT(*),
			       COALESCE(SUM(success_count + error_count_total), 0),
			       COALESCE(SUM(token_consumed), 0),
			       COALESCE(SUM(switch_count), 0)
			FROM ops_metrics_minute WHERE bucket_start >= $1 AND bucket_start < $2
		`, fx.start, fx.end).Scan(&rows, &requests, &tokens, &switches))
		return []int64{rows, requests, tokens, switches}
	}

	before := snapshot()
	require.NotEqual(t, int64(0), before[1], "fixture 应产生非零请求数")

	// 再跑两遍，其中一遍按 30 分钟分段，模拟回填与增量的重叠。
	require.NoError(t, fx.repo.UpsertMinuteMetrics(ctx, fx.start, fx.end))
	for at := fx.start; at.Before(fx.end); at = at.Add(30 * time.Minute) {
		require.NoError(t, fx.repo.UpsertMinuteMetrics(ctx, at, at.Add(30*time.Minute)))
	}

	require.Equal(t, before, snapshot())
}
