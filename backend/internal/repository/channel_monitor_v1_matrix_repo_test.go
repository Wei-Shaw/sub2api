//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func newChannelMonitorV1MatrixRepo(t *testing.T) (*channelMonitorV1MatrixRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &channelMonitorV1MatrixRepository{db: db}, mock
}

func TestChannelMonitorV1MatrixLoadRecentPoints(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)
	firstAt := time.Date(2026, 9, 29, 12, 5, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)
	// 带时区的时间：仓储必须归一化成 UTC（前端横轴按 UTC 解析）。
	thirdAt := time.Date(2026, 9, 29, 20, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	// limit 用产品常量而不是散落的字面量：$2 是「每个监控最多取多少条最新探测」，
	// 必须与 service 侧一致。⚠️ 这里**没有** start/end 参数 —— 柱子与时间档位无关。
	mock.ExpectQuery(channelMonitorV1MatrixRecentPointsSQL).
		WithArgs(pq.Array([]int64{1, 2}), service.ChannelMonitorV1MatrixPointLimit).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "checked_at", "status", "latency_ms"}).
			AddRow(int64(1), firstAt, "operational", int64(120)).
			AddRow(int64(1), secondAt, "degraded", nil).
			AddRow(int64(2), thirdAt, "failed", nil))

	out, err := repo.LoadMonitorV1RecentPoints(context.Background(), []int64{1, 2}, service.ChannelMonitorV1MatrixPointLimit)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Len(t, out[1], 2, "两次探测就是两条记录，不做时间分桶聚合")

	require.Equal(t, int64(1), out[1][0].MonitorID)
	require.Equal(t, firstAt, out[1][0].CheckedAt)
	require.Equal(t, "operational", out[1][0].Status)
	require.NotNil(t, out[1][0].LatencyMs)
	require.EqualValues(t, 120, *out[1][0].LatencyMs)

	// 返回顺序 = SQL 的升序输出（最旧在前、最新在后）。
	require.Equal(t, secondAt, out[1][1].CheckedAt)
	require.Equal(t, "degraded", out[1][1].Status)
	require.Nil(t, out[1][1].LatencyMs, "NULL latency_ms 必须解成 nil（探测失败通常没有延迟样本）")
	require.True(t, out[1][0].CheckedAt.Before(out[1][1].CheckedAt))

	// 非 UTC 的行也要归一化成 UTC。
	require.Equal(t, thirdAt.UTC(), out[2][0].CheckedAt)
	require.Equal(t, time.UTC, out[2][0].CheckedAt.Location())
	require.Equal(t, "failed", out[2][0].Status)

	// 没有记录的监控不出现在 map 里（由 service 兜底）。
	_, ok := out[3]
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV1MatrixLoadRecentPointsSkipsQueryWithoutMonitors(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)

	out, err := repo.LoadMonitorV1RecentPoints(context.Background(), nil, service.ChannelMonitorV1MatrixPointLimit)
	require.NoError(t, err)
	require.Empty(t, out)
	// limit <= 0 等价于「一个点都不要」，同样不发查询。
	out, err = repo.LoadMonitorV1RecentPoints(context.Background(), []int64{1}, 0)
	require.NoError(t, err)
	require.Empty(t, out)
	// 没有任何预期查询被发出。
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestChannelMonitorV1MatrixLoadWindowCounts 验证「窗口内 4 态计数」的解包与折叠：
// 同一监控的多行 (status, total) 必须累加到同一个计数结构里，且 TotalChecks = 各状态之和。
func TestChannelMonitorV1MatrixLoadWindowCounts(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(channelMonitorV1MatrixWindowCountsSQL).
		WithArgs(pq.Array([]int64{1, 2}), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "status", "total"}).
			AddRow(int64(1), "operational", int64(9)).
			AddRow(int64(1), "degraded", int64(2)).
			AddRow(int64(1), "failed", int64(1)).
			AddRow(int64(2), "error", int64(4)))

	out, err := repo.LoadMonitorV1WindowCounts(context.Background(), []int64{1, 2}, start, end)
	require.NoError(t, err)
	require.Len(t, out, 2)

	require.EqualValues(t, 12, out[1].TotalChecks)
	require.EqualValues(t, 9, out[1].Operational)
	require.EqualValues(t, 2, out[1].Degraded)
	require.EqualValues(t, 1, out[1].Failed)
	require.Zero(t, out[1].Error)
	require.InDelta(t, 11.0/12.0, out[1].SuccessRate(), 1e-9, "degraded 算成功：(9+2)/12")

	require.EqualValues(t, 4, out[2].TotalChecks)
	require.EqualValues(t, 4, out[2].Error)
	require.Equal(t, "critical", out[2].Overall())

	// 窗口内零探测的监控不出现在 map 里（由 service 兜底成零值）。
	_, ok := out[3]
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestChannelMonitorV1MatrixLoadWindowCountsSkipsQueryWithoutMonitors 空入参不发查询。
func TestChannelMonitorV1MatrixLoadWindowCountsSkipsQueryWithoutMonitors(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)
	out, err := repo.LoadMonitorV1WindowCounts(context.Background(), nil, time.Now(), time.Now())
	require.NoError(t, err)
	require.Empty(t, out)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestChannelMonitorV1MatrixAddStatusCountIgnoresUnknownStatus 未知状态只进分母、不落桶
// —— 宁可低估成功率，也不能把没见过的状态算成成功。
func TestChannelMonitorV1MatrixAddStatusCountIgnoresUnknownStatus(t *testing.T) {
	var counts service.ChannelMonitorV1StatusCounts
	counts.AddStatusCount("operational", 3)
	counts.AddStatusCount("weird", 2)
	counts.AddStatusCount("failed", 0)
	require.EqualValues(t, 5, counts.TotalChecks, "未知状态也进分母")
	require.EqualValues(t, 3, counts.Operational)
	require.Zero(t, counts.Failed, "count=0 是空操作，不能凭空造出一条失败")
	require.EqualValues(t, 0.6, counts.SuccessRate())
}

func TestChannelMonitorV1MatrixLoadCoverage(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)
	minChecked := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	maxChecked := time.Date(2026, 9, 29, 4, 5, 6, 0, time.UTC)

	mock.ExpectQuery(channelMonitorV1MatrixCoverageSQL).
		WithArgs(pq.Array([]int64{7})).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "min", "max"}).
			AddRow(int64(7), minChecked, maxChecked))

	out, err := repo.LoadMonitorV1Coverage(context.Background(), []int64{7})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, minChecked, out[7].MinCheckedAt)
	require.Equal(t, maxChecked, out[7].MaxCheckedAt)
	// 没有数据的监控不出现在 map 里（由 service 兜底）。
	_, ok := out[8]
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestChannelMonitorV1MatrixLoadLiveGroupIDs 验证「绑定分组是否仍在架」的过滤结果。
//
// 这条查询取代了早期的三级降级候选：现在归属由 channel_monitors.group_id 直接声明，
// 仓储只负责回答「它指向的分组还活着吗」—— 未软删除且 status='active' 才算活着。
func TestChannelMonitorV1MatrixLoadLiveGroupIDs(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)

	mock.ExpectQuery(channelMonitorV1MatrixLiveGroupIDsSQL).
		WithArgs(pq.Array([]int64{5, 9, 37})).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(int64(5)).
			AddRow(int64(37)))

	out, err := repo.LoadMonitorV1LiveGroupIDs(context.Background(), []int64{5, 9, 37})
	require.NoError(t, err)
	// 9 已删除/停用 → 不在集合里；调用方据此把该行的 group_id 留空。
	require.Len(t, out, 2)
	_, ok := out[5]
	require.True(t, ok)
	_, ok = out[37]
	require.True(t, ok)
	_, ok = out[9]
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorV1MatrixLoadLiveGroupIDsSkipsQueryWithoutIDs(t *testing.T) {
	repo, mock := newChannelMonitorV1MatrixRepo(t)
	out, err := repo.LoadMonitorV1LiveGroupIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, out)
	// 入参为空必须短路，不能发出 `= ANY('{}')` 这种无意义查询。
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestChannelMonitorV1MatrixSQLContract 钉住四条 SQL 的关键语义：
// 探测明细直出（一次探测一行、不做时间分桶聚合）+ **每个监控只取最新的 N 条**、
// 窗口内 4 态计数单独一条 SQL（**只有它**带 checked_at 过滤）、
// 按监控批量（无 N+1）、而「绑定分组是否仍在架」用一条**纯 id 集合**查询
// （不再有任何按名字猜的痕迹）—— 后面这段是本文件最重要的一道防回归闸门。
//
// ⚠️ 两条口径必须严格分离（主人明确要求）：
// 柱子 = 最近 N 次探测，**与时间档位无关** → points SQL 里不允许出现任何 checked_at 过滤，
// 也不允许再出现「按时间均匀抽样」表达式；窗口只用于成功率统计 → counts SQL 才带 checked_at。
func TestChannelMonitorV1MatrixSQLContract(t *testing.T) {
	points := strings.ToLower(channelMonitorV1MatrixRecentPointsSQL)
	require.Contains(t, points, "from channel_monitor_histories")
	require.Contains(t, points, "monitor_id = any($1)")
	// 每个监控各自编号：内层 row_number（DESC ⇒ rn=1 是最新一条）。
	require.Contains(t, points, "row_number() over (partition by monitor_id order by checked_at desc, id desc)")
	// 只取最新 N 条（rn <= $2）；总数 count(*) over (...) 已不再需要。
	require.Contains(t, points, "ranked.rn <= $2")
	require.NotContains(t, points, "count(*) over (partition by monitor_id)")
	// 柱子与时间档位彻底解耦：SQL 里不许有任何时间过滤。
	require.NotContains(t, points, "checked_at >=")
	require.NotContains(t, points, "checked_at <")
	require.NotContains(t, points, "between")
	// 「按时间均匀抽样」的旧设计已整体回退，不允许再回来：
	// 它会让柱子不再是「最近 N 次探测」，切档位时柱形整体重排。
	require.NotContains(t, points, "% greatest(")
	require.NotContains(t, points, "ceil(")
	require.NotContains(t, points, "::numeric")
	// 参数收敛成两个：$1 = monitor ids，$2 = limit。
	require.NotContains(t, points, "$3")
	require.NotContains(t, points, "$4")
	// 最终按 checked_at 升序（最旧在前），与前端横轴「过去 → 现在」一致；同秒并列用 rn DESC 兜底。
	require.Contains(t, points, "order by monitor_id, checked_at")
	// 一次探测一个点：不聚合、不分桶、不汇总 latency。
	require.NotContains(t, points, "group by")
	require.NotContains(t, points, "count(*) filter")
	require.NotContains(t, points, "floor(extract(epoch")

	counts := strings.ToLower(channelMonitorV1MatrixWindowCountsSQL)
	require.Contains(t, counts, "from channel_monitor_histories")
	require.Contains(t, counts, "monitor_id = any($1)")
	require.Contains(t, counts, "checked_at >= $2")
	require.Contains(t, counts, "checked_at < $3")
	require.Contains(t, counts, "group by monitor_id, status")
	require.Contains(t, counts, "count(*)::bigint as total")
	// 计数 SQL 不做抽样、不碰明细列；窗口是它唯一的筛选条件。
	require.NotContains(t, counts, "row_number()")
	require.NotContains(t, counts, "latency_ms")
	require.NotContains(t, counts, "greatest(")

	coverage := strings.ToLower(channelMonitorV1MatrixCoverageSQL)
	require.Contains(t, coverage, "min(checked_at)")
	require.Contains(t, coverage, "max(checked_at)")
	require.Contains(t, coverage, "monitor_id = any($1)")

	// 「监控 → 广场分组」的解析已整体重写：归属由 channel_monitors.group_id
	// 直接声明，仓储只判断「绑定的分组还在不在架」。
	//
	// ⚠️ 下面这组 NotContains 是**防回归闸门**，不是凑数：
	// 早期实现靠三级降级猜名字（account_groups ＞ group_name 同名 ＞ 监控名同名），
	// 两级同名匹配遇到「删掉旧分组又建同名新分组」时会稳定认领到已软删的那条，
	// 使模型广场 Pro 的卡片显示「0 个模型」。只要谁把这些痕迹加回来，测试立刻红。
	surviving := strings.ToLower(channelMonitorV1MatrixLiveGroupIDsSQL)
	require.Contains(t, surviving, "from groups")
	require.Contains(t, surviving, "id = any($1::bigint[])")
	// 在架判定必须与 ChannelMonitorV2 完全一致：软删除或停用都算「不可用」。
	// 少了这两条，卡片就会 join 到一个已下线的分组、拿到空模型表。
	require.Contains(t, surviving, "deleted_at is null")
	require.Contains(t, surviving, "status = 'active'")
	// 猜测逻辑的全部痕迹都不允许回来：
	require.NotContains(t, surviving, "group_name", "自由文本分组名标签不得再参与归属判定")
	require.NotContains(t, surviving, "monitor_name", "监控名不得再参与归属判定")
	require.NotContains(t, surviving, "account_groups", "关联账号不得再参与归属判定")
	require.NotContains(t, surviving, "left join lateral")
	require.NotContains(t, surviving, "limit 1", "不再需要防重名的 LIMIT 1：按 id 直查不会放大行数")
	require.NotContains(t, surviving, "unnest(")
}
