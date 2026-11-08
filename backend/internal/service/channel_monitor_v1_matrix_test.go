//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------- 假体 ----------

type channelMonitorV1MatrixMonitorsStub struct {
	monitors []*ChannelMonitor
}

func (s *channelMonitorV1MatrixMonitorsStub) ListEnabledMonitors(context.Context) ([]*ChannelMonitor, error) {
	return s.monitors, nil
}

// channelMonitorV1MatrixRepoStub 同时提供两路假数据：
//
//	points → 柱子（最近 N 次探测，与档位无关）
//	counts → 成功率 / 健康分（所选窗口内计数）
//
// 两路分别记录入参，测试才能钉住「柱子没带时间窗口、计数带了时间窗口」这条核心口径。
type channelMonitorV1MatrixRepoStub struct {
	points     map[int64][]ChannelMonitorV1HistoryPoint
	counts     map[int64]ChannelMonitorV1StatusCounts
	coverage   map[int64]ChannelMonitorV1CoverageBounds
	liveGroups ChannelMonitorV1LiveGroupIDs

	pointCalls      int
	pointMonitorIDs []int64
	pointLimit      int

	countCalls      int
	countMonitorIDs []int64
	countStart      time.Time
	countEnd        time.Time

	groupCalls    int
	groupGroupIDs []int64
}

func (s *channelMonitorV1MatrixRepoStub) LoadMonitorV1RecentPoints(
	_ context.Context,
	monitorIDs []int64,
	limit int,
) (map[int64][]ChannelMonitorV1HistoryPoint, error) {
	s.pointCalls++
	s.pointMonitorIDs, s.pointLimit = monitorIDs, limit
	return s.points, nil
}

func (s *channelMonitorV1MatrixRepoStub) LoadMonitorV1WindowCounts(
	_ context.Context,
	monitorIDs []int64,
	start, end time.Time,
) (map[int64]ChannelMonitorV1StatusCounts, error) {
	s.countCalls++
	s.countMonitorIDs, s.countStart, s.countEnd = monitorIDs, start, end
	return s.counts, nil
}

func (s *channelMonitorV1MatrixRepoStub) LoadMonitorV1Coverage(
	_ context.Context,
	_ []int64,
) (map[int64]ChannelMonitorV1CoverageBounds, error) {
	return s.coverage, nil
}

func (s *channelMonitorV1MatrixRepoStub) LoadMonitorV1LiveGroupIDs(
	_ context.Context,
	groupIDs []int64,
) (ChannelMonitorV1LiveGroupIDs, error) {
	s.groupCalls++
	s.groupGroupIDs = groupIDs
	return s.liveGroups, nil
}

// channelMonitorV1MatrixLatency 构造一次探测的延迟样本指针（LatencyMs 是可空字段）。
func channelMonitorV1MatrixLatency(ms int64) *int64 { return &ms }

// fixedChannelMonitorV1MatrixNow 故意选一个「不在整分钟上」的时刻，
// 用来暴露 1m 档不对齐窗口、5m/1h/12h 档对齐窗口的行为差异。
var fixedChannelMonitorV1MatrixNow = time.Date(2026, 9, 29, 13, 47, 23, 456789000, time.UTC)

func mustChannelMonitorV1MatrixWindow(t *testing.T, token string) ChannelMonitorV1MatrixWindow {
	t.Helper()
	window, err := ParseChannelMonitorV1MatrixWindow(token, fixedChannelMonitorV1MatrixNow)
	require.NoError(t, err)
	return window
}

// ---------- 1. range 档位解析：必须与官方 ParseFilter 逐一相等 ----------

// TestChannelMonitorV1MatrixRangeTokensMatchOfficialParseFilter 是钉住「复制逻辑不漂移」的关键测试。
// ParseChannelMonitorV1MatrixWindow 刻意复制了 ChannelMonitorV2Service.ParseFilter 的窗口计算，
// 这里用同一个 now 逐 token 对比官方解析结果（start/end/bucket/range 四项全等）。
func TestChannelMonitorV1MatrixRangeTokensMatchOfficialParseFilter(t *testing.T) {
	tokens := []string{"30m-1m", "1h-1m", "12h-5m", "24h-5m", "7d-1h", "30d-12h"}
	require.Len(t, tokens, 6)
	require.Len(t, channelMonitorV1MatrixRangeTokens, 6, "V1 白名单必须恰好是这 6 个密集 token")
	require.Len(t, channelMonitorV2DenseRangeTokens, 6, "官方密集 token 表变了，V1 白名单需要同步复核")

	official := NewChannelMonitorV2Service(nil)
	official.now = func() time.Time { return fixedChannelMonitorV1MatrixNow }

	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			window, err := ParseChannelMonitorV1MatrixWindow(token, fixedChannelMonitorV1MatrixNow)
			require.NoError(t, err)

			filter, err := official.ParseFilter(token, nil, nil, nil)
			require.NoError(t, err)

			require.Equal(t, filter.Range, window.Range)
			require.Equal(t, filter.Start, window.Start)
			require.Equal(t, filter.End, window.End)
			require.Equal(t, filter.Bucket, window.Bucket)

			// 白名单 window/bucket 也必须与官方密集表逐项一致。
			dense := channelMonitorV2DenseRangeTokens[token]
			require.Equal(t, dense.window, window.End.Sub(window.Start))
			require.Equal(t, dense.bucket, window.Bucket)
		})
	}
}

// TestChannelMonitorV1MatrixRangeTokensNormalizeWhitespace 与官方一样对 token 做 TrimSpace。
func TestChannelMonitorV1MatrixRangeTokensNormalizeWhitespace(t *testing.T) {
	padded, err := ParseChannelMonitorV1MatrixWindow("  24h-5m  ", fixedChannelMonitorV1MatrixNow)
	require.NoError(t, err)
	require.Equal(t, "24h-5m", padded.Range)

	official := NewChannelMonitorV2Service(nil)
	official.now = func() time.Time { return fixedChannelMonitorV1MatrixNow }
	filter, err := official.ParseFilter("  24h-5m  ", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, filter.Start, padded.Start)
	require.Equal(t, filter.End, padded.End)
	require.Equal(t, filter.Bucket, padded.Bucket)
}

// ---------- 2. 非法 token ----------

func TestParseChannelMonitorV1MatrixWindowRejectsUnknownTokens(t *testing.T) {
	// 官方 V2 还接受 ""/90m/24h/7d/30d，但 V1 矩阵只接受 6 个密集 token。
	for _, token := range []string{
		"", "90m", "24h", "7d", "30d", "30m", "1h", "12h",
		"9999d-1s", "30m-7s", "1h-30s", "30M-1M", "garbage", "30m-1m-1m", " ",
	} {
		t.Run(token, func(t *testing.T) {
			_, err := ParseChannelMonitorV1MatrixWindow(token, fixedChannelMonitorV1MatrixNow)
			require.Error(t, err)
			// 复用官方 sentinel：错误文案与 V2 handler 的 400 完全一致。
			require.ErrorIs(t, err, ErrChannelMonitorV2InvalidRange)
		})
	}
}

// ---------- 3. 状态 → overall 映射（最差状态） ----------

func TestChannelMonitorV1MatrixStatusOverallBands(t *testing.T) {
	tests := []struct {
		name   string
		counts ChannelMonitorV1StatusCounts
		want   string
	}{
		{"no samples", ChannelMonitorV1StatusCounts{}, "unknown"},
		{"all operational", ChannelMonitorV1StatusCounts{TotalChecks: 3, Operational: 3}, "healthy"},
		{"degraded only", ChannelMonitorV1StatusCounts{TotalChecks: 1, Degraded: 1}, "warning"},
		{"failed only", ChannelMonitorV1StatusCounts{TotalChecks: 1, Failed: 1}, "critical"},
		{"error only", ChannelMonitorV1StatusCounts{TotalChecks: 1, Error: 1}, "critical"},
		{"degraded beats operational", ChannelMonitorV1StatusCounts{TotalChecks: 5, Operational: 4, Degraded: 1}, "warning"},
		{"failed beats degraded", ChannelMonitorV1StatusCounts{TotalChecks: 5, Operational: 3, Degraded: 1, Failed: 1}, "critical"},
		{"error beats degraded", ChannelMonitorV1StatusCounts{TotalChecks: 5, Operational: 3, Degraded: 1, Error: 1}, "critical"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.counts.Overall())
			health := ChannelMonitorV1MatrixHealthFor(test.counts)
			require.Equal(t, test.want, health.Overall)
			// 三个 string 字段按同一档位填。
			require.Equal(t, test.want, health.ErrorRate)
			require.Equal(t, test.want, health.TTFT)
			require.Equal(t, test.want, health.Cache)
			require.EqualValues(t, 1, health.MinimumSample)
			require.EqualValues(t, 1, health.Thresholds.MinimumSample)
		})
	}
}

// ---------- 4. score = 100 * (operational + degraded) / total，total=0 → nil ----------
//
// ⚠️ 口径（2026-09-30 变更）：**黄色（degraded）算成功**，只有红色（failed / error）扣分。
// 注意 score 与 Overall() 是两件事：degraded 仍然把档位判成 warning（黄色柱子），
// 但不再拉低成功率 —— 所以 mixed 用例是「score=90 且 overall=critical」。
func TestChannelMonitorV1MatrixHealthScore(t *testing.T) {
	noSamples := ChannelMonitorV1MatrixHealthFor(ChannelMonitorV1StatusCounts{})
	require.Nil(t, noSamples.Score, "total=0 时 score 必须为 nil")
	require.Equal(t, "unknown", noSamples.Overall)

	full := ChannelMonitorV1MatrixHealthFor(ChannelMonitorV1StatusCounts{TotalChecks: 5, Operational: 5})
	require.NotNil(t, full.Score)
	require.InDelta(t, 100, *full.Score, 1e-9)

	// 7 operational + 2 degraded 都算成功，只有 1 次 failed 扣分：100 * 9/10 = 90。
	mixed := ChannelMonitorV1MatrixHealthFor(ChannelMonitorV1StatusCounts{TotalChecks: 10, Operational: 7, Degraded: 2, Failed: 1})
	require.NotNil(t, mixed.Score)
	require.InDelta(t, 90, *mixed.Score, 1e-9)
	require.Equal(t, "critical", mixed.Overall, "有 failed → 档位仍然是 critical（与 score 口径无关）")

	// SuccessRate 与 score 同口径：degraded 计入分子，所以这里恰好是 4/4 = 1。
	rate := ChannelMonitorV1StatusCounts{TotalChecks: 4, Operational: 3, Degraded: 1}
	require.InDelta(t, 1, rate.SuccessRate(), 1e-9, "degraded 算成功：3 operational + 1 degraded = 4/4")
	require.InDelta(t, 0, ChannelMonitorV1StatusCounts{}.SuccessRate(), 1e-9)

	// 只有红色扣分：同样 4 条样本里出现 failed 才掉到 0.75。
	docked := ChannelMonitorV1StatusCounts{TotalChecks: 4, Operational: 3, Failed: 1}
	require.InDelta(t, 0.75, docked.SuccessRate(), 1e-9, "failed 才是失败侧")
}

func TestChannelMonitorV1MatrixMetricsKeepSumInvariant(t *testing.T) {
	// 10 次探测：6 operational + 2 degraded 都算成功（黄色算成功），失败侧只有 failed + error = 2。
	counts := ChannelMonitorV1StatusCounts{TotalChecks: 10, Operational: 6, Degraded: 2, Failed: 1, Error: 1}
	metrics := ChannelMonitorV1MatrixMetricsFor(counts, 3000, 3)
	require.EqualValues(t, 8, metrics.SuccessRequests)
	require.EqualValues(t, 2, metrics.ErrorRequests)
	require.EqualValues(t, 10, metrics.RequestCount)
	require.InDelta(t, 0.8, metrics.SuccessRate, 1e-9)
	require.InDelta(t, 0.2, metrics.ErrorRate, 1e-9)
	// 名称里的不变量：success + error == request_count 且 error_rate == 1 - success_rate。
	require.EqualValues(t, metrics.RequestCount, metrics.SuccessRequests+metrics.ErrorRequests,
		"success + error 必须恰好等于 request_count")
	require.InDelta(t, 1-metrics.SuccessRate, metrics.ErrorRate, 1e-9,
		"error_rate 必须等于 1 - success_rate")
	// V1 主动探测没有用量/吞吐语义。
	require.Zero(t, metrics.RPM)
	require.Zero(t, metrics.TPM)
	require.Zero(t, metrics.TokenCount)
	// duration 由 latency_ms 填充；ttft 不冒充。
	require.EqualValues(t, 3, metrics.Duration.SampleCount)
	require.NotNil(t, metrics.Duration.AvgMs)
	require.InDelta(t, 1000, *metrics.Duration.AvgMs, 1e-9)
	require.Zero(t, metrics.TTFT.SampleCount)
	require.Nil(t, metrics.TTFT.AvgMs)
}

// ---------- 5. 曲线 = 探测明细（一次探测一个点） ----------

// TestChannelMonitorV1MatrixPointsMirrorProbes 钉住「柱子 = 最近 N 次探测」的核心语义：
// 曲线上的每个点就是**一次真实探测**，不按时间桶聚合，也**不受所选时间档位影响**。
//
// 断言：喂进 N 条探测记录 → 曲线恰好 N 个点；顺序与仓储给的 checked_at 升序逐一对应；
// 每个点的 bucket_start 就是这次探测的 checked_at；点级 health 与状态一一对应；
// 并且**没有任何点是 unknown**（旧实现会把没有探测的时间桶补成 unknown 灰柱）。
//
// ⚠️ 行级汇总（成功率 / 健康分）**不再从这些点推导**，而是来自窗口计数 —— 本用例刻意让
// 两者数量不同（柱子 5 根、窗口 12 条），任何「偷偷从 points 汇总」的回归都会立刻变红。
func TestChannelMonitorV1MatrixPointsMirrorProbes(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	monitor := &ChannelMonitor{ID: 7, Name: "渠道 A", Provider: "openai", SortOrder: 10}

	// 探测时刻刻意落在窗口内的整分钟边界附近：前两条相差 45 秒、落在同一个 1 分钟桶里
	// —— 旧实现会合并成一根柱子，新实现必须是两个独立点（这正是本次变更的语义）。
	minute := window.Start.Truncate(time.Minute).Add(2 * time.Minute)
	probes := []struct {
		at      time.Time
		status  string
		latency *int64
		want    string
	}{
		{minute.Add(5 * time.Second), "operational", channelMonitorV1MatrixLatency(120), "healthy"},
		{minute.Add(50 * time.Second), "operational", channelMonitorV1MatrixLatency(130), "healthy"},
		{minute.Add(time.Minute + 10*time.Second), "degraded", nil, "warning"},
		{minute.Add(3*time.Minute + 30*time.Second), "failed", nil, "critical"},
		{minute.Add(5 * time.Minute), "error", nil, "critical"},
	}

	points := make([]ChannelMonitorV1HistoryPoint, 0, len(probes))
	for _, probe := range probes {
		points = append(points, ChannelMonitorV1HistoryPoint{
			MonitorID: monitor.ID,
			CheckedAt: probe.at,
			Status:    probe.status,
			LatencyMs: probe.latency,
		})
	}
	// 窗口计数故意与柱子不同源：窗口内 12 条探测（9 operational + 1 degraded + 1 failed + 1 error），
	// 而柱子只有最新的 5 根 —— 这正是「切档位只改成功率、不改柱子」的由来。
	windowCounts := ChannelMonitorV1StatusCounts{
		TotalChecks: 12, Operational: 9, Degraded: 1, Failed: 1, Error: 1,
	}
	repo := &channelMonitorV1MatrixRepoStub{
		points: map[int64][]ChannelMonitorV1HistoryPoint{monitor.ID: points},
		counts: map[int64]ChannelMonitorV1StatusCounts{monitor.ID: windowCounts},
	}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: []*ChannelMonitor{monitor}}, repo)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Len(t, matrix.Items, 1)
	row := matrix.Items[0]

	// 柱子那一路：只带 monitor ids + 「单条曲线最多 300 个点」的常量上限，
	// **不带任何时间窗口**（柱子与档位无关）。
	require.Equal(t, 1, repo.pointCalls)
	require.Equal(t, []int64{monitor.ID}, repo.pointMonitorIDs)
	// 入参必须是常量本身（而不是散落的字面量）：service 侧只有这一处决定上限。
	require.Equal(t, ChannelMonitorV1MatrixPointLimit, repo.pointLimit)
	require.EqualValues(t, 300, ChannelMonitorV1MatrixPointLimit,
		"点数上限常量必须是 300：前端 PLAZA_PRO_PULSE_SLOTS 与之保持一份真相")

	// 成功率那一路：**一次批量、带请求窗口**。窗口只喂给计数查询，绝不喂给柱子。
	require.Equal(t, 1, repo.countCalls)
	require.Equal(t, []int64{monitor.ID}, repo.countMonitorIDs)
	require.Equal(t, window.Start, repo.countStart, "计数查询用请求窗口的下界")
	require.Equal(t, window.End, repo.countEnd, "计数查询用请求窗口的上界")

	// 一次探测一个点：数量、顺序、时刻逐一镜像，没有任何聚合与补空。
	require.Len(t, row.Buckets, len(probes), "有多少次探测就画多少个点（此处的点与档位无关）")
	for i, point := range row.Buckets {
		require.Equal(t, probes[i].at, point.BucketStart, "bucket_start 就是这次探测的 checked_at")
		require.Equal(t, probes[i].want, point.Health.Overall)
		require.NotEqual(t, "unknown", point.Health.Overall, "曲线上永远不会出现 unknown")
		require.NotNil(t, point.Health.Score)
		require.EqualValues(t, 1, point.Metrics.RequestCount, "每个点恰好 1 次探测")
		// 点级口径：operational 与 degraded（黄）都算成功，只有 failed / error（红）算失败。
		if probes[i].status == "operational" || probes[i].status == "degraded" {
			require.EqualValues(t, 1, point.Metrics.SuccessRequests)
			require.InDelta(t, 1, point.Metrics.SuccessRate, 1e-9)
			require.InDelta(t, 0, point.Metrics.ErrorRate, 1e-9)
		} else {
			require.Zero(t, point.Metrics.SuccessRequests)
			require.InDelta(t, 0, point.Metrics.SuccessRate, 1e-9)
			require.InDelta(t, 1, point.Metrics.ErrorRate, 1e-9)
		}
		if probes[i].latency == nil {
			require.Zero(t, point.Metrics.Duration.SampleCount)
			require.Nil(t, point.Metrics.Duration.AvgMs, "没有延迟样本时 avg 必须是 null")
		} else {
			require.EqualValues(t, 1, point.Metrics.Duration.SampleCount)
			require.NotNil(t, point.Metrics.Duration.AvgMs)
			require.InDelta(t, float64(*probes[i].latency), *point.Metrics.Duration.AvgMs, 1e-9)
		}
	}
	// 前两点间隔 45 秒、同属一个 1 分钟桶，却依然是两根：旧的按桶聚合被彻底移除。
	// 同时也说明这里的点**没有被窗口裁剪**：它们按 checked_at 升序原样呈现。
	require.Equal(t, 45*time.Second, row.Buckets[1].BucketStart.Sub(row.Buckets[0].BucketStart))
	require.Equal(t,
		row.Buckets[0].BucketStart.Truncate(window.Bucket),
		row.Buckets[1].BucketStart.Truncate(window.Bucket),
		"这两次探测落在同一个时间桶里，但必须是两个点",
	)

	// 行级 = **窗口计数**（12 条），不是柱子的 5 条：成功侧 = 9 + 1 = 10（黄色算成功），
	// 失败侧只有 failed + error = 2 ⇒ score = 100 * 10/12 ≈ 83.333、success_rate = 10/12。
	// 这一条断言就是「柱子 5 根、成功率却按 12 条算」的口径证明。
	require.EqualValues(t, 12, row.Metrics.RequestCount, "行级样本量来自窗口计数，不是柱数")
	require.Equal(t, "critical", row.Health.Overall, "窗口内有 failed/error → 档位仍然是 critical")
	require.NotNil(t, row.Health.Score)
	require.InDelta(t, 100.0*10.0/12.0, *row.Health.Score, 1e-9)
	require.EqualValues(t, 10, row.Metrics.SuccessRequests)
	require.EqualValues(t, 2, row.Metrics.ErrorRequests)
	require.InDelta(t, 10.0/12.0, row.Metrics.SuccessRate, 1e-9)
	require.InDelta(t, 2.0/12.0, row.Metrics.ErrorRate, 1e-9)
	// 延迟仍然只取自柱子（窗口计数 SQL 不聚合 latency_ms，Pro 页也不展示延迟）：
	// 只有 2 个点带延迟样本（120 + 130）。
	require.EqualValues(t, 2, row.Metrics.Duration.SampleCount)
	require.NotNil(t, row.Metrics.Duration.AvgMs)
	require.InDelta(t, 125, *row.Metrics.Duration.AvgMs, 1e-9)
}

// TestChannelMonitorV1MatrixNeverEmitsUnknownPoints 覆盖两件事：
//
//  1. 没有探测的时间段**不产生任何点** —— 旧测试（TestChannelMonitorV1MatrixFillsEmptyBuckets）
//     断言的「补 unknown 灰柱」被本用例反转：曲线上永远不会出现 unknown，
//     而「窗口内零探测」的监控曲线就是空的。
//  2. 柱子**不被时间档位裁剪** —— 这里刻意给监控 7 塞了一条**窗口之外**（更早）的探测：
//     它照样画成柱子（最近 300 次探测可以横跨好几天），但**不计入**窗口成功率。
//     这正是主人要的口径：色块看的是「最近 300 次」，时间档位只管成功率。
func TestChannelMonitorV1MatrixNeverEmitsUnknownPoints(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "1h-1m")
	monitors := []*ChannelMonitor{
		{ID: 7, Name: "有探测", Provider: "openai", SortOrder: 10},
		{ID: 8, Name: "少量探测", Provider: "grok", SortOrder: 20},
		{ID: 9, Name: "窗口内零探测", Provider: "gemini", SortOrder: 30},
	}
	// 这条探测比窗口下界还早 3 小时：柱子要画它，窗口成功率不能算它。
	staleAt := window.Start.Add(-3 * time.Hour)
	firstAt := window.Start.Add(7 * time.Minute)
	// 与上一点之间空了 34 分钟，中间不补任何点。
	secondAt := window.Start.Add(41 * time.Minute)
	thirdAt := window.Start.Add(20 * time.Minute)

	repo := &channelMonitorV1MatrixRepoStub{
		points: map[int64][]ChannelMonitorV1HistoryPoint{
			7: {
				{MonitorID: 7, CheckedAt: staleAt, Status: "operational", LatencyMs: channelMonitorV1MatrixLatency(70)},
				{MonitorID: 7, CheckedAt: firstAt, Status: "operational", LatencyMs: channelMonitorV1MatrixLatency(80)},
				{MonitorID: 7, CheckedAt: secondAt, Status: "operational", LatencyMs: channelMonitorV1MatrixLatency(90)},
			},
			8: {
				{MonitorID: 8, CheckedAt: thirdAt, Status: "degraded"},
			},
			// 9 号监控在窗口内没有任何探测 —— 连 map 的 key 都不该有（仓储契约）。
		},
		// 窗口计数只数窗口内的探测：7 号窗口内只有 firstAt / secondAt 两条
		// （staleAt 在窗口外，只出现在柱子里，不进成功率）。
		counts: map[int64]ChannelMonitorV1StatusCounts{
			7: {TotalChecks: 2, Operational: 2},
			8: {TotalChecks: 1, Degraded: 1},
		},
	}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: monitors}, repo)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Len(t, matrix.Items, 3)
	require.Equal(t, []int64{7, 8, 9}, repo.pointMonitorIDs, "三个启用的监控一次批量查完（无 N+1）")
	require.Equal(t, []int64{7, 8, 9}, repo.countMonitorIDs, "窗口计数同样一次批量查完")

	// 有探测的监控：只出它自己的点，一个都不多、一个都不少；**早于窗口的那条也在**。
	require.Len(t, matrix.Items[0].Buckets, 3, "柱子不被窗口裁剪：窗口外那条探测照样画")
	require.Equal(t, staleAt, matrix.Items[0].Buckets[0].BucketStart, "早于窗口下界的探测也在曲线上")
	require.True(t, matrix.Items[0].Buckets[0].BucketStart.Before(window.Start),
		"该点确实在窗口之外 —— 它只属于「最近 300 次探测」，不属于「近 1 小时」")
	require.Equal(t, firstAt, matrix.Items[0].Buckets[1].BucketStart)
	require.Equal(t, secondAt, matrix.Items[0].Buckets[2].BucketStart)
	require.Equal(t, "healthy", matrix.Items[0].Health.Overall)
	// 行级成功率只看窗口内那 2 条（不像柱子那样 3 条）。
	require.EqualValues(t, 2, matrix.Items[0].Metrics.RequestCount, "成功率只数窗口内的探测")
	require.InDelta(t, 1, matrix.Items[0].Metrics.SuccessRate, 1e-9)

	require.Len(t, matrix.Items[1].Buckets, 1)
	require.Equal(t, thirdAt, matrix.Items[1].Buckets[0].BucketStart, "只出自己的点，不串别的监控的探测")
	require.Equal(t, "warning", matrix.Items[1].Health.Overall)

	// 窗口内零探测：曲线为空 —— 不是被补齐的 60 根 unknown 灰柱。
	require.Empty(t, matrix.Items[2].Buckets)
	require.NotNil(t, matrix.Items[2].Buckets, "空曲线必须序列化成 []，不能是 null")
	require.Zero(t, matrix.Items[2].Metrics.RequestCount)
	require.Zero(t, matrix.Items[2].Metrics.SuccessRate)

	// 全局兜底：任何一行、任何一个点都不允许是 unknown。
	for _, row := range matrix.Items {
		for _, point := range row.Buckets {
			require.NotEqual(t, "unknown", point.Health.Overall, "曲线上永远不会出现 unknown")
			require.NotNil(t, point.Health.Score)
		}
	}

	// 行级（整窗口零样本）仍然是 unknown：这是 ChannelMonitorV1StatusCounts.Overall() 的
	// 函数级语义，与「曲线上不出现 unknown」并不矛盾 —— 没有探测就没有点，行级汇总自然无样本。
	require.Equal(t, "unknown", matrix.Items[2].Health.Overall)
	require.Nil(t, matrix.Items[2].Health.Score)
}

// ---------- 6. 用户端脱敏（admin=false） ----------

func TestChannelMonitorV1MatrixRedactsVolumeForOrdinaryUsers(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "1h-1m")
	monitor := &ChannelMonitor{ID: 11, Name: "渠道 B", Provider: "anthropic"}
	// 10 次真实探测：9 次 operational（各带一个 100ms 延迟样本）+ 1 次 degraded（无延迟样本）。
	// 行级口径（**degraded 算成功**）：request_count=10、success_requests=10、success_rate=1.0、
	// error_rate=0、duration.sample_count=9、avg=100ms。
	points := make([]ChannelMonitorV1HistoryPoint, 0, 10)
	for i := 0; i < 9; i++ {
		points = append(points, ChannelMonitorV1HistoryPoint{
			MonitorID: monitor.ID,
			CheckedAt: window.Start.Add(time.Duration(i+1) * time.Minute),
			Status:    "operational",
			LatencyMs: channelMonitorV1MatrixLatency(100),
		})
	}
	points = append(points, ChannelMonitorV1HistoryPoint{
		MonitorID: monitor.ID,
		CheckedAt: window.Start.Add(10 * time.Minute),
		Status:    "degraded",
	})
	repo := &channelMonitorV1MatrixRepoStub{
		points: map[int64][]ChannelMonitorV1HistoryPoint{monitor.ID: points},
		// 窗口计数与柱子（10 条）同量：行级 9 operational + 1 degraded ⇒ 成功率 1.0、error_rate 0。
		counts: map[int64]ChannelMonitorV1StatusCounts{
			monitor.ID: {TotalChecks: 10, Operational: 9, Degraded: 1},
		},
	}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: []*ChannelMonitor{monitor}}, repo)

	admin, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Len(t, admin.Items[0].Buckets, 10, "10 次探测 = 10 个点")
	require.EqualValues(t, 10, admin.Items[0].Metrics.RequestCount)
	require.EqualValues(t, 10, admin.Items[0].Metrics.SuccessRequests, "9 operational + 1 degraded 全算成功")
	require.Zero(t, admin.Items[0].Metrics.ErrorRequests, "degraded 不算失败 ⇒ error_requests=0")
	require.InDelta(t, 1, admin.Items[0].Metrics.SuccessRate, 1e-9)
	require.EqualValues(t, 9, admin.Items[0].Metrics.Duration.SampleCount)
	require.NotNil(t, admin.Items[0].Metrics.Duration.AvgMs)
	require.InDelta(t, 100, *admin.Items[0].Metrics.Duration.AvgMs, 1e-9)

	user, err := svc.Matrix(context.Background(), window, false)
	require.NoError(t, err)
	metrics := user.Items[0].Metrics
	// 绝对量全部置 0（与 V2 用户视图 redactChannelMonitorV2Metric 一致）。
	require.Zero(t, metrics.RequestCount)
	require.Zero(t, metrics.SuccessRequests)
	require.Zero(t, metrics.ErrorRequests)
	require.Zero(t, metrics.TokenCount)
	require.Zero(t, metrics.InputTokens)
	require.Zero(t, metrics.OutputTokens)
	require.Zero(t, metrics.Duration.SampleCount)
	// 比率保留真实值（degraded 算成功 ⇒ 成功率 1.0、错误率 0）。
	require.InDelta(t, 1, metrics.SuccessRate, 1e-9)
	require.InDelta(t, 0, metrics.ErrorRate, 1e-9)
	require.NotNil(t, metrics.Duration.AvgMs)
	require.InDelta(t, 100, *metrics.Duration.AvgMs, 1e-9)
	// 曲线上的点（一次探测一个点）也一并脱敏：绝对量为 0、比率保留。
	require.Len(t, user.Items[0].Buckets, 10)
	for _, point := range user.Items[0].Buckets {
		require.Zero(t, point.Metrics.RequestCount)
		require.Zero(t, point.Metrics.SuccessRequests)
		require.Zero(t, point.Metrics.ErrorRequests)
		require.Zero(t, point.Metrics.Duration.SampleCount)
	}
	require.InDelta(t, 1, user.Items[0].Buckets[0].Metrics.SuccessRate, 1e-9, "operational 点的比率保留")
	require.InDelta(t, 1, user.Items[0].Buckets[9].Metrics.SuccessRate, 1e-9, "degraded 点也算成功，比率保留为 1")
	require.InDelta(t, 0, user.Items[0].Buckets[9].Metrics.ErrorRate, 1e-9, "degraded 点不算失败")
	// 健康度不受脱敏影响。
	require.Equal(t, admin.Items[0].Health.Overall, user.Items[0].Health.Overall)
	require.Equal(t, "warning", user.Items[0].Health.Overall, "有 degraded → warning")
}

// ---------- 7. 分组归属：只认显式绑定的 group_id ----------

// TestChannelMonitorV1MatrixRowUsesBoundGroupIDOnly 钉住「不再猜名字」这条新口径。
//
// 旧实现按三级降级猜（account_id → group_name 同名 → 监控名同名），
// 在同名分组存在时会稳定认领到已软删的那条，使模型广场 Pro 的卡片显示「0 个模型」。
// 现在归属完全由 ChannelMonitor.GroupID 决定：监控名、自由文本分组名标签、
// 关联账号都无权影响它；绑定到已删除/已停用的分组时 group_id 必须留空
// （卡片照常渲染，只是没有模型与定价明细，不会整行消失）。
func TestChannelMonitorV1MatrixRowUsesBoundGroupIDOnly(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	accountID := int64(42)
	boundAlive := int64(5) // 绑定且分组仍在架
	boundDead := int64(9)  // 绑定但分组已删除/停用
	boundZero := int64(0)  // 非法值，视为未绑定
	monitors := []*ChannelMonitor{
		// 故意带上 account_id 与同名 group_name 标签，验证它们都不再参与归属判定。
		{ID: 1, Name: "同名分组渠道", Provider: "openai", AccountID: &accountID, GroupName: "同名分组", GroupID: &boundAlive, SortOrder: 1},
		{ID: 2, Name: "绑到已删除分组", Provider: "grok", GroupID: &boundDead, SortOrder: 2},
		{ID: 3, Name: "没绑定", Provider: "gemini", SortOrder: 3},
		{ID: 4, Name: "绑了非法值", Provider: "anthropic", GroupID: &boundZero, SortOrder: 4},
	}
	repo := &channelMonitorV1MatrixRepoStub{
		liveGroups: ChannelMonitorV1LiveGroupIDs{5: {}},
	}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: monitors}, repo)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Len(t, matrix.Items, 4)

	require.Equal(t, ChannelMonitorV2GroupByPlatformGroup, matrix.GroupBy)

	// 只把「实际绑定过的、去重后的」分组 id 送去查存活：5 与 9，
	// 0 在入参构造阶段就被拦下（<= 0 一律视为未绑定）。
	require.ElementsMatch(t, []int64{5, 9}, repo.groupGroupIDs)

	require.NotNil(t, matrix.Items[0].GroupID)
	require.EqualValues(t, 5, *matrix.Items[0].GroupID)
	require.Equal(t, "同名分组渠道", matrix.Items[0].GroupName)
	require.Equal(t, "openai", matrix.Items[0].Platform)
	require.Empty(t, matrix.Items[0].Model, "V1 一行一个监控，model 留空")

	require.Nil(t, matrix.Items[1].GroupID, "绑定的分组已删除/停用 → 必须留空，而不是照抄 group_id")
	require.Nil(t, matrix.Items[2].GroupID, "未绑定 → nil（字段省略）")
	require.Nil(t, matrix.Items[3].GroupID, "<= 0 视为未绑定")
}

// TestChannelMonitorV1MatrixRowDedupesGroupIDLookup 验证同一分组被多个监控绑定时
// 只向仓储查一次（按 id 去重），避免 N+1 与重复入参。
func TestChannelMonitorV1MatrixRowDedupesGroupIDLookup(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	shared := int64(7)
	other := int64(8)
	monitors := []*ChannelMonitor{
		{ID: 1, Name: "a", Provider: "openai", GroupID: &shared, SortOrder: 1},
		{ID: 2, Name: "b", Provider: "grok", GroupID: &shared, SortOrder: 2},
		{ID: 3, Name: "c", Provider: "gemini", GroupID: &other, SortOrder: 3},
	}
	repo := &channelMonitorV1MatrixRepoStub{
		liveGroups: ChannelMonitorV1LiveGroupIDs{7: {}, 8: {}},
	}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: monitors}, repo)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)

	require.Equal(t, 1, repo.groupCalls, "一次批量查询，不引入 N+1")
	require.Len(t, repo.groupGroupIDs, 2, "7 被两个监控共用，只应出现一次")
	require.ElementsMatch(t, []int64{7, 8}, repo.groupGroupIDs)

	for i, want := range []int64{7, 7, 8} {
		require.NotNil(t, matrix.Items[i].GroupID)
		require.EqualValues(t, want, *matrix.Items[i].GroupID)
	}
}

// TestChannelMonitorV1MatrixNoBoundGroupSkipsLookup 验证「一个都没绑定」时
// 完全不查存活表（入参为空直接短路）。
func TestChannelMonitorV1MatrixNoBoundGroupSkipsLookup(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	monitors := []*ChannelMonitor{
		{ID: 1, Name: "a", Provider: "openai", SortOrder: 1},
		{ID: 2, Name: "b", Provider: "grok", SortOrder: 2},
	}
	repo := &channelMonitorV1MatrixRepoStub{}
	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{monitors: monitors}, repo)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)

	require.Empty(t, repo.groupGroupIDs, "没有任何绑定 → 入参为空，仓储侧短路")
	for _, item := range matrix.Items {
		require.Nil(t, item.GroupID)
	}
}

// ---------- 8. coverage ----------

func TestChannelMonitorV1MatrixCoverage(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "12h-5m")
	minChecked := window.Start.Add(-6 * time.Hour)
	maxChecked := window.Now.Add(-time.Minute)
	repo := &channelMonitorV1MatrixRepoStub{
		coverage: map[int64]ChannelMonitorV1CoverageBounds{
			1: {MinCheckedAt: minChecked, MaxCheckedAt: maxChecked},
		},
	}
	svc := NewChannelMonitorV1MatrixService(
		&channelMonitorV1MatrixMonitorsStub{monitors: []*ChannelMonitor{{ID: 1, Name: "有数据", Provider: "openai"}}},
		repo,
	)

	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Equal(t, window.Start, matrix.Coverage.RequestedStart)
	require.Equal(t, window.End, matrix.Coverage.RequestedEnd)
	require.EqualValues(t, 300, matrix.Coverage.BucketSeconds)
	require.Equal(t, window.Now, matrix.Coverage.ComputedAt)
	require.Equal(t, minChecked, matrix.Coverage.CoverageStart)
	require.Equal(t, maxChecked, matrix.Coverage.DataThrough)
	require.True(t, matrix.Coverage.CoverageComplete, "历史覆盖到窗口起点")
	require.EqualValues(t, int64(time.Minute.Seconds()), matrix.Coverage.AggregationLagSeconds)
	require.Nil(t, matrix.Coverage.Bootstrap)

	// 无数据：data_through 回落 start、coverage_start 回落 end、coverage_complete=false。
	empty := &channelMonitorV1MatrixRepoStub{}
	emptySvc := NewChannelMonitorV1MatrixService(
		&channelMonitorV1MatrixMonitorsStub{monitors: []*ChannelMonitor{{ID: 2, Name: "无数据", Provider: "grok"}}},
		empty,
	)
	emptyMatrix, err := emptySvc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Equal(t, window.Start, emptyMatrix.Coverage.DataThrough)
	require.Equal(t, window.End, emptyMatrix.Coverage.CoverageStart)
	require.False(t, emptyMatrix.Coverage.CoverageComplete)

	// 没有启用的监控：仍然是 200 同形结构（空 items、非 nil）。
	noMonitors := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{}, &channelMonitorV1MatrixRepoStub{})
	emptyItems, err := noMonitors.Matrix(context.Background(), window, false)
	require.NoError(t, err)
	require.NotNil(t, emptyItems.Items)
	require.Empty(t, emptyItems.Items)
	require.Equal(t, ChannelMonitorV2GroupByPlatformGroup, emptyItems.GroupBy)
}

// TestChannelMonitorV1MatrixOrdersRowsBySortOrder 保证前端卡片顺序稳定。
func TestChannelMonitorV1MatrixOrdersRowsBySortOrder(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	monitors := []*ChannelMonitor{
		{ID: 3, Name: "c", Provider: "openai", SortOrder: 20},
		{ID: 1, Name: "a", Provider: "openai", SortOrder: 10},
		{ID: 2, Name: "b", Provider: "openai", SortOrder: 10},
	}
	svc := NewChannelMonitorV1MatrixService(
		&channelMonitorV1MatrixMonitorsStub{monitors: monitors},
		&channelMonitorV1MatrixRepoStub{},
	)
	matrix, err := svc.Matrix(context.Background(), window, true)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, []string{
		matrix.Items[0].GroupName, matrix.Items[1].GroupName, matrix.Items[2].GroupName,
	})
}

// TestNewEmptyChannelMonitorV1MatrixIsFrontendSafe 功能未开启时的 200 空返回
// 仍要带可用的 coverage，避免前端拿到零值时间戳算出无意义横轴。
func TestNewEmptyChannelMonitorV1MatrixIsFrontendSafe(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "7d-1h")
	matrix := NewEmptyChannelMonitorV1Matrix(window)
	require.Equal(t, ChannelMonitorV2GroupByPlatformGroup, matrix.GroupBy)
	require.NotNil(t, matrix.Items)
	require.Empty(t, matrix.Items)
	require.Equal(t, window.Start, matrix.Coverage.RequestedStart)
	require.Equal(t, window.End, matrix.Coverage.RequestedEnd)
	require.EqualValues(t, 3600, matrix.Coverage.BucketSeconds)
	require.Equal(t, window.Now, matrix.Coverage.ComputedAt)
	require.Equal(t, window.Start, matrix.Coverage.DataThrough)
	require.Equal(t, window.End, matrix.Coverage.CoverageStart)
	require.False(t, matrix.Coverage.CoverageComplete)
}

// TestChannelMonitorV1MatrixServiceRejectsUnconfigured 防御性：未接线的 service 明确报错。
func TestChannelMonitorV1MatrixServiceRejectsUnconfigured(t *testing.T) {
	window := mustChannelMonitorV1MatrixWindow(t, "30m-1m")
	_, err := NewChannelMonitorV1MatrixService(nil, nil).Matrix(context.Background(), window, false)
	require.Error(t, err)

	svc := NewChannelMonitorV1MatrixService(&channelMonitorV1MatrixMonitorsStub{}, &channelMonitorV1MatrixRepoStub{})
	_, err = svc.Matrix(context.Background(), ChannelMonitorV1MatrixWindow{}, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrChannelMonitorV2InvalidRange))
}
