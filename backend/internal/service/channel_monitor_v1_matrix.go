package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 本文件为「模型广场 Pro × 渠道监控 V1 主动探测」新增的只读矩阵端点提供 service 层能力。
//
// 设计目标：响应 JSON 与 V2 matrix 完全同形，直接复用 ChannelMonitorV2Matrix /
// ChannelMonitorV2MatrixRow / ChannelMonitorV2TrendPoint / ChannelMonitorV2Metric /
// ChannelMonitorV2Health / ChannelMonitorV2Coverage，前端零改动解析。
//
// 与 V2 的唯一语义差别：V1 一个渠道监控 = 一张卡，
// 因此每行是「一个启用的监控」而不是「platform × group × model」。
//
// ⚠️ V1 行内有**两种时间口径**，改代码前必须先分清（主人明确要求）：
//
//	· buckets（脉冲色块）= 最近 300 次探测，**与所选时间档位完全无关**；
//	· metrics / health（探测成功率、健康分）= **所选窗口内**的统计。
//
// 于是切档位只会让成功率的样本量变化，柱子的根数不会变；反之刷新时柱子会跟着新增探测增长。

const (
	channelMonitorV1MatrixHealthHealthy  = "healthy"
	channelMonitorV1MatrixHealthWarning  = "warning"
	channelMonitorV1MatrixHealthCritical = "critical"
	channelMonitorV1MatrixHealthUnknown  = "unknown"
)

// ChannelMonitorV1MatrixMinimumSample V1 探测矩阵的最小样本阈值。
// V1 的健康度由「最差状态」判定（不依赖样本量打分），因此固定为 1：
// 只要桶里有一条探测记录，healthy/warning/critical 就有意义。
const ChannelMonitorV1MatrixMinimumSample int64 = 1

// ChannelMonitorV1MatrixPointLimit 是单条脉冲曲线最多画出的探测点数。
//
// ⚠️ 语义是「一个点 = 一次探测」，**不是**「一个点 = 一段时间」。历史教训：
// 早期按 range 固定切时间桶（近 30 分钟 = 30 个 1 分钟格），再把没有探测的格子补成
// `unknown` 渲染成灰色「样本不足」。但探测间隔是可以配的（dev 环境就有 300 秒的监控），
// 300 秒间隔在 1 分钟格里必然 5 格空 4 格 —— 于是页面上出现大片灰色，看起来像「数据缺失」，
// 其实是格子切得比探测频率细得多。改成「有多少条探测就画多少个点」之后：
//
//	· 300 秒间隔的监控在「近 30 分钟」里就是 5 个点，一格灰的都没有；
//	· 渠道监控被关掉的那段时间本来就没有探测记录，因此不产生任何点，
//	  相邻两点自然相连（而不是插一段灰色空档）；
//	· 渠道开着但挂了的时候探测照常发生、状态是 failed/error，于是画成红色 —— 红色只代表「探测失败」，
//	  「样本不足」这个第四态在脉冲曲线上彻底不再出现。
//
// ⚠️⚠️ 取数口径（与前端槽位数必须一致：`PLAZA_PRO_PULSE_SLOTS`）：
// **柱子 = 最近 N 次探测，与所选时间档位完全无关**。不按时间过滤、也不做均匀抽样，
// 就是「最新的 N 条」（SQL 侧实现见 `channelMonitorV1MatrixRecentPointsSQL` 的 `rn <= N`）。
//
// 这一条是主人明确要求的：早期版本让档位同时决定柱子与成功率，于是「近 1 小时」和
// 「近 7 天」的柱子在抽样差异下看起来是两个东西；中间还试过「按时间均匀抽样铺满整个窗口」，
// 结果是柱子不再是「最近 N 次探测」，用户切档位时柱形会整体重排，反而更难读。
// 现在两者彻底解耦：
//
//	· 柱子   → 最近 300 次探测（恒为 300 或更少，与档位无关）；
//	· 成功率 → 所选窗口内的探测统计（见 ChannelMonitorV1MatrixRepo.LoadMonitorV1WindowCounts）。
//
// 柱数变少时**柱宽不变**：前端柱宽由固定槽位数决定，与本次实际返回多少条无关。
const ChannelMonitorV1MatrixPointLimit = 300

// channelMonitorV1MatrixRangeTokens 是 V1 矩阵接受的 range token 白名单。
//
// 与 channelMonitorV2DenseRangeTokens 的 window/bucket 一一对应（30m-1m / 1h-1m /
// 12h-5m / 24h-5m / 7d-1h / 30d-12h）。显式白名单而非通用字符串解析，保证
// "9999d-1s"、"30m-7s" 这类组合被拒绝而不是被静默接受。
//
// 与官方解析的一致性由 channel_monitor_v1_matrix_test.go 的
// TestChannelMonitorV1MatrixRangeTokensMatchOfficialParseFilter 钉住：
// 该测试逐 token 对比 ChannelMonitorV2Service.ParseFilter 的输出。
var channelMonitorV1MatrixRangeTokens = map[string]struct {
	window time.Duration
	bucket time.Duration
}{
	"30m-1m":  {window: 30 * time.Minute, bucket: time.Minute},
	"1h-1m":   {window: time.Hour, bucket: time.Minute},
	"12h-5m":  {window: 12 * time.Hour, bucket: 5 * time.Minute},
	"24h-5m":  {window: 24 * time.Hour, bucket: 5 * time.Minute},
	"7d-1h":   {window: 7 * 24 * time.Hour, bucket: time.Hour},
	"30d-12h": {window: 30 * 24 * time.Hour, bucket: 12 * time.Hour},
}

// ChannelMonitorV1MatrixWindow 是一次请求解析后的展示窗口。
type ChannelMonitorV1MatrixWindow struct {
	// Range 归一化后的 token（已 TrimSpace）。
	Range string
	// Now 请求时刻（UTC），同时作为 coverage.computed_at。
	Now time.Time
	// Start 窗口下界（含），对齐规则与 ChannelMonitorV2Service.ParseFilter 完全一致。
	Start time.Time
	// End 窗口上界（不含）；bucket > 1m 时可能略晚于 Now（对齐到下一个整桶）。
	End time.Time
	// Bucket 分桶粒度。
	Bucket time.Duration
}

// ParseChannelMonitorV1MatrixWindow 解析 range token 并对齐窗口。
//
// ⚠️ 这里的 switch / 对齐分支是**刻意复制** ChannelMonitorV2Service.ParseFilter 的
// 窗口计算逻辑（ParseFilter 是 V2 service 的方法，需要 V2 service 实例，且按最小改动
// 原则不能改动官方文件）。另有测试
// TestChannelMonitorV1MatrixRangeTokensMatchOfficialParseFilter 逐 token 钉住
// 两者 (start, end, bucket) 完全相等，防止这段复制逻辑漂移。
func ParseChannelMonitorV1MatrixWindow(rangeValue string, now time.Time) (ChannelMonitorV1MatrixWindow, error) {
	now = now.UTC()
	token := strings.TrimSpace(rangeValue)
	dense, ok := channelMonitorV1MatrixRangeTokens[token]
	if !ok {
		// 复用官方 sentinel，错误文案与 V2 handler 的 400 完全一致。
		return ChannelMonitorV1MatrixWindow{}, fmt.Errorf("%w: %s", ErrChannelMonitorV2InvalidRange, rangeValue)
	}
	window, bucket := dense.window, dense.bucket
	start, end := now.Add(-window), now
	if bucket > time.Minute {
		// 与 ParseFilter 相同：对齐到整数个整桶，尾桶可能指向未来（SQL 侧无未来行）。
		end = now.Truncate(bucket).Add(bucket)
		start = end.Add(-window)
	}
	return ChannelMonitorV1MatrixWindow{Range: token, Now: now, Start: start, End: end, Bucket: bucket}, nil
}

// ChannelMonitorV1StatusCounts 是一段时间范围内的 4 态探测计数。
//
// 在矩阵行上，它**只来自所选时间窗口的 SQL 聚合**（LoadMonitorV1WindowCounts），
// 与脉冲色块的「最近 N 次探测」不是同一批数据 —— 窗口里可能有上万条探测，
// 而柱子只有最新的 300 根。成功率的样本量因此可以远大于柱数，这是刻意的。
type ChannelMonitorV1StatusCounts struct {
	TotalChecks int64
	Operational int64
	Degraded    int64
	Failed      int64
	Error       int64
}

// Total 返回该范围内的探测总次数；total=0 表示无样本。
func (c ChannelMonitorV1StatusCounts) Total() int64 {
	return c.TotalChecks
}

// AddStatusCount 把一条 (status, count) 聚合结果累加进计数。
//
// status 取值与 channel_monitor_histories.status 的 CHECK 约束一致：
// operational / degraded / failed / error。任何其它取值只进分母（TotalChecks），
// 不落进任何一个桶 —— 未知状态按失败侧对待，绝不高估成功率。
func (c *ChannelMonitorV1StatusCounts) AddStatusCount(status string, count int64) {
	if c == nil || count <= 0 {
		return
	}
	c.TotalChecks += count
	switch status {
	case "operational":
		c.Operational += count
	case "degraded":
		c.Degraded += count
	case "failed":
		c.Failed += count
	case "error":
		c.Error += count
	}
}

// SuccessRate 返回 (operational + degraded) / total；total=0 时为 0。
//
// ⚠️ 口径（主人 2026-09-30 明确）：**黄色（degraded / 波动）也算成功**，只有红色
// （failed / error）不算。理由：「波动」表示这次探测**有响应、只是质量不理想**，
// 并不是探测失败 —— 把它当失败扣分会把成功率高台跳水。
// 注意这与展示档位是两件事：`Overall()` 仍然把 degraded 判成 warning（黄色柱子），
// 只是成功率统计不再把它计入失败。
func (c ChannelMonitorV1StatusCounts) SuccessRate() float64 {
	if c.TotalChecks <= 0 {
		return 0
	}
	return float64(c.Operational+c.Degraded) / float64(c.TotalChecks)
}

// Overall 用最差状态判定健康档位：
//   - 有 error / failed → critical
//   - 否则有 degraded → warning
//   - 否则全部 operational → healthy
//   - 无样本 → unknown
func (c ChannelMonitorV1StatusCounts) Overall() string {
	switch {
	case c.Failed > 0 || c.Error > 0:
		return channelMonitorV1MatrixHealthCritical
	case c.Degraded > 0:
		return channelMonitorV1MatrixHealthWarning
	case c.Operational > 0:
		return channelMonitorV1MatrixHealthHealthy
	default:
		return channelMonitorV1MatrixHealthUnknown
	}
}

// ChannelMonitorV1MatrixHealthFor 由 4 态计数构建 V2 同形健康度。
//
// overall / error_rate / ttft / cache 四个 string 字段填同一档位（V1 只有单一状态信号，
// 没有 TTFT/缓存率的独立判据）。
// score = 100 * (operational + degraded) / total —— **与 SuccessRate 同口径：黄色算成功**，
// 只有红色扣分；total=0 时为 nil。
// Thresholds 填 V2 默认值（仅作展示元数据，V1 判定不读它），minimum_sample 固定为 1。
func ChannelMonitorV1MatrixHealthFor(counts ChannelMonitorV1StatusCounts) ChannelMonitorV2Health {
	band := counts.Overall()
	thresholds := NormalizeChannelMonitorV2HealthThresholds(
		ChannelMonitorV2HealthThresholds{MinimumSample: ChannelMonitorV1MatrixMinimumSample},
	)
	thresholds.MinimumSample = ChannelMonitorV1MatrixMinimumSample
	health := ChannelMonitorV2Health{
		Overall:       band,
		ErrorRate:     band,
		TTFT:          band,
		Cache:         band,
		MinimumSample: ChannelMonitorV1MatrixMinimumSample,
		Thresholds:    thresholds,
	}
	if total := counts.Total(); total > 0 {
		// 与 SuccessRate 同口径：degraded（黄）计成功，只有 failed/error（红）扣分。
		score := 100 * float64(counts.Operational+counts.Degraded) / float64(total)
		health.Score = &score
	}
	return health
}

// ChannelMonitorV1MatrixMetricsFor 由 4 态计数构建 V2 同形 metrics。
//
// 口径说明（V1 主动探测没有用量/吞吐语义）：
//   - success_requests = operational + degraded；error_requests = total - 上面这个
//     （**黄色算成功**，与 SuccessRate 同口径；保证 success_requests + error_requests
//     且 error_rate == 1 - success_rate，与 V2 metrics 的不变量一致）；
//   - rpm / tpm / 各 token 字段恒为 0（无数据源，不是被脱敏掉的）；
//   - duration.sample_count / duration.avg_ms 由 latency_ms 填充（探测往返耗时）。
//     ttft 保持空：V1 的 latency_ms 不是首 token 时延，不冒充 TTFT。
func ChannelMonitorV1MatrixMetricsFor(counts ChannelMonitorV1StatusCounts, latencySumMs, latencySamples int64) ChannelMonitorV2Metric {
	total := counts.Total()
	// 黄色（degraded）算成功：与 SuccessRate / score 保持同一口径，不能三处各说各话。
	succeeded := counts.Operational + counts.Degraded
	metrics := ChannelMonitorV2Metric{
		SuccessRequests: succeeded,
		ErrorRequests:   total - succeeded,
		RequestCount:    total,
		SuccessRate:     counts.SuccessRate(),
		Duration:        ChannelMonitorV2Latency{SampleCount: latencySamples},
	}
	if total > 0 {
		metrics.ErrorRate = float64(metrics.ErrorRequests) / float64(total)
	}
	if latencySamples > 0 {
		avg := float64(latencySumMs) / float64(latencySamples)
		metrics.Duration.AvgMs = &avg
	}
	return metrics
}

// ChannelMonitorV1HistoryPoint 是 repository 返回的单次探测记录（原始值，未脱敏）。
//
// 这是脉冲曲线的最小单元：一个点就是一次探测，不再经过时间分桶聚合，
// 也**不经过时间窗口筛选**（这一路取数永远是「最近 N 次探测」，与档位无关）。
type ChannelMonitorV1HistoryPoint struct {
	MonitorID int64
	CheckedAt time.Time
	// Status 取值与 channel_monitor_histories.status 的 CHECK 约束一致：
	// operational / degraded / failed / error。
	Status string
	// LatencyMs 可空（探测失败时通常没有延迟样本）。
	LatencyMs *int64
}

// Counts 把单次探测折算成 4 态计数（恰好一项为 1），便于复用既有的
// ChannelMonitorV1MatrixHealthFor / ChannelMonitorV1MatrixMetricsFor。
func (p ChannelMonitorV1HistoryPoint) Counts() ChannelMonitorV1StatusCounts {
	counts := ChannelMonitorV1StatusCounts{TotalChecks: 1}
	switch p.Status {
	case "operational":
		counts.Operational = 1
	case "degraded":
		counts.Degraded = 1
	case "failed":
		counts.Failed = 1
	case "error":
		counts.Error = 1
	}
	return counts
}

// ChannelMonitorV1CoverageBounds 是单个监控的历史时间边界。
type ChannelMonitorV1CoverageBounds struct {
	MinCheckedAt time.Time
	MaxCheckedAt time.Time
}

// ChannelMonitorV1LiveGroupIDs 是「绑定分组当前仍可用」的判定结果集：
// key = 分组 id；出现在集合里即表示该分组存在、未软删除且 status='active'。
//
// 为什么需要这一步：channel_monitors.group_id 上的外键只保证「指向的分组曾经存在」，
// 而后台删除分组是**软删除**（写 deleted_at、status 仍可能为 active），
// ON DELETE SET NULL 不会触发。因此读取侧必须再过滤一次
// `deleted_at IS NULL AND status = 'active'`（与 ChannelMonitorV2 的既有做法一致），
// 否则卡片会 join 到一个已经下线的分组、拿到空模型表。
type ChannelMonitorV1LiveGroupIDs map[int64]struct{}

// ChannelMonitorV1MatrixRepository 是 V1 矩阵专用的只读数据访问接口。
//
// 刻意独立于既有的 ChannelMonitorRepository：后者已被其它实现与测试假体实现，
// 改动它会破坏它们（最小改动原则）。本接口只服务新端点。
//
// 两路数据的分工（**这是本页最容易被改错的一条口径**）：
//
//   - LoadMonitorV1RecentPoints → **柱子**：最近 limit 次探测，与时间档位无关；
//   - LoadMonitorV1WindowCounts → **成功率 / 健康分**：所选窗口内的状态计数。
type ChannelMonitorV1MatrixRepository interface {
	// LoadMonitorV1RecentPoints 批量返回每个监控**最新的至多 limit 条**探测记录，
	// 按 checked_at 升序（最旧在前、最新在后，与前端「过去 → 现在」的横轴一致）。无记录的监控不出现在 map 中。
	//
	// ⚠️ 没有 start/end 参数：柱子的取数**与所选时间档位完全无关**，永远是「最近 limit 次探测」，
	// 既不做时间过滤、也不做均匀抽样。详见 `ChannelMonitorV1MatrixPointLimit`。
	LoadMonitorV1RecentPoints(ctx context.Context, monitorIDs []int64, limit int) (map[int64][]ChannelMonitorV1HistoryPoint, error)
	// LoadMonitorV1WindowCounts 批量返回每个监控在 [start, end) 窗口内的 4 态探测计数；
	// 只被成功率 / 健康分 / success_requests / error_requests 使用。
	// 窗口内零探测的监控不出现在 map 中（调用方按零值处理）。
	LoadMonitorV1WindowCounts(ctx context.Context, monitorIDs []int64, start, end time.Time) (map[int64]ChannelMonitorV1StatusCounts, error)
	// LoadMonitorV1Coverage 批量返回每个监控历史的最早/最晚 checked_at（无数据的监控不在 map 中）。
	LoadMonitorV1Coverage(ctx context.Context, monitorIDs []int64) (map[int64]ChannelMonitorV1CoverageBounds, error)
	// LoadMonitorV1LiveGroupIDs 从给定的分组 id 里筛出「当前仍可用」的子集：
	// 只保留存在、未软删除且 status='active' 的分组。入参为空时返回空集合（不发查询）。
	//
	// 这是「监控 → 广场分组」的**唯一**校验步骤：归属由
	// ChannelMonitor.GroupID 直接声明，这里只判断它是否还活着，不再做任何名字猜测。
	LoadMonitorV1LiveGroupIDs(ctx context.Context, groupIDs []int64) (ChannelMonitorV1LiveGroupIDs, error)
}

// ChannelMonitorV1EnabledMonitorReader 提供 V1 矩阵所需的启用监控清单。
// 生产实现是 *ChannelMonitorService（既有 ListEnabledMonitors），此处用窄接口
// 让 service 单测可以注入轻量假体。
type ChannelMonitorV1EnabledMonitorReader interface {
	ListEnabledMonitors(ctx context.Context) ([]*ChannelMonitor, error)
}

// ChannelMonitorV1MatrixService 提供 V1 主动探测的矩阵视图（只读）。
type ChannelMonitorV1MatrixService struct {
	monitors ChannelMonitorV1EnabledMonitorReader
	repo     ChannelMonitorV1MatrixRepository
}

// NewChannelMonitorV1MatrixService 创建 V1 矩阵服务。
func NewChannelMonitorV1MatrixService(
	monitors ChannelMonitorV1EnabledMonitorReader,
	repo ChannelMonitorV1MatrixRepository,
) *ChannelMonitorV1MatrixService {
	return &ChannelMonitorV1MatrixService{monitors: monitors, repo: repo}
}

// NewEmptyChannelMonitorV1Matrix 返回 200 + 空 items 的同形响应
// （功能未开启 / 模式不是 v1 时使用，照抄 ChannelMonitorUserHandler.List 的空返回风格，不返回 403）。
// coverage 仍按请求窗口填充，避免前端拿到零值时间戳后算出无意义的横轴。
func NewEmptyChannelMonitorV1Matrix(window ChannelMonitorV1MatrixWindow) *ChannelMonitorV2Matrix {
	return &ChannelMonitorV2Matrix{
		GroupBy:  ChannelMonitorV2GroupByPlatformGroup,
		Coverage: channelMonitorV1MatrixCoverageFor(window, ChannelMonitorV1CoverageBounds{}, false),
		Items:    []ChannelMonitorV2MatrixRow{},
	}
}

// channelMonitorV1MatrixCoverageFor 构建单行口径的 coverage。
// 无数据时 data_through 回落到 start、coverage_start 回落到 end（照约定）。
func channelMonitorV1MatrixCoverageFor(
	window ChannelMonitorV1MatrixWindow,
	bounds ChannelMonitorV1CoverageBounds,
	hasData bool,
) ChannelMonitorV2Coverage {
	coverage := ChannelMonitorV2Coverage{
		RequestedStart: window.Start,
		RequestedEnd:   window.End,
		CoverageStart:  window.End,
		DataThrough:    window.Start,
		ComputedAt:     window.Now,
		BucketSeconds:  int(window.Bucket.Seconds()),
	}
	if hasData {
		coverage.CoverageStart = bounds.MinCheckedAt.UTC()
		coverage.DataThrough = bounds.MaxCheckedAt.UTC()
		if lag := window.Now.Sub(coverage.DataThrough); lag > 0 {
			coverage.AggregationLagSeconds = int64(lag.Seconds())
		}
	}
	coverage.CoverageComplete = !coverage.CoverageStart.After(window.Start)
	return coverage
}

// Matrix 构建 V1 矩阵。
//
// 固定 group_by=platform_group：每个启用的监控一行（platform=provider、group_name=监控名、
// model 留空），buckets 是「最近 N 次探测」的脉冲（与档位无关）。
//
// ⚠️ 两路数据、两种口径，别混：
//
//	· 柱子（row.Buckets）        ← LoadMonitorV1RecentPoints：最近 300 次探测，**不分档位**；
//	· 成功率 / 健康分（row.Metrics / row.Health）← LoadMonitorV1WindowCounts：所选窗口内的计数。
//
// 所以「近 1 小时」只让成功率的分母变小，柱子该是多少根还是多少根。
//
// admin=false 时复用官方 redactChannelMonitorV2Metric 做与 V2 用户视图一致的脱敏。
func (s *ChannelMonitorV1MatrixService) Matrix(
	ctx context.Context,
	window ChannelMonitorV1MatrixWindow,
	admin bool,
) (*ChannelMonitorV2Matrix, error) {
	if s == nil || s.monitors == nil || s.repo == nil {
		return nil, fmt.Errorf("channel monitor v1 matrix service is not configured")
	}
	if window.Bucket <= 0 {
		return nil, fmt.Errorf("%w: empty bucket", ErrChannelMonitorV2InvalidRange)
	}
	monitors, err := s.monitors.ListEnabledMonitors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors for v1 matrix: %w", err)
	}
	// 与 admin 列表一致：按 sort_order、id 稳定排序，保证前端卡片顺序不抖动。
	sort.SliceStable(monitors, func(i, j int) bool {
		if monitors[i].SortOrder != monitors[j].SortOrder {
			return monitors[i].SortOrder < monitors[j].SortOrder
		}
		return monitors[i].ID < monitors[j].ID
	})

	matrix := &ChannelMonitorV2Matrix{
		GroupBy:  ChannelMonitorV2GroupByPlatformGroup,
		Coverage: channelMonitorV1MatrixCoverageFor(window, ChannelMonitorV1CoverageBounds{}, false),
		Items:    make([]ChannelMonitorV2MatrixRow, 0, len(monitors)),
	}
	if len(monitors) == 0 {
		return matrix, nil
	}

	monitorIDs := make([]int64, 0, len(monitors))
	// 分组归属只认监控自己声明的 group_id（见 ChannelMonitor.GroupID 的注释）。
	// 早期那套三级降级猜测（账号 → group_name 同名 → 监控名同名）已整体下线：
	// 它在同名分组存在时会认领到已软删的那条，使卡片稳定显示「0 个模型」。
	groupIDs := make([]int64, 0, len(monitors))
	seenGroupID := make(map[int64]struct{}, len(monitors))
	for _, monitor := range monitors {
		monitorIDs = append(monitorIDs, monitor.ID)
		if monitor.GroupID == nil || *monitor.GroupID <= 0 {
			continue
		}
		id := *monitor.GroupID
		if _, ok := seenGroupID[id]; ok {
			continue
		}
		seenGroupID[id] = struct{}{}
		groupIDs = append(groupIDs, id)
	}

	// 两路取数互不依赖：
	//   1. 最近 300 次探测 → 柱子（与档位无关）；
	//   2. 窗口内状态计数   → 成功率 / 健康分（只受档位影响）。
	pointsByMonitor, err := s.repo.LoadMonitorV1RecentPoints(ctx, monitorIDs, ChannelMonitorV1MatrixPointLimit)
	if err != nil {
		return nil, err
	}
	countsByMonitor, err := s.repo.LoadMonitorV1WindowCounts(ctx, monitorIDs, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	coverageByMonitor, err := s.repo.LoadMonitorV1Coverage(ctx, monitorIDs)
	if err != nil {
		return nil, err
	}
	liveGroups, err := s.repo.LoadMonitorV1LiveGroupIDs(ctx, groupIDs)
	if err != nil {
		return nil, err
	}

	aggregate := channelMonitorV1MatrixCoverageFor(window, ChannelMonitorV1CoverageBounds{}, false)
	anyData := false

	for _, monitor := range monitors {
		row := channelMonitorV1MatrixRow(
			monitor,
			pointsByMonitor[monitor.ID],
			countsByMonitor[monitor.ID],
			liveGroups,
		)
		bounds, hasData := coverageByMonitor[monitor.ID]
		rowCoverage := channelMonitorV1MatrixCoverageFor(window, bounds, hasData)
		if hasData {
			if !anyData {
				aggregate = rowCoverage
				anyData = true
			} else {
				if rowCoverage.CoverageStart.Before(aggregate.CoverageStart) {
					aggregate.CoverageStart = rowCoverage.CoverageStart
				}
				if rowCoverage.DataThrough.After(aggregate.DataThrough) {
					aggregate.DataThrough = rowCoverage.DataThrough
				}
				aggregate.AggregationLagSeconds = 0
				if lag := window.Now.Sub(aggregate.DataThrough); lag > 0 {
					aggregate.AggregationLagSeconds = int64(lag.Seconds())
				}
				aggregate.CoverageComplete = !aggregate.CoverageStart.After(window.Start)
			}
		}
		matrix.Items = append(matrix.Items, row)
	}
	matrix.Coverage = aggregate

	if !admin {
		// 与 V2 用户视图一致：绝对量置 0，比率与（duration）分位保留。
		// V1 的 rpm/tpm/token 本来就是 0，此处 hideThroughput 与否观察结果相同。
		for i := range matrix.Items {
			redactChannelMonitorV2Metric(&matrix.Items[i].Metrics, false)
			for j := range matrix.Items[i].Buckets {
				redactChannelMonitorV2Metric(&matrix.Items[i].Buckets[j].Metrics, false)
			}
		}
	}
	return matrix, nil
}

// channelMonitorV1MatrixRow 构建单个监控的矩阵行（未脱敏）。
//
// ⚠️ 两种数据源、两种口径，**不要互相推导**：
//
//   - `points` = 最近 ChannelMonitorV1MatrixPointLimit 次探测 → 只负责 `buckets`（一根柱子 = 一次探测），
//     与所选时间档位**无关**；窗口里没有探测的时间段不产生任何点，因此渠道监控被关掉的那段时间
//     不会留下灰色空档，相邻两次探测自然相连；渠道开着却挂掉时探测照常发生、状态是 failed/error，
//     于是画成红色 —— 红色只表示「这次探测失败」。
//   - `counts` = **所选窗口内**的状态计数 → 负责 `metrics`（成功率 / success_requests /
//     error_requests）与 `health`（档位与健康分）。这部分**不再从 points 汇总**：
//     窗口里可能有上万条探测而柱子只有 300 根，从柱子推成功率等于悄悄换了个窗口。
//
// 唯一仍取自 `points` 的汇总量是延迟（`duration.avg_ms`）：窗口计数 SQL 不聚合 latency_ms，
// 且 Pro 页不展示延迟，所以它保持「最近 N 次探测的延迟均值」这个口径，与柱子的样本一致。
func channelMonitorV1MatrixRow(
	monitor *ChannelMonitor,
	points []ChannelMonitorV1HistoryPoint,
	counts ChannelMonitorV1StatusCounts,
	liveGroups ChannelMonitorV1LiveGroupIDs,
) ChannelMonitorV2MatrixRow {
	trend := make([]ChannelMonitorV2TrendPoint, 0, len(points))
	var (
		pointLatencySum int64
		pointLatencyCnt int64
	)
	for _, point := range points {
		// 点级一律「这一次探测」的计数（1 条样本），与窗口计数无关。
		pointCounts := point.Counts()
		var latencySum, latencyCnt int64
		if point.LatencyMs != nil {
			latencySum = *point.LatencyMs
			latencyCnt = 1
		}
		trend = append(trend, ChannelMonitorV2TrendPoint{
			BucketStart: point.CheckedAt.UTC(),
			Metrics:     ChannelMonitorV1MatrixMetricsFor(pointCounts, latencySum, latencyCnt),
			Health:      ChannelMonitorV1MatrixHealthFor(pointCounts),
		})

		pointLatencySum += latencySum
		pointLatencyCnt += latencyCnt
	}

	// 分组归属只有一个来源：监控自己绑定的 group_id —— 不再猜名字。
	// 未绑定、或绑定的分组已被删除/停用 → group_id 留空；卡片照常渲染，
	// 只是没有模型与定价明细（前端既有的降级行为，不会整行消失）。
	var groupID *int64
	if monitor.GroupID != nil && *monitor.GroupID > 0 {
		if _, alive := liveGroups[*monitor.GroupID]; alive {
			id := *monitor.GroupID
			groupID = &id
		}
	}

	return ChannelMonitorV2MatrixRow{
		Platform:  monitor.Provider,
		GroupID:   groupID,
		GroupName: monitor.Name,
		Metrics:   ChannelMonitorV1MatrixMetricsFor(counts, pointLatencySum, pointLatencyCnt),
		Health:    ChannelMonitorV1MatrixHealthFor(counts),
		Buckets:   trend,
	}
}
