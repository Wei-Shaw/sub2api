package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// channelMonitorV1MatrixRepository 实现 service.ChannelMonitorV1MatrixRepository。
//
// 纯只读：只查 channel_monitor_histories / account_groups / groups 三张既有表的聚合，
// 不新增迁移、不写任何数据。选型与 channel_monitor_repo.go 的聚合查询一致——
// 原生 SQL + r.db.QueryContext，避免 ent 在 GROUP BY/unnest 上的样板代码，
// 并保证 (monitor_id, model, checked_at) 索引能被命中。
type channelMonitorV1MatrixRepository struct {
	db *sql.DB
}

// NewChannelMonitorV1MatrixRepository 创建 V1 矩阵仓储实例。
func NewChannelMonitorV1MatrixRepository(db *sql.DB) service.ChannelMonitorV1MatrixRepository {
	return &channelMonitorV1MatrixRepository{db: db}
}

// channelMonitorV1MatrixRecentPointsSQL 取每个监控**最新 N 条**探测明细 —— 与任何时间窗口无关。
//
// ⚠️ 语义（主人 2026-10 明确）：脉冲色块永远显示「最近 N 次探测」，
// 时间档位（近 30 分钟 / 近 1 小时 / … / 近 30 天）**只影响探测成功率统计**，
// 不参与柱子的筛选：切到「近 1 小时」不会让柱子缩成只剩 1 小时内的那几条。
// 所以本 SQL **没有 checked_at 过滤**，也**不做任何按时间的均匀抽样**
// （上一轮的「按时间均匀抽样」设计已整体回退，不要再加回来）。
//
// 一次查询覆盖所有监控（monitor_id = ANY($1)），避免 N+1。
//
// 之所以套一层窗口函数而不是直接 ORDER BY checked_at DESC LIMIT $2：
// 需要「每个监控各自取最新的 N 条」，全局 LIMIT 做不到；同时最终返回又必须按时间升序
// （前端横轴是「过去 → 现在」）。所以内层按 checked_at DESC（加上 id 兜底）编号，
// 外层用 `rn <= N` 只保留最新 N 条，再升序返回；同秒并列时用 rn DESC 让「更早那条」排在前面。
//
// 参数：$1 = monitor id 数组；$2 = 每个监控最多取多少条（ChannelMonitorV1MatrixPointLimit）。
const channelMonitorV1MatrixRecentPointsSQL = `
SELECT monitor_id, checked_at, status, latency_ms
FROM (
  SELECT monitor_id,
         checked_at,
         status,
         latency_ms,
         row_number() OVER (PARTITION BY monitor_id ORDER BY checked_at DESC, id DESC) AS rn
  FROM channel_monitor_histories
  WHERE monitor_id = ANY($1)
) ranked
WHERE ranked.rn <= $2
ORDER BY monitor_id, checked_at, ranked.rn DESC
`

// LoadMonitorV1RecentPoints 返回 map[monitorID] -> 该监控**最新的至多 limit 条**探测记录，
// 按 checked_at 升序（最旧在前）。没有记录的监控不会出现在结果里。
//
// ⚠️ 这里**没有时间窗口参数**，是刻意的：色块 = 最近 limit 次探测，与所选档位无关。
// 窗口内的状态计数走另一条路（LoadMonitorV1WindowCounts），只有成功率统计会用到它。
//
// 每个元素就是一次真实探测、**不做时间分桶**：脉冲曲线因此「有多少次探测就画多少个点」，
// 没有探测的时间段不产生任何点（渠道监控被关掉时不留下灰色空档）。
func (r *channelMonitorV1MatrixRepository) LoadMonitorV1RecentPoints(
	ctx context.Context,
	monitorIDs []int64,
	limit int,
) (map[int64][]service.ChannelMonitorV1HistoryPoint, error) {
	out := make(map[int64][]service.ChannelMonitorV1HistoryPoint, len(monitorIDs))
	if len(monitorIDs) == 0 || limit <= 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, channelMonitorV1MatrixRecentPointsSQL, pq.Array(monitorIDs), limit)
	if err != nil {
		return nil, fmt.Errorf("query channel monitor v1 matrix recent points: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			monitorID int64
			point     service.ChannelMonitorV1HistoryPoint
			latency   sql.NullInt64
		)
		if err := rows.Scan(&monitorID, &point.CheckedAt, &point.Status, &latency); err != nil {
			return nil, fmt.Errorf("scan channel monitor v1 matrix recent point: %w", err)
		}
		point.MonitorID = monitorID
		point.CheckedAt = point.CheckedAt.UTC()
		point.LatencyMs = nullInt64Pointer(latency)
		out[monitorID] = append(out[monitorID], point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// channelMonitorV1MatrixWindowCountsSQL 取每个监控在 [start, end) 窗口内的 4 态探测计数。
//
// 这是**唯一**与所选时间档位相关的一路数据：卡片的探测成功率 / 健康分 / success_requests /
// error_requests 全部由它算出来。它**不**决定脉冲色块 —— 色块恒为「最近 N 次探测」，
// 与窗口无关（见 channelMonitorV1MatrixRecentPointsSQL）。
//
// 按 (monitor_id, status) 分组返回原始计数，由 service 侧折成 ChannelMonitorV1StatusCounts；
// 一次批量查完所有监控，避免 N+1。窗口内零探测的监控不会出现在结果里（service 兜底成零值）。
//
// 参数：$1 = monitor id 数组；$2 = 窗口下界（含）；$3 = 窗口上界（不含）。
const channelMonitorV1MatrixWindowCountsSQL = `
SELECT monitor_id, status, count(*)::bigint AS total
FROM channel_monitor_histories
WHERE monitor_id = ANY($1)
  AND checked_at >= $2
  AND checked_at < $3
GROUP BY monitor_id, status
`

// LoadMonitorV1WindowCounts 返回 map[monitorID] -> 该监控在 [start, end) 内的 4 态探测计数。
// 窗口内零探测的监控不会出现在 map 中（调用方按零值处理，即 Health=unknown / 成功率 0）。
//
// start/end 就是请求解析出来的展示窗口（与 ChannelMonitorV1MatrixWindow 完全一致）。
func (r *channelMonitorV1MatrixRepository) LoadMonitorV1WindowCounts(
	ctx context.Context,
	monitorIDs []int64,
	start time.Time,
	end time.Time,
) (map[int64]service.ChannelMonitorV1StatusCounts, error) {
	out := make(map[int64]service.ChannelMonitorV1StatusCounts, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, channelMonitorV1MatrixWindowCountsSQL, pq.Array(monitorIDs), start, end)
	if err != nil {
		return nil, fmt.Errorf("query channel monitor v1 matrix window counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			monitorID int64
			status    string
			total     int64
		)
		if err := rows.Scan(&monitorID, &status, &total); err != nil {
			return nil, fmt.Errorf("scan channel monitor v1 matrix window count: %w", err)
		}
		counts := out[monitorID]
		counts.AddStatusCount(status, total)
		out[monitorID] = counts
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// channelMonitorV1MatrixCoverageSQL 取每个监控全部历史（明细保留 30 天）的最早/最晚探测时间。
// 用于 coverage.coverage_start / data_through；只返回有数据的监控，缺行由 service 兜底。
const channelMonitorV1MatrixCoverageSQL = `
SELECT monitor_id, min(checked_at), max(checked_at)
FROM channel_monitor_histories
WHERE monitor_id = ANY($1)
GROUP BY monitor_id
`

// LoadMonitorV1Coverage 返回 map[monitorID] -> 该监控历史的最早/最晚 checked_at。
func (r *channelMonitorV1MatrixRepository) LoadMonitorV1Coverage(
	ctx context.Context,
	monitorIDs []int64,
) (map[int64]service.ChannelMonitorV1CoverageBounds, error) {
	out := make(map[int64]service.ChannelMonitorV1CoverageBounds, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, channelMonitorV1MatrixCoverageSQL, pq.Array(monitorIDs))
	if err != nil {
		return nil, fmt.Errorf("query channel monitor v1 matrix coverage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			monitorID int64
			bounds    service.ChannelMonitorV1CoverageBounds
		)
		if err := rows.Scan(&monitorID, &bounds.MinCheckedAt, &bounds.MaxCheckedAt); err != nil {
			return nil, fmt.Errorf("scan channel monitor v1 matrix coverage: %w", err)
		}
		bounds.MinCheckedAt = bounds.MinCheckedAt.UTC()
		bounds.MaxCheckedAt = bounds.MaxCheckedAt.UTC()
		out[monitorID] = bounds
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// channelMonitorV1MatrixLiveGroupIDsSQL 从给定分组 id 里筛出「当前仍可用」的那些。
//
// 判定口径与 ChannelMonitorV2 完全一致（`deleted_at IS NULL AND status = 'active'`），
// 这是刻意的：V1 与 V2 只是取数方式不同，对「一个分组算不算在架」的定义必须一致，
// 否则同一张卡片在两种模式下会得出不同结论。
//
// 为什么不能只靠外键：channel_monitors.group_id 上的外键只覆盖**硬删除**，
// 而后台删除分组写的是 deleted_at（软删除），ON DELETE SET NULL 不会触发。
// 少了这道过滤，卡片就会 join 到一个已下线的分组、拿到空模型表。
const channelMonitorV1MatrixLiveGroupIDsSQL = `
SELECT id
FROM groups
WHERE id = ANY($1::bigint[])
  AND deleted_at IS NULL
  AND status = 'active'
`

// LoadMonitorV1LiveGroupIDs 返回给定分组 id 中「仍然可用」的子集。
//
// 取代了早期的 LoadMonitorV1GroupCandidates：那时要传监控 id、账号 id、分组名标签、
// 监控名四列去做三级降级匹配，现在归属由 channel_monitors.group_id 直接声明，
// 这里只负责回答「它绑定分组的还活着吗」。入参为空时直接返回空集合，不发查询。
func (r *channelMonitorV1MatrixRepository) LoadMonitorV1LiveGroupIDs(
	ctx context.Context,
	groupIDs []int64,
) (service.ChannelMonitorV1LiveGroupIDs, error) {
	out := make(service.ChannelMonitorV1LiveGroupIDs, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.QueryContext(ctx, channelMonitorV1MatrixLiveGroupIDsSQL, pq.Array(groupIDs))
	if err != nil {
		return nil, fmt.Errorf("query channel monitor v1 matrix live group ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			return nil, fmt.Errorf("scan channel monitor v1 matrix live group id: %w", err)
		}
		out[groupID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// nullInt64Pointer 把可空 bigint 解包成 *int64（NULL → nil）。
func nullInt64Pointer(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
